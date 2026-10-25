package model

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestNutritionMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../../migrations/027_create_nutrition.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration := string(body)
	for _, invariant := range []string{
		"CREATE TABLE nutrition_days",
		"CREATE TABLE nutrition_targets",
		"REFERENCES auth.users(id) ON DELETE CASCADE",
		"CHECK (energy_kcal BETWEEN 0 AND 20000)",
		"CHECK (protein_g BETWEEN 0 AND 2000)",
		"CHECK (source IN ('manual', 'apple_health'))",
		"CHECK (revision > 0)",
		"CONSTRAINT ck_nutrition_days_has_value CHECK",
		"CREATE UNIQUE INDEX ux_nutrition_days_user_date",
		"ON nutrition_days(user_id, date)",
		"CHECK (protein_g > 0 AND protein_g <= 2000)",
		"CHECK (energy_kcal BETWEEN 1 AND 20000)",
		"CREATE UNIQUE INDEX ux_nutrition_targets_user_effective",
		"ON nutrition_targets(user_id, effective_from)",
		"ALTER TABLE public.nutrition_days ENABLE ROW LEVEL SECURITY",
		"ALTER TABLE public.nutrition_targets ENABLE ROW LEVEL SECURITY",
		"REVOKE ALL PRIVILEGES ON TABLE public.nutrition_days",
		"REVOKE ALL PRIVILEGES ON TABLE public.nutrition_targets",
		"CREATE POLICY gym_pulse_api_access ON public.nutrition_days",
		"CREATE POLICY gym_pulse_api_access ON public.nutrition_targets",
	} {
		if !strings.Contains(migration, invariant) {
			t.Errorf("nutrition migration missing %q", invariant)
		}
	}
	if strings.Count(migration, "REFERENCES auth.users(id) ON DELETE CASCADE") != 2 {
		t.Fatal("both tables must cascade from auth.users")
	}
}

func TestNutritionMigrationRollbackIsScoped(t *testing.T) {
	body, err := os.ReadFile("../../migrations/027_create_nutrition.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != "DROP TABLE IF EXISTS nutrition_targets;\nDROP TABLE IF EXISTS nutrition_days;" {
		t.Fatalf("unexpected rollback scope: %s", body)
	}
}

func TestNutritionJSONShape(t *testing.T) {
	var req SaveNutritionDayRequest
	if err := json.Unmarshal([]byte(`{"protein_g":162.5,"expected_revision":0,"operation_key":"op"}`), &req); err != nil {
		t.Fatal(err)
	}
	if req.ProteinG == nil || *req.ProteinG != 162.5 || req.EnergyKcal != nil || req.ExpectedRevision == nil || *req.ExpectedRevision != 0 || req.Source != "" {
		t.Fatalf("request decoded wrong: %+v", req)
	}
	if !req.HasValue() || (SaveNutritionDayRequest{}).HasValue() {
		t.Fatal("HasValue must reflect the four totals")
	}

	protein := 162.5
	day, err := json.Marshal(NutritionDay{Date: "2026-10-25", ProteinG: &protein, Source: NutritionSourceManual, Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, present := range []string{`"date":"2026-10-25"`, `"protein_g":162.5`, `"source":"manual"`, `"revision":1`, `"logged_at"`} {
		if !strings.Contains(string(day), present) {
			t.Errorf("day JSON missing %s: %s", present, day)
		}
	}
	for _, absent := range []string{"energy_kcal", "carbs_g", "fat_g", "user_id"} {
		if strings.Contains(string(day), absent) {
			t.Errorf("day JSON must omit %s when unset: %s", absent, day)
		}
	}

	target, err := json.Marshal(NutritionTarget{EffectiveFrom: "2026-10-25", ProteinG: 150, Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, present := range []string{`"effective_from":"2026-10-25"`, `"protein_g":150`, `"energy_kcal":null`, `"carbs_g":null`, `"fat_g":null`} {
		if !strings.Contains(string(target), present) {
			t.Errorf("target JSON missing %s: %s", present, target)
		}
	}

	empty, err := json.Marshal(NutritionRange{Days: []NutritionDay{}, Targets: []NutritionTarget{}})
	if err != nil || string(empty) != `{"days":[],"targets":[]}` {
		t.Fatalf("empty range JSON = %s, err=%v", empty, err)
	}
}
