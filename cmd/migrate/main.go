// Command migrate applies pending database migrations as the schema owner and
// exits. Release environments run it as the pre-deploy step so the long-running
// server never needs the owner credential; see docs/RELEASE_RUNBOOK.md.
package main

import (
	"log/slog"
	"os"

	"github.com/gym-pulse/gym-pulse-api/internal/config"
	"github.com/gym-pulse/gym-pulse-api/internal/dbmigrate"
)

func main() {
	cfg, err := config.LoadMigrate()
	if err != nil {
		slog.Error("failed to load migrate config", "error", err)
		os.Exit(1)
	}

	var handler slog.Handler = slog.NewTextHandler(os.Stdout, nil)
	if cfg.Environment == "production" {
		handler = slog.NewJSONHandler(os.Stdout, nil)
	}
	logger := slog.New(handler)

	result, err := dbmigrate.Up(cfg.DatabaseMigrateURL, dbmigrate.DefaultSource)
	if err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}
	logger.Info("migrations complete", "version", result.Version, "changed", result.Changed)
}
