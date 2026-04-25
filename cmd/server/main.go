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
	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
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

	// Create shared validator.
	v := validator.New()

	// Build dependency graph.
	templateRepo := dao.NewTemplateDAO(pool)
	logRepo := dao.NewLogDAO(pool)
	statsRepo := dao.NewStatsDAO(pool)
	settingsRepo := dao.NewSettingsDAO(pool)

	templateSvc := service.NewTemplateService(templateRepo, v)
	logSvc := service.NewLogService(logRepo, templateRepo, v)
	statsSvc := service.NewStatsService(statsRepo, settingsRepo)
	settingsSvc := service.NewSettingsService(settingsRepo, v)

	templateHandler := handler.NewTemplateHandler(templateSvc)
	logHandler := handler.NewLogHandler(logSvc)
	statsHandler := handler.NewStatsHandler(statsSvc)
	settingsHandler := handler.NewSettingsHandler(settingsSvc)

	// Create router.
	r := router.New(cfg, logger, templateHandler, logHandler, statsHandler, settingsHandler)

	// Start server with graceful shutdown.
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
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
