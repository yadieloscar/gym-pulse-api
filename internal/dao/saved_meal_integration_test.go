//go:build integration

package dao

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type savedMealFixture struct {
	*mobilityFixture
	meals   SavedMealDAO
	foods   FoodDAO
	entries FoodEntryDAO
	keys    int
}

func newSavedMealFixture(t *testing.T) *savedMealFixture {
	t.Helper()
	base := newMobilityFixture(t)
	return &savedMealFixture{mobilityFixture: base, meals: NewSavedMealDAO(base.pool), foods: NewFoodDAO(base.pool), entries: NewFoodEntryDAO(base.pool)}
}

// key returns a fresh operation key and matching request hash.
func (f *savedMealFixture) key(prefix string) (string, string) {
	f.keys++
	k := fmt.Sprintf("%s-%d-%s", prefix, f.keys, uuid.NewString())
	return k, "hash-" + k
}

func (f *savedMealFixture) catalogFood(t *testing.T, name string, per model.Per100g, servings ...model.Serving) model.Food {
	t.Helper()
	servings = append(servings, model.HundredGramServing)
	saved, err := f.foods.UpsertCatalogFoods(f.ctx, []model.Food{{
		Source: model.FoodSourceUSDA, SourceID: "it-" + uuid.NewString(), Name: name,
		Servings: servings, Per100g: per, FetchedAt: time.Now(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	return saved[0]
}

func (f *savedMealFixture) ownFood(t *testing.T, userID uuid.UUID, name string) model.Food {
	t.Helper()
	k, h := f.key("own-food")
	food, _, err := f.foods.CreateUserFood(f.ctx, userID, model.SaveUserFoodRequest{
		Name: name, Serving: model.Serving{Label: "1 scoop", Grams: 30},
		EnergyKcal: 120, ProteinG: 24, CarbsG: 3, FatG: 1.5,
	}, nil, k, h)
	if err != nil {
		t.Fatal(err)
	}
	return *food
}

func itemFor(food model.Food, serving model.Serving, amount float64) model.MealItemInput {
	return model.MealItemInput{Food: model.FoodRef{ID: food.ID}, Serving: serving, Amount: amount}
}

func (f *savedMealFixture) createMeal(t *testing.T, userID uuid.UUID, name string, items ...model.MealItemInput) *model.SavedMeal {
	t.Helper()
	k, h := f.key("meal")
	meal, outcome, err := f.meals.CreateMeal(f.ctx, userID, model.SaveMealRequest{Name: name, Items: items}, k, h)
	if err != nil || outcome != model.SavedMealCreated {
		t.Fatalf("create %q: outcome=%v err=%v", name, outcome, err)
	}
	return meal
}

func (f *savedMealFixture) logMeal(t *testing.T, userID, mealID uuid.UUID, date string, items []model.MealItemInput) *model.MealLogged {
	t.Helper()
	k, h := f.key("log")
	logged, outcome, err := f.meals.LogMeal(f.ctx, userID, mealID, date, items, time.Now(), k, h)
	if err != nil || outcome != model.SavedMealLogged {
		t.Fatalf("log: outcome=%v err=%v", outcome, err)
	}
	return logged
}

func (f *savedMealFixture) mealCount(t *testing.T, userID uuid.UUID) (meals, items int) {
	t.Helper()
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM saved_meals WHERE user_id=$1`, userID).Scan(&meals); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM saved_meal_items i JOIN saved_meals m ON m.id=i.meal_id WHERE m.user_id=$1`, userID).Scan(&items); err != nil {
		t.Fatal(err)
	}
	return meals, items
}

func (f *savedMealFixture) day(t *testing.T, userID uuid.UUID, date string) *model.DayEntries {
	t.Helper()
	day, err := f.entries.ListDayEntries(f.ctx, userID, date)
	if err != nil {
		t.Fatal(err)
	}
	return day
}

// assertDayIsSumOfEntries checks spec 014's invariant after a meal log.
func assertDayIsSumOfEntries(t *testing.T, day *model.DayEntries) {
	t.Helper()
	if day.Day == nil {
		t.Fatal("day totals missing")
	}
	var energy int
	var protein, carbs, fat float64
	for _, e := range day.Entries {
		if e.EnergyKcal != nil {
			energy += *e.EnergyKcal
		}
		if e.ProteinG != nil {
			protein += *e.ProteinG
		}
		if e.CarbsG != nil {
			carbs += *e.CarbsG
		}
		if e.FatG != nil {
			fat += *e.FatG
		}
	}
	close := func(a, b float64) bool { return math.Abs(a-b) < 0.05 }
	if day.Day.Source != model.NutritionSourceFoods || *day.Day.EnergyKcal != energy || !close(*day.Day.ProteinG, protein) || !close(*day.Day.CarbsG, carbs) || !close(*day.Day.FatG, fat) {
		t.Fatalf("day %+v is not the sum of its %d entries (%d kcal, %.1f/%.1f/%.1f)", day.Day, len(day.Entries), energy, protein, carbs, fat)
	}
}

func wantItemFault(t *testing.T, label string, err error, index int) {
	t.Helper()
	var validation *model.ValidationError
	if !errors.As(err, &validation) || validation.Field != "items" || validation.Index == nil || *validation.Index != index {
		t.Fatalf("%s: want a validation error on items[%d], got %v (%+v)", label, index, err, validation)
	}
}

var (
	oatsPer   = model.Per100g{EnergyKcal: 379, ProteinG: 13.2, CarbsG: 67.7, FatG: 6.5}
	bananaPer = model.Per100g{EnergyKcal: 89, ProteinG: 1.1, CarbsG: 22.8, FatG: 0.3}
	cup       = model.Serving{Label: "1 cup (81 g)", Grams: 81}
	medium    = model.Serving{Label: "1 medium (118 g)", Grams: 118}
	scoop     = model.Serving{Label: "1 scoop", Grams: 30}
)

func TestSavedMealCreateReplayNameListUpdateDelete(t *testing.T) {
	f := newSavedMealFixture(t)
	oats := f.catalogFood(t, "Oats", oatsPer, cup)
	whey := f.ownFood(t, f.userID, "Whey")

	req := model.SaveMealRequest{Name: "Usual breakfast", Items: []model.MealItemInput{itemFor(oats, cup, 1), itemFor(whey, scoop, 1.5)}}
	k, h := f.key("meal")
	meal, outcome, err := f.meals.CreateMeal(f.ctx, f.userID, req, k, h)
	if err != nil || outcome != model.SavedMealCreated {
		t.Fatalf("create: %v %v", outcome, err)
	}
	if meal.ID == uuid.Nil || meal.Revision != 1 || meal.LastLoggedAt != nil || len(meal.Items) != 2 ||
		meal.Items[0].Food.ID != oats.ID || meal.Items[1].Food.ID != whey.ID || meal.Items[1].Food.Source != model.FoodSourceOwn ||
		meal.Items[0].Serving != cup || meal.Items[1].Amount != 1.5 || !meal.Items[0].Available {
		t.Fatalf("stored meal wrong: %+v", meal)
	}
	wantEnergy, wantProtein, _, _ := model.EntryValues(oatsPer, 81, 1)
	if *meal.Items[0].EnergyKcal != wantEnergy || *meal.Items[0].ProteinG != wantProtein || meal.Totals == nil ||
		meal.Totals.EnergyKcal != wantEnergy+*meal.Items[1].EnergyKcal {
		t.Fatalf("item values and totals follow the foods' facts: %+v totals %+v", meal.Items[0], meal.Totals)
	}

	replay, outcome, err := f.meals.CreateMeal(f.ctx, f.userID, req, k, h)
	if err != nil || outcome != model.SavedMealCreated || replay.ID != meal.ID || len(replay.Items) != 2 || replay.Totals.EnergyKcal != meal.Totals.EnergyKcal {
		t.Fatalf("a verbatim retry replays the meal: %+v %v %v", replay, outcome, err)
	}
	if meals, items := f.mealCount(t, f.userID); meals != 1 || items != 2 {
		t.Fatalf("a retry adds no rows: %d meals, %d items", meals, items)
	}
	var conflict *model.ConflictError
	if _, _, err := f.meals.CreateMeal(f.ctx, f.userID, req, k, "another-hash"); !errors.As(err, &conflict) || !strings.Contains(conflict.Message, "idempotency") {
		t.Fatalf("same key, different payload → idempotency conflict: %v", err)
	}

	k2, h2 := f.key("meal")
	var validation *model.ValidationError
	if _, _, err := f.meals.CreateMeal(f.ctx, f.userID, model.SaveMealRequest{Name: "USUAL BREAKFAST", Items: req.Items}, k2, h2); !errors.As(err, &validation) || validation.Field != "name" {
		t.Fatalf("a name differing only by case is refused on name: %v", err)
	}
	other := f.newUser(t)
	f.createMeal(t, other, "Usual breakfast", itemFor(oats, cup, 1)) // another athlete may use the same name

	second := f.createMeal(t, f.userID, "Snack", itemFor(oats, model.HundredGramServing, 0.5))
	list, err := f.meals.ListMeals(f.ctx, f.userID)
	if err != nil || len(list) != 2 || list[0].ID != second.ID || list[1].ID != meal.ID {
		t.Fatalf("never-logged meals list newest first: %v %+v", err, list)
	}
	f.logMeal(t, f.userID, meal.ID, "2026-11-10", nil)
	list, err = f.meals.ListMeals(f.ctx, f.userID)
	if err != nil || list[0].ID != meal.ID || list[0].LastLoggedAt == nil || list[0].Revision != 1 || len(list[0].Items) != 2 || len(list[1].Items) != 1 {
		t.Fatalf("a logged meal lists first, keeps its revision, and each meal carries its own items: %v %+v", err, list)
	}
	if empty, err := f.meals.ListMeals(f.ctx, f.newUser(t)); err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("an athlete with no meals gets an empty, non-nil list: %v %v", err, empty)
	}

	// Update: stale revision conflicts and carries the stored meal.
	stale, current := int64(7), int64(1)
	update := model.SaveMealRequest{Name: "Big breakfast", Items: []model.MealItemInput{itemFor(whey, scoop, 2)}, ExpectedRevision: &stale}
	uk, uh := f.key("update")
	_, _, err = f.meals.UpdateMeal(f.ctx, f.userID, meal.ID, update, uk, uh)
	if !errors.As(err, &conflict) || conflict.Message != "resource revision conflict" || conflict.Expected != 7 || conflict.Actual != 1 {
		t.Fatalf("stale update conflicts: %v", err)
	}
	if stored, ok := conflict.Authoritative.(*model.SavedMeal); !ok || stored.ID != meal.ID || stored.Name != "Usual breakfast" || len(stored.Items) != 2 {
		t.Fatalf("the conflict carries the stored meal with its items: %#v", conflict.Authoritative)
	}
	update.ExpectedRevision = &current
	uk, uh = f.key("update")
	updated, outcome, err := f.meals.UpdateMeal(f.ctx, f.userID, meal.ID, update, uk, uh)
	if err != nil || outcome != model.SavedMealReplaced || updated.Revision != 2 || updated.Name != "Big breakfast" || len(updated.Items) != 1 || updated.Items[0].Amount != 2 || updated.LastLoggedAt == nil {
		t.Fatalf("update replaces name and items and advances the revision: %+v %v %v", updated, outcome, err)
	}
	if replayed, outcome, err := f.meals.UpdateMeal(f.ctx, f.userID, meal.ID, update, uk, uh); err != nil || outcome != model.SavedMealReplaced || replayed.Revision != 2 {
		t.Fatalf("a verbatim update retry replays although the revision moved on: %+v %v", replayed, err)
	}
	// Renaming to its own name in another case is allowed; to another meal's name is not.
	sameName := model.SaveMealRequest{Name: "BIG BREAKFAST", Items: update.Items, ExpectedRevision: ptrInt64(2)}
	uk, uh = f.key("update")
	if renamed, _, err := f.meals.UpdateMeal(f.ctx, f.userID, meal.ID, sameName, uk, uh); err != nil || renamed.Revision != 3 {
		t.Fatalf("a meal may keep its own name in another case: %v", err)
	}
	taken := model.SaveMealRequest{Name: "snack", Items: update.Items, ExpectedRevision: ptrInt64(3)}
	uk, uh = f.key("update")
	if _, _, err := f.meals.UpdateMeal(f.ctx, f.userID, meal.ID, taken, uk, uh); !errors.As(err, &validation) || validation.Field != "name" {
		t.Fatalf("renaming onto another meal's name is refused: %v", err)
	}

	// Delete: stale, then current, then gone.
	dk, dh := f.key("delete")
	if _, err := f.meals.DeleteMeal(f.ctx, f.userID, meal.ID, 1, dk, dh); !errors.As(err, &conflict) || conflict.Actual != 3 {
		t.Fatalf("stale delete conflicts: %v", err)
	}
	dk, dh = f.key("delete")
	if outcome, err := f.meals.DeleteMeal(f.ctx, f.userID, meal.ID, 3, dk, dh); err != nil || outcome != model.SavedMealRemoved {
		t.Fatalf("delete: %v %v", outcome, err)
	}
	if outcome, err := f.meals.DeleteMeal(f.ctx, f.userID, meal.ID, 3, dk, dh); err != nil || outcome != model.SavedMealRemoved {
		t.Fatalf("a verbatim delete retry replays: %v %v", outcome, err)
	}
	var notFound *model.NotFoundError
	dk, dh = f.key("delete")
	if _, err := f.meals.DeleteMeal(f.ctx, f.userID, meal.ID, 3, dk, dh); !errors.As(err, &notFound) {
		t.Fatalf("a deleted meal is not found: %v", err)
	}
	if meals, items := f.mealCount(t, f.userID); meals != 1 || items != 1 {
		t.Fatalf("delete cascades the items: %d meals, %d items", meals, items)
	}
	if day := f.day(t, f.userID, "2026-11-10"); len(day.Entries) != 2 {
		t.Fatalf("entries the meal logged stay on their day: %d", len(day.Entries))
	}
}

func ptrInt64(v int64) *int64 { return &v }

func TestSavedMealLimitAndNameRace(t *testing.T) {
	f := newSavedMealFixture(t)
	oats := f.catalogFood(t, "Oats", oatsPer, cup)
	items := []model.MealItemInput{itemFor(oats, cup, 1)}

	// Two concurrent creates of one name: the nutrition lock serialises them,
	// so exactly one wins and the other is refused on name.
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		k, h := f.key("race")
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, errs[i] = f.meals.CreateMeal(f.ctx, f.userID, model.SaveMealRequest{Name: "Race", Items: items}, k, h)
		}()
	}
	wg.Wait()
	var validation *model.ValidationError
	won, refused := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case errors.As(err, &validation) && validation.Field == "name":
			refused++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if won != 1 || refused != 1 {
		t.Fatalf("one create wins and one is refused on name: won=%d refused=%d", won, refused)
	}

	if _, err := f.pool.Exec(f.ctx, `
		INSERT INTO saved_meals (id, user_id, name)
		SELECT gen_random_uuid(), $1, 'Filler ' || n FROM generate_series(1, 49) n`, f.userID); err != nil {
		t.Fatal(err)
	}
	k, h := f.key("meal")
	var limit *model.SavedMealLimitError
	if _, _, err := f.meals.CreateMeal(f.ctx, f.userID, model.SaveMealRequest{Name: "One too many", Items: items}, k, h); !errors.As(err, &limit) || limit.Limit != 50 {
		t.Fatalf("the 51st meal is refused with the limit: %v", err)
	}
	if meals, _ := f.mealCount(t, f.userID); meals != 50 {
		t.Fatalf("a refused create adds no row: %d", meals)
	}
	// The limit is per athlete.
	f.createMeal(t, f.newUser(t), "Mine", items...)
}

func TestSavedMealLogWritesOrdinaryEntriesAtomically(t *testing.T) {
	f := newSavedMealFixture(t)
	oats := f.catalogFood(t, "Oats", oatsPer, cup)
	banana := f.catalogFood(t, "Banana", bananaPer, medium)
	whey := f.ownFood(t, f.userID, "Whey")
	meal := f.createMeal(t, f.userID, "Usual breakfast", itemFor(oats, cup, 1), itemFor(whey, scoop, 1), itemFor(banana, medium, 1))
	const date = "2026-11-12"

	at := time.Date(2026, 11, 12, 8, 0, 0, 0, time.UTC)
	k, h := f.key("log")
	logged, outcome, err := f.meals.LogMeal(f.ctx, f.userID, meal.ID, date, nil, at, k, h)
	if err != nil || outcome != model.SavedMealLogged {
		t.Fatalf("log: %v %v", outcome, err)
	}
	if len(logged.Entries) != 3 || len(logged.Converted) != 0 || logged.Day == nil || logged.Day.Source != model.NutritionSourceFoods {
		t.Fatalf("three entries, nothing converted, a foods day: %+v", logged)
	}
	for i, want := range []struct {
		food model.Food
		per  model.Per100g
		g    float64
	}{{oats, oatsPer, 81}, {whey, whey.Per100g, 30}, {banana, bananaPer, 118}} {
		e := logged.Entries[i]
		energy, protein, carbs, fat := model.EntryValues(want.per, want.g, 1)
		if e.Kind != model.FoodEntryKindFood || e.Food == nil || e.Food.ID != want.food.ID || e.Date != date || e.Revision != 1 ||
			*e.EnergyKcal != energy || *e.ProteinG != protein || *e.CarbsG != carbs || *e.FatG != fat || *e.Amount != 1 {
			t.Fatalf("entry %d is not the item's snapshot: %+v", i, e)
		}
	}
	day := f.day(t, f.userID, date)
	if len(day.Entries) != 3 || day.Entries[0].Food.ID != oats.ID || day.Entries[1].Food.ID != whey.ID || day.Entries[2].Food.ID != banana.ID {
		t.Fatalf("the day lists the meal's foods in item order: %+v", day.Entries)
	}
	assertDayIsSumOfEntries(t, day)
	if *day.Day.EnergyKcal != meal.Totals.EnergyKcal {
		t.Fatalf("the day rose by the meal's totals: day %d, meal %d", *day.Day.EnergyKcal, meal.Totals.EnergyKcal)
	}

	// A verbatim retry replays the same entries and logs nothing twice.
	replay, outcome, err := f.meals.LogMeal(f.ctx, f.userID, meal.ID, date, nil, at, k, h)
	if err != nil || outcome != model.SavedMealLogged || len(replay.Entries) != 3 || replay.Entries[0].ID != logged.Entries[0].ID {
		t.Fatalf("replay: %+v %v", replay, err)
	}
	if got := len(f.day(t, f.userID, date).Entries); got != 3 {
		t.Fatalf("a retry logs nothing twice: %d entries", got)
	}
	var conflict *model.ConflictError
	if _, _, err := f.meals.LogMeal(f.ctx, f.userID, meal.ID, date, nil, at, k, "another-hash"); !errors.As(err, &conflict) || !strings.Contains(conflict.Message, "idempotency") {
		t.Fatalf("same key, different payload → idempotency conflict: %v", err)
	}

	// An override list logs those items and leaves the saved meal alone.
	override := []model.MealItemInput{itemFor(oats, cup, 2)}
	overridden := f.logMeal(t, f.userID, meal.ID, date, override)
	if len(overridden.Entries) != 1 || *overridden.Entries[0].Amount != 2 {
		t.Fatalf("override logs its own items: %+v", overridden.Entries)
	}
	list, err := f.meals.ListMeals(f.ctx, f.userID)
	if err != nil || len(list[0].Items) != 3 || list[0].Items[0].Amount != 1 || list[0].Revision != 1 || list[0].LastLoggedAt == nil {
		t.Fatalf("the saved meal is unchanged by logs: %v %+v", err, list[0])
	}
	assertDayIsSumOfEntries(t, f.day(t, f.userID, date))

	// One bad item: nothing is logged, the day and last_logged_at are untouched.
	before := f.day(t, f.userID, date)
	stampBefore := *list[0].LastLoggedAt
	bad := []model.MealItemInput{itemFor(oats, cup, 1), itemFor(banana, medium, 1), {Food: model.FoodRef{ID: uuid.New()}, Serving: cup, Amount: 1}}
	fk, fh := f.key("log")
	_, _, err = f.meals.LogMeal(f.ctx, f.userID, meal.ID, date, bad, time.Now(), fk, fh)
	wantItemFault(t, "unknown food in an override", err, 2)
	wrongServing := []model.MealItemInput{itemFor(oats, medium, 1)}
	fk, fh = f.key("log")
	_, _, err = f.meals.LogMeal(f.ctx, f.userID, meal.ID, date, wrongServing, time.Now(), fk, fh)
	wantItemFault(t, "a serving the food does not have", err, 0)
	after := f.day(t, f.userID, date)
	if len(after.Entries) != len(before.Entries) || after.Day.Revision != before.Day.Revision {
		t.Fatalf("a failed log changes nothing: %d→%d entries, day revision %d→%d", len(before.Entries), len(after.Entries), before.Day.Revision, after.Day.Revision)
	}
	list, _ = f.meals.ListMeals(f.ctx, f.userID)
	if !list[0].LastLoggedAt.Equal(stampBefore) {
		t.Fatal("a failed log does not stamp the meal")
	}

	// Entries a meal logged are ordinary entries: the existing entry
	// transactions change and delete them, and recents include their foods.
	target := logged.Entries[2]
	half := 0.5
	ek, eh := f.key("entry")
	changed, _, err := f.entries.UpdateEntry(f.ctx, f.userID, target.ID, model.UpdateFoodEntryRequest{Serving: &medium, Amount: &half, ExpectedRevision: 1}, ek, eh)
	if err != nil || *changed.Entry.Amount != 0.5 || changed.Entry.Revision != 2 {
		t.Fatalf("an entry logged by a meal is editable: %+v %v", changed, err)
	}
	ek, eh = f.key("entry")
	if _, _, err := f.entries.DeleteEntry(f.ctx, f.userID, target.ID, 2, ek, eh); err != nil {
		t.Fatalf("an entry logged by a meal is deletable: %v", err)
	}
	assertDayIsSumOfEntries(t, f.day(t, f.userID, date))
	recents, err := f.foods.ListRecentFoods(f.ctx, f.userID, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), 20)
	if err != nil || len(recents) < 2 {
		t.Fatalf("foods logged through a meal appear in recents: %v %d", err, len(recents))
	}

	// Unknown meal.
	var notFound *model.NotFoundError
	fk, fh = f.key("log")
	if _, _, err := f.meals.LogMeal(f.ctx, f.userID, uuid.New(), date, nil, time.Now(), fk, fh); !errors.As(err, &notFound) {
		t.Fatalf("an unknown meal is not found: %v", err)
	}
}

func TestSavedMealLogConvertsTypedTotalsAndSerialisesConcurrentLogs(t *testing.T) {
	f := newSavedMealFixture(t)
	oats := f.catalogFood(t, "Oats", oatsPer, cup)
	banana := f.catalogFood(t, "Banana", bananaPer, medium)
	meal := f.createMeal(t, f.userID, "Oats and banana", itemFor(oats, cup, 1), itemFor(banana, medium, 1))

	// A day that held typed totals keeps them as one quick add.
	const typedDate = "2026-11-08"
	energy, protein := 600, 40.0
	if _, _, err := NewNutritionDAO(f.pool).SaveDay(f.ctx, f.userID, &model.NutritionDay{
		Date: typedDate, EnergyKcal: &energy, ProteinG: &protein, Source: model.NutritionSourceAppleHealth, LoggedAt: time.Now(),
	}, nil, false, "typed-"+uuid.NewString(), "typed-hash"); err != nil {
		t.Fatal(err)
	}
	logged := f.logMeal(t, f.userID, meal.ID, typedDate, nil)
	if len(logged.Converted) != 1 || logged.Converted[0].Kind != model.FoodEntryKindQuickAdd || *logged.Converted[0].EnergyKcal != 600 {
		t.Fatalf("typed totals become one quick add: %+v", logged.Converted)
	}
	day := f.day(t, f.userID, typedDate)
	if len(day.Entries) != 3 || *day.Day.EnergyKcal != 600+meal.Totals.EnergyKcal {
		t.Fatalf("nothing the athlete saved disappears: %d entries, %d kcal", len(day.Entries), *day.Day.EnergyKcal)
	}
	assertDayIsSumOfEntries(t, day)

	// Two devices log at once: both land and the sum is exact.
	const date = "2026-11-09"
	var wg sync.WaitGroup
	errs := make([]error, 6)
	for i := range errs {
		k, h := f.key("concurrent")
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, errs[i] = f.meals.LogMeal(f.ctx, f.userID, meal.ID, date, nil, time.Now(), k, h)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("concurrent log failed: %v", err)
		}
	}
	day = f.day(t, f.userID, date)
	if len(day.Entries) != 12 || *day.Day.EnergyKcal != 6*meal.Totals.EnergyKcal {
		t.Fatalf("six concurrent logs of two foods → 12 entries and an exact sum: %d entries, %d kcal", len(day.Entries), *day.Day.EnergyKcal)
	}
	assertDayIsSumOfEntries(t, day)
}

func TestSavedMealArchivedFoodRule(t *testing.T) {
	f := newSavedMealFixture(t)
	oats := f.catalogFood(t, "Oats", oatsPer, cup)
	whey := f.ownFood(t, f.userID, "Whey")
	other := f.ownFood(t, f.userID, "Casein")
	meal := f.createMeal(t, f.userID, "Shake", itemFor(whey, scoop, 1), itemFor(oats, cup, 1))

	for _, food := range []model.Food{whey, other} {
		k, h := f.key("archive")
		if _, err := f.foods.ArchiveUserFood(f.ctx, f.userID, food.ID, *food.Revision, k, h); err != nil {
			t.Fatal(err)
		}
	}

	// A meal that already holds the archived food still logs it.
	logged := f.logMeal(t, f.userID, meal.ID, "2026-11-11", nil)
	if len(logged.Entries) != 2 || logged.Entries[0].Food.ID != whey.ID {
		t.Fatalf("an archived food the meal holds still logs: %+v", logged.Entries)
	}
	// It may stay through an edit and in an override list.
	uk, uh := f.key("update")
	if _, _, err := f.meals.UpdateMeal(f.ctx, f.userID, meal.ID, model.SaveMealRequest{Name: "Shake", Items: []model.MealItemInput{itemFor(whey, scoop, 2)}, ExpectedRevision: ptrInt64(1)}, uk, uh); err != nil {
		t.Fatalf("an archived food the meal holds survives an edit: %v", err)
	}
	f.logMeal(t, f.userID, meal.ID, "2026-11-11", []model.MealItemInput{itemFor(whey, scoop, 1)})

	// An archived food cannot be added: not to a new meal, not by an edit,
	// not through an override list.
	k, h := f.key("meal")
	_, _, err := f.meals.CreateMeal(f.ctx, f.userID, model.SaveMealRequest{Name: "New", Items: []model.MealItemInput{itemFor(oats, cup, 1), itemFor(whey, scoop, 1)}}, k, h)
	wantItemFault(t, "archived food in a new meal", err, 1)
	uk, uh = f.key("update")
	_, _, err = f.meals.UpdateMeal(f.ctx, f.userID, meal.ID, model.SaveMealRequest{Name: "Shake", Items: []model.MealItemInput{itemFor(other, scoop, 1)}, ExpectedRevision: ptrInt64(2)}, uk, uh)
	wantItemFault(t, "archived food added by an edit", err, 0)
	lk, lh := f.key("log")
	_, _, err = f.meals.LogMeal(f.ctx, f.userID, meal.ID, "2026-11-11", []model.MealItemInput{itemFor(other, scoop, 1)}, time.Now(), lk, lh)
	wantItemFault(t, "archived food introduced by an override", err, 0)
}

func TestSavedMealOwnership(t *testing.T) {
	f := newSavedMealFixture(t)
	oats := f.catalogFood(t, "Oats", oatsPer, cup)
	owner, intruder := f.userID, f.newUser(t)
	secret := f.ownFood(t, owner, "Owner's private food")
	meal := f.createMeal(t, owner, "Owner's meal", itemFor(oats, cup, 1), itemFor(secret, scoop, 1))

	var notFound *model.NotFoundError
	k, h := f.key("x")
	if _, _, err := f.meals.UpdateMeal(f.ctx, intruder, meal.ID, model.SaveMealRequest{Name: "Taken", Items: []model.MealItemInput{itemFor(oats, cup, 1)}, ExpectedRevision: ptrInt64(1)}, k, h); !errors.As(err, &notFound) {
		t.Fatalf("another athlete's meal is not found on update: %v", err)
	}
	k, h = f.key("x")
	if _, err := f.meals.DeleteMeal(f.ctx, intruder, meal.ID, 1, k, h); !errors.As(err, &notFound) {
		t.Fatalf("another athlete's meal is not found on delete: %v", err)
	}
	k, h = f.key("x")
	if _, _, err := f.meals.LogMeal(f.ctx, intruder, meal.ID, "2026-11-10", nil, time.Now(), k, h); !errors.As(err, &notFound) {
		t.Fatalf("another athlete's meal is not found on log: %v", err)
	}
	if list, err := f.meals.ListMeals(f.ctx, intruder); err != nil || len(list) != 0 {
		t.Fatalf("another athlete sees no meals: %v %v", err, list)
	}

	// Another athlete's private food is indistinguishable from an unknown id.
	k, h = f.key("x")
	_, _, foreign := f.meals.CreateMeal(f.ctx, intruder, model.SaveMealRequest{Name: "Mine", Items: []model.MealItemInput{itemFor(secret, scoop, 1)}}, k, h)
	wantItemFault(t, "foreign private food", foreign, 0)
	k, h = f.key("x")
	_, _, unknown := f.meals.CreateMeal(f.ctx, intruder, model.SaveMealRequest{Name: "Mine", Items: []model.MealItemInput{{Food: model.FoodRef{ID: uuid.New()}, Serving: scoop, Amount: 1}}}, k, h)
	wantItemFault(t, "unknown food", unknown, 0)
	if foreign.Error() != unknown.Error() {
		t.Fatalf("foreign and unknown foods share one answer: %q vs %q", foreign, unknown)
	}
	if meals, _ := f.mealCount(t, intruder); meals != 0 {
		t.Fatal("a refused create leaves no meal")
	}
	stored, err := f.meals.ListMeals(f.ctx, owner)
	if err != nil || len(stored) != 1 || stored[0].Name != "Owner's meal" || stored[0].Revision != 1 || len(stored[0].Items) != 2 {
		t.Fatalf("the owner's meal is untouched: %v %+v", err, stored)
	}
}

func TestSavedMealChangesNeverRewriteLoggedDays(t *testing.T) {
	f := newSavedMealFixture(t)
	oats := f.catalogFood(t, "Oats", oatsPer, cup)
	whey := f.ownFood(t, f.userID, "Whey")
	meal := f.createMeal(t, f.userID, "Breakfast", itemFor(oats, cup, 1), itemFor(whey, scoop, 1))
	const date = "2026-11-07"
	f.logMeal(t, f.userID, meal.ID, date, nil)
	snapshot := func() string {
		day := f.day(t, f.userID, date)
		out := fmt.Sprintf("day %d %.1f rev %d;", *day.Day.EnergyKcal, *day.Day.ProteinG, day.Day.Revision)
		for _, e := range day.Entries {
			out += fmt.Sprintf(" %s %s %.2f %d %.1f %.1f %.1f rev %d;", e.ID, e.Food.ID, *e.Amount, *e.EnergyKcal, *e.ProteinG, *e.CarbsG, *e.FatG, e.Revision)
		}
		return out
	}
	before := snapshot()

	k, h := f.key("update")
	if _, _, err := f.meals.UpdateMeal(f.ctx, f.userID, meal.ID, model.SaveMealRequest{Name: "Changed", Items: []model.MealItemInput{itemFor(oats, cup, 3)}, ExpectedRevision: ptrInt64(1)}, k, h); err != nil {
		t.Fatal(err)
	}
	// The private food's label changes: current facts move, snapshots do not.
	k, h = f.key("food")
	if _, _, err := f.foods.UpdateUserFood(f.ctx, f.userID, whey.ID, model.SaveUserFoodRequest{
		Name: "Whey v2", Serving: scoop, EnergyKcal: 200, ProteinG: 10, CarbsG: 20, FatG: 5, ExpectedRevision: whey.Revision,
	}, nil, k, h); err != nil {
		t.Fatal(err)
	}
	// The catalog refreshes.
	refreshed := oats
	refreshed.Per100g = model.Per100g{EnergyKcal: 500, ProteinG: 20, CarbsG: 50, FatG: 20}
	refreshed.FetchedAt = time.Now()
	if _, err := f.foods.UpsertCatalogFoods(f.ctx, []model.Food{refreshed}); err != nil {
		t.Fatal(err)
	}
	list, err := f.meals.ListMeals(f.ctx, f.userID)
	wantEnergy, _, _, _ := model.EntryValues(refreshed.Per100g, 81, 3)
	if err != nil || list[0].Totals.EnergyKcal != wantEnergy {
		t.Fatalf("a saved meal shows its foods' current facts: %v %+v", err, list[0].Totals)
	}
	k, h = f.key("delete")
	if _, err := f.meals.DeleteMeal(f.ctx, f.userID, meal.ID, 2, k, h); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(); after != before {
		t.Fatalf("a logged day changed:\nbefore %s\nafter  %s", before, after)
	}
}

func TestSavedMealAccountDeletionCascadeAndExport(t *testing.T) {
	f := newSavedMealFixture(t)
	oats := f.catalogFood(t, "Oats", oatsPer, cup)
	whey := f.ownFood(t, f.userID, "Whey")
	f.createMeal(t, f.userID, "Breakfast", itemFor(oats, cup, 1), itemFor(whey, scoop, 1.5))
	f.createMeal(t, f.userID, "Snack", itemFor(whey, scoop, 1))

	export, err := NewExportDAO(f.pool).Export(f.ctx, f.userID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	meals := export.Nutrition.SavedMeals
	if len(meals) != 2 || meals[0].Name != "Breakfast" || len(meals[0].Items) != 2 || meals[0].Items[0].FoodName != "Oats" ||
		meals[0].Items[1].FoodName != "Whey" || meals[0].Items[1].Amount != 1.5 || meals[0].Items[0].ServingG != 81 ||
		meals[1].Name != "Snack" || len(meals[1].Items) != 1 {
		t.Fatalf("the export lists saved meals with their foods: %+v", meals)
	}
	empty, err := NewExportDAO(f.pool).Export(f.ctx, f.newUser(t), time.Now())
	if err != nil || empty.Nutrition.SavedMeals == nil || len(empty.Nutrition.SavedMeals) != 0 {
		t.Fatalf("an athlete with no meals exports an empty list: %v %v", err, empty.Nutrition.SavedMeals)
	}

	// A meal item references the own food with RESTRICT, so the order of
	// account deletion matters.
	if err := NewAccountDAO(f.pool).DeleteUserData(f.ctx, f.userID); err != nil {
		t.Fatalf("account deletion with a meal that holds an own food: %v", err)
	}
	if meals, items := f.mealCount(t, f.userID); meals != 0 || items != 0 {
		t.Fatalf("account deletion removes saved meals: %d meals, %d items", meals, items)
	}
	var catalog int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM food_catalog WHERE id=$1`, oats.ID).Scan(&catalog); err != nil || catalog != 1 {
		t.Fatalf("shared catalog foods stay: %d %v", catalog, err)
	}

	// Removing the identity alone also clears everything.
	cascaded := f.newUser(t)
	own := f.ownFood(t, cascaded, "Own")
	f.createMeal(t, cascaded, "Gone", itemFor(own, scoop, 1), itemFor(oats, cup, 1))
	if _, err := f.pool.Exec(f.ctx, `DELETE FROM auth.users WHERE id=$1`, cascaded); err != nil {
		t.Fatalf("deleting the identity cascades through meals and own foods: %v", err)
	}
	if meals, items := f.mealCount(t, cascaded); meals != 0 || items != 0 {
		t.Fatalf("identity removal cascades: %d meals, %d items", meals, items)
	}
}

func TestSavedMealDatabaseConstraints(t *testing.T) {
	f := newSavedMealFixture(t)
	oats := f.catalogFood(t, "Oats", oatsPer, cup)
	meal := f.createMeal(t, f.userID, "Constraints", itemFor(oats, cup, 1))
	for label, statement := range map[string]string{
		"position above 19":          `INSERT INTO saved_meal_items (id, meal_id, position, catalog_food_id, serving_label, serving_g, amount) VALUES (gen_random_uuid(), $1, 20, $2, '100 g', 100, 1)`,
		"duplicate position":         `INSERT INTO saved_meal_items (id, meal_id, position, catalog_food_id, serving_label, serving_g, amount) VALUES (gen_random_uuid(), $1, 0, $2, '100 g', 100, 1)`,
		"duplicate food and serving": `INSERT INTO saved_meal_items (id, meal_id, position, catalog_food_id, serving_label, serving_g, amount) VALUES (gen_random_uuid(), $1, 1, $2, '1 cup (81 g)', 81, 2)`,
		"no food reference":          `INSERT INTO saved_meal_items (id, meal_id, position, serving_label, serving_g, amount) VALUES (gen_random_uuid(), $1, 2, '100 g', 100, 1 + 0 * length($2::text))`,
		"amount above 50":            `INSERT INTO saved_meal_items (id, meal_id, position, catalog_food_id, serving_label, serving_g, amount) VALUES (gen_random_uuid(), $1, 3, $2, '100 g', 100, 50.01)`,
	} {
		if _, err := f.pool.Exec(f.ctx, statement, meal.ID, oats.ID); err == nil {
			t.Errorf("%s must be refused by the database", label)
		}
	}
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO saved_meals (id, user_id, name) VALUES (gen_random_uuid(), $1, 'CONSTRAINTS')`, f.userID); err == nil {
		t.Error("a name differing only by case must be refused by the unique index")
	}
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO saved_meals (id, user_id, name) VALUES (gen_random_uuid(), $1, $2)`, f.userID, strings.Repeat("a", 61)); err == nil {
		t.Error("a 61 character name must be refused")
	}
	if _, err := f.pool.Exec(f.ctx, `DELETE FROM food_catalog WHERE id=$1`, oats.ID); err == nil {
		t.Error("a catalog food a meal holds cannot be deleted")
	}
}

// A saved serving's weight is stored to one decimal, and a food's servings
// can change after a meal is saved. Neither may leave a meal that the list
// shows as loggable but the log refuses.
func TestSavedMealLogsAsSavedAfterWeightRoundingAndServingChanges(t *testing.T) {
	f := newSavedMealFixture(t)
	ounce := model.Serving{Label: "1 oz (28.35 g)", Grams: 28.35}
	nutsPer := model.Per100g{EnergyKcal: 607, ProteinG: 20, CarbsG: 21, FatG: 54}
	nuts := f.catalogFood(t, "Mixed nuts", nutsPer, ounce)
	whey := f.ownFood(t, f.userID, "Whey")
	meal := f.createMeal(t, f.userID, "Snack", itemFor(nuts, ounce, 2), itemFor(whey, scoop, 1))

	var storedGrams float64
	if err := f.pool.QueryRow(f.ctx, `
		SELECT i.serving_g FROM saved_meal_items i JOIN saved_meals m ON m.id=i.meal_id
		WHERE m.user_id=$1 AND i.position=0`, f.userID).Scan(&storedGrams); err != nil {
		t.Fatal(err)
	}
	if storedGrams != 28.4 {
		t.Fatalf("precondition: the database rounds the stored weight to one decimal, got %v", storedGrams)
	}

	// Rounded in storage, exact when read and logged.
	wantEnergy, wantProtein, _, _ := model.EntryValues(nutsPer, 28.35, 2)
	list, err := f.meals.ListMeals(f.ctx, f.userID)
	if err != nil || list[0].Items[0].Serving != ounce || *list[0].Items[0].EnergyKcal != wantEnergy || *meal.Items[0].EnergyKcal != wantEnergy {
		t.Fatalf("the list shows the food's own weight and the values the create returned: %v %+v", err, list[0].Items[0])
	}
	logged := f.logMeal(t, f.userID, meal.ID, "2026-11-05", nil)
	// The entry's own serving column is also one decimal (spec 014), so the
	// entry echoes 28.4 g; its values are computed from the exact 28.35 g.
	if len(logged.Entries) != 2 || logged.Entries[0].Serving.Label != ounce.Label || *logged.Entries[0].EnergyKcal != wantEnergy || *logged.Entries[0].ProteinG != wantProtein {
		t.Fatalf("a meal with a two-decimal source weight logs as saved with exact values: %+v (%d kcal, want %d)", *logged.Entries[0].Serving, *logged.Entries[0].EnergyKcal, wantEnergy)
	}
	if *logged.Day.EnergyKcal != list[0].Totals.EnergyKcal {
		t.Fatalf("the day rises by what the list showed: day %d, list %d", *logged.Day.EnergyKcal, list[0].Totals.EnergyKcal)
	}

	// The athlete re-weighs the scoop: same label, new weight and facts.
	k, h := f.key("food")
	if _, _, err := f.foods.UpdateUserFood(f.ctx, f.userID, whey.ID, model.SaveUserFoodRequest{
		Name: "Whey", Serving: model.Serving{Label: "1 scoop", Grams: 32}, EnergyKcal: 128, ProteinG: 25.6, CarbsG: 3.2, FatG: 1.6, ExpectedRevision: whey.Revision,
	}, nil, k, h); err != nil {
		t.Fatal(err)
	}
	list, err = f.meals.ListMeals(f.ctx, f.userID)
	if err != nil || !list[0].Items[1].Available || list[0].Items[1].Serving.Grams != 32 || *list[0].Items[1].EnergyKcal != 128 {
		t.Fatalf("the list follows the re-weighed serving: %v %+v", err, list[0].Items[1])
	}
	logged = f.logMeal(t, f.userID, meal.ID, "2026-11-06", nil)
	if len(logged.Entries) != 2 || logged.Entries[1].Serving.Grams != 32 || *logged.Entries[1].EnergyKcal != 128 || *logged.Day.EnergyKcal != list[0].Totals.EnergyKcal {
		t.Fatalf("the meal still logs as saved, with what the list showed: %+v day %d list %d", logged.Entries[1], *logged.Day.EnergyKcal, list[0].Totals.EnergyKcal)
	}

	// The label itself changes: the saved label and weight are used as they are.
	k, h = f.key("food")
	if _, _, err := f.foods.UpdateUserFood(f.ctx, f.userID, whey.ID, model.SaveUserFoodRequest{
		Name: "Whey", Serving: model.Serving{Label: "1 heaped scoop", Grams: 40}, EnergyKcal: 160, ProteinG: 32, CarbsG: 4, FatG: 2, ExpectedRevision: ptrInt64(*whey.Revision + 1),
	}, nil, k, h); err != nil {
		t.Fatal(err)
	}
	list, err = f.meals.ListMeals(f.ctx, f.userID)
	if err != nil || !list[0].Items[1].Available || list[0].Items[1].Serving != scoop {
		t.Fatalf("a label the food no longer has keeps the saved serving: %v %+v", err, list[0].Items[1])
	}
	logged = f.logMeal(t, f.userID, meal.ID, "2026-11-04", nil)
	keptEnergy, _, _, _ := model.EntryValues(model.Per100g{EnergyKcal: 400, ProteinG: 80, CarbsG: 10, FatG: 5}, 30, 1)
	if len(logged.Entries) != 2 || *logged.Entries[1].Serving != scoop || *logged.Entries[1].EnergyKcal != keptEnergy || *logged.Day.EnergyKcal != list[0].Totals.EnergyKcal {
		t.Fatalf("the meal logs the saved weight at the food's current facts: %+v", logged.Entries[1])
	}
	for _, date := range []string{"2026-11-04", "2026-11-05", "2026-11-06"} {
		assertDayIsSumOfEntries(t, f.day(t, f.userID, date))
	}

	// An override list is new input and stays strict.
	lk, lh := f.key("log")
	_, _, err = f.meals.LogMeal(f.ctx, f.userID, meal.ID, "2026-11-04", []model.MealItemInput{itemFor(whey, scoop, 1)}, time.Now(), lk, lh)
	wantItemFault(t, "an override naming a serving the food no longer has", err, 0)
}

func TestSavedMealDuplicateAfterRoundingAndReplayAfterDelete(t *testing.T) {
	f := newSavedMealFixture(t)
	oats := f.catalogFood(t, "Oats", oatsPer, cup)

	// The service refuses this first; called directly, the unique index is
	// the backstop and still answers with the item, never a server error.
	k, h := f.key("meal")
	_, _, err := f.meals.CreateMeal(f.ctx, f.userID, model.SaveMealRequest{Name: "Twice", Items: []model.MealItemInput{
		itemFor(oats, model.HundredGramServing, 1),
		itemFor(oats, model.Serving{Label: "100 g", Grams: 100.04}, 2),
	}}, k, h)
	wantItemFault(t, "two weights that store as the same serving", err, 1)
	if meals, items := f.mealCount(t, f.userID); meals != 0 || items != 0 {
		t.Fatalf("a refused create leaves nothing behind: %d meals, %d items", meals, items)
	}

	// A log retried after the meal was deleted still replays the original.
	meal := f.createMeal(t, f.userID, "Short lived", itemFor(oats, cup, 1))
	lk, lh := f.key("log")
	at := time.Date(2026, 11, 3, 8, 0, 0, 0, time.UTC)
	first, _, err := f.meals.LogMeal(f.ctx, f.userID, meal.ID, "2026-11-03", nil, at, lk, lh)
	if err != nil {
		t.Fatal(err)
	}
	dk, dh := f.key("delete")
	if _, err := f.meals.DeleteMeal(f.ctx, f.userID, meal.ID, 1, dk, dh); err != nil {
		t.Fatal(err)
	}
	replay, outcome, err := f.meals.LogMeal(f.ctx, f.userID, meal.ID, "2026-11-03", nil, at, lk, lh)
	if err != nil || outcome != model.SavedMealLogged || len(replay.Entries) != 1 || replay.Entries[0].ID != first.Entries[0].ID {
		t.Fatalf("a retry after the meal is gone replays the original log: %+v %v", replay, err)
	}
	if got := len(f.day(t, f.userID, "2026-11-03").Entries); got != 1 {
		t.Fatalf("the replay logs nothing more: %d entries", got)
	}
	var notFound *model.NotFoundError
	nk, nh := f.key("log")
	if _, _, err := f.meals.LogMeal(f.ctx, f.userID, meal.ID, "2026-11-03", nil, at, nk, nh); !errors.As(err, &notFound) {
		t.Fatalf("a new log of the deleted meal is not found: %v", err)
	}
}
