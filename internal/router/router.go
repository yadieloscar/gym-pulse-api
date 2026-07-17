package router

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	httpswagger "github.com/swaggo/http-swagger"

	"github.com/gym-pulse/gym-pulse-api/internal/config"
	"github.com/gym-pulse/gym-pulse-api/internal/handler"
	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
)

func New(
	cfg *config.Config,
	logger *slog.Logger,
	templateHandler *handler.TemplateHandler,
	logHandler *handler.LogHandler,
	statsHandler *handler.StatsHandler,
	settingsHandler *handler.SettingsHandler,
	profileHandler *handler.ProfileHandler,
	bodyWeightHandler *handler.BodyWeightHandler,
	exerciseCatalogHandler *handler.ExerciseCatalogHandler,
	planHandler *handler.PlanHandler,
	accountHandler *handler.AccountHandler,
	trainingProfileHandler *handler.TrainingProfileHandler,
	programHandler *handler.ProgramHandler,
	scheduleHandler *handler.ScheduleHandler,
	workoutSessionHandler *handler.WorkoutSessionHandler,
	participationHandler *handler.ParticipationHandler,
) http.Handler {
	r := chi.NewRouter()

	// Global middleware.
	r.Use(middleware.LoggingMiddleware(logger))
	r.Use(middleware.CORSMiddleware(cfg.AllowedOrigins))
	r.Use(chimiddleware.Recoverer)

	// Public routes.
	r.Get("/health", handler.HealthCheck)
	r.Get("/docs/*", httpswagger.WrapHandler)

	// Authenticated routes.
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(cfg.SupabaseJWTSecret, cfg.SupabaseJWKSURL))

		r.Route("/api/v1", func(r chi.Router) {
			// Goal-based training
			r.Get("/training-profile", trainingProfileHandler.Get)
			r.Put("/training-profile", trainingProfileHandler.Update)
			r.Get("/starter-programs", programHandler.ListStarters)
			r.Route("/programs", func(r chi.Router) {
				r.Get("/", programHandler.List)
				r.Post("/", programHandler.Create)
				r.Post("/from-starter", programHandler.CloneStarter)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", programHandler.Get)
					r.Put("/", programHandler.Update)
				})
			})
			r.Get("/schedule", scheduleHandler.List)
			r.Post("/schedule/materialize", scheduleHandler.Materialize)
			r.Post("/schedule/regenerate", scheduleHandler.Regenerate)
			r.Route("/scheduled-workouts/{id}", func(r chi.Router) {
				r.Patch("/", scheduleHandler.Patch)
				r.Put("/sets/{set_id}", scheduleHandler.PutSet)
				r.Post("/extra-sets", scheduleHandler.AddExtra)
				r.Post("/complete", scheduleHandler.Complete)
			})
			r.Route("/workout-sessions", func(r chi.Router) {
				r.Get("/", workoutSessionHandler.List)
				r.Post("/", workoutSessionHandler.Create)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", workoutSessionHandler.Get)
					r.Patch("/", workoutSessionHandler.Patch)
				})
			})
			r.Get("/participation", participationHandler.List)

			// Templates
			r.Route("/templates", func(r chi.Router) {
				r.Get("/", templateHandler.List)
				r.Post("/", templateHandler.Create)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", templateHandler.GetByID)
					r.Put("/", templateHandler.Update)
					r.Delete("/", templateHandler.Delete)
				})
			})

			// Day Logs
			r.Route("/logs", func(r chi.Router) {
				r.Get("/", logHandler.ListByWeek)
				r.Post("/", logHandler.Create)
				r.Route("/{date}", func(r chi.Router) {
					r.Get("/", logHandler.GetByDate)
					r.Put("/", logHandler.Update)
					r.Delete("/", logHandler.Delete)
				})
			})

			// Account
			r.Delete("/account", accountHandler.Delete)

			// Stats
			r.Get("/stats", statsHandler.Summary)
			r.Get("/stats/summary", statsHandler.Summary)
			r.Get("/stats/distribution", statsHandler.Distribution)
			r.Get("/stats/volume", statsHandler.Volume)

			// Settings
			r.Get("/settings", settingsHandler.Get)
			r.Put("/settings", settingsHandler.Update)

			// Profile
			r.Get("/profile", profileHandler.Get)
			r.Put("/profile", profileHandler.Update)

			// Exercise catalog (read-only v1)
			r.Get("/exercises", exerciseCatalogHandler.List)
			// Per-exercise set history ("last time you did X")
			r.Get("/exercises/history", logHandler.ExerciseHistory)
			// Per-exercise all-time records (heaviest + best e1RM)
			r.Get("/exercises/records", logHandler.ExerciseRecords)

			// Weekly plan
			r.Route("/plan", func(r chi.Router) {
				r.Get("/", planHandler.Get)
				r.Put("/weekly", planHandler.PutWeekly)
				r.Put("/overrides/{date}", planHandler.PutOverride)
				r.Delete("/overrides/{date}", planHandler.DeleteOverride)
			})

			// Body Weight
			r.Route("/body/weight", func(r chi.Router) {
				r.Post("/", bodyWeightHandler.Create)
				r.Get("/", bodyWeightHandler.List)
				r.Delete("/{id}", bodyWeightHandler.Delete)
			})
		})
	})

	return r
}
