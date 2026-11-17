//go:build integration

package dbmigrate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// TestCheckCurrentAgainstADatabaseThatIsBehind replays the 2026-11-17
// production failure: a build that ships migration N starts on a database
// that stopped earlier. The check must refuse every such state and accept
// the database once ./migrate has run.
func TestCheckCurrentAgainstADatabaseThatIsBehind(t *testing.T) {
	databaseURL := scratchDatabase(t)
	m := newMigrator(t, databaseURL)
	latest, err := LatestVersion(testSource)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.WithoutCancel(ctx))

	// Before migration 048 the version function itself does not exist.
	if err := m.Migrate(47); err != nil {
		t.Fatalf("migrating to 47: %v", err)
	}
	if _, err := CheckCurrent(ctx, conn, testSource); !errors.Is(err, ErrSchemaBehind) {
		t.Errorf("at version 47: error = %v, want ErrSchemaBehind", err)
	}

	if err := m.Up(); err != nil {
		t.Fatalf("migrating up: %v", err)
	}
	got, err := CheckCurrent(ctx, conn, testSource)
	if err != nil {
		t.Fatalf("at the newest version: %v", err)
	}
	if got != latest {
		t.Errorf("version = %d, want %d", got, latest)
	}

	// The function exists but the ledger is one version short.
	if err := m.Force(int(latest) - 1); err != nil {
		t.Fatal(err)
	}
	if got, err := CheckCurrent(ctx, conn, testSource); !errors.Is(err, ErrSchemaBehind) || got != latest-1 {
		t.Errorf("one version short: version = %d, error = %v, want %d and ErrSchemaBehind", got, err, latest-1)
	}

	// A failed migration leaves the newest version dirty.
	if _, err := conn.Exec(ctx, `UPDATE schema_migrations SET version=$1, dirty=true`, int64(latest)); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckCurrent(ctx, conn, testSource); !errors.Is(err, ErrDirty) {
		t.Errorf("dirty ledger: error = %v, want ErrDirty", err)
	}
}
