package nutritionplan

import (
	"math"
	"slices"
)

// Day types a template is built for.
const (
	DayStandard = "standard"
	DayTraining = "training"
	DayRest     = "rest"
)

// MealSlot is one of the four fixed meals of the template.
type MealSlot struct {
	Slot int
	Name string
}

// MealSlots lists the four meals in order.
var MealSlots = []MealSlot{
	{Slot: 1, Name: "Breakfast"},
	{Slot: 2, Name: "Lunch"},
	{Slot: 3, Name: "Pre/Post-Workout Fuel"},
	{Slot: 4, Name: "Dinner"},
}

// Shares of the day each meal carries, in percent. Protein is even; the
// largest carbohydrate share and the smallest fat share sit at Meal 3, around
// training.
var (
	proteinShares = []int{25, 25, 25, 25}
	carbShares    = []int{25, 20, 35, 20}
	fatShares     = []int{30, 30, 10, 30}
)

// Provided is what a meal's rounded portions actually supply.
type Provided struct {
	EnergyKcal int
	ProteinG   float64
	CarbsG     float64
	FatG       float64
}

// Item is one food in a meal with its portion. Amount is in Food.Unit at the
// food's step; Grams is the weight the values were computed from.
type Item struct {
	Food       Food
	Role       string
	Amount     float64
	Grams      float64
	EnergyKcal int
	ProteinG   float64
	CarbsG     float64
	FatG       float64
}

// Meal is one meal of a template: its share of the day, the foods chosen for
// it, and what their rounded portions provide.
type Meal struct {
	Slot     int
	Name     string
	Target   DayMacros
	Items    []Item
	Provided Provided
}

// Template is the four meals for one day type.
type Template struct {
	DayType  string
	Day      DayMacros
	Meals    []Meal
	Provided Provided
}

// mealFoods is the selection for one meal.
type mealFoods struct {
	protein Food
	carb    Food
	fat     Food
}

// mealTargets splits a day into four meal targets whose grams sum exactly to
// the day.
func mealTargets(day DayMacros) []DayMacros {
	protein := splitByWeights(day.ProteinG, proteinShares)
	carbs := splitByWeights(day.CarbsG, carbShares)
	fat := splitByWeights(day.FatG, fatShares)
	out := make([]DayMacros, len(MealSlots))
	for i := range MealSlots {
		out[i] = newDayMacros(protein[i], carbs[i], fat[i])
	}
	return out
}

// selectFoods picks the foods for all four meals. The same foods are used on
// every day type so a training day and a rest day differ only in portions.
func selectFoods(in Input) []mealFoods {
	excluded := in.Exclusions
	usedProteins := []string{}
	usedCarbs := []string{}
	out := make([]mealFoods, len(MealSlots))
	for i, slot := range MealSlots {
		choice := choiceFor(in, slot.Slot)
		var m mealFoods
		if f, ok := FoodByID(choice.Protein); ok {
			m.protein = f
		} else {
			m.protein = pick(RoleProtein, slot.Slot, in.PreferredProteins, excluded, usedProteins)
		}
		if f, ok := FoodByID(choice.Carb); ok {
			m.carb = f
		} else {
			m.carb = pick(RoleCarb, slot.Slot, in.PreferredCarbs, excluded, usedCarbs)
		}
		if f, ok := FoodByID(choice.Fat); ok {
			m.fat = f
		} else {
			m.fat = pick(RoleFat, slot.Slot, nil, excluded, nil)
		}
		usedProteins = append(usedProteins, m.protein.ID)
		usedCarbs = append(usedCarbs, m.carb.ID)
		out[i] = m
	}
	return out
}

func choiceFor(in Input, slot int) MealChoice {
	for _, c := range in.MealChoices {
		if c.Slot == slot {
			return c
		}
	}
	return MealChoice{Slot: slot}
}

// pick chooses a food of role for slot. In order: a preferred food suited to
// the meal and not yet used that day; a preferred food suited to the meal;
// any preferred food; a food suited to the meal; any food. Excluded foods
// are never candidates. An empty preferred list means no preference.
func pick(role string, slot int, preferred, excluded, used []string) Food {
	suited := []Food{}
	for _, id := range slotOrder[role][slot] {
		if f, ok := FoodByID(id); ok && f.allowed(excluded) {
			suited = append(suited, f)
		}
	}
	isPreferred := func(f Food) bool { return slices.Contains(preferred, f.ID) }
	if len(preferred) > 0 {
		for _, f := range suited {
			if isPreferred(f) && !slices.Contains(used, f.ID) {
				return f
			}
		}
		for _, f := range suited {
			if isPreferred(f) {
				return f
			}
		}
		for _, f := range Foods {
			if f.Role == role && f.Selectable && f.allowed(excluded) && isPreferred(f) && !slices.Contains(used, f.ID) {
				return f
			}
		}
		for _, f := range Foods {
			if f.Role == role && f.Selectable && f.allowed(excluded) && isPreferred(f) {
				return f
			}
		}
	}
	for _, f := range suited {
		if !slices.Contains(used, f.ID) {
			return f
		}
	}
	if len(suited) > 0 {
		return suited[0]
	}
	for _, f := range Foods {
		if f.Role == role && f.Selectable && f.allowed(excluded) {
			return f
		}
	}
	// Unreachable while the list keeps an untagged food in every role, which
	// TestEveryRoleSurvivesAllExclusions pins.
	return Food{}
}

// portion is a food and the grams of it the solver asked for.
type portion struct {
	food  Food
	role  string
	grams float64
}

// Weights that put a gram of each macro on the same footing (its energy) when
// the solver compares how far two candidate meals are from the target.
var macroWeights = [3]float64{kcalPerGProtein, kcalPerGCarbs, kcalPerGFat}

// solveMeal returns the grams of each food that bring the meal closest to its
// protein, carbohydrate, and fat targets with non-negative portions.
//
// The candidates are the meal's protein food, carbohydrate food, fat source,
// and the protein food's lean companion when it has one. Every combination of
// up to three of them is fitted by least squares on the energy-weighted macro
// error; combinations that would need a negative portion are discarded and
// the closest remaining one wins. Three foods and three targets usually fit
// exactly. When they cannot (a fatty protein food, a carbohydrate food that
// already carries the meal's fat, a tiny carbohydrate target), this keeps
// whichever foods get nearest instead of always falling back to protein and
// carbohydrate. On a tie the earlier candidate wins: no companion before a
// companion, more of the athlete's chosen foods before fewer.
func solveMeal(target DayMacros, foods mealFoods, excluded []string) []portion {
	want := [3]float64{float64(target.ProteinG), float64(target.CarbsG), float64(target.FatG)}
	candidates := []portion{{food: foods.protein, role: RoleProtein}, {food: foods.carb, role: RoleCarb}}
	if foods.fat.ID != "" {
		candidates = append(candidates, portion{food: foods.fat, role: RoleFat})
	}
	if partner, ok := FoodByID(foods.protein.LeanPartner); ok && partner.allowed(excluded) {
		candidates = append(candidates, portion{food: partner, role: RoleProtein})
	}

	var best []portion
	bestErr := math.Inf(1)
	for _, subset := range subsetsInPreferenceOrder(len(candidates)) {
		chosen := make([]portion, len(subset))
		for i, index := range subset {
			chosen[i] = candidates[index]
		}
		grams, ok := fit(chosen, want)
		if !ok {
			continue
		}
		if err := macroError(chosen, grams, want); err < bestErr-1e-9 {
			for i := range chosen {
				chosen[i].grams = grams[i]
			}
			best, bestErr = chosen, err
		}
	}
	return best
}

// subsetsInPreferenceOrder lists the index subsets of 1 to 3 of n candidates
// (n is 2 to 4: protein, carbohydrate, fat source, lean companion). Subsets
// without the companion (index 3) come first, then larger before smaller, so
// an equally good fit prefers the athlete's own foods and all of them.
func subsetsInPreferenceOrder(n int) [][]int {
	var out [][]int
	for _, withCompanion := range []bool{false, true} {
		for size := 3; size >= 1; size-- {
			for mask := 1; mask < 1<<n; mask++ {
				subset := []int{}
				for i := range n {
					if mask&(1<<i) != 0 {
						subset = append(subset, i)
					}
				}
				if len(subset) == size && (mask&(1<<3) != 0) == withCompanion {
					out = append(out, subset)
				}
			}
		}
	}
	return out
}

// macros returns a food's per-gram protein, carbohydrate, and fat.
func (f Food) macros() [3]float64 { return [3]float64{f.protein(), f.carbs(), f.fat()} }

// fit solves the energy-weighted least-squares grams of the chosen foods for
// the wanted macros through the normal equations. ok is false when the
// system is singular or any amount would be negative.
func fit(chosen []portion, want [3]float64) ([]float64, bool) {
	k := len(chosen)
	// Augmented normal matrix [AᵀWA | AᵀWb].
	m := make([][]float64, k)
	for i := range k {
		m[i] = make([]float64, k+1)
		mi := chosen[i].food.macros()
		for j := range k {
			mj := chosen[j].food.macros()
			for r := range 3 {
				m[i][j] += macroWeights[r] * macroWeights[r] * mi[r] * mj[r]
			}
		}
		for r := range 3 {
			m[i][k] += macroWeights[r] * macroWeights[r] * mi[r] * want[r]
		}
	}
	// Gaussian elimination with partial pivoting.
	for col := range k {
		pivot := col
		for row := col + 1; row < k; row++ {
			if math.Abs(m[row][col]) > math.Abs(m[pivot][col]) {
				pivot = row
			}
		}
		if math.Abs(m[pivot][col]) < 1e-12 {
			return nil, false
		}
		m[col], m[pivot] = m[pivot], m[col]
		for row := range k {
			if row == col {
				continue
			}
			factor := m[row][col] / m[col][col]
			for c := col; c <= k; c++ {
				m[row][c] -= factor * m[col][c]
			}
		}
	}
	grams := make([]float64, k)
	for i := range k {
		grams[i] = m[i][k] / m[i][i]
		if math.IsNaN(grams[i]) || math.IsInf(grams[i], 0) || grams[i] < -1e-9 {
			return nil, false
		}
		grams[i] = math.Max(grams[i], 0)
	}
	return grams, true
}

// macroError is the energy-weighted squared distance between what the
// portions provide and what the meal wants.
func macroError(chosen []portion, grams []float64, want [3]float64) float64 {
	var total float64
	for r := range 3 {
		got := 0.0
		for i, p := range chosen {
			got += grams[i] * p.food.macros()[r]
		}
		d := macroWeights[r] * (got - want[r])
		total += d * d
	}
	return total
}

// buildMeal sizes, rounds, and totals one meal.
func buildMeal(slot MealSlot, target DayMacros, foods mealFoods, excluded []string) Meal {
	meal := Meal{Slot: slot.Slot, Name: slot.Name, Target: target, Items: []Item{}}
	portions := solveMeal(target, foods, excluded)
	// Protein foods first (the chosen one before its companion), then the
	// carbohydrate food, then the fat source.
	roleOrder := map[string]int{RoleProtein: 0, RoleCarb: 1, RoleFat: 2}
	slices.SortStableFunc(portions, func(a, b portion) int { return roleOrder[a.role] - roleOrder[b.role] })
	for _, p := range portions {
		if p.food.GramsPerUnit <= 0 || p.food.Step <= 0 {
			continue
		}
		amount := math.Round(p.grams/p.food.GramsPerUnit/p.food.Step) * p.food.Step
		if amount <= 0 {
			continue
		}
		grams := amount * p.food.GramsPerUnit
		item := Item{
			Food:       p.food,
			Role:       p.role,
			Amount:     round(amount, 2),
			Grams:      round(grams, 1),
			EnergyKcal: int(math.Round(grams * p.food.Kcal / 100)),
			ProteinG:   round(grams*p.food.protein(), 1),
			CarbsG:     round(grams*p.food.carbs(), 1),
			FatG:       round(grams*p.food.fat(), 1),
		}
		meal.Items = append(meal.Items, item)
		meal.Provided = meal.Provided.plus(Provided{EnergyKcal: item.EnergyKcal, ProteinG: item.ProteinG, CarbsG: item.CarbsG, FatG: item.FatG})
	}
	return meal
}

func (p Provided) plus(o Provided) Provided {
	return Provided{
		EnergyKcal: p.EnergyKcal + o.EnergyKcal,
		ProteinG:   round(p.ProteinG+o.ProteinG, 1),
		CarbsG:     round(p.CarbsG+o.CarbsG, 1),
		FatG:       round(p.FatG+o.FatG, 1),
	}
}

func round(v float64, places int) float64 {
	scale := math.Pow10(places)
	return math.Round(v*scale) / scale
}

// buildTemplate builds the four meals of one day type.
func buildTemplate(dayType string, day DayMacros, foods []mealFoods, excluded []string) Template {
	t := Template{DayType: dayType, Day: day, Meals: make([]Meal, len(MealSlots))}
	for i, target := range mealTargets(day) {
		t.Meals[i] = buildMeal(MealSlots[i], target, foods[i], excluded)
		t.Provided = t.Provided.plus(t.Meals[i].Provided)
	}
	return t
}
