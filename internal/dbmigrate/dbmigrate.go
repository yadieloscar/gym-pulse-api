// Package dbmigrate applies the SQL files in migrations/ to PostgreSQL as the
// schema owner. Both the cmd/migrate pre-deploy job and the development-time
// server startup use it, so the two paths cannot drift.
package dbmigrate

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file" // file:// migration source
	_ "github.com/jackc/pgx/v5/stdlib"                   // pgx database/sql driver
)

// DefaultSource is the migrations directory relative to the working directory
// of the server and migrate binaries (/app in the container image).
const DefaultSource = "file://migrations"

// ErrDirty means a previous run failed part-way and golang-migrate recorded
// the version as dirty. The file rolled back, but the ledger needs an
// operator to fix the cause and reset the flag (docs/RELEASE_RUNBOOK.md).
var ErrDirty = errors.New("migration version is dirty; fix the cause, reset schema_migrations.dirty, and rerun")

// Result reports the schema version after Up.
type Result struct {
	// Version is the latest applied migration; zero for an empty schema.
	Version uint
	// Changed is false when the schema was already current.
	Changed bool
}

// Up applies every pending up migration from source through databaseURL,
// which must connect as the schema owner over a direct or session-mode
// connection because golang-migrate holds a session advisory lock.
func Up(databaseURL, source string) (Result, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return Result{}, fmt.Errorf("opening migration db: %w", err)
	}
	defer db.Close()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return Result{}, fmt.Errorf("creating migration driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(source, "postgres", driver)
	if err != nil {
		return Result{}, fmt.Errorf("creating migrator: %w", err)
	}

	changed := true
	if err := m.Up(); err != nil {
		if !errors.Is(err, migrate.ErrNoChange) {
			return Result{}, fmt.Errorf("running migrations: %w", err)
		}
		changed = false
	}

	version, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return Result{}, fmt.Errorf("reading migration version: %w", err)
	}
	if dirty {
		return Result{}, fmt.Errorf("version %d: %w", version, ErrDirty)
	}
	return Result{Version: version, Changed: changed}, nil
}
