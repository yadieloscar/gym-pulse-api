package migration

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
	*dest[0].(*int64) = r.version //nolint:forcetypeassert // RequireCurrent's scan targets are fixed
	*dest[1].(*bool) = r.dirty    //nolint:forcetypeassert // RequireCurrent's scan targets are fixed
	return nil
}

type fakeQuerier struct{ row fakeRow }

func (q fakeQuerier) QueryRow(context.Context, string, ...any) pgx.Row { return q.row }

func migrationsDir(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("SELECT 1;"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLatest(t *testing.T) {
	dir := migrationsDir(t,
		"001_init.up.sql", "001_init.down.sql",
		"010_more.up.sql", "010_more.down.sql",
		"011_only_down.down.sql",
		"README.md",
	)
	if err := os.Mkdir(filepath.Join(dir, "999_dir.up.sql"), 0o700); err != nil {
		t.Fatal(err)
	}

	got, err := Latest(dir)
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	if got != 10 {
		t.Errorf("Latest() = %d, want 10", got)
	}
}

func TestLatestErrors(t *testing.T) {
	if _, err := Latest(migrationsDir(t, "001_init.down.sql")); !errors.Is(err, ErrNoMigrations) {
		t.Errorf("Latest(no up files) error = %v, want ErrNoMigrations", err)
	}
	if _, err := Latest(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("Latest(missing dir) error = nil, want error")
	}
}

func TestLatestMatchesShippedMigrations(t *testing.T) {
	// Guards the release gate itself: the shipped directory must parse.
	got, err := Latest("../../migrations")
	if err != nil {
		t.Fatalf("Latest(shipped) error = %v", err)
	}
	if got == 0 {
		t.Error("Latest(shipped) = 0, want a positive version")
	}
}

func TestRequireCurrent(t *testing.T) {
	dir := migrationsDir(t, "005_x.up.sql", "005_x.down.sql")
	scanFailure := errors.New("connection reset")

	tests := []struct {
		name    string
		row     fakeRow
		wantErr error
	}{
		{name: "current", row: fakeRow{version: 5}},
		{name: "newer schema accepted for rollback images", row: fakeRow{version: 6}},
		{name: "behind", row: fakeRow{version: 4}, wantErr: ErrSchemaBehind},
		{name: "dirty at this build", row: fakeRow{version: 5, dirty: true}, wantErr: ErrSchemaDirty},
		{name: "dirty below this build", row: fakeRow{version: 4, dirty: true}, wantErr: ErrSchemaDirty},
		{name: "dirty newer migration", row: fakeRow{version: 6, dirty: true}, wantErr: ErrSchemaDirtyAhead},
		{name: "empty version table", row: fakeRow{err: pgx.ErrNoRows}, wantErr: ErrSchemaBehind},
		{name: "missing version table", row: fakeRow{err: &pgconn.PgError{Code: undefinedTable}}, wantErr: ErrSchemaBehind},
		{name: "query failure", row: fakeRow{err: scanFailure}, wantErr: scanFailure},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RequireCurrent(t.Context(), fakeQuerier{row: tt.row}, dir)
			if tt.wantErr == nil && err != nil {
				t.Fatalf("RequireCurrent() error = %v, want nil", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("RequireCurrent() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestRequireCurrentWithoutMigrations(t *testing.T) {
	err := RequireCurrent(t.Context(), fakeQuerier{}, migrationsDir(t))
	if !errors.Is(err, ErrNoMigrations) {
		t.Errorf("RequireCurrent(empty dir) error = %v, want ErrNoMigrations", err)
	}
}

func TestUpHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// sql.Open is lazy, so a cancelled context fails before any network call.
	_, err := Up(ctx, "postgres://unused.invalid:5432/db", migrationsDir(t, "001_x.up.sql"))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Up(cancelled) error = %v, want context.Canceled", err)
	}
}
