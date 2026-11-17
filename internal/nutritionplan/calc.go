// Package nutritionplan turns an athlete's own numbers into estimated daily
// energy and macro targets, an optional training/rest carbohydrate split, and
// a four-meal template built from a fixed list of USDA foods. It is pure
// arithmetic: no I/O, no clock, no stored state. See specs/032-macro-calculator.
package nutritionplan

import (
	"fmt"
	"math"
	"slices"
)

// Unit systems, sexes, goals, and activity levels accepted on input.
const (
	UnitImperial = "imperial"
	UnitMetric   = "metric"

	SexMale   = "male"
	SexFemale = "female"

	GoalFatLoss     = "fat_loss"
	GoalRecomp      = "recomp"
	GoalMaintenance = "maintenance"
	GoalMuscleGain  = "muscle_gain"

	LevelLight    = "light"
	LevelModerate = "moderate"
	LevelVery     = "very"
)

// Notices a plan may carry. They are codes for the client, not messages.
const (
	// NoticeCarbsFloor means protein and fat alone meet or exceed the target
	// energy, so carbohydrate is 0 rather than negative.
	NoticeCarbsFloor = "carbs_floor"
	// NoticeBelowBMR means the target energy is under resting energy.
	NoticeBelowBMR = "below_bmr"
	// NoticeCyclingNotApplied means cycling was requested but the week has no
	// training day or no rest day to move carbohydrate between.
	NoticeCyclingNotApplied = "cycling_not_applied"
)

const (
	kgPerLb = 0.45359237
	cmPerIn = 2.54

	kcalPerGProtein = 4
	kcalPerGCarbs   = 4
	kcalPerGFat     = 9

	// restCarbShare is the share of the daily carbohydrate figure a rest day
	// gives up when cycling is on: the middle of the 15–20% range.
	restCarbShare = 0.175

	// DefaultFatPerLb is the default fat rate in grams per pound of body weight.
	DefaultFatPerLb = 0.33
	// Protein and fat rates the athlete may choose, in grams per pound.
	MinProteinPerLb = 0.70
	MaxProteinPerLb = 1.10
	MinFatPerLb     = 0.25
	MaxFatPerLb     = 0.45
)

// Goal is one selectable goal with the calorie adjustment range it allows.
type Goal struct {
	ID                  string
	MinAdjustmentKcal   int
	MaxAdjustmentKcal   int
	DefaultAdjustment   int
	DefaultProteinPerLb float64
}

// Goals lists the goals in display order. The default adjustment is the middle
// of each range.
var Goals = []Goal{
	{ID: GoalFatLoss, MinAdjustmentKcal: -500, MaxAdjustmentKcal: -300, DefaultAdjustment: -400, DefaultProteinPerLb: 0.95},
	{ID: GoalRecomp, MinAdjustmentKcal: -300, MaxAdjustmentKcal: -250, DefaultAdjustment: -275, DefaultProteinPerLb: 0.95},
	{ID: GoalMaintenance, MinAdjustmentKcal: 0, MaxAdjustmentKcal: 0, DefaultAdjustment: 0, DefaultProteinPerLb: 0.85},
	{ID: GoalMuscleGain, MinAdjustmentKcal: 200, MaxAdjustmentKcal: 400, DefaultAdjustment: 300, DefaultProteinPerLb: 0.85},
}

// ActivityLevel is one activity multiplier and the weekly session count that
// suggests it. MaxSessions is -1 on the last level.
type ActivityLevel struct {
	ID          string
	Multiplier  float64
	MinSessions int
	MaxSessions int
}

// ActivityLevels lists the levels from least to most active.
var ActivityLevels = []ActivityLevel{
	{ID: LevelLight, Multiplier: 1.375, MinSessions: 0, MaxSessions: 3},
	{ID: LevelModerate, Multiplier: 1.55, MinSessions: 4, MaxSessions: 5},
	{ID: LevelVery, Multiplier: 1.725, MinSessions: 6, MaxSessions: -1},
}

// Limits on the athlete's numbers, per unit system: pounds and total inches,
// or kilograms and centimetres.
const (
	MinWeightLb, MaxWeightLb = 66.0, 660.0
	MinHeightIn, MaxHeightIn = 48.0, 96.0
	MinWeightKg, MaxWeightKg = 30.0, 300.0
	MinHeightCm, MaxHeightCm = 122.0, 244.0
	MinAgeYears, MaxAgeYears = 18, 100
)

// FieldError is a validation failure naming the offending input field by its
// wire name.
type FieldError struct {
	Field   string
	Message string
}

func (e *FieldError) Error() string { return e.Message }

func fieldError(field, format string, args ...any) *FieldError {
	return &FieldError{Field: field, Message: fmt.Sprintf(format, args...)}
}

// MealChoice overrides the selected food for one or more roles of one meal.
// An empty role keeps the selection.
type MealChoice struct {
	Slot    int
	Protein string
	Carb    string
	Fat     string
}

// Input is what the athlete entered. Weight and Height are in UnitSystem:
// pounds and total inches, or kilograms and centimetres. ActivityLevel is
// empty to follow the suggestion. AdjustmentKcal, ProteinPerLb, and FatPerLb
// are nil to take the goal's defaults; Resolve fills them.
type Input struct {
	UnitSystem        string
	Weight            float64
	Height            float64
	AgeYears          int
	Sex               string
	StrengthDays      int
	CardioDays        int
	ActivityLevel     string
	Goal              string
	AdjustmentKcal    *int
	ProteinPerLb      *float64
	FatPerLb          *float64
	CarbCycling       bool
	PreferredProteins []string
	PreferredCarbs    []string
	Exclusions        []string
	MealChoices       []MealChoice
}

// Targets are the daily numbers computed from an Input, in whole units.
type Targets struct {
	BMRKcal        int
	TDEEKcal       int
	AdjustmentKcal int
	TargetKcal     int
	ProteinG       int
	CarbsG         int
	FatG           int
}

// DayMacros is one day's (or one meal's) energy and macro grams. EnergyKcal
// is always 4·protein + 4·carbs + 9·fat.
type DayMacros struct {
	EnergyKcal int
	ProteinG   int
	CarbsG     int
	FatG       int
}

func newDayMacros(protein, carbs, fat int) DayMacros {
	return DayMacros{
		EnergyKcal: kcalPerGProtein*protein + kcalPerGCarbs*carbs + kcalPerGFat*fat,
		ProteinG:   protein,
		CarbsG:     carbs,
		FatG:       fat,
	}
}

// Cycling is the training-day and rest-day split of one week. Protein and fat
// equal the daily figures; TrainingDays×Training.CarbsG + RestDays×Rest.CarbsG
// is exactly seven times the daily carbohydrate figure.
type Cycling struct {
	TrainingDays      int
	RestDays          int
	Training          DayMacros
	Rest              DayMacros
	WeeklyAverageKcal int
}

// GoalByID returns the goal with the given id.
func GoalByID(id string) (Goal, bool) {
	for _, g := range Goals {
		if g.ID == id {
			return g, true
		}
	}
	return Goal{}, false
}

// LevelByID returns the activity level with the given id.
func LevelByID(id string) (ActivityLevel, bool) {
	for _, l := range ActivityLevels {
		if l.ID == id {
			return l, true
		}
	}
	return ActivityLevel{}, false
}

// SuggestLevel maps weekly sessions (strength days plus cardio or sport days)
// to an activity level: up to 3 light, 4–5 moderate, 6 or more very active.
func SuggestLevel(strengthDays, cardioDays int) ActivityLevel {
	sessions := strengthDays + cardioDays
	for _, l := range ActivityLevels {
		if l.MaxSessions < 0 || sessions <= l.MaxSessions {
			return l
		}
	}
	return ActivityLevels[len(ActivityLevels)-1]
}

// TrainingDays is the number of days in the week that count as training days
// for carbohydrate cycling: strength plus cardio or sport days, at most 7.
func TrainingDays(strengthDays, cardioDays int) int {
	return min(7, strengthDays+cardioDays)
}

// Resolve validates in and returns it with defaults filled and lists
// de-duplicated. The returned error is a *FieldError.
func Resolve(in Input) (Input, error) {
	var minWeight, maxWeight, minHeight, maxHeight float64
	switch in.UnitSystem {
	case UnitImperial:
		minWeight, maxWeight, minHeight, maxHeight = MinWeightLb, MaxWeightLb, MinHeightIn, MaxHeightIn
	case UnitMetric:
		minWeight, maxWeight, minHeight, maxHeight = MinWeightKg, MaxWeightKg, MinHeightCm, MaxHeightCm
	default:
		return in, fieldError("unit_system", "unit_system must be imperial or metric")
	}
	if !inRange(in.Weight, minWeight, maxWeight) || !decimals(in.Weight, 1) {
		return in, fieldError("weight", "weight must be between %g and %g with at most one decimal", minWeight, maxWeight)
	}
	if !inRange(in.Height, minHeight, maxHeight) || !decimals(in.Height, 1) {
		return in, fieldError("height", "height must be between %g and %g with at most one decimal", minHeight, maxHeight)
	}
	if in.AgeYears < MinAgeYears || in.AgeYears > MaxAgeYears {
		return in, fieldError("age_years", "age_years must be between 18 and 100")
	}
	if in.Sex != SexMale && in.Sex != SexFemale {
		return in, fieldError("sex", "sex must be male or female")
	}
	if in.StrengthDays < 0 || in.StrengthDays > 7 {
		return in, fieldError("strength_days", "strength_days must be between 0 and 7")
	}
	if in.CardioDays < 0 || in.CardioDays > 7 {
		return in, fieldError("cardio_days", "cardio_days must be between 0 and 7")
	}
	if in.ActivityLevel != "" {
		if _, ok := LevelByID(in.ActivityLevel); !ok {
			return in, fieldError("activity_level", "activity_level must be light, moderate, or very")
		}
	}
	goal, ok := GoalByID(in.Goal)
	if !ok {
		return in, fieldError("goal", "goal must be fat_loss, recomp, maintenance, or muscle_gain")
	}
	if in.AdjustmentKcal == nil {
		in.AdjustmentKcal = &goal.DefaultAdjustment
	} else if *in.AdjustmentKcal < goal.MinAdjustmentKcal || *in.AdjustmentKcal > goal.MaxAdjustmentKcal {
		return in, fieldError("calorie_adjustment_kcal", "calorie_adjustment_kcal must be between %d and %d for this goal", goal.MinAdjustmentKcal, goal.MaxAdjustmentKcal)
	}
	if in.ProteinPerLb == nil {
		in.ProteinPerLb = &goal.DefaultProteinPerLb
	} else if !inRange(*in.ProteinPerLb, MinProteinPerLb, MaxProteinPerLb) || !decimals(*in.ProteinPerLb, 2) {
		return in, fieldError("protein_g_per_lb", "protein_g_per_lb must be between 0.7 and 1.1 with at most two decimals")
	}
	if in.FatPerLb == nil {
		fat := DefaultFatPerLb
		in.FatPerLb = &fat
	} else if !inRange(*in.FatPerLb, MinFatPerLb, MaxFatPerLb) || !decimals(*in.FatPerLb, 2) {
		return in, fieldError("fat_g_per_lb", "fat_g_per_lb must be between 0.25 and 0.45 with at most two decimals")
	}
	return resolveFoods(in)
}

func inRange(v, lo, hi float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= lo-1e-9 && v <= hi+1e-9
}

// decimals reports whether v has at most places decimal places.
func decimals(v float64, places int) bool {
	scaled := v * math.Pow10(places)
	return math.Abs(scaled-math.Round(scaled)) < 1e-6
}

// weightLb returns the athlete's weight in pounds.
func weightLb(in Input) float64 {
	if in.UnitSystem == UnitMetric {
		return in.Weight / kgPerLb
	}
	return in.Weight
}

// bmr is resting energy by the Mifflin-St Jeor equation (Mifflin et al.,
// Am J Clin Nutr 1990;51:241-7): 10·kg + 6.25·cm − 5·age + s, with s = +5 for
// male and −161 for female. The imperial form uses the owner-specified
// coefficients 4.536 and 15.875 (10 × 0.4536 and 6.25 × 2.54).
func bmr(in Input) float64 {
	s := 5.0
	if in.Sex == SexFemale {
		s = -161
	}
	age := 5 * float64(in.AgeYears)
	if in.UnitSystem == UnitMetric {
		return 10*in.Weight + 6.25*in.Height - age + s
	}
	return 4.536*in.Weight + 15.875*in.Height - age + s
}

// EffectiveLevel returns the level used for the multiplier: the athlete's
// override when set, otherwise the suggestion.
func EffectiveLevel(in Input) ActivityLevel {
	if l, ok := LevelByID(in.ActivityLevel); ok {
		return l
	}
	return SuggestLevel(in.StrengthDays, in.CardioDays)
}

// Calculate computes the daily targets for a resolved Input.
func Calculate(in Input) Targets {
	resting := bmr(in)
	tdee := int(math.Round(resting * EffectiveLevel(in).Multiplier))
	target := tdee + *in.AdjustmentKcal
	lb := weightLb(in)
	protein := int(math.Round(lb * *in.ProteinPerLb))
	fat := int(math.Round(lb * *in.FatPerLb))
	remaining := target - kcalPerGProtein*protein - kcalPerGFat*fat
	carbs := 0
	if remaining > 0 {
		carbs = int(math.Round(float64(remaining) / kcalPerGCarbs))
	}
	return Targets{
		BMRKcal:        int(math.Round(resting)),
		TDEEKcal:       tdee,
		AdjustmentKcal: *in.AdjustmentKcal,
		TargetKcal:     target,
		ProteinG:       protein,
		CarbsG:         carbs,
		FatG:           fat,
	}
}

// notices lists the notice codes that apply to in and t, in a stable order.
func notices(in Input, t Targets, cycling *Cycling) []string {
	out := []string{}
	if t.TargetKcal <= kcalPerGProtein*t.ProteinG+kcalPerGFat*t.FatG {
		out = append(out, NoticeCarbsFloor)
	}
	if t.TargetKcal < t.BMRKcal {
		out = append(out, NoticeBelowBMR)
	}
	if in.CarbCycling && cycling == nil {
		out = append(out, NoticeCyclingNotApplied)
	}
	return out
}

// cycle splits the daily carbohydrate figure into a training-day and a
// rest-day figure. It returns nil when the week lacks a training day or a
// rest day. The rest-day figure is the whole number nearest to
// carbs×(1−restCarbShare) for which the week's total stays exactly 7×carbs
// in whole grams; because 7 is prime such a number always lies within
// trainingDays consecutive integers. Keeping the week exact takes priority
// over the share, so with a small daily figure and many training days the
// rest-day reduction can land a fraction outside 15–20% (for example 20.3%
// at 118 g with six training days).
func cycle(in Input, t Targets) *Cycling {
	training := TrainingDays(in.StrengthDays, in.CardioDays)
	rest := 7 - training
	if !in.CarbCycling || training == 0 || rest == 0 {
		return nil
	}
	week := 7 * t.CarbsG
	ideal := int(math.Round(float64(t.CarbsG) * (1 - restCarbShare)))
	restCarbs := t.CarbsG // always valid: 7C − rest·C = training·C
	for d := 0; d <= training; d++ {
		if c := ideal - d; c >= 0 && (week-rest*c)%training == 0 {
			restCarbs = c
			break
		}
		if c := ideal + d; c <= t.CarbsG && (week-rest*c)%training == 0 {
			restCarbs = c
			break
		}
	}
	trainingDay := newDayMacros(t.ProteinG, (week-rest*restCarbs)/training, t.FatG)
	restDay := newDayMacros(t.ProteinG, restCarbs, t.FatG)
	return &Cycling{
		TrainingDays:      training,
		RestDays:          rest,
		Training:          trainingDay,
		Rest:              restDay,
		WeeklyAverageKcal: (training*trainingDay.EnergyKcal + rest*restDay.EnergyKcal) / 7,
	}
}

// splitByWeights divides total into len(weights) whole parts proportional to
// weights that sum exactly to total, giving leftover units to the largest
// remainders (earlier parts first on a tie).
func splitByWeights(total int, weights []int) []int {
	sum := 0
	for _, w := range weights {
		sum += w
	}
	parts := make([]int, len(weights))
	remainders := make([]int, len(weights))
	assigned := 0
	for i, w := range weights {
		parts[i] = total * w / sum
		remainders[i] = total * w % sum
		assigned += parts[i]
	}
	order := make([]int, len(weights))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return remainders[b] - remainders[a] })
	for i := range total - assigned {
		parts[order[i]]++
	}
	return parts
}
