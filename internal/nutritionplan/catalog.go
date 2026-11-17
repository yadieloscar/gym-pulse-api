package nutritionplan

import "slices"

// Roles a template food plays in a meal.
const (
	RoleProtein = "protein"
	RoleCarb    = "carb"
	RoleFat     = "fat"
)

// Units a portion is shown in.
const (
	UnitOz    = "oz"
	UnitGram  = "g"
	UnitCup   = "cup"
	UnitCount = "count"
)

// Exclusion tags an athlete may set. A food carrying an excluded tag never
// appears in a template.
const (
	TagRice    = "rice"
	TagDairy   = "dairy"
	TagNuts    = "nuts"
	TagGluten  = "gluten"
	TagEggs    = "eggs"
	TagRedMeat = "red_meat"
)

// ExclusionTags lists the tags in display order.
var ExclusionTags = []string{TagRice, TagDairy, TagNuts, TagGluten, TagEggs, TagRedMeat}

const gramsPerOz = 28.35

// Food is one entry of the fixed template list. Facts are per 100 g and are
// copies of the USDA FoodData Central SR Legacy record FDCID (release
// 2021-10-28); nothing here is estimated. GramsPerUnit is the weight of one
// Unit, taken from the same record's portions where the unit is a cup or a
// count. Portions are rounded to multiples of Step.
type Food struct {
	ID           string
	Name         string
	Role         string
	Tags         []string
	Unit         string
	GramsPerUnit float64
	Step         float64
	Kcal         float64
	Protein      float64
	Carbs        float64
	Fat          float64
	FDCID        int
	// USDAName is the record's own description, used when the food is written
	// to the shared food catalog so it reads the same as a searched food.
	USDAName string
	// LeanPartner names a leaner food that carries the rest of a meal's
	// protein when this food alone would exceed the meal's fat.
	LeanPartner string
	// Selectable is false for a food that appears only as a LeanPartner.
	Selectable bool
}

// Per-gram densities used by the solver.
func (f Food) protein() float64 { return f.Protein / 100 }
func (f Food) carbs() float64   { return f.Carbs / 100 }
func (f Food) fat() float64     { return f.Fat / 100 }

// allowed reports whether f carries none of the excluded tags.
func (f Food) allowed(excluded []string) bool {
	for _, tag := range f.Tags {
		if slices.Contains(excluded, tag) {
			return false
		}
	}
	return true
}

// Foods is the fixed list, proteins then carbohydrates then fat sources.
// Meats and potatoes are cooked weights; oats are dry.
var Foods = []Food{
	{ID: "chicken_breast", Name: "Chicken breast, grilled", Role: RoleProtein, Unit: UnitOz, GramsPerUnit: gramsPerOz, Step: 0.5,
		Kcal: 151, Protein: 30.5, Carbs: 0, Fat: 3.17, FDCID: 171534, USDAName: "Chicken, broiler or fryers, breast, skinless, boneless, meat only, cooked, grilled", Selectable: true},
	{ID: "ground_beef_93", Name: "93/7 ground beef, pan-browned", Role: RoleProtein, Tags: []string{TagRedMeat}, Unit: UnitOz, GramsPerUnit: gramsPerOz, Step: 0.5,
		Kcal: 209, Protein: 28.9, Carbs: 0, Fat: 9.51, FDCID: 174755, USDAName: "Beef, ground, 93% lean meat / 7% fat, crumbles, cooked, pan-browned", Selectable: true},
	{ID: "sirloin_steak", Name: "Top sirloin steak, lean, grilled", Role: RoleProtein, Tags: []string{TagRedMeat}, Unit: UnitOz, GramsPerUnit: gramsPerOz, Step: 0.5,
		Kcal: 174, Protein: 30.4, Carbs: 0, Fat: 5.83, FDCID: 174694, USDAName: "Beef, loin, top sirloin filet, boneless, separable lean only, trimmed to 0\" fat, choice, cooked, grilled", Selectable: true},
	{ID: "eggs", Name: "Eggs, whole, hard-boiled", Role: RoleProtein, Tags: []string{TagEggs}, Unit: UnitCount, GramsPerUnit: 50, Step: 1,
		Kcal: 155, Protein: 12.6, Carbs: 1.12, Fat: 10.6, FDCID: 173424, USDAName: "Egg, whole, cooked, hard-boiled", LeanPartner: "egg_whites", Selectable: true},
	{ID: "egg_whites", Name: "Egg whites", Role: RoleProtein, Tags: []string{TagEggs}, Unit: UnitCount, GramsPerUnit: 33, Step: 1,
		Kcal: 52, Protein: 10.9, Carbs: 0.73, Fat: 0.17, FDCID: 172183, USDAName: "Egg, white, raw, fresh", Selectable: false},
	{ID: "greek_yogurt", Name: "Greek yogurt, plain, nonfat", Role: RoleProtein, Tags: []string{TagDairy}, Unit: UnitGram, GramsPerUnit: 1, Step: 10,
		Kcal: 59, Protein: 10.2, Carbs: 3.6, Fat: 0.39, FDCID: 170894, USDAName: "Yogurt, Greek, plain, nonfat (Includes foods for USDA's Food Distribution Program)", Selectable: true},
	{ID: "whey", Name: "Whey protein powder", Role: RoleProtein, Tags: []string{TagDairy}, Unit: UnitGram, GramsPerUnit: 1, Step: 5,
		Kcal: 352, Protein: 78.1, Carbs: 6.25, Fat: 1.56, FDCID: 173180, USDAName: "Beverages, Protein powder whey based", Selectable: true},

	{ID: "sourdough_bread", Name: "Sourdough bread", Role: RoleCarb, Tags: []string{TagGluten}, Unit: UnitOz, GramsPerUnit: gramsPerOz, Step: 0.5,
		Kcal: 272, Protein: 10.8, Carbs: 51.9, Fat: 2.42, FDCID: 172675, USDAName: "Bread, french or vienna (includes sourdough)", Selectable: true},
	{ID: "potatoes", Name: "Potatoes, boiled", Role: RoleCarb, Unit: UnitOz, GramsPerUnit: gramsPerOz, Step: 0.5,
		Kcal: 86, Protein: 1.71, Carbs: 20.0, Fat: 0.1, FDCID: 170440, USDAName: "Potatoes, boiled, cooked without skin, flesh, without salt", Selectable: true},
	{ID: "sweet_potatoes", Name: "Sweet potato, baked", Role: RoleCarb, Unit: UnitOz, GramsPerUnit: gramsPerOz, Step: 0.5,
		Kcal: 90, Protein: 2.01, Carbs: 20.7, Fat: 0.15, FDCID: 168483, USDAName: "Sweet potato, cooked, baked in skin, flesh, without salt", Selectable: true},
	{ID: "flour_tortillas", Name: "Flour tortillas", Role: RoleCarb, Tags: []string{TagGluten}, Unit: UnitCount, GramsPerUnit: 49, Step: 0.5,
		Kcal: 297, Protein: 8.01, Carbs: 49.3, Fat: 7.58, FDCID: 167535, USDAName: "Tortillas, ready-to-bake or -fry, flour, shelf stable", Selectable: true},
	{ID: "oats", Name: "Oats, dry", Role: RoleCarb, Unit: UnitCup, GramsPerUnit: 81, Step: 0.25,
		Kcal: 379, Protein: 13.2, Carbs: 67.7, Fat: 6.52, FDCID: 173904, USDAName: "Cereals, oats, regular and quick, not fortified, dry", Selectable: true},
	{ID: "banana", Name: "Banana", Role: RoleCarb, Unit: UnitCount, GramsPerUnit: 118, Step: 0.5,
		Kcal: 89, Protein: 1.09, Carbs: 22.8, Fat: 0.33, FDCID: 173944, USDAName: "Bananas, raw", Selectable: true},
	{ID: "white_rice", Name: "White rice, cooked", Role: RoleCarb, Tags: []string{TagRice}, Unit: UnitCup, GramsPerUnit: 158, Step: 0.25,
		Kcal: 130, Protein: 2.69, Carbs: 28.2, Fat: 0.28, FDCID: 168878, USDAName: "Rice, white, long-grain, regular, enriched, cooked", Selectable: true},

	{ID: "olive_oil", Name: "Olive oil", Role: RoleFat, Unit: UnitGram, GramsPerUnit: 1, Step: 1,
		Kcal: 884, Protein: 0, Carbs: 0, Fat: 100, FDCID: 171413, USDAName: "Oil, olive, salad or cooking", Selectable: true},
	{ID: "avocado", Name: "Avocado", Role: RoleFat, Unit: UnitGram, GramsPerUnit: 1, Step: 10,
		Kcal: 160, Protein: 2.0, Carbs: 8.53, Fat: 14.7, FDCID: 171705, USDAName: "Avocados, raw, all commercial varieties", Selectable: true},
	{ID: "almonds", Name: "Almonds", Role: RoleFat, Tags: []string{TagNuts}, Unit: UnitOz, GramsPerUnit: gramsPerOz, Step: 0.25,
		Kcal: 579, Protein: 21.2, Carbs: 21.6, Fat: 49.9, FDCID: 170567, USDAName: "Nuts, almonds", Selectable: true},
	{ID: "butter", Name: "Butter, salted", Role: RoleFat, Tags: []string{TagDairy}, Unit: UnitGram, GramsPerUnit: 1, Step: 1,
		Kcal: 717, Protein: 0.85, Carbs: 0.06, Fat: 81.1, FDCID: 173410, USDAName: "Butter, salted", Selectable: true},
}

// slotOrder ranks, per role and meal, the foods suited to that meal. Meal 3
// (around training) leads with quick carbohydrate and lean, fast protein.
var slotOrder = map[string]map[int][]string{
	RoleProtein: {
		1: {"eggs", "greek_yogurt", "whey"},
		2: {"chicken_breast", "ground_beef_93", "sirloin_steak"},
		3: {"whey", "greek_yogurt", "chicken_breast"},
		4: {"sirloin_steak", "ground_beef_93", "chicken_breast"},
	},
	RoleCarb: {
		1: {"oats", "sourdough_bread", "banana", "flour_tortillas"},
		2: {"potatoes", "white_rice", "flour_tortillas", "sweet_potatoes", "sourdough_bread"},
		3: {"banana", "white_rice", "sourdough_bread", "flour_tortillas", "potatoes"},
		4: {"sweet_potatoes", "potatoes", "white_rice", "flour_tortillas"},
	},
	RoleFat: {
		1: {"almonds", "avocado", "butter", "olive_oil"},
		2: {"olive_oil", "avocado"},
		3: {"almonds", "avocado", "olive_oil"},
		4: {"olive_oil", "avocado", "butter"},
	},
}

// FoodByID returns the food with the given id. An empty id is never found.
func FoodByID(id string) (Food, bool) {
	if id == "" {
		return Food{}, false
	}
	for _, f := range Foods {
		if f.ID == id {
			return f, true
		}
	}
	return Food{}, false
}
