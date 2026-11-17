package dbmigrate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrSchemaBehind means the database has not received every migration this
// build ships. The server must not serve: its queries name columns and
// functions that do not exist yet.
var ErrSchemaBehind = errors.New("database schema is behind this build; run ./migrate (the pre-deploy step) and start again")

// errSource means the migration files this build ships cannot be read, so
// there is no version to compare the database against.
var errSource = errors.New("migration source is not usable")

// schemaVersionQuery reads the ledger through the owner's function from
// migration 048; the runtime role has no privilege on schema_migrations.
const schemaVersionQuery = `SELECT version, dirty FROM gym_pulse_private.schema_version()`

// rowQuerier is the part of a pgx pool or connection CheckCurrent needs.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// LatestVersion returns the highest up-migration version in source, a
// file:// URL such as DefaultSource.
func LatestVersion(source string) (uint, error) {
	dir, ok := strings.CutPrefix(source, "file://")
	if !ok {
		return 0, fmt.Errorf("%w: %q is not a file:// URL", errSource, source)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("reading migration source: %w", err)
	}
	var latest uint64
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		prefix, _, found := strings.Cut(name, "_")
		if !found {
			continue
		}
		version, err := strconv.ParseUint(prefix, 10, 32)
		if err != nil {
			return 0, fmt.Errorf("migration %q has no numeric version: %w", name, err)
		}
		latest = max(latest, version)
	}
	if latest == 0 {
		return 0, fmt.Errorf("%w: %q holds no up migrations", errSource, source)
	}
	return uint(latest), nil
}

// CheckCurrent reports the applied schema version and fails when the database
// is dirty or older than the newest migration in source. A newer database is
// accepted: during a deploy the previous build keeps serving after ./migrate
// has run, and migrations stay compatible with it.
func CheckCurrent(ctx context.Context, db rowQuerier, source string) (uint, error) {
	latest, err := LatestVersion(source)
	if err != nil {
		return 0, err
	}

	var (
		version int64
		dirty   bool
	)
	err = db.QueryRow(ctx, schemaVersionQuery).Scan(&version, &dirty)
	var pgErr *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return 0, fmt.Errorf("no migration is applied, build needs version %d: %w", latest, ErrSchemaBehind)
	case errors.As(err, &pgErr) && (pgErr.Code == "42883" || pgErr.Code == "3F000"):
		// undefined_function or invalid_schema_name: migration 048 itself is missing.
		return 0, fmt.Errorf("schema predates version 48, build needs version %d: %w", latest, ErrSchemaBehind)
	case err != nil:
		return 0, fmt.Errorf("reading schema version: %w", err)
	}
	// golang-migrate stores no negative version; treat one as an empty ledger.
	applied := uint(max(version, 0))
	if dirty {
		return applied, fmt.Errorf("version %d: %w", applied, ErrDirty)
	}
	if applied < latest {
		return applied, fmt.Errorf("database is at version %d, build needs version %d: %w", applied, latest, ErrSchemaBehind)
	}
	return applied, nil
}
