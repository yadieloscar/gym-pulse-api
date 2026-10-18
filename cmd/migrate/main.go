// Command migrate applies pending schema migrations as a release step.
//
// It loads the same validated configuration as the server and migrates over
// DATABASE_LOCK_URL, which release environments require to be a direct or
// session-mode connection; golang-migrate's advisory lock needs one. Railway
// runs it as the pre-deploy command, so a failure stops the deployment before
// the new version takes traffic. SIGINT or SIGTERM stops it between
// migrations, never inside one.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gym-pulse/gym-pulse-api/internal/buildinfo"
	"github.com/gym-pulse/gym-pulse-api/internal/config"
	"github.com/gym-pulse/gym-pulse-api/internal/migration"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, logger)
	stop()
	if err != nil {
		logger.Error("migrate failed", "error", err, "build", buildinfo.Version)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	version, err := migration.Up(ctx, cfg.DatabaseLockURL, migration.Dir)
	if err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}
	logger.Info("migrations complete", "version", version, "env", cfg.Environment, "build", buildinfo.Version)
	return nil
}
