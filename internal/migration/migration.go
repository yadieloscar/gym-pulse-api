// Package migration applies and checks the SQL schema migrations shipped with
// the API.
package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source"
	_ "github.com/golang-migrate/migrate/v4/source/file" // registers file://
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the pgx database/sql driver
)

// Dir is the migrations directory relative to the working directory of the
// server and migrate binaries, both locally and in the release image.
const Dir = "migrations"

var (
	// ErrSchemaBehind means the database has not applied every migration
	// shipped with this build.
	ErrSchemaBehind = errors.New("database schema is behind this build")
	// ErrSchemaDirty means a previous migration failed partway and needs
	// operator repair before any version can serve.
	ErrSchemaDirty = errors.New("database schema is dirty")
	// ErrNoMigrations means the migrations directory holds no up migration.
	ErrNoMigrations = errors.New("no up migrations found")
	// ErrSchemaDirtyAhead means a migration newer than this build failed
	// partway. This build needs none of it, so callers may serve with a
	// warning; operators must still repair the schema before the next release.
	ErrSchemaDirtyAhead = errors.New("database schema is dirty at a version newer than this build")
)

// versionSchema and versionTable pin where golang-migrate records the version,
// so Up and RequireCurrent agree even if the two connections' roles have
// different search paths.
const (
	versionSchema = "public"
	versionTable  = "schema_migrations"
)

// undefinedTable is PostgreSQL's SQLSTATE for a missing relation.
const undefinedTable = "42P01"

// Up applies every pending migration in dir and returns the resulting schema
// version. databaseURL must be a direct or session-mode connection, because
// golang-migrate holds a session-level advisory lock while it runs.
func Up(databaseURL, dir string) (uint, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return 0, fmt.Errorf("opening migration db: %w", err)
	}
	defer db.Close()

	driver, err := postgres.WithInstance(db, &postgres.Config{SchemaName: versionSchema, MigrationsTable: versionTable})
	if err != nil {
		return 0, fmt.Errorf("creating migration driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance("file://"+dir, "postgres", driver)
	if err != nil {
		return 0, fmt.Errorf("creating migrator: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return 0, fmt.Errorf("running migrations: %w", err)
	}

	version, _, err := m.Version()
	if err != nil {
		return 0, fmt.Errorf("reading migrated version: %w", err)
	}
	return version, nil
}

// Latest returns the highest up-migration version in dir.
func Latest(dir string) (uint, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("reading migrations directory: %w", err)
	}
	var latest uint
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		parsed, err := source.DefaultParse(entry.Name())
		if err != nil || parsed.Direction != source.Up {
			continue
		}
		latest = max(latest, parsed.Version)
	}
	if latest == 0 {
		return 0, fmt.Errorf("%w in %s", ErrNoMigrations, dir)
	}
	return latest, nil
}

// RowQuerier is the part of a pgx pool RequireCurrent needs.
type RowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// RequireCurrent returns nil when the database has applied at least the
// newest migration in dir and is not dirty. A newer schema is accepted so an
// earlier image stays deployable after a forward-compatible migration. A dirty
// version newer than this build returns ErrSchemaDirtyAhead, so a failed
// release migration does not stop the previous build from restarting.
//
// It reads golang-migrate's version table with a plain query and takes no
// advisory lock, so it is safe over a transaction-mode pooler.
func RequireCurrent(ctx context.Context, db RowQuerier, dir string) error {
	want, err := Latest(dir)
	if err != nil {
		return err
	}

	var (
		version int64
		dirty   bool
	)
	err = db.QueryRow(ctx, `SELECT version, dirty FROM `+versionSchema+`.`+versionTable+` LIMIT 1`).Scan(&version, &dirty)
	var pgErr *pgconn.PgError
	switch {
	case errors.Is(err, pgx.ErrNoRows), errors.As(err, &pgErr) && pgErr.Code == undefinedTable:
		return fmt.Errorf("%w: no migrations applied, want %d", ErrSchemaBehind, want)
	case err != nil:
		return fmt.Errorf("reading schema version: %w", err)
	case dirty && version > int64(want):
		return fmt.Errorf("%w: dirty at %d, this build needs %d", ErrSchemaDirtyAhead, version, want)
	case dirty:
		return fmt.Errorf("%w at version %d", ErrSchemaDirty, version)
	case version < int64(want):
		return fmt.Errorf("%w: at %d, want %d", ErrSchemaBehind, version, want)
	}
	return nil
}
