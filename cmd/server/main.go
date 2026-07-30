// @title           Gym Pulse API
// @version         1.0
// @description     REST API for the Gym Pulse workout tracking app.
// @host            localhost:8080
// @BasePath        /
// @securityDefinitions.apikey BearerAuth
// @in              header
// @name            Authorization
// @description     JWT Bearer token — prefix value with "Bearer "
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	_ "github.com/gym-pulse/gym-pulse-api/docs"
	"github.com/gym-pulse/gym-pulse-api/internal/config"
	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/handler"
	"github.com/gym-pulse/gym-pulse-api/internal/router"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

func main() {
	// Load config.
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Setup logger.
	var logLevel slog.Level
	switch cfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	var slogHandler slog.Handler
	if cfg.Environment == "production" {
		slogHandler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	} else {
		slogHandler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})
	}
	logger := slog.New(slogHandler)
	slog.SetDefault(logger)

	// Connect to database.
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to parse database pool configuration", "error", err)
		os.Exit(1)
	}
	poolConfig.MaxConns = cfg.DatabaseMaxConns
	poolConfig.MinConns = cfg.DatabaseMinConns
	poolConfig.MaxConnLifetime = cfg.DatabaseMaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.DatabaseMaxConnIdleTime
	poolConfig.HealthCheckPeriod = cfg.DatabaseHealthCheckPeriod

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		logger.Error("failed to create connection pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		logger.Error("failed to ping database", "error", err)
		os.Exit(1) //nolint:gocritic // pool.Close defer intentionally skipped on fatal startup error
	}
	logger.Info("connected to database")

	// Run migrations.
	if err := runMigrations(cfg.DatabaseURL); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}
	logger.Info("migrations complete")

	// Advisory locks must never consume the main query pool. A separate,
	// bounded pool prevents N concurrent deletion/avatar operations from
	// holding all N query connections while each waits for one more.
	lockPoolConfig, err := pgxpool.ParseConfig(cfg.DatabaseLockURL)
	if err != nil {
		logger.Error("failed to parse database lock pool configuration", "error", err)
		os.Exit(1)
	}
	lockPoolConfig.MaxConns = cfg.DatabaseLockMaxConns
	lockPoolConfig.MinConns = 0
	lockPoolConfig.MaxConnLifetime = cfg.DatabaseMaxConnLifetime
	lockPoolConfig.MaxConnIdleTime = cfg.DatabaseMaxConnIdleTime
	lockPoolConfig.HealthCheckPeriod = cfg.DatabaseHealthCheckPeriod

	lockPool, err := pgxpool.NewWithConfig(context.Background(), lockPoolConfig)
	if err != nil {
		logger.Error("failed to create database lock pool", "error", err)
		os.Exit(1)
	}
	defer lockPool.Close()

	if err := lockPool.Ping(context.Background()); err != nil {
		logger.Error("failed to ping database lock pool", "error", err)
		os.Exit(1)
	}

	// Create shared validator.
	v := validator.New()

	// Build dependency graph.
	templateRepo := dao.NewTemplateDAO(pool)
	logRepo := dao.NewLogDAO(pool)
	statsRepo := dao.NewStatsDAO(pool)
	settingsRepo := dao.NewSettingsDAO(pool)
	profileRepo := dao.NewProfileDAO(pool)
	bodyWeightRepo := dao.NewBodyWeightDAO(pool)
	exerciseCatalogRepo := dao.NewExerciseCatalogDAO(pool)
	planRepo := dao.NewPlanDAO(pool)
	trainingProfileRepo := dao.NewTrainingProfileDAO(pool)
	starterProgramRepo := dao.NewStarterProgramDAO(pool)
	programRepo := dao.NewProgramDAO(pool)
	scheduleRepo := dao.NewScheduleDAO(pool)
	workoutSessionRepo := dao.NewWorkoutSessionDAO(pool)
	performedSetRepo := dao.NewPerformedSetDAO(pool)
	participationRepo := dao.NewParticipationDAO(pool)
	idempotencyRepo := dao.NewIdempotencyDAO(pool)
	planTransitionRepo := dao.NewPlanTransitionDAO(pool)
	authUserRepo := dao.NewAuthUserDAO(pool)
	userOperationLocker := dao.NewUserOperationLocker(lockPool)

	templateSvc := service.NewTemplateService(templateRepo, v)
	logSvc := service.NewLogService(logRepo, templateRepo, v)
	statsSvc := service.NewStatsService(statsRepo, settingsRepo)
	settingsSvc := service.NewSettingsService(settingsRepo, v)
	var avatarStorage service.AvatarStorage
	var avatarDeleter service.AvatarDeleter
	if cfg.SupabaseURL != "" && cfg.SupabaseServiceRoleKey != "" {
		storage := service.NewSupabaseAvatarStorage(cfg.SupabaseURL, cfg.SupabaseServiceRoleKey, cfg.SupabaseAvatarBucket)
		avatarStorage = storage
		avatarDeleter = storage
	}
	profileSvc := service.NewProfileServiceWithUserBoundary(profileRepo, v, avatarStorage, authUserRepo, userOperationLocker)
	bodyWeightSvc := service.NewBodyWeightService(bodyWeightRepo, v)
	exerciseCatalogSvc := service.NewExerciseCatalogService(exerciseCatalogRepo)
	planSvc := service.NewPlanService(planRepo, templateRepo, v)
	trainingProfileSvc := service.NewTrainingProfileService(trainingProfileRepo)
	programSvc := service.NewProgramService(starterProgramRepo, programRepo, idempotencyRepo, v)
	scheduleSvc := service.NewScheduleService(scheduleRepo, programRepo, trainingProfileRepo, workoutSessionRepo, performedSetRepo, participationRepo, idempotencyRepo, v)
	workoutSessionSvc := service.NewWorkoutSessionService(workoutSessionRepo, scheduleRepo, participationRepo, trainingProfileRepo, idempotencyRepo, v)
	participationSvc := service.NewParticipationService(scheduleSvc, participationRepo)
	planTransitionSvc := service.NewPlanTransitionService(starterProgramRepo, programRepo, planTransitionRepo, v)

	accountRepo := dao.NewAccountDAO(pool)
	var authDeleter service.AuthUserDeleter
	if cfg.SupabaseURL != "" && cfg.SupabaseServiceRoleKey != "" {
		authDeleter = service.NewSupabaseAdmin(cfg.SupabaseURL, cfg.SupabaseServiceRoleKey)
	}
	accountSvc := service.NewAccountService(accountRepo, avatarDeleter, authDeleter, userOperationLocker)

	templateHandler := handler.NewTemplateHandler(templateSvc)
	logHandler := handler.NewLogHandler(logSvc)
	statsHandler := handler.NewStatsHandler(statsSvc)
	settingsHandler := handler.NewSettingsHandler(settingsSvc)
	profileHandler := handler.NewProfileHandler(profileSvc)
	bodyWeightHandler := handler.NewBodyWeightHandler(bodyWeightSvc)
	exerciseCatalogHandler := handler.NewExerciseCatalogHandler(exerciseCatalogSvc)
	planHandler := handler.NewPlanHandler(planSvc)
	accountHandler := handler.NewAccountHandler(accountSvc)
	readinessHandler := handler.NewReadinessHandler(pool)
	trainingProfileHandler := handler.NewTrainingProfileHandler(trainingProfileSvc)
	programHandler := handler.NewProgramHandler(programSvc)
	scheduleHandler := handler.NewScheduleHandler(scheduleSvc)
	workoutSessionHandler := handler.NewWorkoutSessionHandler(workoutSessionSvc)
	participationHandler := handler.NewParticipationHandler(participationSvc)
	planTransitionHandler := handler.NewPlanTransitionHandler(planTransitionSvc)

	// Create router.
	r := router.New(cfg, logger, authUserRepo, templateHandler, logHandler, statsHandler, settingsHandler, profileHandler, bodyWeightHandler, exerciseCatalogHandler, planHandler, accountHandler, readinessHandler, trainingProfileHandler, programHandler, scheduleHandler, planTransitionHandler, workoutSessionHandler, participationHandler)

	// Start server with graceful shutdown.
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		logger.Info("shutting down server")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("server shutdown error", "error", err)
		}
	}()

	logger.Info("server starting", "port", cfg.Port, "env", cfg.Environment)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
	logger.Info("server stopped")
}

func runMigrations(databaseURL string) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("opening migration db: %w", err)
	}
	defer db.Close()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("creating migration driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance("file://migrations", "postgres", driver)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}
