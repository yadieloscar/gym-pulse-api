package model

import "time"

// MealChoice names the athlete's own food for one or more roles of one meal
// of the template. An omitted role keeps the selected food.
type MealChoice struct {
	Slot    int    `json:"slot"`
	Protein string `json:"protein,omitempty"`
	Carb    string `json:"carb,omitempty"`
	Fat     string `json:"fat,omitempty"`
}

// CalculatorInput is what the athlete entered into the macro calculator.
// Weight and Height are in UnitSystem: pounds and total inches, or kilograms
// and centimetres. Optional values are nil on a request to take the default;
// a response always carries the resolved value, except ActivityLevel, which
// stays null when the athlete follows the suggestion.
type CalculatorInput struct {
	UnitSystem            string       `json:"unit_system"`
	Weight                float64      `json:"weight"`
	Height                float64      `json:"height"`
	AgeYears              int          `json:"age_years"`
	Sex                   string       `json:"sex"`
	StrengthDays          int          `json:"strength_days"`
	CardioDays            int          `json:"cardio_days"`
	ActivityLevel         *string      `json:"activity_level"`
	Goal                  string       `json:"goal"`
	CalorieAdjustmentKcal *int         `json:"calorie_adjustment_kcal"`
	ProteinGPerLb         *float64     `json:"protein_g_per_lb"`
	FatGPerLb             *float64     `json:"fat_g_per_lb"`
	CarbCycling           bool         `json:"carb_cycling"`
	PreferredProteins     []string     `json:"preferred_proteins"`
	PreferredCarbs        []string     `json:"preferred_carbs"`
	Exclusions            []string     `json:"exclusions"`
	MealChoices           []MealChoice `json:"meal_choices"`
}

// SaveNutritionProfileRequest saves the calculator input as the athlete's
// plan. The service recomputes every target; the client sends none.
// ExpectedRevision is the saved plan's revision, 0 when none exists.
type SaveNutritionProfileRequest struct {
	CalculatorInput
	ExpectedRevision int64  `json:"expected_revision"`
	OperationKey     string `json:"operation_key"`
}

// PlanActivity reports the suggested activity level and the one used.
type PlanActivity struct {
	SuggestedLevel string  `json:"suggested_level"`
	Level          string  `json:"level"`
	Multiplier     float64 `json:"multiplier"`
}

// PlanMacro is one macro's grams, energy, and share of the day's energy.
type PlanMacro struct {
	G    int `json:"g"`
	Kcal int `json:"kcal"`
	Pct  int `json:"pct"`
}

// PlanMacros groups the three macros.
type PlanMacros struct {
	Protein PlanMacro `json:"protein"`
	Carbs   PlanMacro `json:"carbs"`
	Fat     PlanMacro `json:"fat"`
}

// PlanDayMacros is a day's or a meal's target in whole units.
type PlanDayMacros struct {
	EnergyKcal int `json:"energy_kcal"`
	ProteinG   int `json:"protein_g"`
	CarbsG     int `json:"carbs_g"`
	FatG       int `json:"fat_g"`
}

// PlanCarbCycling is the training-day and rest-day split of one week.
type PlanCarbCycling struct {
	TrainingDays      int           `json:"training_days"`
	RestDays          int           `json:"rest_days"`
	Training          PlanDayMacros `json:"training"`
	Rest              PlanDayMacros `json:"rest"`
	WeeklyAverageKcal int           `json:"weekly_average_kcal"`
}

// PlanProvided is what rounded portions actually supply.
type PlanProvided struct {
	EnergyKcal int     `json:"energy_kcal"`
	ProteinG   float64 `json:"protein_g"`
	CarbsG     float64 `json:"carbs_g"`
	FatG       float64 `json:"fat_g"`
}

// PlanMealItem is one food in a meal with its portion and values.
type PlanMealItem struct {
	Food       string  `json:"food"`
	Name       string  `json:"name"`
	Role       string  `json:"role"`
	Amount     float64 `json:"amount"`
	Unit       string  `json:"unit"`
	Grams      float64 `json:"grams"`
	EnergyKcal int     `json:"energy_kcal"`
	ProteinG   float64 `json:"protein_g"`
	CarbsG     float64 `json:"carbs_g"`
	FatG       float64 `json:"fat_g"`
	Source     string  `json:"source"`
	FDCID      int     `json:"fdc_id"`
}

// PlanMeal is one meal of a template.
type PlanMeal struct {
	Slot     int            `json:"slot"`
	Name     string         `json:"name"`
	Target   PlanDayMacros  `json:"target"`
	Items    []PlanMealItem `json:"items"`
	Provided PlanProvided   `json:"provided"`
}

// PlanMealTemplate is the four meals of one day type (standard, training,
// or rest).
type PlanMealTemplate struct {
	DayType string `json:"day_type"`
	PlanDayMacros
	Meals    []PlanMeal   `json:"meals"`
	Provided PlanProvided `json:"provided"`
}

// NutritionPlan is a calculation result: estimated energy, macro targets,
// the optional carbohydrate cycle, and the meal templates.
type NutritionPlan struct {
	Input                 CalculatorInput    `json:"input"`
	Activity              PlanActivity       `json:"activity"`
	BMRKcal               int                `json:"bmr_kcal"`
	TDEEKcal              int                `json:"tdee_kcal"`
	CalorieAdjustmentKcal int                `json:"calorie_adjustment_kcal"`
	TargetKcal            int                `json:"target_kcal"`
	Macros                PlanMacros         `json:"macros"`
	CarbCycling           *PlanCarbCycling   `json:"carb_cycling"`
	Notices               []string           `json:"notices"`
	MealTemplates         []PlanMealTemplate `json:"meal_templates"`
}

// SavedNutritionPlan is the athlete's saved plan: the stored input and the
// targets computed when it was saved, with templates built from them.
type SavedNutritionPlan struct {
	NutritionPlan
	Revision  int64     `json:"revision"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NutritionProfile is the stored row behind a saved plan. Input always has
// its defaults resolved.
type NutritionProfile struct {
	Input      CalculatorInput `json:"input"`
	BMRKcal    int             `json:"bmr_kcal"`
	TDEEKcal   int             `json:"tdee_kcal"`
	TargetKcal int             `json:"target_kcal"`
	ProteinG   int             `json:"protein_g"`
	CarbsG     int             `json:"carbs_g"`
	FatG       int             `json:"fat_g"`
	Revision   int64           `json:"revision"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// NutritionProfileOutcome says what a profile save did.
type NutritionProfileOutcome int

const (
	// NutritionProfileCreated means the athlete's first plan was stored (201).
	NutritionProfileCreated NutritionProfileOutcome = iota
	// NutritionProfileReplaced means the saved plan was replaced (200).
	NutritionProfileReplaced
)

// CalculatorGoalOption is one goal with its adjustment range and defaults.
type CalculatorGoalOption struct {
	ID                    string  `json:"id"`
	MinAdjustmentKcal     int     `json:"min_adjustment_kcal"`
	MaxAdjustmentKcal     int     `json:"max_adjustment_kcal"`
	DefaultAdjustmentKcal int     `json:"default_adjustment_kcal"`
	DefaultProteinGPerLb  float64 `json:"default_protein_g_per_lb"`
}

// CalculatorActivityOption is one activity level and the weekly sessions
// that suggest it. MaxSessions is null on the last level.
type CalculatorActivityOption struct {
	ID          string  `json:"id"`
	Multiplier  float64 `json:"multiplier"`
	MinSessions int     `json:"min_sessions"`
	MaxSessions *int    `json:"max_sessions"`
}

// CalculatorRateRange is the range of a grams-per-pound rate.
type CalculatorRateRange struct {
	Min     float64  `json:"min"`
	Max     float64  `json:"max"`
	Step    float64  `json:"step"`
	Default *float64 `json:"default,omitempty"`
}

// CalculatorFood is one food of the fixed template list with its published
// per-100 g facts and where they came from.
type CalculatorFood struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	Role       string       `json:"role"`
	Tags       []string     `json:"tags"`
	Unit       string       `json:"unit"`
	Step       float64      `json:"step"`
	Selectable bool         `json:"selectable"`
	Source     string       `json:"source"`
	FDCID      int          `json:"fdc_id"`
	Per100G    PlanProvided `json:"per_100g"`
}

// CalculatorMealOption names one of the four meals.
type CalculatorMealOption struct {
	Slot int    `json:"slot"`
	Name string `json:"name"`
}

// CalculatorOptions is everything the client needs to render the calculator
// without keeping its own copy of a number the service owns.
type CalculatorOptions struct {
	Goals          []CalculatorGoalOption     `json:"goals"`
	ActivityLevels []CalculatorActivityOption `json:"activity_levels"`
	ProteinGPerLb  CalculatorRateRange        `json:"protein_g_per_lb"`
	FatGPerLb      CalculatorRateRange        `json:"fat_g_per_lb"`
	Exclusions     []string                   `json:"exclusions"`
	Foods          []CalculatorFood           `json:"foods"`
	Meals          []CalculatorMealOption     `json:"meals"`
}
