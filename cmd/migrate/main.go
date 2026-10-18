// Command migrate applies pending schema migrations as a release step.
//
// It loads the same validated configuration as the server and migrates over
// DATABASE_LOCK_URL, which release environments require to be a direct or
// session-mode connection; golang-migrate's advisory lock needs one. Railway
// runs it as the pre-deploy command, so a failure stops the deployment before
// the new version takes traffic.
package main

import (
	"log/slog"
	"os"

	"github.com/gym-pulse/gym-pulse-api/internal/buildinfo"
	"github.com/gym-pulse/gym-pulse-api/internal/config"
	"github.com/gym-pulse/gym-pulse-api/internal/migration"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	version, err := migration.Up(cfg.DatabaseLockURL, migration.Dir)
	if err != nil {
		logger.Error("failed to run migrations", "error", err, "build", buildinfo.Version)
		os.Exit(1)
	}
	logger.Info("migrations complete", "version", version, "env", cfg.Environment, "build", buildinfo.Version)
}
