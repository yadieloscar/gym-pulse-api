package dbmigrate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeRow struct {
	version int64
	dirty   bool
	err     error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	version, ok := dest[0].(*int64)
	if !ok {
		return errors.New("first scan target is not *int64")
	}
	dirty, ok := dest[1].(*bool)
	if !ok {
		return errors.New("second scan target is not *bool")
	}
	*version, *dirty = r.version, r.dirty
	return nil
}

type fakeQuerier struct {
	row   fakeRow
	query string
}

func (q *fakeQuerier) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	q.query = sql
	return q.row
}

// sourceWith returns a file:// source holding empty migrations named names.
func sourceWith(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return "file://" + dir
}

func TestLatestVersion(t *testing.T) {
	source := sourceWith(t, "001_first.up.sql", "001_first.down.sql", "048_newest.up.sql", "049_only_down.down.sql", "README.md")
	got, err := LatestVersion(source)
	if err != nil {
		t.Fatal(err)
	}
	if got != 48 {
		t.Errorf("LatestVersion() = %d, want 48", got)
	}
}

func TestLatestVersionRejectsUnusableSources(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "not a file URL", source: "postgres://example"},
		{name: "missing directory", source: "file://" + filepath.Join(t.TempDir(), "absent")},
		{name: "no migrations", source: sourceWith(t, "README.md")},
		{name: "unnumbered migration", source: sourceWith(t, "first_thing.up.sql")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := LatestVersion(tc.source); err == nil {
				t.Error("LatestVersion() succeeded, want an error")
			}
		})
	}
}

// TestLatestVersionReadsTheRepositoryMigrations keeps the parser honest
// against the real file names the image ships.
func TestLatestVersionReadsTheRepositoryMigrations(t *testing.T) {
	got, err := LatestVersion("file://../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	if got < 48 {
		t.Errorf("LatestVersion() = %d, want at least 48", got)
	}
}

func TestCheckCurrent(t *testing.T) {
	source := sourceWith(t, "047_older.up.sql", "048_newest.up.sql")
	tests := []struct {
		name        string
		row         fakeRow
		wantVersion uint
		wantErr     error
		wantAnyErr  bool
	}{
		{name: "current", row: fakeRow{version: 48}, wantVersion: 48},
		{name: "database newer than build", row: fakeRow{version: 50}, wantVersion: 50},
		{name: "behind", row: fakeRow{version: 47}, wantVersion: 47, wantErr: ErrSchemaBehind},
		{name: "dirty at the newest version", row: fakeRow{version: 48, dirty: true}, wantVersion: 48, wantErr: ErrDirty},
		{name: "empty ledger", row: fakeRow{err: pgx.ErrNoRows}, wantErr: ErrSchemaBehind},
		{name: "version function missing", row: fakeRow{err: &pgconn.PgError{Code: "42883"}}, wantErr: ErrSchemaBehind},
		{name: "private schema missing", row: fakeRow{err: &pgconn.PgError{Code: "3F000"}}, wantErr: ErrSchemaBehind},
		{name: "other database failure", row: fakeRow{err: errors.New("connection reset")}, wantAnyErr: true},
		{name: "negative version", row: fakeRow{version: -1}, wantErr: ErrSchemaBehind},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			querier := &fakeQuerier{row: tc.row}
			got, err := CheckCurrent(context.Background(), querier, source)
			if got != tc.wantVersion {
				t.Errorf("CheckCurrent() version = %d, want %d", got, tc.wantVersion)
			}
			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("CheckCurrent() error = %v, want %v", err, tc.wantErr)
				}
			case tc.wantAnyErr:
				if err == nil || errors.Is(err, ErrSchemaBehind) || errors.Is(err, ErrDirty) {
					t.Errorf("CheckCurrent() error = %v, want an unclassified error", err)
				}
			case err != nil:
				t.Errorf("CheckCurrent() error = %v, want nil", err)
			}
			if querier.query != schemaVersionQuery {
				t.Errorf("CheckCurrent() ran %q, want the schema_version function", querier.query)
			}
		})
	}
}

func TestCheckCurrentFailsWithoutMigrationFiles(t *testing.T) {
	querier := &fakeQuerier{row: fakeRow{version: 48}}
	if _, err := CheckCurrent(context.Background(), querier, "file://"+filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Error("CheckCurrent() succeeded without migration files, want an error")
	}
	if querier.query != "" {
		t.Error("CheckCurrent() queried the database before it knew the build's version")
	}
}
