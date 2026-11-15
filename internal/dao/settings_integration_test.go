//go:build integration

package dao

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func settingsStrPtr(s string) *string { return &s }

func TestSettingsDefaultsForAccountWithoutRow(t *testing.T) {
	f := newMobilityFixture(t)
	settings := NewSettingsDAO(f.pool)

	got, err := settings.Get(f.ctx, f.userID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	want := model.UserSettings{WeightUnit: "lb", WeeklyGoal: 5, Palette: "voltLime"}
	if *got != want {
		t.Errorf("Get with no row = %+v, want %+v", *got, want)
	}
	var rows int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM user_settings WHERE user_id = $1`, f.userID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("Get stored %d rows, want 0", rows)
	}

	// A first write that omits the palette stores the default.
	goal := 3
	got, err = settings.Patch(f.ctx, f.userID, model.UpdateUserSettingsRequest{WeeklyGoal: &goal})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	want = model.UserSettings{WeightUnit: "lb", WeeklyGoal: 3, Palette: "voltLime"}
	if *got != want {
		t.Errorf("first Patch = %+v, want %+v", *got, want)
	}
}

func TestSettingsPaletteColumnDefaultIsVolt(t *testing.T) {
	f := newMobilityFixture(t)

	var palette string
	err := f.pool.QueryRow(f.ctx,
		`INSERT INTO user_settings (user_id, weight_unit, weekly_goal) VALUES ($1, 'kg', 4) RETURNING palette`,
		f.userID).Scan(&palette)
	if err != nil {
		t.Fatal(err)
	}
	if palette != model.DefaultUserSettings().Palette {
		t.Errorf("column default = %q, want %q", palette, model.DefaultUserSettings().Palette)
	}
}

func TestSettingsStoresEveryAcceptedPalette(t *testing.T) {
	f := newMobilityFixture(t)
	settings := NewSettingsDAO(f.pool)

	unit, goal := "kg", 4
	if _, err := settings.Patch(f.ctx, f.userID, model.UpdateUserSettingsRequest{WeightUnit: &unit, WeeklyGoal: &goal}); err != nil {
		t.Fatalf("seed Patch: %v", err)
	}

	for _, palette := range []string{"irisViolet", "obsidianEmber", "abyssCerulean", "mossChartreuse", "oliveCitrine", "slateMint", "voltLime"} {
		got, err := settings.Patch(f.ctx, f.userID, model.UpdateUserSettingsRequest{Palette: settingsStrPtr(palette)})
		if err != nil {
			t.Fatalf("Patch(%q): %v", palette, err)
		}
		want := model.UserSettings{WeightUnit: "kg", WeeklyGoal: 4, Palette: palette}
		if *got != want {
			t.Errorf("Patch(%q) = %+v, want %+v", palette, *got, want)
		}
		read, err := settings.Get(f.ctx, f.userID)
		if err != nil {
			t.Fatalf("Get after Patch(%q): %v", palette, err)
		}
		if *read != want {
			t.Errorf("Get after Patch(%q) = %+v, want %+v", palette, *read, want)
		}
	}

	if err := settings.Upsert(f.ctx, f.userID, &model.UserSettings{WeightUnit: "lb", WeeklyGoal: 2, Palette: "irisViolet"}); err != nil {
		t.Fatalf("Upsert irisViolet: %v", err)
	}
}

func TestSettingsPaletteCheckRejectsUnknownName(t *testing.T) {
	f := newMobilityFixture(t)
	settings := NewSettingsDAO(f.pool)

	if _, err := settings.Patch(f.ctx, f.userID, model.UpdateUserSettingsRequest{Palette: settingsStrPtr("irisViolet")}); err != nil {
		t.Fatalf("seed Patch: %v", err)
	}
	_, err := settings.Patch(f.ctx, f.userID, model.UpdateUserSettingsRequest{Palette: settingsStrPtr("neonGrape")})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "user_settings_palette_check" {
		t.Fatalf("Patch(neonGrape) error = %v, want a user_settings_palette_check violation", err)
	}
	got, err := settings.Get(f.ctx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Palette != "irisViolet" {
		t.Errorf("palette after rejected write = %q, want irisViolet", got.Palette)
	}
}
