package nutritionplan

import (
	"math"
	"slices"
	"testing"
)

func TestCatalogIsComplete(t *testing.T) {
	seen := map[string]bool{}
	fdc := map[int]bool{}
	for _, f := range Foods {
		if f.ID == "" || f.Name == "" || seen[f.ID] {
			t.Errorf("food %q: missing or duplicate id or name", f.ID)
		}
		seen[f.ID] = true
		if f.USDAName == "" {
			t.Errorf("%s: missing USDA description", f.ID)
		}
		if f.FDCID == 0 || fdc[f.FDCID] {
			t.Errorf("%s: missing or duplicate FDC id", f.ID)
		}
		fdc[f.FDCID] = true
		if !slices.Contains([]string{RoleProtein, RoleCarb, RoleFat}, f.Role) {
			t.Errorf("%s: role %q", f.ID, f.Role)
		}
		if !slices.Contains([]string{UnitOz, UnitGram, UnitCup, UnitCount}, f.Unit) || f.GramsPerUnit <= 0 || f.Step <= 0 {
			t.Errorf("%s: unit %q, grams per unit %v, step %v", f.ID, f.Unit, f.GramsPerUnit, f.Step)
		}
		for _, tag := range f.Tags {
			if !slices.Contains(ExclusionTags, tag) {
				t.Errorf("%s: unknown tag %q", f.ID, tag)
			}
		}
		// USDA energy uses food-specific factors, so it only approximates
		// 4/4/9; a large gap means a miscopied value.
		atwater := 4*f.Protein + 4*f.Carbs + 9*f.Fat
		if gap := math.Abs(atwater-f.Kcal) / f.Kcal; gap > 0.12 {
			t.Errorf("%s: %v kcal vs %.0f from 4/4/9 (%.0f%% apart)", f.ID, f.Kcal, atwater, gap*100)
		}
		if f.LeanPartner != "" {
			partner, ok := FoodByID(f.LeanPartner)
			if !ok || partner.Fat >= f.Fat || !slices.Equal(partner.Tags, f.Tags) {
				t.Errorf("%s: lean partner %q must exist, be leaner, and share tags", f.ID, f.LeanPartner)
			}
		}
	}
	for role, slots := range slotOrder {
		for slot, ids := range slots {
			for _, id := range ids {
				if f, ok := FoodByID(id); !ok || f.Role != role || !f.Selectable {
					t.Errorf("slot order %s/%d names %q", role, slot, id)
				}
			}
		}
	}
}

func TestEveryRoleSurvivesAllExclusions(t *testing.T) {
	for _, role := range []string{RoleProtein, RoleCarb, RoleFat} {
		for slot := 1; slot <= 4; slot++ {
			if f := pick(role, slot, nil, ExclusionTags, nil); f.ID == "" {
				t.Errorf("no %s food for meal %d with every exclusion on", role, slot)
			}
		}
	}
}

func TestMealTargetsSumToTheDay(t *testing.T) {
	for protein := 60; protein <= 320; protein += 7 {
		for carbs := 0; carbs <= 600; carbs += 13 {
			for fat := 30; fat <= 200; fat += 11 {
				day := newDayMacros(protein, carbs, fat)
				var p, c, f, kcal int
				for _, m := range mealTargets(day) {
					p, c, f, kcal = p+m.ProteinG, c+m.CarbsG, f+m.FatG, kcal+m.EnergyKcal
				}
				if p != protein || c != carbs || f != fat || kcal != day.EnergyKcal {
					t.Fatalf("day %+v: meals sum to P%d C%d F%d %dkcal", day, p, c, f, kcal)
				}
			}
		}
	}
	meals := mealTargets(newDayMacros(200, 400, 100))
	want := []DayMacros{newDayMacros(50, 100, 30), newDayMacros(50, 80, 30), newDayMacros(50, 140, 10), newDayMacros(50, 80, 30)}
	if !slices.Equal(meals, want) {
		t.Errorf("mealTargets = %+v, want %+v", meals, want)
	}
}

func buildFor(t *testing.T, mutate func(*Input)) Plan {
	t.Helper()
	in := imperialMale()
	if mutate != nil {
		mutate(&in)
	}
	in = mustResolve(t, in)
	return Build(in, Calculate(in))
}

func itemIDs(m Meal) []string {
	ids := []string{}
	for _, item := range m.Items {
		ids = append(ids, item.Food.ID)
	}
	return ids
}

func TestTemplateUsesPreferredFoods(t *testing.T) {
	plan := buildFor(t, func(in *Input) {
		in.PreferredProteins = []string{"chicken_breast", "greek_yogurt"}
		in.PreferredCarbs = []string{"oats", "white_rice", "sweet_potatoes"}
	})
	if len(plan.Templates) != 1 || plan.Templates[0].DayType != DayStandard {
		t.Fatalf("templates = %+v, want one standard day", plan.Templates)
	}
	meals := plan.Templates[0].Meals
	if len(meals) != 4 {
		t.Fatalf("meals = %d, want 4", len(meals))
	}
	for i, want := range []struct{ name, protein, carb string }{
		{"Breakfast", "greek_yogurt", "oats"},
		{"Lunch", "chicken_breast", "white_rice"},
		{"Pre/Post-Workout Fuel", "greek_yogurt", "white_rice"},
		{"Dinner", "chicken_breast", "sweet_potatoes"},
	} {
		ids := itemIDs(meals[i])
		if meals[i].Slot != i+1 || meals[i].Name != want.name || !slices.Contains(ids, want.protein) || !slices.Contains(ids, want.carb) {
			t.Errorf("meal %d = %s %v, want %s with %s and %s", i+1, meals[i].Name, ids, want.name, want.protein, want.carb)
		}
	}
}

func TestMealThreeLeadsWithQuickCarbohydrate(t *testing.T) {
	plan := buildFor(t, nil)
	if ids := itemIDs(plan.Templates[0].Meals[2]); !slices.Contains(ids, "banana") || !slices.Contains(ids, "whey") {
		t.Errorf("meal 3 = %v, want banana and whey by default", ids)
	}
}

func TestExclusionsAreNeverServed(t *testing.T) {
	// Every subset of the six tags, with and without preferences that name
	// excluded foods, on both day types.
	all := []string{}
	for _, f := range Foods {
		if f.Selectable && f.Role != RoleFat {
			all = append(all, f.ID)
		}
	}
	for mask := range 1 << len(ExclusionTags) {
		excluded := []string{}
		for i, tag := range ExclusionTags {
			if mask&(1<<i) != 0 {
				excluded = append(excluded, tag)
			}
		}
		for _, preferAll := range []bool{false, true} {
			plan := buildFor(t, func(in *Input) {
				in.Exclusions = excluded
				in.CarbCycling = true
				if preferAll {
					for _, id := range all {
						f, _ := FoodByID(id)
						if f.Role == RoleProtein {
							in.PreferredProteins = append(in.PreferredProteins, id)
						} else {
							in.PreferredCarbs = append(in.PreferredCarbs, id)
						}
					}
				}
			})
			if len(plan.Templates) != 2 {
				t.Fatalf("excluded %v: %d templates, want training and rest", excluded, len(plan.Templates))
			}
			for _, tmpl := range plan.Templates {
				for _, meal := range tmpl.Meals {
					if len(meal.Items) == 0 {
						t.Errorf("excluded %v: %s meal %d is empty", excluded, tmpl.DayType, meal.Slot)
					}
					for _, item := range meal.Items {
						if !item.Food.allowed(excluded) {
							t.Errorf("excluded %v: %s meal %d serves %s", excluded, tmpl.DayType, meal.Slot, item.Food.ID)
						}
					}
				}
			}
		}
	}
}

func TestEggsBringEggWhites(t *testing.T) {
	plan := buildFor(t, func(in *Input) { in.PreferredProteins = []string{"eggs"} })
	breakfast := plan.Templates[0].Meals[0]
	ids := itemIDs(breakfast)
	if !slices.Contains(ids, "eggs") || !slices.Contains(ids, "egg_whites") {
		t.Fatalf("breakfast = %v, want eggs with egg whites", ids)
	}
	if over := breakfast.Provided.FatG - float64(breakfast.Target.FatG); over > 4 {
		t.Errorf("breakfast fat %.1f g is %.1f g over its %d g target", breakfast.Provided.FatG, over, breakfast.Target.FatG)
	}
}

func TestPortionsUseUnitStepsAndReportWhatTheyProvide(t *testing.T) {
	for _, weight := range []float64{110, 150, 180, 220, 280} {
		for _, goal := range Goals {
			plan := buildFor(t, func(in *Input) { in.Weight, in.Goal = weight, goal.ID })
			for _, meal := range plan.Templates[0].Meals {
				var sum Provided
				for _, item := range meal.Items {
					steps := item.Amount / item.Food.Step
					if item.Amount <= 0 || math.Abs(steps-math.Round(steps)) > 1e-6 {
						t.Errorf("%s: amount %v is not a positive multiple of %v %s", item.Food.ID, item.Amount, item.Food.Step, item.Food.Unit)
					}
					if want := round(item.Amount*item.Food.GramsPerUnit, 1); item.Grams != want {
						t.Errorf("%s: grams %v, want %v", item.Food.ID, item.Grams, want)
					}
					sum = sum.plus(Provided{EnergyKcal: item.EnergyKcal, ProteinG: item.ProteinG, CarbsG: item.CarbsG, FatG: item.FatG})
				}
				if sum != meal.Provided {
					t.Errorf("meal %d provided %+v, items sum to %+v", meal.Slot, meal.Provided, sum)
				}
				// Rounding to kitchen units moves a meal by at most a few grams.
				if d := math.Abs(meal.Provided.ProteinG - float64(meal.Target.ProteinG)); d > 8 {
					t.Errorf("%v lb %s meal %d: protein %.1f vs target %d", weight, goal.ID, meal.Slot, meal.Provided.ProteinG, meal.Target.ProteinG)
				}
				if d := math.Abs(meal.Provided.CarbsG - float64(meal.Target.CarbsG)); d > 15 {
					t.Errorf("%v lb %s meal %d: carbs %.1f vs target %d", weight, goal.ID, meal.Slot, meal.Provided.CarbsG, meal.Target.CarbsG)
				}
				if d := math.Abs(meal.Provided.FatG - float64(meal.Target.FatG)); d > 8 {
					t.Errorf("%v lb %s meal %d: fat %.1f vs target %d (%v)", weight, goal.ID, meal.Slot, meal.Provided.FatG, meal.Target.FatG, itemIDs(meal))
				}
			}
		}
	}
}

func TestSwapChangesOnlyItsMeal(t *testing.T) {
	prefs := func(in *Input) { in.PreferredCarbs = []string{"potatoes", "oats", "banana"} }
	before := buildFor(t, prefs).Templates[0]
	after := buildFor(t, func(in *Input) {
		prefs(in)
		in.MealChoices = []MealChoice{{Slot: 2, Carb: "flour_tortillas"}}
	}).Templates[0]

	for i := range before.Meals {
		if i == 1 {
			continue
		}
		if !slices.Equal(itemIDs(before.Meals[i]), itemIDs(after.Meals[i])) || before.Meals[i].Provided != after.Meals[i].Provided {
			t.Errorf("meal %d changed: %v → %v", i+1, itemIDs(before.Meals[i]), itemIDs(after.Meals[i]))
		}
	}
	lunchBefore, lunch := before.Meals[1], after.Meals[1]
	if lunch.Target != lunchBefore.Target {
		t.Errorf("lunch target changed: %+v → %+v", lunchBefore.Target, lunch.Target)
	}
	if ids := itemIDs(lunch); !slices.Contains(ids, "flour_tortillas") || slices.Contains(ids, "potatoes") {
		t.Errorf("lunch = %v, want tortillas instead of potatoes", ids)
	}
	if d := math.Abs(lunch.Provided.CarbsG - lunchBefore.Provided.CarbsG); d > 15 {
		t.Errorf("lunch carbs %.1f → %.1f", lunchBefore.Provided.CarbsG, lunch.Provided.CarbsG)
	}
	if d := math.Abs(lunch.Provided.ProteinG - lunchBefore.Provided.ProteinG); d > 8 {
		t.Errorf("lunch protein %.1f → %.1f", lunchBefore.Provided.ProteinG, lunch.Provided.ProteinG)
	}
}

func TestSwapValidation(t *testing.T) {
	for name, mutate := range map[string]func(*Input){
		"unknown food":   func(in *Input) { in.MealChoices = []MealChoice{{Slot: 1, Protein: "tofu"}} },
		"wrong role":     func(in *Input) { in.MealChoices = []MealChoice{{Slot: 1, Protein: "oats"}} },
		"companion food": func(in *Input) { in.MealChoices = []MealChoice{{Slot: 1, Protein: "egg_whites"}} },
		"excluded food": func(in *Input) {
			in.Exclusions = []string{TagRice}
			in.MealChoices = []MealChoice{{Slot: 2, Carb: "white_rice"}}
		},
		"slot out of range": func(in *Input) { in.MealChoices = []MealChoice{{Slot: 5, Carb: "oats"}} },
		"duplicate slot":    func(in *Input) { in.MealChoices = []MealChoice{{Slot: 2, Carb: "oats"}, {Slot: 2, Fat: "avocado"}} },
	} {
		t.Run(name, func(t *testing.T) {
			in := imperialMale()
			mutate(&in)
			assertField(t, in, "meal_choices")
		})
	}
	for field, mutate := range map[string]func(*Input){
		"exclusions":         func(in *Input) { in.Exclusions = []string{"soy"} },
		"preferred_proteins": func(in *Input) { in.PreferredProteins = []string{"oats"} },
		"preferred_carbs":    func(in *Input) { in.PreferredCarbs = []string{"egg_whites"} },
	} {
		in := imperialMale()
		mutate(&in)
		assertField(t, in, field)
	}
}

func TestResolveNormalizesLists(t *testing.T) {
	in := imperialMale()
	in.Exclusions = []string{TagDairy, TagDairy}
	in.PreferredProteins = []string{"whey", "chicken_breast", "chicken_breast"}
	in.MealChoices = []MealChoice{{Slot: 4, Fat: "avocado"}, {Slot: 1}}
	got := mustResolve(t, in)
	if !slices.Equal(got.Exclusions, []string{TagDairy}) {
		t.Errorf("exclusions = %v", got.Exclusions)
	}
	if !slices.Equal(got.PreferredProteins, []string{"chicken_breast"}) {
		t.Errorf("preferred proteins = %v, want the excluded whey dropped and duplicates collapsed", got.PreferredProteins)
	}
	if got.PreferredCarbs == nil || len(got.PreferredCarbs) != 0 {
		t.Errorf("preferred carbs = %#v, want empty non-nil", got.PreferredCarbs)
	}
	if !slices.Equal(got.MealChoices, []MealChoice{{Slot: 4, Fat: "avocado"}}) {
		t.Errorf("meal choices = %+v, want the empty entry dropped", got.MealChoices)
	}
}

func TestBuildCyclingTemplatesAndShares(t *testing.T) {
	plan := buildFor(t, func(in *Input) { in.CarbCycling = true })
	if plan.Cycling == nil || plan.Cycling.TrainingDays != 5 || plan.Cycling.RestDays != 2 {
		t.Fatalf("cycling = %+v, want 5 training and 2 rest days", plan.Cycling)
	}
	if got := []string{plan.Templates[0].DayType, plan.Templates[1].DayType}; !slices.Equal(got, []string{DayTraining, DayRest}) {
		t.Errorf("day types = %v", got)
	}
	if plan.Templates[0].Day != plan.Cycling.Training || plan.Templates[1].Day != plan.Cycling.Rest {
		t.Error("templates must carry the cycled day macros")
	}
	// Same foods on both day types; only the portions differ.
	for i := range plan.Templates[0].Meals {
		a, b := itemIDs(plan.Templates[0].Meals[i]), itemIDs(plan.Templates[1].Meals[i])
		if a[0] != b[0] {
			t.Errorf("meal %d protein differs between day types: %v vs %v", i+1, a, b)
		}
	}
	if sum := plan.Protein.Pct + plan.Carbs.Pct + plan.Fat.Pct; sum != 100 {
		t.Errorf("shares sum to %d, want 100", sum)
	}
	if plan.Protein.Kcal != 4*plan.Targets.ProteinG || plan.Fat.Kcal != 9*plan.Targets.FatG {
		t.Errorf("macro energy = %+v %+v", plan.Protein, plan.Fat)
	}
	if plan.SuggestedLevel.ID != LevelModerate || plan.Level.ID != LevelModerate {
		t.Errorf("levels = %s, %s", plan.SuggestedLevel.ID, plan.Level.ID)
	}
}

func TestBuildWithZeroCarbohydrateStillServesProtein(t *testing.T) {
	in := mustResolve(t, imperialMale())
	plan := Build(in, Targets{BMRKcal: 1800, TDEEKcal: 2500, TargetKcal: 1000, ProteinG: 160, CarbsG: 0, FatG: 60})
	for _, meal := range plan.Templates[0].Meals {
		if meal.Provided.ProteinG < 25 {
			t.Errorf("meal %d provides %.1f g protein", meal.Slot, meal.Provided.ProteinG)
		}
	}
	if !slices.Contains(plan.Notices, NoticeCarbsFloor) {
		t.Errorf("notices = %v", plan.Notices)
	}
}

// mealFor returns the meal in slot for an input, built on its first template.
func mealFor(t *testing.T, in Input, slot int) Meal {
	t.Helper()
	in = mustResolve(t, in)
	return Build(in, Calculate(in)).Templates[0].Meals[slot-1]
}

func TestSolverKeepsTheCompanionWhenTheCarbohydrateFoodIsFatty(t *testing.T) {
	// Flour tortillas alone carry more fat than Meal 3 allows. Whole eggs on
	// top would triple it; egg whites must carry the protein instead.
	meal := mealFor(t, Input{
		UnitSystem: UnitImperial, Weight: 260, Height: 68, AgeYears: 30, Sex: SexMale,
		StrengthDays: 4, Goal: GoalRecomp,
		MealChoices: []MealChoice{{Slot: 3, Protein: "eggs", Carb: "flour_tortillas", Fat: "olive_oil"}},
	}, 3)
	ids := itemIDs(meal)
	if !slices.Contains(ids, "egg_whites") {
		t.Fatalf("meal 3 = %v, want egg whites carrying the protein", ids)
	}
	if over := meal.Provided.FatG - float64(meal.Target.FatG); over > 12 {
		t.Errorf("meal 3 fat %.1f g vs target %d g (%v)", meal.Provided.FatG, meal.Target.FatG, ids)
	}
	if d := math.Abs(meal.Provided.ProteinG - float64(meal.Target.ProteinG)); d > 8 {
		t.Errorf("meal 3 protein %.1f g vs target %d g", meal.Provided.ProteinG, meal.Target.ProteinG)
	}
}

func TestSolverKeepsAFatSourceWhenCarbohydrateIsTiny(t *testing.T) {
	in := mustResolve(t, imperialMale())
	plan := Build(in, Targets{BMRKcal: 1800, TDEEKcal: 2500, TargetKcal: 1400, ProteinG: 176, CarbsG: 12, FatG: 70})
	for _, meal := range plan.Templates[0].Meals {
		if d := math.Abs(meal.Provided.FatG - float64(meal.Target.FatG)); d > 6 {
			t.Errorf("meal %d fat %.1f g vs target %d g (%v)", meal.Slot, meal.Provided.FatG, meal.Target.FatG, itemIDs(meal))
		}
		if d := math.Abs(meal.Provided.ProteinG - float64(meal.Target.ProteinG)); d > 8 {
			t.Errorf("meal %d protein %.1f g vs target %d g (%v)", meal.Slot, meal.Provided.ProteinG, meal.Target.ProteinG, itemIDs(meal))
		}
	}
}

// TestSolverIsNeverBeatenByASimplerMeal sweeps weights, goals, rates, and
// every swap of protein, carbohydrate, and fat food in every meal, and checks
// two things for each meal: the portions are finite and non-negative, and no
// one- or two-food meal from the same foods is closer to the target than the
// one the solver chose.
func TestSolverIsNeverBeatenByASimplerMeal(t *testing.T) {
	byRole := map[string][]Food{}
	for _, f := range Foods {
		if f.Selectable {
			byRole[f.Role] = append(byRole[f.Role], f)
		}
	}
	checked := 0
	for _, weight := range []float64{100, 180, 265} {
		for _, goal := range []string{GoalFatLoss, GoalMuscleGain} {
			for _, rates := range [][2]float64{{0.7, 0.25}, {1.1, 0.45}} {
				in := imperialMale()
				in.Weight, in.Goal = weight, goal
				in.ProteinPerLb, in.FatPerLb = floatPtr(rates[0]), floatPtr(rates[1])
				in = mustResolve(t, in)
				day := Calculate(in)
				for slot, target := range mealTargets(newDayMacros(day.ProteinG, day.CarbsG, day.FatG)) {
					want := [3]float64{float64(target.ProteinG), float64(target.CarbsG), float64(target.FatG)}
					for _, protein := range byRole[RoleProtein] {
						for _, carb := range byRole[RoleCarb] {
							for _, fat := range byRole[RoleFat] {
								foods := mealFoods{protein: protein, carb: carb, fat: fat}
								solved := solveMeal(target, foods, nil)
								if len(solved) == 0 {
									t.Fatalf("slot %d %s/%s/%s: no meal", slot+1, protein.ID, carb.ID, fat.ID)
								}
								grams := make([]float64, len(solved))
								for i, p := range solved {
									if math.IsNaN(p.grams) || math.IsInf(p.grams, 0) || p.grams < 0 {
										t.Fatalf("slot %d %s/%s/%s: %s = %v g", slot+1, protein.ID, carb.ID, fat.ID, p.food.ID, p.grams)
									}
									grams[i] = p.grams
								}
								chosen := macroError(solved, grams, want)
								for _, pair := range [][]portion{
									{{food: protein}, {food: carb}}, {{food: protein}, {food: fat}}, {{food: carb}, {food: fat}},
									{{food: protein}}, {{food: carb}},
								} {
									if g, ok := fit(pair, want); ok && macroError(pair, g, want) < chosen-1e-6 {
										t.Fatalf("slot %d %s/%s/%s: a simpler meal is closer (%.2f < %.2f)", slot+1, protein.ID, carb.ID, fat.ID, macroError(pair, g, want), chosen)
									}
								}
								checked++
							}
						}
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("sweep checked nothing")
	}
}

func TestSubsetsPreferTheAthletesFoodsOverTheCompanion(t *testing.T) {
	subsets := subsetsInPreferenceOrder(4)
	if len(subsets) != 14 { // C(4,1)+C(4,2)+C(4,3)
		t.Fatalf("subsets = %d, want 14", len(subsets))
	}
	if !slices.Equal(subsets[0], []int{0, 1, 2}) {
		t.Errorf("first subset = %v, want protein, carbohydrate, fat", subsets[0])
	}
	seenCompanion := false
	for _, subset := range subsets {
		has := slices.Contains(subset, 3)
		if seenCompanion && !has {
			t.Fatalf("a subset without the companion comes after one with it: %v", subsets)
		}
		seenCompanion = seenCompanion || has
	}
	if got := subsetsInPreferenceOrder(2); len(got) != 3 || !slices.Equal(got[0], []int{0, 1}) {
		t.Errorf("two candidates = %v", got)
	}
}
