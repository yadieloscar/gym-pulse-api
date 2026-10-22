//go:build integration

package migration

import (
	"context"
	"errors"
	"net/url"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const shippedDir = "../../migrations"

// TestReleaseSchemaGate proves the release step and the startup gate against
// a real, initially empty database: the gate rejects it, the release step
// migrates it, the gate then accepts it, and a repeated release step is a
// no-op.
func TestReleaseSchemaGate(t *testing.T) {
	base := os.Getenv("GYMPULSE_TEST_DATABASE_URL")
	if base == "" {
		t.Fatal("integration tests require GYMPULSE_TEST_DATABASE_URL pointing to a disposable database server")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)

	name := "gympulse_gate_" + uuid.NewString()[:8]
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.WithoutCancel(ctx), `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`); err != nil {
			t.Errorf("dropping %s: %v", name, err)
		}
	})
	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + name
	target := parsed.String()

	pool, err := pgxpool.New(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	if err := RequireCurrent(ctx, pool, shippedDir); !errors.Is(err, ErrSchemaBehind) {
		t.Fatalf("RequireCurrent(empty db) error = %v, want ErrSchemaBehind", err)
	}

	want, err := Latest(shippedDir)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := range 2 {
		got, err := Up(ctx, target, shippedDir)
		if err != nil {
			t.Fatalf("Up() attempt %d error = %v", attempt+1, err)
		}
		if got != want {
			t.Fatalf("Up() attempt %d version = %d, want %d", attempt+1, got, want)
		}
	}

	if err := RequireCurrent(ctx, pool, shippedDir); err != nil {
		t.Fatalf("RequireCurrent(migrated db) error = %v, want nil", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE schema_migrations SET dirty = true`); err != nil {
		t.Fatal(err)
	}
	if err := RequireCurrent(ctx, pool, shippedDir); !errors.Is(err, ErrSchemaDirty) {
		t.Fatalf("RequireCurrent(dirty db) error = %v, want ErrSchemaDirty", err)
	}
}
