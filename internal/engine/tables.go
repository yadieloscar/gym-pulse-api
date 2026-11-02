// Package engine builds a program draft from the athlete's answers with no
// provider, database, or clock. The programming knowledge lives in this file
// as reviewable data; every table names its source in
// specs/015-ai-program-generation/research.md ("Goal guide") and
// specs/016-rule-based-program-engine/research.md (R3).
package engine

import (
	"slices"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// Workout kinds a split is made of.
const (
	KindFull    = "full"
	KindUpper   = "upper"
	KindLower   = "lower"
	KindPush    = "push"
	KindPull    = "pull"
	KindLegs    = "legs"
	KindCardio  = "cardio"
	KindCircuit = "circuit"
)

// Body areas a focus can name; slots carry the area their candidates train.
const (
	AreaUpper = "upper"
	AreaLower = "lower"
	AreaCore  = "core"
	AreaFull  = "full"
)

const (
	minExercises = 3
	maxExercises = 8
	maxWorkouts  = 6
)

// Source: ACSM 2009 progression stand (2–3 days per muscle group), Schoenfeld
// 2016 frequency meta-analysis (twice a week per muscle beats once), Garber
// 2011 (2–3 cardio sessions). Full-body for 1–3 days, upper/lower for 4,
// push/pull/legs from 5 for hypertrophy, cardio-led alternation for conditioning.
var defaultSplit = map[int][]string{
	1: {KindFull},
	2: {KindFull, KindFull},
	3: {KindFull, KindFull, KindFull},
	4: {KindUpper, KindLower, KindUpper, KindLower},
	5: {KindUpper, KindLower, KindFull, KindUpper, KindLower},
	6: {KindUpper, KindLower, KindFull, KindUpper, KindLower, KindFull},
}

var hypertrophySplit = map[int][]string{
	1: {KindFull},
	2: {KindFull, KindFull},
	3: {KindFull, KindFull, KindFull},
	4: {KindUpper, KindLower, KindUpper, KindLower},
	5: {KindPush, KindPull, KindLegs, KindUpper, KindLower},
	6: {KindPush, KindPull, KindLegs, KindPush, KindPull, KindLegs},
}

var conditioningSplit = map[int][]string{
	1: {KindCircuit},
	2: {KindCardio, KindCircuit},
	3: {KindCardio, KindCircuit, KindCardio},
	4: {KindCardio, KindCircuit, KindCardio, KindCircuit},
	5: {KindCardio, KindCircuit, KindCardio, KindCircuit, KindCardio},
	6: {KindCardio, KindCircuit, KindCardio, KindCircuit, KindCardio, KindCircuit},
}

// splitFor returns the workout kinds for a goal and workout count (1..6).
func splitFor(goal string, workouts int) []string {
	switch goal {
	case model.GoalHypertrophy:
		return hypertrophySplit[workouts]
	case model.GoalConditioning:
		return conditioningSplit[workouts]
	default:
		return defaultSplit[workouts]
	}
}

// Slot is one position in a workout: which catalog entries may fill it and
// which body area it trains. Cardio slots carry a duration instead of reps.
type Slot struct {
	Name    string
	Pattern string
	Area    string
	Cardio  bool
	Match   func(e model.CatalogExercise) bool
}

func pattern(p string) func(model.CatalogExercise) bool {
	return func(e model.CatalogExercise) bool { return e.Pattern == p }
}

func isolationOf(groups ...string) func(model.CatalogExercise) bool {
	return func(e model.CatalogExercise) bool {
		return e.Pattern == model.PatternIsolation && slices.Contains(groups, e.MuscleGroup)
	}
}

func isolationIn(group, category string) func(model.CatalogExercise) bool {
	return func(e model.CatalogExercise) bool {
		return e.Pattern == model.PatternIsolation && e.MuscleGroup == group && e.Category == category
	}
}

func cardioEntry(e model.CatalogExercise) bool {
	return e.Modality == "cardio" && e.Pattern == model.PatternLocomotion
}

var (
	slotSquat     = Slot{Name: "squat", Pattern: model.PatternSquat, Area: AreaLower, Match: pattern(model.PatternSquat)}
	slotHinge     = Slot{Name: "hinge", Pattern: model.PatternHinge, Area: AreaLower, Match: pattern(model.PatternHinge)}
	slotLunge     = Slot{Name: "lunge", Pattern: model.PatternLunge, Area: AreaLower, Match: pattern(model.PatternLunge)}
	slotHPush     = Slot{Name: "horizontal push", Pattern: model.PatternHorizontalPush, Area: AreaUpper, Match: pattern(model.PatternHorizontalPush)}
	slotVPush     = Slot{Name: "vertical push", Pattern: model.PatternVerticalPush, Area: AreaUpper, Match: pattern(model.PatternVerticalPush)}
	slotHPull     = Slot{Name: "horizontal pull", Pattern: model.PatternHorizontalPull, Area: AreaUpper, Match: pattern(model.PatternHorizontalPull)}
	slotVPull     = Slot{Name: "vertical pull", Pattern: model.PatternVerticalPull, Area: AreaUpper, Match: pattern(model.PatternVerticalPull)}
	slotCore      = Slot{Name: "core", Pattern: model.PatternCore, Area: AreaCore, Match: pattern(model.PatternCore)}
	slotCarry     = Slot{Name: "carry", Pattern: model.PatternCarry, Area: AreaFull, Match: pattern(model.PatternCarry)}
	slotExplosive = Slot{Name: "explosive", Pattern: model.PatternExplosive, Area: AreaFull, Match: pattern(model.PatternExplosive)}
	slotIsoPush   = Slot{Name: "chest or shoulder isolation", Pattern: model.PatternIsolation, Area: AreaUpper, Match: isolationOf("chest", "shoulders")}
	slotIsoPull   = Slot{Name: "back or arm isolation", Pattern: model.PatternIsolation, Area: AreaUpper, Match: isolationIn("arms", "pull")}
	slotTriceps   = Slot{Name: "triceps", Pattern: model.PatternIsolation, Area: AreaUpper, Match: isolationIn("arms", "push")}
	slotBiceps    = Slot{Name: "biceps", Pattern: model.PatternIsolation, Area: AreaUpper, Match: isolationIn("arms", "pull")}
	slotRearDelt  = Slot{Name: "rear delts", Pattern: model.PatternIsolation, Area: AreaUpper, Match: isolationIn("shoulders", "pull")}
	slotHamIso    = Slot{Name: "hamstring isolation", Pattern: model.PatternIsolation, Area: AreaLower, Match: isolationOf("hamstrings")}
	slotQuadIso   = Slot{Name: "quad isolation", Pattern: model.PatternIsolation, Area: AreaLower, Match: isolationOf("quads")}
	slotCalf      = Slot{Name: "calves", Pattern: model.PatternIsolation, Area: AreaLower, Match: isolationOf("calves")}
	slotCardio    = Slot{Name: "cardio", Pattern: model.PatternLocomotion, Area: AreaFull, Cardio: true, Match: cardioEntry}
	slotLocomote  = Slot{Name: "conditioning", Pattern: model.PatternLocomotion, Area: AreaFull, Match: pattern(model.PatternLocomotion)}
)

// Source: ACSM 2009 (multi-joint lifts first, each pattern covered), NSCA
// Essentials (push/pull/legs and upper/lower exercise order: large to small).
var slotOrder = map[string][]Slot{
	KindFull:    {slotSquat, slotHPush, slotHinge, slotHPull, slotVPush, slotVPull, slotCore, slotLunge},
	KindUpper:   {slotHPush, slotHPull, slotVPush, slotVPull, slotIsoPush, slotIsoPull, slotCore, slotRearDelt},
	KindLower:   {slotSquat, slotHinge, slotLunge, slotHamIso, slotQuadIso, slotCalf, slotCore, slotCarry},
	KindPush:    {slotHPush, slotVPush, slotIsoPush, slotTriceps, slotCore, slotHPush, slotVPush, slotTriceps},
	KindPull:    {slotHinge, slotVPull, slotHPull, slotRearDelt, slotBiceps, slotCore, slotHPull, slotBiceps},
	KindLegs:    {slotSquat, slotHinge, slotLunge, slotHamIso, slotQuadIso, slotCalf, slotCore, slotCarry},
	KindCircuit: {slotSquat, slotHPush, slotHPull, slotLunge, slotCarry, slotCore, slotLocomote, slotVPush},
	KindCardio:  {slotCardio, slotCore, slotCardio, slotCarry, slotLunge, slotCore},
}

// Source: ACSM 2009 and NSCA Essentials time budgets: about 8–10 minutes per
// strength exercise including rest, less for isolation work; beginners
// progress with fewer exercises (Garber 2011: 8–10 exercises across a week).
type sessionBudget struct {
	Slots, BeginnerCap int
}

func budgetFor(minutes int) sessionBudget {
	switch {
	case minutes < 30:
		return sessionBudget{3, 3}
	case minutes < 45:
		return sessionBudget{4, 3}
	case minutes < 60:
		return sessionBudget{5, 4}
	case minutes < 75:
		return sessionBudget{6, 5}
	case minutes < 90:
		return sessionBudget{7, 5}
	default:
		return sessionBudget{8, 5}
	}
}

// Targets are the reps, sets, and rest for the main compound work and for
// accessory or isolation work, per goal and experience.
type Targets struct {
	MainReps, MainSets, MainRest int
	AccReps, AccSets, AccRest    int
}

// Source: ACSM 2009 (novice 8–12 reps at 60–70% 1RM, advanced 1–6 reps at
// 80–100%, 2–3 min rest on core lifts); Schoenfeld 2017 load meta-analysis
// (strength favors heavier loads; hypertrophy similar across loads to near
// failure); Schoenfeld 2016 and Grgic 2018 (longer rest on compounds);
// Garber 2011 and WHO 2020 (general health 8–12 reps, 1–3 sets); Donnelly
// 2009, Willis 2012, Clark 2015 (resistance leads for body composition);
// Milanović 2015 (intervals for conditioning).
var targets = map[string]map[string]Targets{
	model.GoalStrength: {
		"beginner":     {6, 3, 150, 8, 3, 90},
		"intermediate": {5, 4, 180, 8, 3, 90},
		"advanced":     {3, 5, 240, 6, 3, 120},
	},
	model.GoalHypertrophy: {
		"beginner":     {10, 3, 90, 12, 3, 60},
		"intermediate": {8, 4, 120, 15, 3, 75},
		"advanced":     {6, 4, 150, 20, 3, 90},
	},
	model.GoalPower: {
		"beginner":     {6, 3, 150, 8, 3, 90},
		"intermediate": {3, 4, 180, 6, 3, 120},
		"advanced":     {3, 5, 240, 5, 3, 150},
	},
	model.GoalConditioning: {
		"beginner":     {12, 3, 45, 12, 3, 45},
		"intermediate": {15, 3, 45, 15, 3, 45},
		"advanced":     {15, 4, 30, 15, 4, 30},
	},
	model.GoalGeneralHealth: {
		"beginner":     {10, 2, 90, 10, 2, 90},
		"intermediate": {10, 3, 90, 12, 3, 75},
		"advanced":     {10, 3, 75, 12, 3, 60},
	},
	model.GoalBodyComposition: {
		"beginner":     {12, 3, 75, 12, 3, 60},
		"intermediate": {10, 3, 75, 12, 3, 60},
		"advanced":     {10, 4, 60, 15, 4, 60},
	},
}

// Source: Cormie 2011 and NSCA Essentials: explosive work first while fresh,
// 1–5 reps, 3–5 sets, 2–5 minutes rest, after a strength base is established.
var explosiveTargets = map[string]Targets{
	"intermediate": {3, 4, 180, 3, 4, 180},
	"advanced":     {2, 5, 240, 2, 5, 240},
}

// Secondary strength: one heavier compound per session (ACSM 2009 loading).
var secondaryStrength = Targets{5, 4, 180, 5, 4, 180}

// Source: Garber 2011 and WHO 2020 (150 minutes of moderate work a week in
// 15–30 minute bouts), Milanović 2015 (intervals of 1–4 minutes improve VO2max
// at least as much as continuous work), Wilson 2012 and Schumann 2022 (keep
// cardio short and after lifting when strength or size is the primary goal).
type cardioDose struct {
	Sets, DurationSeconds, RestSeconds int
}

var (
	conditioningSteady    = cardioDose{1, 1200, 60}
	conditioningSecond    = cardioDose{1, 600, 60}
	conditioningIntervals = cardioDose{4, 240, 120}
	healthFinisher        = cardioDose{1, 900, 60}
	bodyCompIntervals     = cardioDose{4, 180, 60}
	secondaryFinisher     = cardioDose{1, 600, 60}
	circuitFinisher       = cardioDose{1, 600, 60}
	maxIntervalDays       = 2
	maxFinisherDays       = 2
	// A finisher costs a strength slot in short sessions (under 45 minutes).
	finisherCostsSlotBelow = 45
)

func focusAreaOf(muscleGroup string) string {
	switch muscleGroup {
	case "chest", "back", "shoulders", "arms":
		return AreaUpper
	case "quads", "hamstrings", "glutes", "calves":
		return AreaLower
	case "core":
		return AreaCore
	default:
		return AreaFull
	}
}
