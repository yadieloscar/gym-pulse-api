package model

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNewSavedMealItemUsesCurrentFoodFacts(t *testing.T) {
	food := Food{ID: uuid.New(), Per100g: Per100g{EnergyKcal: 379, ProteinG: 13.2, CarbsG: 67.7, FatG: 6.5}}
	item := NewSavedMealItem(food, Serving{Label: "1 cup (81 g)", Grams: 81}, 1.5)
	if !item.Available {
		t.Fatal("a plausible food is available")
	}
	energy, protein, carbs, fat := EntryValues(food.Per100g, 81, 1.5)
	if *item.EnergyKcal != energy || *item.ProteinG != protein || *item.CarbsG != carbs || *item.FatG != fat {
		t.Fatalf("item values %d %.1f %.1f %.1f differ from the entry snapshot maths", *item.EnergyKcal, *item.ProteinG, *item.CarbsG, *item.FatG)
	}
}

func TestSavedMealItemWithImplausibleFoodHasNoValues(t *testing.T) {
	for name, per := range map[string]Per100g{
		"energy above 900": {EnergyKcal: 901},
		"negative protein": {ProteinG: -1},
		"not a number":     {FatG: math.NaN()},
	} {
		item := NewSavedMealItem(Food{ID: uuid.New(), Per100g: per}, Serving{Label: "100 g", Grams: 100}, 1)
		if item.Available || item.EnergyKcal != nil || item.ProteinG != nil || item.CarbsG != nil || item.FatG != nil {
			t.Errorf("%s: an implausible food must be unavailable with no values", name)
		}
		if SumMealItems([]SavedMealItem{item}) != nil {
			t.Errorf("%s: totals are nil when an item is unavailable", name)
		}
	}
}

func TestSumMealItemsEqualsTheSumOfEntrySnapshots(t *testing.T) {
	oats := NewSavedMealItem(Food{Per100g: Per100g{EnergyKcal: 379, ProteinG: 13.2, CarbsG: 67.7, FatG: 6.5}}, Serving{Label: "81 g", Grams: 81}, 1)
	whey := NewSavedMealItem(Food{Per100g: Per100g{EnergyKcal: 400, ProteinG: 80, CarbsG: 10, FatG: 5}}, Serving{Label: "1 scoop", Grams: 30}, 1)
	banana := NewSavedMealItem(Food{Per100g: Per100g{EnergyKcal: 89, ProteinG: 1.1, CarbsG: 22.8, FatG: 0.3}}, Serving{Label: "1 medium", Grams: 118}, 1)
	totals := SumMealItems([]SavedMealItem{oats, whey, banana})
	if totals == nil {
		t.Fatal("totals expected")
	}
	wantEnergy := *oats.EnergyKcal + *whey.EnergyKcal + *banana.EnergyKcal
	wantProtein := math.Round((*oats.ProteinG+*whey.ProteinG+*banana.ProteinG)*10) / 10
	if totals.EnergyKcal != wantEnergy || totals.ProteinG != wantProtein {
		t.Fatalf("totals = %+v, want energy %d protein %.1f", totals, wantEnergy, wantProtein)
	}
	if totals.ProteinG*10 != math.Round(totals.ProteinG*10) {
		t.Fatalf("protein %v is not rounded to one decimal", totals.ProteinG)
	}
}

func TestSavedMealJSONShape(t *testing.T) {
	meal := SavedMeal{ID: uuid.New(), Name: "Usual breakfast", Items: []SavedMealItem{}, Revision: 1}
	body, err := json.Marshal(meal)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"id"`, `"name"`, `"items":[]`, `"totals":null`, `"revision":1`, `"last_logged_at":null`, `"created_at"`, `"updated_at"`} {
		if !strings.Contains(string(body), key) {
			t.Errorf("saved meal JSON missing %s: %s", key, body)
		}
	}
	empty, err := json.Marshal(SavedMeals{Meals: []SavedMeal{}})
	if err != nil {
		t.Fatal(err)
	}
	if string(empty) != `{"meals":[]}` {
		t.Fatalf("empty list = %s", empty)
	}
}

func TestSavedMealsMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../../migrations/040_create_saved_meals.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration := string(body)
	for _, marker := range []string{
		"CREATE TABLE saved_meals",
		"user_id         UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE",
		"CHECK (char_length(name) BETWEEN 1 AND 60)",
		"CREATE UNIQUE INDEX ux_saved_meals_user_name",
		"ON saved_meals(user_id, lower(name))",
		"CREATE TABLE saved_meal_items",
		"REFERENCES saved_meals(id) ON DELETE CASCADE",
		"CHECK (position BETWEEN 0 AND 19)",
		"REFERENCES food_catalog(id) ON DELETE RESTRICT",
		"REFERENCES user_foods(id) ON DELETE NO ACTION DEFERRABLE INITIALLY DEFERRED",
		"CHECK (amount BETWEEN 0.1 AND 50)",
		"CONSTRAINT ck_saved_meal_items_food CHECK",
		"CREATE UNIQUE INDEX ux_saved_meal_items_position",
		"CREATE UNIQUE INDEX ux_saved_meal_items_food_serving",
		"ALTER TABLE public.saved_meals ENABLE ROW LEVEL SECURITY",
		"ALTER TABLE public.saved_meal_items ENABLE ROW LEVEL SECURITY",
		"CREATE POLICY gym_pulse_api_access ON public.saved_meals",
		"CREATE POLICY gym_pulse_api_access ON public.saved_meal_items",
	} {
		if !strings.Contains(migration, marker) {
			t.Errorf("migration 040 missing %q", marker)
		}
	}
	// A meal stores no values: they come from its foods' current facts.
	for _, column := range []string{"energy_kcal", "protein_g", "carbs_g", "fat_g"} {
		if strings.Contains(migration, column) {
			t.Errorf("migration 040 must not store %s on a meal", column)
		}
	}
	down, err := os.ReadFile("../../migrations/040_create_saved_meals.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	items := strings.Index(string(down), "DROP TABLE IF EXISTS saved_meal_items;")
	meals := strings.Index(string(down), "DROP TABLE IF EXISTS saved_meals;")
	if items < 0 || meals < 0 || items > meals {
		t.Fatal("the down migration drops items before meals")
	}
}

func TestMatchSavedServingFollowsTheFoodByLabel(t *testing.T) {
	food := &Food{Servings: []Serving{{Label: "1 oz (28.35 g)", Grams: 28.35}, {Label: "1 scoop", Grams: 32}, HundredGramServing}}
	for _, test := range []struct {
		label string
		saved Serving
		want  Serving
	}{
		{"a weight the database rounded comes back exact", Serving{Label: "1 oz (28.35 g)", Grams: 28.4}, Serving{Label: "1 oz (28.35 g)", Grams: 28.35}},
		{"a serving the athlete re-weighed uses the new weight", Serving{Label: "1 scoop", Grams: 30}, Serving{Label: "1 scoop", Grams: 32}},
		{"a grams-only serving is kept as saved", Serving{Label: "45 g", Grams: 45}, Serving{Label: "45 g", Grams: 45}},
		{"a label the food no longer has keeps the saved weight", Serving{Label: "1 bar", Grams: 60}, Serving{Label: "1 bar", Grams: 60}},
	} {
		if got := MatchSavedServing(food, test.saved); got != test.want {
			t.Errorf("%s: got %+v, want %+v", test.label, got, test.want)
		}
	}
}

func TestMealSectionValuesAndJSON(t *testing.T) {
	for _, value := range []MealSection{MealSectionBreakfast, MealSectionLunch, MealSectionDinner, MealSectionSnacks} {
		if !ValidMealSection(&value) {
			t.Errorf("%q is one of the four headings", value)
		}
	}
	if !ValidMealSection(nil) {
		t.Error("no heading is valid")
	}
	for _, value := range []MealSection{"", "Breakfast", "snack", "brunch", "other"} {
		if ValidMealSection(&value) {
			t.Errorf("%q is not a heading", value)
		}
	}

	// Readers always get the key; a request from a client that knows no
	// headings marshals exactly as before, so its idempotency hash is unchanged.
	entry, err := json.Marshal(FoodEntry{Kind: FoodEntryKindQuickAdd})
	if err != nil || !strings.Contains(string(entry), `"section":null`) {
		t.Fatalf("an entry with no heading carries section null: %s %v", entry, err)
	}
	amount := 1.0
	request, err := json.Marshal(CreateFoodEntryRequest{Food: &FoodRef{}, Serving: &HundredGramServing, Amount: &amount, OperationKey: "k"})
	if err != nil || strings.Contains(string(request), "section") {
		t.Fatalf("a request without a heading must not mention it: %s %v", request, err)
	}
	meal, err := json.Marshal(SavedMeal{Items: []SavedMealItem{}})
	if err != nil || !strings.Contains(string(meal), `"section":null`) {
		t.Fatalf("a saved meal with no heading carries section null: %s %v", meal, err)
	}
	breakfast := MealSectionBreakfast
	meal, _ = json.Marshal(SavedMeal{Section: &breakfast, Items: []SavedMealItem{}})
	if !strings.Contains(string(meal), `"section":"breakfast"`) {
		t.Fatalf("a remembered heading is returned: %s", meal)
	}
}

func TestMealSectionsMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../../migrations/041_add_meal_sections.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration := string(body)
	check := "ADD COLUMN section TEXT CHECK (section IN ('breakfast', 'lunch', 'dinner', 'snacks'))"
	if strings.Count(migration, check) != 2 || !strings.Contains(migration, "ALTER TABLE food_entries") || !strings.Contains(migration, "ALTER TABLE saved_meals") {
		t.Fatal("migration 041 adds one nullable, checked section column to food_entries and to saved_meals")
	}
	// Nullable with no default: no row is rewritten and nothing is back-filled.
	for _, forbidden := range []string{"NOT NULL", "DEFAULT", "UPDATE ", "CREATE INDEX"} {
		if strings.Contains(migration, forbidden) {
			t.Errorf("migration 041 must not contain %q", forbidden)
		}
	}
	down, err := os.ReadFile("../../migrations/041_add_meal_sections.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(down), "DROP COLUMN IF EXISTS section") != 2 {
		t.Fatal("the down migration drops both columns")
	}
}
