package model

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestFoodLoggingMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../../migrations/030_create_food_logging.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration := string(body)
	for _, invariant := range []string{
		"CREATE TABLE food_catalog",
		"CREATE TABLE user_foods",
		"CREATE TABLE food_entries",
		"CREATE TABLE food_barcode_misses",
		"CHECK (source IN ('usda', 'open_food_facts'))",
		"CHECK (energy_kcal_100g BETWEEN 0 AND 900)",
		"CHECK (protein_g_100g BETWEEN 0 AND 100)",
		"GENERATED ALWAYS AS (to_tsvector('simple', name || ' ' || coalesce(brand, ''))) STORED",
		"CREATE UNIQUE INDEX ux_food_catalog_source_id",
		"CREATE INDEX ix_food_catalog_gtin",
		"CREATE INDEX ix_food_catalog_search",
		"USING GIN (search_vector)",
		"CHECK (serving_g > 0 AND serving_g <= 5000)",
		"CHECK (kind IN ('food', 'quick_add'))",
		"REFERENCES food_catalog(id) ON DELETE RESTRICT",
		"REFERENCES user_foods(id) ON DELETE RESTRICT",
		"CHECK (amount BETWEEN 0.1 AND 50)",
		"CONSTRAINT ck_food_entries_kind CHECK",
		"CREATE INDEX ix_food_entries_user_date",
		"CREATE INDEX ix_food_entries_user_logged",
		"CHECK (source IN ('manual', 'apple_health', 'foods'))",
		"ALTER TABLE public.food_catalog ENABLE ROW LEVEL SECURITY",
		"ALTER TABLE public.user_foods ENABLE ROW LEVEL SECURITY",
		"ALTER TABLE public.food_entries ENABLE ROW LEVEL SECURITY",
		"ALTER TABLE public.food_barcode_misses ENABLE ROW LEVEL SECURITY",
		"CREATE POLICY gym_pulse_api_access ON public.food_catalog",
		"CREATE POLICY gym_pulse_api_access ON public.user_foods",
		"CREATE POLICY gym_pulse_api_access ON public.food_entries",
		"CREATE POLICY gym_pulse_api_access ON public.food_barcode_misses",
	} {
		if !strings.Contains(migration, invariant) {
			t.Errorf("food logging migration missing %q", invariant)
		}
	}
	if strings.Count(migration, "REFERENCES auth.users(id) ON DELETE CASCADE") != 2 {
		t.Fatal("user_foods and food_entries must cascade from auth.users; the catalog has no owner")
	}
	if strings.Contains(migration, "user_id") && strings.Contains(migration[strings.Index(migration, "CREATE TABLE food_catalog"):strings.Index(migration, "CREATE TABLE user_foods")], "user_id") {
		t.Fatal("food_catalog must not carry a user_id")
	}
}

func TestFoodLoggingMigrationRollbackIsGuarded(t *testing.T) {
	body, err := os.ReadFile("../../migrations/030_create_food_logging.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	down := string(body)
	for _, invariant := range []string{
		"WHERE source = 'foods'",
		"RAISE EXCEPTION",
		"CHECK (source IN ('manual', 'apple_health'))",
		"DROP TABLE IF EXISTS food_entries;",
		"DROP TABLE IF EXISTS food_barcode_misses;",
		"DROP TABLE IF EXISTS user_foods;",
		"DROP TABLE IF EXISTS food_catalog;",
	} {
		if !strings.Contains(down, invariant) {
			t.Errorf("rollback missing %q", invariant)
		}
	}
	if strings.Index(down, "DROP TABLE IF EXISTS food_entries;") > strings.Index(down, "DROP TABLE IF EXISTS food_catalog;") {
		t.Fatal("entries must drop before the tables they reference")
	}
}

func TestNormalizeGTIN(t *testing.T) {
	for _, test := range []struct {
		in, want string
		ok       bool
	}{
		{"096619123456", "", false}, // UPC-A with a wrong check digit
		{"0 96619 12345 2", "00096619123452", true},
		{"96619123457", "", false}, // 11 digits
		{"5000112637922", "05000112637922", true},
		{"05000112637922", "05000112637922", true},
		{"50001126", "", false}, // EAN-8 wrong check
		{"96385074", "00000096385074", true},
		{"4006381333931", "04006381333931", true},
		{"abc", "", false},
		{"", "", false},
	} {
		got, err := NormalizeGTIN(test.in)
		if test.ok && (err != nil || got != test.want) {
			t.Errorf("%q: got %q, %v; want %q", test.in, got, err, test.want)
		}
		if !test.ok && err == nil {
			t.Errorf("%q: expected an error, got %q", test.in, got)
		}
	}
	if DisplayGTIN("00096619123452") != "0096619123452" {
		t.Errorf("display drops only one leading zero: %s", DisplayGTIN("00096619123452"))
	}
	if DisplayGTIN("14006381333938") != "14006381333938" {
		t.Error("a true 14-digit code displays whole")
	}
	forms := ShortGTINForms("00096619123452")
	if len(forms) != 2 || forms[0] != "0096619123452" || forms[1] != "096619123452" {
		t.Errorf("UPC-A forms: %v", forms)
	}
	forms = ShortGTINForms("00000096385074")
	if len(forms) != 2 || forms[1] != "96385074" {
		t.Errorf("EAN-8 forms: %v", forms)
	}
}

func TestEntryValuesAndServings(t *testing.T) {
	per := Per100g{EnergyKcal: 59, ProteinG: 10.2, CarbsG: 3.6, FatG: 0.4}
	energy, protein, carbs, fat := EntryValues(per, 170, 1.5)
	if energy != 150 || protein != 26 || carbs != 9.2 || fat != 1 {
		t.Errorf("values: %d %v %v %v", energy, protein, carbs, fat)
	}
	servings := EnsureHundredGramServing([]Serving{{Label: "1 cup", Grams: 245}, {Label: "bad", Grams: 0}})
	if len(servings) != 2 || servings[1] != HundredGramServing {
		t.Errorf("servings: %+v", servings)
	}
	if got := EnsureHundredGramServing([]Serving{HundredGramServing}); len(got) != 1 {
		t.Error("100 g is not duplicated")
	}
	food := &Food{Servings: servings}
	if _, err := ResolveServing(food, Serving{Label: "1 cup", Grams: 245}); err != nil {
		t.Error(err)
	}
	if _, err := ResolveServing(food, Serving{Label: "35 g", Grams: 35}); err != nil {
		t.Error("grams-only serving accepted:", err)
	}
	if _, err := ResolveServing(food, Serving{Label: "1 cup", Grams: 200}); err == nil {
		t.Error("a serving with changed grams is rejected")
	}
	if !PlausiblePer100g(per) || PlausiblePer100g(Per100g{EnergyKcal: 950}) || PlausiblePer100g(Per100g{ProteinG: 101}) || PlausiblePer100g(Per100g{FatG: -1}) {
		t.Error("plausibility bounds")
	}
	if got := UserFoodPer100g(30, 120, 24, 3, 1.5); got.EnergyKcal != 400 || got.ProteinG != 80 || got.CarbsG != 10 || got.FatG != 5 {
		t.Errorf("per 100 g from a 30 g serving: %+v", got)
	}
}

func TestFoodJSONShape(t *testing.T) {
	body, err := json.Marshal(Food{Servings: []Serving{HundredGramServing}})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"brand":null`, `"barcode":null`, `"attribution":null`, `"revision":null`, `"archived":false`} {
		if !strings.Contains(string(body), key) {
			t.Errorf("food JSON lacks %s: %s", key, body)
		}
	}
	if strings.Contains(string(body), "fetched_at") || strings.Contains(string(body), "SourceID") {
		t.Error("catalog bookkeeping leaked into JSON")
	}
	entry, _ := json.Marshal(FoodEntry{Kind: FoodEntryKindQuickAdd})
	if !strings.Contains(string(entry), `"food":null`) || strings.Contains(string(entry), "energy_kcal") {
		t.Errorf("quick add omits absent values and keeps nullable keys: %s", entry)
	}
	var req CreateFoodEntryRequest
	if err := json.Unmarshal([]byte(`{"food":{"id":"6f1d2a4e-0b3c-4d5e-8f90-123456789abc"},"serving":{"label":"100 g","grams":100},"amount":1.5,"operation_key":"op"}`), &req); err != nil {
		t.Fatal(err)
	}
	if req.Food == nil || req.Amount == nil || *req.Amount != 1.5 || req.QuickAdd != nil {
		t.Errorf("request decode: %+v", req)
	}
}
