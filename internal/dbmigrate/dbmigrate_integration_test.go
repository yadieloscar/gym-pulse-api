//go:build integration

package dbmigrate

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const testSource = "file://../../migrations"

func ownerURL(t *testing.T) string {
	t.Helper()
	raw := os.Getenv("GYMPULSE_TEST_DATABASE_URL")
	if raw == "" {
		t.Fatal("migration integration tests require GYMPULSE_TEST_DATABASE_URL pointing to a disposable migrated database as a role that can CREATE DATABASE")
	}
	return raw
}

// scratchDatabase creates an empty database on the same server and returns
// its URL; it is dropped when the test ends.
func scratchDatabase(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	base := ownerURL(t)
	conn, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.WithoutCancel(ctx))

	name := "gympulse_migrate_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("creating scratch database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 30*time.Second)
		defer cancel()
		conn, err := pgx.Connect(ctx, base)
		if err != nil {
			t.Errorf("dropping scratch database: %v", err)
			return
		}
		defer conn.Close(ctx)
		if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("dropping scratch database: %v", err)
		}
	})

	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + name
	return parsed.String()
}

// schemaSnapshot lists every column, constraint, index, policy, grant, and
// function the migrations own, in a stable order, so two databases can be
// compared for drift.
func schemaSnapshot(t *testing.T, databaseURL string) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.WithoutCancel(ctx))

	rows, err := conn.Query(ctx, `
		SELECT format('column %s.%s %s null=%s default=%s', table_name, column_name, data_type, is_nullable, coalesce(column_default, ''))
		FROM information_schema.columns
		WHERE table_schema IN ('public', 'gym_pulse_private') AND table_name <> 'schema_migrations'
		UNION ALL
		SELECT format('constraint %s %s', conrelid::regclass, pg_get_constraintdef(oid))
		FROM pg_constraint
		WHERE connamespace = 'public'::regnamespace AND conrelid <> 0
		  AND conrelid <> 'public.schema_migrations'::regclass
		UNION ALL
		SELECT format('index %s', indexdef)
		FROM pg_indexes
		WHERE schemaname = 'public' AND tablename <> 'schema_migrations'
		UNION ALL
		SELECT format('policy %s.%s %s %s %s', tablename, policyname, cmd, roles, coalesce(qual, ''))
		FROM pg_policies
		WHERE schemaname IN ('public', 'gym_pulse_private')
		UNION ALL
		SELECT format('rls %s enabled=%s forced=%s', c.oid::regclass, c.relrowsecurity, c.relforcerowsecurity)
		FROM pg_class c
		WHERE c.relnamespace = 'public'::regnamespace AND c.relkind IN ('r', 'p') AND c.relname <> 'schema_migrations'
		UNION ALL
		SELECT format('grant %s.%s %s %s', table_schema, table_name, grantee, privilege_type)
		FROM information_schema.role_table_grants
		WHERE table_schema IN ('public', 'gym_pulse_private') AND table_name <> 'schema_migrations'
		  AND grantee <> current_user
		UNION ALL
		SELECT format('function %s.%s(%s)', n.nspname, p.proname, pg_get_function_identity_arguments(p.oid))
		FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname IN ('public', 'gym_pulse_private')`)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(snapshot)
	return snapshot
}

func newMigrator(t *testing.T, databaseURL string) *migrate.Migrate {
	t.Helper()
	m, err := migrate.New(testSource, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = m.Close() })
	return m
}

// TestMigrationsRoundTrip applies every migration to an empty database, rolls
// all of them back, and applies them again. The schema after the second
// apply must match the first exactly, which proves every down migration
// fully undoes its up and that no up depends on leftovers.
func TestMigrationsRoundTrip(t *testing.T) {
	databaseURL := scratchDatabase(t)

	first, err := Up(databaseURL, testSource)
	if err != nil {
		t.Fatalf("first up: %v", err)
	}
	if !first.Changed || first.Version == 0 {
		t.Fatalf("first up = %+v, want a fresh schema to change", first)
	}
	want := schemaSnapshot(t, databaseURL)
	if len(want) == 0 {
		t.Fatal("empty schema snapshot after up; the snapshot query is not seeing the migrated objects")
	}

	again, err := Up(databaseURL, testSource)
	if err != nil {
		t.Fatalf("second up: %v", err)
	}
	if again.Changed || again.Version != first.Version {
		t.Fatalf("second up = %+v, want no change at version %d", again, first.Version)
	}

	if err := newMigrator(t, databaseURL).Down(); err != nil {
		t.Fatalf("down: %v", err)
	}
	if leftover := schemaSnapshot(t, databaseURL); len(leftover) != 0 {
		t.Fatalf("down left %d objects behind, first: %v", len(leftover), leftover[:min(5, len(leftover))])
	}

	if _, err := Up(databaseURL, testSource); err != nil {
		t.Fatalf("up after down: %v", err)
	}
	if got := schemaSnapshot(t, databaseURL); !slices.Equal(got, want) {
		t.Fatalf("schema after up/down/up differs:\nmissing: %v\nextra: %v", difference(want, got), difference(got, want))
	}
}

// TestStrictMigrationsRerunCleanly executes each strict up migration a second
// time against the fully migrated database, inside a transaction that is
// rolled back. A non-idempotent statement fails here instead of in an
// operator's recovery.
func TestStrictMigrationsRerunCleanly(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	conn, err := pgx.Connect(ctx, ownerURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.WithoutCancel(ctx))

	paths, err := filepath.Glob("../../migrations/*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	ran := 0
	for _, path := range paths {
		var version int
		if _, err := fmt.Sscanf(filepath.Base(path), "%03d_", &version); err != nil || version < strictFromVersion {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(filepath.Base(path), func(t *testing.T) {
			tx, err := conn.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := tx.Rollback(context.WithoutCancel(ctx)); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
					t.Errorf("rollback: %v", err)
				}
			}()
			if _, err := tx.Exec(ctx, string(body)); err != nil {
				t.Fatalf("re-running an applied migration failed: %v", err)
			}
		})
		ran++
	}
	if ran == 0 {
		t.Fatal("no strict migrations found")
	}
}

func difference(a, b []string) []string {
	var out []string
	for _, s := range a {
		if !slices.Contains(b, s) {
			out = append(out, s)
		}
	}
	return out
}
