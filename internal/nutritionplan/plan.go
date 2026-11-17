package nutritionplan

import "slices"

// MacroShare is one macro's grams, energy, and percentage share of the day's
// macro energy.
type MacroShare struct {
	G    int
	Kcal int
	Pct  int
}

// Plan is everything a calculation returns for a resolved Input.
type Plan struct {
	Input          Input
	SuggestedLevel ActivityLevel
	Level          ActivityLevel
	Targets        Targets
	Protein        MacroShare
	Carbs          MacroShare
	Fat            MacroShare
	Cycling        *Cycling
	Notices        []string
	Templates      []Template
}

// Build assembles the plan for a resolved Input and its targets. Calculating
// passes Calculate(in); reading a saved plan passes the targets stored when
// it was saved, so a saved plan's numbers never move on their own.
func Build(in Input, t Targets) Plan {
	energy := []int{kcalPerGProtein * t.ProteinG, kcalPerGCarbs * t.CarbsG, kcalPerGFat * t.FatG}
	pct := []int{0, 0, 0}
	if total := energy[0] + energy[1] + energy[2]; total > 0 {
		pct = splitByWeights(100, energy)
	}
	cycling := cycle(in, t)
	plan := Plan{
		Input:          in,
		SuggestedLevel: SuggestLevel(in.StrengthDays, in.CardioDays),
		Level:          EffectiveLevel(in),
		Targets:        t,
		Protein:        MacroShare{G: t.ProteinG, Kcal: energy[0], Pct: pct[0]},
		Carbs:          MacroShare{G: t.CarbsG, Kcal: energy[1], Pct: pct[1]},
		Fat:            MacroShare{G: t.FatG, Kcal: energy[2], Pct: pct[2]},
		Cycling:        cycling,
		Notices:        notices(in, t, cycling),
	}
	foods := selectFoods(in)
	if cycling != nil {
		plan.Templates = []Template{
			buildTemplate(DayTraining, cycling.Training, foods, in.Exclusions),
			buildTemplate(DayRest, cycling.Rest, foods, in.Exclusions),
		}
	} else {
		plan.Templates = []Template{
			buildTemplate(DayStandard, newDayMacros(t.ProteinG, t.CarbsG, t.FatG), foods, in.Exclusions),
		}
	}
	return plan
}

// resolveFoods validates and normalizes the food-related fields of in:
// preferences, exclusions, and meal choices.
func resolveFoods(in Input) (Input, error) {
	var err error
	if in.Exclusions, err = normalizeList(in.Exclusions, "exclusions", func(tag string) bool {
		return slices.Contains(ExclusionTags, tag)
	}); err != nil {
		return in, err
	}
	if in.PreferredProteins, err = normalizeList(in.PreferredProteins, "preferred_proteins", selectableWithRole(RoleProtein)); err != nil {
		return in, err
	}
	if in.PreferredCarbs, err = normalizeList(in.PreferredCarbs, "preferred_carbs", selectableWithRole(RoleCarb)); err != nil {
		return in, err
	}
	// The exclusion wins over a preference for the same food.
	allowed := func(id string) bool {
		f, _ := FoodByID(id)
		return f.allowed(in.Exclusions)
	}
	in.PreferredProteins = filter(in.PreferredProteins, allowed)
	in.PreferredCarbs = filter(in.PreferredCarbs, allowed)

	if len(in.MealChoices) > len(MealSlots) {
		return in, fieldError("meal_choices", "meal_choices allows at most one entry per meal")
	}
	seen := map[int]bool{}
	choices := make([]MealChoice, 0, len(in.MealChoices))
	for _, c := range in.MealChoices {
		if c.Slot < 1 || c.Slot > len(MealSlots) {
			return in, fieldError("meal_choices", "meal_choices slot must be between 1 and 4")
		}
		if seen[c.Slot] {
			return in, fieldError("meal_choices", "meal_choices allows at most one entry per meal")
		}
		seen[c.Slot] = true
		for _, pair := range []struct{ id, role string }{{c.Protein, RoleProtein}, {c.Carb, RoleCarb}, {c.Fat, RoleFat}} {
			if pair.id == "" {
				continue
			}
			f, ok := FoodByID(pair.id)
			if !ok || !f.Selectable || f.Role != pair.role {
				return in, fieldError("meal_choices", "meal_choices names an unknown %s food", pair.role)
			}
			if !f.allowed(in.Exclusions) {
				return in, fieldError("meal_choices", "meal_choices names an excluded food")
			}
		}
		if c.Protein != "" || c.Carb != "" || c.Fat != "" {
			choices = append(choices, c)
		}
	}
	slices.SortFunc(choices, func(a, b MealChoice) int { return a.Slot - b.Slot })
	in.MealChoices = choices
	return in, nil
}

func selectableWithRole(role string) func(string) bool {
	return func(id string) bool {
		f, ok := FoodByID(id)
		return ok && f.Selectable && f.Role == role
	}
}

// normalizeList de-duplicates values in order and rejects unknown ones. It
// always returns a non-nil slice.
func normalizeList(values []string, field string, known func(string) bool) ([]string, error) {
	const maxEntries = 20
	if len(values) > maxEntries {
		return nil, fieldError(field, "%s allows at most %d entries", field, maxEntries)
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if !known(v) {
			return nil, fieldError(field, "%s has an unknown value", field)
		}
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out, nil
}

func filter(values []string, keep func(string) bool) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}
