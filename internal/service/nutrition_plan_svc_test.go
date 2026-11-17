package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// nutritionProfileStore is an in-memory NutritionProfileDAO with the real
// DAO's revision rule.
type nutritionProfileStore struct {
	stored *model.NutritionProfile
	err    error
	hashes []string
}

func (s *nutritionProfileStore) Get(context.Context, uuid.UUID) (*model.NutritionProfile, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.stored == nil {
		return nil, &model.NotFoundError{Message: "nutrition plan not found"}
	}
	return s.stored, nil
}

func (s *nutritionProfileStore) Save(_ context.Context, _ uuid.UUID, profile *model.NutritionProfile, expected int64, _, hash string) (*model.NutritionProfile, model.NutritionProfileOutcome, error) {
	if s.err != nil {
		return nil, model.NutritionProfileCreated, s.err
	}
	s.hashes = append(s.hashes, hash)
	var actual int64
	if s.stored != nil {
		actual = s.stored.Revision
	}
	if expected != actual {
		var authoritative any
		if s.stored != nil {
			authoritative = s.stored
		}
		return nil, model.NutritionProfileCreated, &model.ConflictError{Message: "resource revision conflict", Expected: expected, Actual: actual, Authoritative: authoritative}
	}
	outcome := model.NutritionProfileCreated
	saved := *profile
	saved.Revision = actual + 1
	saved.CreatedAt = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	saved.UpdatedAt = saved.CreatedAt
	if s.stored != nil {
		outcome = model.NutritionProfileReplaced
	}
	s.stored = &saved
	return &saved, outcome, nil
}

func calculatorInput() model.CalculatorInput {
	return model.CalculatorInput{
		UnitSystem: "imperial", Weight: 180, Height: 70, AgeYears: 30, Sex: "male",
		StrengthDays: 4, CardioDays: 1, Goal: "maintenance",
	}
}

func TestNutritionPlanCalculateWorkedExample(t *testing.T) {
	plan, err := NewNutritionPlanService(&nutritionProfileStore{}, nil).Calculate(context.Background(), calculatorInput())
	if err != nil {
		t.Fatal(err)
	}
	if plan.BMRKcal != 1783 || plan.TDEEKcal != 2763 || plan.TargetKcal != 2763 || plan.CalorieAdjustmentKcal != 0 {
		t.Errorf("energy = %d, %d, %d, %d", plan.BMRKcal, plan.TDEEKcal, plan.TargetKcal, plan.CalorieAdjustmentKcal)
	}
	if plan.Macros.Protein.G != 153 || plan.Macros.Carbs.G != 405 || plan.Macros.Fat.G != 59 {
		t.Errorf("macros = %+v", plan.Macros)
	}
	if plan.Macros.Protein.Pct+plan.Macros.Carbs.Pct+plan.Macros.Fat.Pct != 100 {
		t.Errorf("shares = %+v", plan.Macros)
	}
	if plan.Activity != (model.PlanActivity{SuggestedLevel: "moderate", Level: "moderate", Multiplier: 1.55}) {
		t.Errorf("activity = %+v", plan.Activity)
	}
	// Defaults are echoed resolved; the override stays null.
	in := plan.Input
	if in.ActivityLevel != nil || in.CalorieAdjustmentKcal == nil || *in.CalorieAdjustmentKcal != 0 ||
		in.ProteinGPerLb == nil || *in.ProteinGPerLb != 0.85 || in.FatGPerLb == nil || *in.FatGPerLb != 0.33 {
		t.Errorf("resolved input = %+v", in)
	}
	if plan.CarbCycling != nil || len(plan.MealTemplates) != 1 || len(plan.MealTemplates[0].Meals) != 4 {
		t.Fatalf("templates = %+v", plan.MealTemplates)
	}
	body, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"notices":[]`, `"carb_cycling":null`, `"activity_level":null`, `"preferred_proteins":[]`, `"exclusions":[]`, `"meal_choices":[]`, `"source":"usda"`, `"day_type":"standard"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("body lacks %s", want)
		}
	}
	if strings.Contains(string(body), `"items":null`) {
		t.Error("items must never be null")
	}
}

func TestNutritionPlanCalculateCyclingAndOverride(t *testing.T) {
	in := calculatorInput()
	in.CarbCycling = true
	very := "very"
	in.ActivityLevel = &very
	plan, err := NewNutritionPlanService(&nutritionProfileStore{}, nil).Calculate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	c := plan.CarbCycling
	if c == nil || c.TrainingDays != 5 || c.RestDays != 2 {
		t.Fatalf("cycling = %+v", c)
	}
	if 5*c.Training.CarbsG+2*c.Rest.CarbsG != 7*plan.Macros.Carbs.G || c.Training.ProteinG != plan.Macros.Protein.G || c.Rest.FatG != plan.Macros.Fat.G {
		t.Errorf("cycling does not keep the week: %+v vs %+v", c, plan.Macros)
	}
	if got := []string{plan.MealTemplates[0].DayType, plan.MealTemplates[1].DayType}; got[0] != "training" || got[1] != "rest" {
		t.Errorf("day types = %v", got)
	}
	if plan.Activity.SuggestedLevel != "moderate" || plan.Activity.Level != "very" || plan.Activity.Multiplier != 1.725 {
		t.Errorf("activity = %+v", plan.Activity)
	}
	if plan.Input.ActivityLevel == nil || *plan.Input.ActivityLevel != "very" {
		t.Errorf("override not echoed: %+v", plan.Input.ActivityLevel)
	}
}

func TestNutritionPlanValidationNamesTheField(t *testing.T) {
	empty := ""
	bad := 1.5
	tooLow := -501
	for field, mutate := range map[string]func(*model.CalculatorInput){
		"unit_system":             func(in *model.CalculatorInput) { in.UnitSystem = "" },
		"weight":                  func(in *model.CalculatorInput) { in.Weight = 0 },
		"height":                  func(in *model.CalculatorInput) { in.Height = 500 },
		"age_years":               func(in *model.CalculatorInput) { in.AgeYears = 12 },
		"sex":                     func(in *model.CalculatorInput) { in.Sex = "other" },
		"strength_days":           func(in *model.CalculatorInput) { in.StrengthDays = 9 },
		"cardio_days":             func(in *model.CalculatorInput) { in.CardioDays = -2 },
		"activity_level":          func(in *model.CalculatorInput) { in.ActivityLevel = &empty },
		"goal":                    func(in *model.CalculatorInput) { in.Goal = "" },
		"calorie_adjustment_kcal": func(in *model.CalculatorInput) { in.Goal = "fat_loss"; in.CalorieAdjustmentKcal = &tooLow },
		"protein_g_per_lb":        func(in *model.CalculatorInput) { in.ProteinGPerLb = &bad },
		"fat_g_per_lb":            func(in *model.CalculatorInput) { in.FatGPerLb = &bad },
		"preferred_proteins":      func(in *model.CalculatorInput) { in.PreferredProteins = []string{"tofu"} },
		"preferred_carbs":         func(in *model.CalculatorInput) { in.PreferredCarbs = []string{"eggs"} },
		"exclusions":              func(in *model.CalculatorInput) { in.Exclusions = []string{"soy"} },
		"meal_choices":            func(in *model.CalculatorInput) { in.MealChoices = []model.MealChoice{{Slot: 9, Carb: "oats"}} },
	} {
		t.Run(field, func(t *testing.T) {
			in := calculatorInput()
			mutate(&in)
			_, err := NewNutritionPlanService(&nutritionProfileStore{}, nil).Calculate(context.Background(), in)
			var validation *model.ValidationError
			if !errors.As(err, &validation) || validation.Field != field {
				t.Fatalf("error = %v, want validation on %s", err, field)
			}
		})
	}
}

func TestNutritionPlanOptions(t *testing.T) {
	options := NewNutritionPlanService(&nutritionProfileStore{}, nil).Options(context.Background())
	if len(options.Goals) != 4 || len(options.ActivityLevels) != 3 || len(options.Exclusions) != 6 || len(options.Foods) != 18 || len(options.Meals) != 4 {
		t.Fatalf("options sizes: %d goals, %d levels, %d exclusions, %d foods, %d meals",
			len(options.Goals), len(options.ActivityLevels), len(options.Exclusions), len(options.Foods), len(options.Meals))
	}
	if g := options.Goals[0]; g.ID != "fat_loss" || g.MinAdjustmentKcal != -500 || g.MaxAdjustmentKcal != -300 || g.DefaultAdjustmentKcal != -400 || g.DefaultProteinGPerLb != 0.95 {
		t.Errorf("first goal = %+v", g)
	}
	if last := options.ActivityLevels[2]; last.MaxSessions != nil || last.Multiplier != 1.725 {
		t.Errorf("last level = %+v", last)
	}
	if first := options.ActivityLevels[0]; first.MaxSessions == nil || *first.MaxSessions != 3 {
		t.Errorf("first level = %+v", first)
	}
	if options.FatGPerLb.Default == nil || *options.FatGPerLb.Default != 0.33 || options.ProteinGPerLb.Min != 0.7 || options.ProteinGPerLb.Max != 1.1 {
		t.Errorf("rates = %+v %+v", options.ProteinGPerLb, options.FatGPerLb)
	}
	if l := options.Limits; l.Imperial.Weight != (model.CalculatorRange{Min: 66, Max: 660}) || l.Imperial.Height != (model.CalculatorRange{Min: 48, Max: 96}) ||
		l.Metric.Weight != (model.CalculatorRange{Min: 30, Max: 300}) || l.Metric.Height != (model.CalculatorRange{Min: 122, Max: 244}) || l.AgeYears != (model.CalculatorRange{Min: 18, Max: 100}) {
		t.Errorf("limits = %+v", l)
	}
	selectable := 0
	for _, f := range options.Foods {
		if f.Source != "usda" || f.FDCID == 0 || f.Tags == nil || f.Per100G.EnergyKcal == 0 {
			t.Errorf("food %+v lacks a source, tags array, or facts", f)
		}
		if f.Selectable {
			selectable++
		}
	}
	if selectable != 17 {
		t.Errorf("selectable foods = %d, want 17 (egg whites are a companion)", selectable)
	}
	body, _ := json.Marshal(options)
	if !strings.Contains(string(body), `"max_sessions":null`) {
		t.Error("last level must carry max_sessions null")
	}
}

func saveRequest(revision int64, key string) model.SaveNutritionProfileRequest {
	return model.SaveNutritionProfileRequest{CalculatorInput: calculatorInput(), ExpectedRevision: revision, OperationKey: key}
}

func TestNutritionPlanSaveAndRead(t *testing.T) {
	store := &nutritionProfileStore{}
	svc := NewNutritionPlanService(store, nil)
	ctx, user := context.Background(), uuid.New()

	if _, err := svc.GetPlan(ctx, user); !isNotFound(err) {
		t.Fatalf("GetPlan before save = %v, want not found", err)
	}

	created, outcome, err := svc.SaveProfile(ctx, user, saveRequest(0, " nutrition-profile:1 "))
	if err != nil || outcome != model.NutritionProfileCreated || created.Revision != 1 {
		t.Fatalf("create = %+v, %v, %v", created, outcome, err)
	}
	if store.stored.BMRKcal != 1783 || store.stored.TargetKcal != 2763 || store.stored.ProteinG != 153 || store.stored.CarbsG != 405 || store.stored.FatG != 59 {
		t.Errorf("stored targets = %+v", store.stored)
	}
	if in := store.stored.Input; in.ProteinGPerLb == nil || in.FatGPerLb == nil || in.CalorieAdjustmentKcal == nil || in.Exclusions == nil || in.MealChoices == nil {
		t.Errorf("stored input must have defaults resolved and non-nil lists: %+v", in)
	}

	read, err := svc.GetPlan(ctx, user)
	if err != nil || read.TargetKcal != created.TargetKcal || read.Macros != created.Macros || read.Revision != 1 {
		t.Fatalf("read = %+v, %v", read, err)
	}

	replaceReq := saveRequest(1, "nutrition-profile:2")
	replaceReq.Goal = "fat_loss"
	replaced, outcome, err := svc.SaveProfile(ctx, user, replaceReq)
	if err != nil || outcome != model.NutritionProfileReplaced || replaced.Revision != 2 || replaced.TargetKcal != 2363 {
		t.Fatalf("replace = %+v, %v, %v", replaced, outcome, err)
	}

	_, _, err = svc.SaveProfile(ctx, user, saveRequest(1, "nutrition-profile:3"))
	var conflict *model.ConflictError
	if !errors.As(err, &conflict) || conflict.Actual != 2 {
		t.Fatalf("stale save = %v, want revision conflict at 2", err)
	}
	if plan, ok := conflict.Authoritative.(*model.SavedNutritionPlan); !ok || plan.Revision != 2 || plan.TargetKcal != 2363 {
		t.Errorf("conflict resource = %#v, want the stored plan", conflict.Authoritative)
	}
}

func TestNutritionPlanSavedTargetsDoNotMove(t *testing.T) {
	// A stored row whose targets differ from what today's maths would give
	// (as after a formula change) is served as stored.
	store := &nutritionProfileStore{}
	svc := NewNutritionPlanService(store, nil)
	if _, _, err := svc.SaveProfile(context.Background(), uuid.New(), saveRequest(0, "k")); err != nil {
		t.Fatal(err)
	}
	store.stored.TargetKcal, store.stored.ProteinG, store.stored.CarbsG, store.stored.FatG = 2500, 160, 300, 70
	plan, err := svc.GetPlan(context.Background(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if plan.TargetKcal != 2500 || plan.Macros.Protein.G != 160 || plan.Macros.Carbs.G != 300 || plan.Macros.Fat.G != 70 {
		t.Errorf("plan = %d kcal %+v, want the stored targets", plan.TargetKcal, plan.Macros)
	}
	if day := plan.MealTemplates[0]; day.ProteinG != 160 || day.CarbsG != 300 || day.FatG != 70 {
		t.Errorf("template day = %+v, want it built from the stored targets", day.PlanDayMacros)
	}
}

func TestNutritionPlanReadSurvivesRemovedFoods(t *testing.T) {
	store := &nutritionProfileStore{}
	svc := NewNutritionPlanService(store, nil)
	if _, _, err := svc.SaveProfile(context.Background(), uuid.New(), saveRequest(0, "k")); err != nil {
		t.Fatal(err)
	}
	store.stored.Input.PreferredProteins = []string{"retired_food", "chicken_breast"}
	store.stored.Input.Exclusions = []string{"retired_tag", "dairy"}
	store.stored.Input.MealChoices = []model.MealChoice{{Slot: 2, Carb: "retired_food"}}
	plan, err := svc.GetPlan(context.Background(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Input.PreferredProteins; len(got) != 1 || got[0] != "chicken_breast" {
		t.Errorf("preferred proteins = %v", got)
	}
	if got := plan.Input.Exclusions; len(got) != 1 || got[0] != "dairy" {
		t.Errorf("exclusions = %v", got)
	}
	if len(plan.Input.MealChoices) != 0 || len(plan.MealTemplates[0].Meals) != 4 {
		t.Errorf("stale swap must be dropped: %+v", plan.Input.MealChoices)
	}
}

func TestNutritionPlanSaveValidation(t *testing.T) {
	svc := NewNutritionPlanService(&nutritionProfileStore{}, nil)
	ctx, user := context.Background(), uuid.New()
	for field, req := range map[string]model.SaveNutritionProfileRequest{
		"operation_key":     saveRequest(0, "  "),
		"expected_revision": saveRequest(-1, "k"),
		"weight":            {CalculatorInput: model.CalculatorInput{UnitSystem: "metric"}, OperationKey: "k"},
	} {
		_, _, err := svc.SaveProfile(ctx, user, req)
		var validation *model.ValidationError
		if !errors.As(err, &validation) || validation.Field != field {
			t.Errorf("%s: error = %v", field, err)
		}
	}
	long := saveRequest(0, strings.Repeat("k", 129))
	if _, _, err := svc.SaveProfile(ctx, user, long); err == nil {
		t.Error("a 129-character operation key must be rejected")
	}
}

func TestNutritionPlanSaveHashIsStableAndPayloadSensitive(t *testing.T) {
	store := &nutritionProfileStore{}
	svc := NewNutritionPlanService(store, nil)
	ctx, user := context.Background(), uuid.New()
	if _, _, err := svc.SaveProfile(ctx, user, saveRequest(0, "k")); err != nil {
		t.Fatal(err)
	}
	store.stored = nil
	if _, _, err := svc.SaveProfile(ctx, user, saveRequest(0, "k")); err != nil {
		t.Fatal(err)
	}
	store.stored = nil
	changed := saveRequest(0, "k")
	changed.Weight = 181
	if _, _, err := svc.SaveProfile(ctx, user, changed); err != nil {
		t.Fatal(err)
	}
	if store.hashes[0] != store.hashes[1] || store.hashes[0] == store.hashes[2] {
		t.Errorf("hashes = %v, want equal for a verbatim retry and different for a changed body", store.hashes)
	}
}

func TestNutritionPlanStoreErrorsPassThrough(t *testing.T) {
	boom := errors.New("database unavailable")
	svc := NewNutritionPlanService(&nutritionProfileStore{err: boom}, nil)
	if _, err := svc.GetPlan(context.Background(), uuid.New()); !errors.Is(err, boom) {
		t.Errorf("GetPlan error = %v", err)
	}
	if _, _, err := svc.SaveProfile(context.Background(), uuid.New(), saveRequest(0, "k")); !errors.Is(err, boom) {
		t.Errorf("SaveProfile error = %v", err)
	}
}

// templateCatalog is an in-memory food catalog keyed by source id.
type templateCatalog struct {
	calls int
	err   error
	rows  map[string]model.Food
}

func (c *templateCatalog) UpsertCatalogFoods(_ context.Context, foods []model.Food) ([]model.Food, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	if c.rows == nil {
		c.rows = map[string]model.Food{}
	}
	out := make([]model.Food, 0, len(foods))
	for _, f := range foods {
		if existing, ok := c.rows[f.SourceID]; ok {
			f.ID = existing.ID
		} else {
			f.ID = uuid.New()
		}
		// The real table keeps per-100 g facts to one decimal.
		round1 := func(v float64) float64 { return math.Round(v*10) / 10 }
		f.Per100g = model.Per100g{EnergyKcal: round1(f.Per100g.EnergyKcal), ProteinG: round1(f.Per100g.ProteinG), CarbsG: round1(f.Per100g.CarbsG), FatG: round1(f.Per100g.FatG)}
		c.rows[f.SourceID] = f
		out = append(out, f)
	}
	return out, nil
}

func TestNutritionPlanItemsCarryCatalogIDsAndALoggableServing(t *testing.T) {
	catalog := &templateCatalog{}
	svc := NewNutritionPlanService(&nutritionProfileStore{}, catalog)
	plan, err := svc.Calculate(context.Background(), calculatorInput())
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.rows) != 18 {
		t.Fatalf("catalog rows = %d, want the 18 template foods", len(catalog.rows))
	}
	chicken := catalog.rows["171534"]
	if chicken.Source != model.FoodSourceUSDA || chicken.DataType != "sr_legacy" ||
		chicken.Name != "Chicken, broiler or fryers, breast, skinless, boneless, meat only, cooked, grilled" ||
		chicken.Per100g != (model.Per100g{EnergyKcal: 151, ProteinG: 30.5, CarbsG: 0, FatG: 3.2}) {
		t.Errorf("chicken catalog row = %+v", chicken)
	}
	if got := chicken.Servings; len(got) != 2 || got[0] != (model.Serving{Label: "1 oz", Grams: 28.35}) || got[1] != model.HundredGramServing {
		t.Errorf("chicken servings = %+v", got)
	}
	if got := catalog.rows["171413"].Servings; len(got) != 1 || got[0] != model.HundredGramServing {
		t.Errorf("olive oil servings = %+v, want only 100 g", got)
	}
	for _, meal := range plan.MealTemplates[0].Meals {
		for _, item := range meal.Items {
			row, ok := catalog.rows[strconv.Itoa(item.FDCID)]
			if !ok || item.CatalogFoodID == nil || *item.CatalogFoodID != row.ID {
				t.Fatalf("%s: catalog id %v does not name its catalog row", item.Food, item.CatalogFoodID)
			}
			// The serving must be one the entry routes accept for any food, and
			// logging it once must give the item's own values.
			serving, err := model.ResolveServing(&row, item.Serving)
			if err != nil || serving.Grams != item.Grams {
				t.Fatalf("%s: serving %+v not loggable: %v", item.Food, item.Serving, err)
			}
			energy, protein, carbs, fat := model.EntryValues(row.Per100g, serving.Grams, 1)
			if energy != item.EnergyKcal || protein != item.ProteinG || carbs != item.CarbsG || fat != item.FatG {
				t.Errorf("%s: logging gives %d kcal %.1f/%.1f/%.1f, the template says %d kcal %.1f/%.1f/%.1f",
					item.Food, energy, protein, carbs, fat, item.EnergyKcal, item.ProteinG, item.CarbsG, item.FatG)
			}
		}
	}

	// The catalog is written once per process, and the ids are stable.
	options := svc.Options(context.Background())
	if _, err := svc.Calculate(context.Background(), calculatorInput()); err != nil {
		t.Fatal(err)
	}
	if catalog.calls != 1 {
		t.Errorf("catalog writes = %d, want 1", catalog.calls)
	}
	for _, f := range options.Foods {
		if f.CatalogFoodID == nil || *f.CatalogFoodID != catalog.rows[strconv.Itoa(f.FDCID)].ID {
			t.Errorf("option %s has catalog id %v", f.ID, f.CatalogFoodID)
		}
	}
}

func TestNutritionPlanIsServedWithoutCatalogIDsWhenTheCatalogFails(t *testing.T) {
	catalog := &templateCatalog{err: errors.New("database unavailable")}
	svc := NewNutritionPlanService(&nutritionProfileStore{}, catalog)
	plan, err := svc.Calculate(context.Background(), calculatorInput())
	if err != nil || plan.TargetKcal != 2763 {
		t.Fatalf("plan = %+v, err = %v; a catalog failure must not fail the calculation", plan, err)
	}
	item := plan.MealTemplates[0].Meals[0].Items[0]
	if item.CatalogFoodID != nil || item.Serving.Grams != item.Grams {
		t.Errorf("item = %+v, want no catalog id and the serving still filled", item)
	}
	body, _ := json.Marshal(plan)
	if !strings.Contains(string(body), `"catalog_food_id":null`) {
		t.Error("catalog_food_id must be present and null")
	}
	// The next request tries again and succeeds.
	catalog.err = nil
	plan, err = svc.Calculate(context.Background(), calculatorInput())
	if err != nil || plan.MealTemplates[0].Meals[0].Items[0].CatalogFoodID == nil {
		t.Fatalf("retry: item = %+v, err = %v", plan.MealTemplates[0].Meals[0].Items[0], err)
	}
	if catalog.calls != 2 {
		t.Errorf("catalog writes = %d, want 2", catalog.calls)
	}
}

func TestNutritionPlanSavedPlanCarriesCatalogIDs(t *testing.T) {
	svc := NewNutritionPlanService(&nutritionProfileStore{}, &templateCatalog{})
	ctx, user := context.Background(), uuid.New()
	saved, _, err := svc.SaveProfile(ctx, user, saveRequest(0, "k"))
	if err != nil || saved.MealTemplates[0].Meals[1].Items[0].CatalogFoodID == nil {
		t.Fatalf("saved = %+v, err = %v", saved, err)
	}
	read, err := svc.GetPlan(ctx, user)
	if err != nil || *read.MealTemplates[0].Meals[1].Items[0].CatalogFoodID != *saved.MealTemplates[0].Meals[1].Items[0].CatalogFoodID {
		t.Fatalf("read = %+v, err = %v", read, err)
	}
}

// TestNutritionPlanLoggedPortionsMatchTheTemplateAcrossInputs sweeps weights,
// goals, cycling, and exclusions: for every item of every meal, logging its
// catalog food with its serving once gives the values the template shows.
func TestNutritionPlanLoggedPortionsMatchTheTemplateAcrossInputs(t *testing.T) {
	catalog := &templateCatalog{}
	svc := NewNutritionPlanService(&nutritionProfileStore{}, catalog)
	checked := 0
	for _, weight := range []float64{100, 137.5, 180, 242, 300} {
		for _, goal := range []string{"fat_loss", "recomp", "maintenance", "muscle_gain"} {
			for _, exclusions := range [][]string{nil, {"dairy", "nuts"}, {"eggs", "gluten", "red_meat", "rice"}} {
				in := calculatorInput()
				in.Weight, in.Goal, in.Exclusions, in.CarbCycling = weight, goal, exclusions, true
				plan, err := svc.Calculate(context.Background(), in)
				if err != nil {
					t.Fatal(err)
				}
				for _, template := range plan.MealTemplates {
					for _, meal := range template.Meals {
						for _, item := range meal.Items {
							row := catalog.rows[strconv.Itoa(item.FDCID)]
							serving, err := model.ResolveServing(&row, item.Serving)
							if err != nil {
								t.Fatalf("%v lb %s: %s serving %+v: %v", weight, goal, item.Food, item.Serving, err)
							}
							energy, protein, carbs, fat := model.EntryValues(row.Per100g, serving.Grams, 1)
							if energy != item.EnergyKcal || protein != item.ProteinG || carbs != item.CarbsG || fat != item.FatG {
								t.Fatalf("%v lb %s: %s logs as %d kcal %.1f/%.1f/%.1f, template says %d kcal %.1f/%.1f/%.1f",
									weight, goal, item.Food, energy, protein, carbs, fat, item.EnergyKcal, item.ProteinG, item.CarbsG, item.FatG)
							}
							checked++
						}
					}
				}
			}
		}
	}
	if checked < 500 {
		t.Fatalf("checked only %d items", checked)
	}
}
