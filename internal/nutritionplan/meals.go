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

// solveMeal returns the grams of each food that meet the meal's protein,
// carbohydrate, and fat targets as closely as non-negative portions allow.
func solveMeal(target DayMacros, foods mealFoods, excluded []string) []portion {
	p, c, f := float64(target.ProteinG), float64(target.CarbsG), float64(target.FatG)
	a, b, z := foods.protein, foods.carb, foods.fat

	// All three foods, all three targets.
	if z.ID != "" {
		if g, ok := solve3(a, b, z, p, c, f); ok {
			return []portion{{a, RoleProtein, g[0]}, {b, RoleCarb, g[1]}, {z, RoleFat, g[2]}}
		}
	}
	// The protein and carbohydrate foods already carry the meal's fat (or
	// more): meet protein and carbohydrate exactly without a fat source.
	ga, gb := solve2(a, b, p, c)
	// A fatty protein food with a lean companion: let the companion carry the
	// protein the fat target leaves no room for.
	if partner, ok := FoodByID(a.LeanPartner); ok && partner.allowed(excluded) && ga*a.fat()+gb*b.fat() > f {
		if g, ok := solve3(a, partner, b, p, c, f); ok {
			return []portion{{a, RoleProtein, g[0]}, {partner, RoleProtein, g[1]}, {b, RoleCarb, g[2]}}
		}
	}
	return []portion{{a, RoleProtein, ga}, {b, RoleCarb, gb}}
}

// solve3 solves grams of x, y, z for protein p, carbohydrate c, and fat f by
// Cramer's rule. ok is false when the system is singular or any amount would
// be negative.
func solve3(x, y, z Food, p, c, f float64) ([3]float64, bool) {
	m := [3][3]float64{
		{x.protein(), y.protein(), z.protein()},
		{x.carbs(), y.carbs(), z.carbs()},
		{x.fat(), y.fat(), z.fat()},
	}
	rhs := [3]float64{p, c, f}
	det := det3(m)
	if math.Abs(det) < 1e-12 {
		return [3]float64{}, false
	}
	var out [3]float64
	for col := range 3 {
		replaced := m
		for row := range 3 {
			replaced[row][col] = rhs[row]
		}
		out[col] = det3(replaced) / det
		if out[col] < -1e-9 {
			return [3]float64{}, false
		}
		out[col] = math.Max(out[col], 0)
	}
	return out, true
}

func det3(m [3][3]float64) float64 {
	return m[0][0]*(m[1][1]*m[2][2]-m[1][2]*m[2][1]) -
		m[0][1]*(m[1][0]*m[2][2]-m[1][2]*m[2][0]) +
		m[0][2]*(m[1][0]*m[2][1]-m[1][1]*m[2][0])
}

// solve2 solves grams of protein food a and carbohydrate food b for protein p
// and carbohydrate c. When one amount would be negative that food is left out
// and the other is sized for its own macro.
func solve2(a, b Food, p, c float64) (ga, gb float64) {
	det := a.protein()*b.carbs() - b.protein()*a.carbs()
	if math.Abs(det) > 1e-12 {
		ga = (p*b.carbs() - b.protein()*c) / det
		gb = (a.protein()*c - p*a.carbs()) / det
		if ga >= 0 && gb >= 0 {
			return ga, gb
		}
	}
	if ga < 0 || a.protein() == 0 {
		return 0, safeDiv(c, b.carbs())
	}
	return safeDiv(p, a.protein()), 0
}

func safeDiv(n, d float64) float64 {
	if d <= 0 {
		return 0
	}
	return n / d
}

// buildMeal sizes, rounds, and totals one meal.
func buildMeal(slot MealSlot, target DayMacros, foods mealFoods, excluded []string) Meal {
	meal := Meal{Slot: slot.Slot, Name: slot.Name, Target: target, Items: []Item{}}
	for _, p := range solveMeal(target, foods, excluded) {
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
