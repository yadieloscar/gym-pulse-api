//go:build integration

package dao

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const runtimeRoleName = "gym_pulse_api"

// newRuntimeRoleFixture opens the owner pool from GYMPULSE_TEST_DATABASE_URL
// and a pool connected as the least-privilege runtime role from
// GYMPULSE_TEST_RUNTIME_DATABASE_URL. The rest of the DAO suite runs as the
// owner because its fixtures write auth.users; this test is the one that
// proves the role the API actually serves requests with can do its job.
func newRuntimeRoleFixture(t *testing.T) (context.Context, *pgxpool.Pool, *pgxpool.Pool) {
	t.Helper()
	ownerURL := os.Getenv("GYMPULSE_TEST_DATABASE_URL")
	runtimeURL := os.Getenv("GYMPULSE_TEST_RUNTIME_DATABASE_URL")
	if ownerURL == "" || runtimeURL == "" {
		t.Fatal("runtime role integration tests require GYMPULSE_TEST_DATABASE_URL (schema owner) and GYMPULSE_TEST_RUNTIME_DATABASE_URL (gym_pulse_api login) for a disposable migrated database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)

	owner, err := pgxpool.New(ctx, ownerURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(owner.Close)
	runtime, err := pgxpool.New(ctx, runtimeURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtime.Close)

	var user string
	if err := runtime.QueryRow(ctx, `SELECT current_user`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if user != runtimeRoleName {
		t.Fatalf("GYMPULSE_TEST_RUNTIME_DATABASE_URL connects as %q, want %q", user, runtimeRoleName)
	}
	return ctx, owner, runtime
}

func TestRuntimeRoleReachesEveryApplicationTable(t *testing.T) {
	ctx, owner, runtime := newRuntimeRoleFixture(t)

	rows, err := owner.Query(ctx, `
		SELECT c.relname
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public'
		  AND c.relkind IN ('r', 'p')
		  AND c.relname <> 'schema_migrations'
		ORDER BY c.relname`)
	if err != nil {
		t.Fatal(err)
	}
	tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) == 0 {
		t.Fatal("no application tables found; is the database migrated?")
	}

	for _, table := range tables {
		query := "SELECT count(*) FROM " + pgx.Identifier{"public", table}.Sanitize()
		var count int64
		if err := runtime.QueryRow(ctx, query).Scan(&count); err != nil {
			t.Errorf("runtime role cannot read public.%s: %v", table, err)
		}
	}

	// The identity check reads auth.users.id only (migrations 026 and 039).
	var ids int64
	if err := runtime.QueryRow(ctx, `SELECT count(id) FROM auth.users`).Scan(&ids); err != nil {
		t.Errorf("runtime role cannot read auth.users.id: %v", err)
	}
}

func TestRuntimeRoleIsDeniedSchemaPower(t *testing.T) {
	ctx, _, runtime := newRuntimeRoleFixture(t)

	probe := "runtime_role_probe_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	denied := []struct {
		name string
		sql  string
	}{
		{name: "create table", sql: fmt.Sprintf("CREATE TABLE public.%s (id int)", probe)},
		{name: "alter table", sql: "ALTER TABLE public.user_settings ADD COLUMN runtime_role_probe int"},
		{name: "truncate", sql: "TRUNCATE public.idempotency_records"},
		{name: "migration ledger", sql: "SELECT version FROM public.schema_migrations"},
		{name: "secure_table helper", sql: "SELECT gym_pulse_private.secure_table('public.user_settings')"},
		{name: "create role", sql: "CREATE ROLE " + probe},
	}
	for _, tt := range denied {
		t.Run(tt.name, func(t *testing.T) {
			tx, err := runtime.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			// Roll back even on an unexpected success so nothing persists.
			defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

			_, err = tx.Exec(ctx, tt.sql)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
				t.Fatalf("%s: error = %v, want insufficient_privilege (42501)", tt.sql, err)
			}
		})
	}
}

func TestSecureTableHelperProvisionsRuntimeRole(t *testing.T) {
	ctx, owner, runtime := newRuntimeRoleFixture(t)

	table := "secure_table_probe_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	qualified := pgx.Identifier{"public", table}.Sanitize()
	if _, err := owner.Exec(ctx, "CREATE TABLE "+qualified+" (id int PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.WithoutCancel(ctx), "DROP TABLE IF EXISTS "+qualified); err != nil {
			t.Errorf("dropping probe table: %v", err)
		}
	})

	// Twice, to prove the helper is idempotent.
	for range 2 {
		if _, err := owner.Exec(ctx, `SELECT gym_pulse_private.secure_table($1::regclass)`, "public."+table); err != nil {
			t.Fatalf("secure_table: %v", err)
		}
	}

	var rlsEnabled, rlsForced bool
	var policies int
	if err := owner.QueryRow(ctx, `
		SELECT c.relrowsecurity, c.relforcerowsecurity,
		       (SELECT count(*) FROM pg_policy p WHERE p.polrelid = c.oid AND p.polname = 'gym_pulse_api_access')
		FROM pg_class c WHERE c.oid = $1::regclass`, "public."+table).Scan(&rlsEnabled, &rlsForced, &policies); err != nil {
		t.Fatal(err)
	}
	if !rlsEnabled || rlsForced || policies != 1 {
		t.Fatalf("rls enabled=%t forced=%t policies=%d, want enabled, not forced, one policy", rlsEnabled, rlsForced, policies)
	}

	if _, err := runtime.Exec(ctx, "INSERT INTO "+qualified+" (id) VALUES (1)"); err != nil {
		t.Fatalf("runtime insert: %v", err)
	}
	var got int
	if err := runtime.QueryRow(ctx, "SELECT id FROM "+qualified).Scan(&got); err != nil || got != 1 {
		t.Fatalf("runtime select = %d, %v; want 1", got, err)
	}
	if _, err := runtime.Exec(ctx, "DELETE FROM "+qualified); err != nil {
		t.Fatalf("runtime delete: %v", err)
	}
}
