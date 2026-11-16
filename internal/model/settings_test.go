package model

import (
	"os"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
)

// settingsPalettes is every palette name the API accepts. irisViolet was added
// in contract 1.9.0; mossChartreuse, oliveCitrine, and slateMint are no longer
// offered by the app but older builds still send them.
var settingsPalettes = []string{
	"obsidianEmber", "abyssCerulean", "mossChartreuse", "voltLime", "oliveCitrine", "slateMint", "irisViolet",
}

func TestDefaultUserSettings(t *testing.T) {
	got := DefaultUserSettings()
	want := UserSettings{WeightUnit: "lb", WeeklyGoal: 5, Palette: "voltLime"}
	if got != want {
		t.Errorf("DefaultUserSettings() = %+v, want %+v", got, want)
	}
	if err := validator.New().Struct(got); err != nil {
		t.Errorf("default settings do not validate: %v", err)
	}
}

func TestSettingsPaletteValidation(t *testing.T) {
	v := validator.New()
	for _, palette := range settingsPalettes {
		if err := v.Struct(UserSettings{WeightUnit: "lb", WeeklyGoal: 5, Palette: palette}); err != nil {
			t.Errorf("UserSettings palette %q rejected: %v", palette, err)
		}
		if err := v.Struct(UpdateUserSettingsRequest{Palette: &palette}); err != nil {
			t.Errorf("UpdateUserSettingsRequest palette %q rejected: %v", palette, err)
		}
	}
	for _, palette := range []string{"neonGrape", "irisviolet", "iris", ""} {
		if err := v.Struct(UserSettings{WeightUnit: "lb", WeeklyGoal: 5, Palette: palette}); err == nil {
			t.Errorf("UserSettings palette %q accepted", palette)
		}
		if err := v.Struct(UpdateUserSettingsRequest{Palette: &palette}); err == nil {
			t.Errorf("UpdateUserSettingsRequest palette %q accepted", palette)
		}
	}
	if err := v.Struct(UpdateUserSettingsRequest{}); err != nil {
		t.Errorf("empty update rejected: %v", err)
	}
}

// TestSettingsPaletteMigration keeps the database check, the column default,
// and the request validator in step. A palette the validator accepts but the
// check rejects would surface as a 500 instead of a stored value.
func TestSettingsPaletteMigration(t *testing.T) {
	up, err := os.ReadFile("../../migrations/042_add_settings_palette_iris.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	upSQL := string(up)
	for _, palette := range settingsPalettes {
		if !strings.Contains(upSQL, "'"+palette+"'") {
			t.Errorf("migration 042 check does not allow %q", palette)
		}
	}
	if !strings.Contains(upSQL, "SET DEFAULT '"+DefaultUserSettings().Palette+"'") {
		t.Errorf("migration 042 column default does not match DefaultUserSettings palette %q", DefaultUserSettings().Palette)
	}
	if strings.Contains(upSQL, "UPDATE ") {
		t.Error("migration 042 must not rewrite stored palettes")
	}

	down, err := os.ReadFile("../../migrations/042_add_settings_palette_iris.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	downSQL := string(down)
	_, check, found := strings.Cut(downSQL, "CHECK")
	if !found {
		t.Fatal("down migration has no CHECK")
	}
	if strings.Contains(check, "irisViolet") {
		t.Error("down migration must remove irisViolet from the check")
	}
	for _, palette := range settingsPalettes[:6] {
		if !strings.Contains(check, "'"+palette+"'") {
			t.Errorf("down migration check drops %q", palette)
		}
	}
	if !strings.Contains(downSQL, "WHERE palette = 'irisViolet'") || !strings.Contains(downSQL, "SET DEFAULT 'obsidianEmber'") {
		t.Error("down migration must move irisViolet rows first and restore the obsidianEmber default")
	}
}
