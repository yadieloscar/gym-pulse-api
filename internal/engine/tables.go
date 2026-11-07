// Package engine builds a program draft from the athlete's answers with no
// provider, database, or clock. The programming knowledge lives in this file
// as reviewable data; every table names its source in
// specs/015-ai-program-generation/research.md ("Goal guide"),
// specs/016-rule-based-program-engine/research.md (R3 and the evidence
// amendment), and the workspace brief docs/briefs/2026-11-07-training-regimes-research.md.
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
// 2016 frequency meta-analysis, and the 2019/2024 volume-equated reviews (the
// split is a scheduling tool once weekly sets are matched). Full-body for 1–3
// days, upper/lower for 4, push/pull/legs from 5 for hypertrophy, cardio-led
// alternation for conditioning.
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

// subPattern selects by the catalog's sub_pattern (migration 034), never by name.
func subPattern(values ...string) func(model.CatalogExercise) bool {
	return func(e model.CatalogExercise) bool {
		return e.SubPattern != nil && slices.Contains(values, *e.SubPattern)
	}
}

func isolationOf(groups ...string) func(model.CatalogExercise) bool {
	return func(e model.CatalogExercise) bool {
		return e.Pattern == model.PatternIsolation && slices.Contains(groups, e.MuscleGroup)
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
	slotIsoPush   = Slot{Name: "chest or shoulder isolation", Pattern: model.PatternIsolation, Area: AreaUpper, Match: subPattern(model.SubPatternChestIsolation, model.SubPatternShoulderIsolation)}
	slotIsoPull   = Slot{Name: "back or arm isolation", Pattern: model.PatternIsolation, Area: AreaUpper, Match: subPattern(model.SubPatternBiceps, model.SubPatternRearDelt)}
	slotTriceps   = Slot{Name: "triceps", Pattern: model.PatternIsolation, Area: AreaUpper, Match: subPattern(model.SubPatternTriceps)}
	slotBiceps    = Slot{Name: "biceps", Pattern: model.PatternIsolation, Area: AreaUpper, Match: subPattern(model.SubPatternBiceps)}
	slotRearDelt  = Slot{Name: "rear delts", Pattern: model.PatternIsolation, Area: AreaUpper, Match: subPattern(model.SubPatternRearDelt)}
	slotHamIso    = Slot{Name: "hamstring isolation", Pattern: model.PatternIsolation, Area: AreaLower, Match: subPattern(model.SubPatternHamstringIsolation)}
	slotQuadIso   = Slot{Name: "quad isolation", Pattern: model.PatternIsolation, Area: AreaLower, Match: subPattern(model.SubPatternQuadIsolation)}
	slotCalf      = Slot{Name: "calves", Pattern: model.PatternIsolation, Area: AreaLower, Match: subPattern(model.SubPatternCalf)}
	slotIsoLower  = Slot{Name: "lower isolation", Pattern: model.PatternIsolation, Area: AreaLower, Match: isolationOf("quads", "hamstrings", "glutes", "calves")}
	slotCardio    = Slot{Name: "cardio", Pattern: model.PatternLocomotion, Area: AreaFull, Cardio: true, Match: cardioEntry}
	slotLocomote  = Slot{Name: "conditioning", Pattern: model.PatternLocomotion, Area: AreaFull, Match: pattern(model.PatternLocomotion)}
)

// Source: ACSM 2009 (multi-joint lifts first, each pattern covered), NSCA
// Essentials (push/pull/legs and upper/lower exercise order: large to small);
// Nunes 2021 (order matters for strength on the first lift, not for size).
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

// Slots that keep a focus area's share when avoided patterns thin it, and the
// isolation slot that may open a hypertrophy session with a focus (Nunes 2021).
var extraFocusSlots = map[string][]Slot{
	AreaUpper: {slotIsoPush, slotIsoPull, slotRearDelt, slotTriceps, slotBiceps, slotHPush, slotHPull, slotVPull},
	AreaLower: {slotLunge, slotHamIso, slotQuadIso, slotCalf, slotSquat, slotHinge, slotLunge},
	AreaCore:  {slotCore, slotCarry, slotCore},
}

var focusOpener = map[string]Slot{AreaUpper: slotIsoPush, AreaLower: slotIsoLower, AreaCore: slotCore}

// Source: ACSM 2009 and NSCA Essentials time budgets (about 8–10 minutes per
// strength exercise including rest, less for isolation); the minimum-dose
// reviews (one to two hard sets per lift still move 1RM) for sessions under
// 20 minutes; Garber 2011 for beginners (fewer exercises).
type sessionBudget struct {
	Slots, BeginnerCap int
	// Minimal sessions keep three compounds at two sets each.
	Minimal bool
}

const minimalSets = 2

func budgetFor(minutes int) sessionBudget {
	switch {
	case minutes < 20:
		return sessionBudget{3, 3, true}
	case minutes < 30:
		return sessionBudget{3, 3, false}
	case minutes < 45:
		return sessionBudget{4, 3, false}
	case minutes < 60:
		return sessionBudget{5, 4, false}
	case minutes < 75:
		return sessionBudget{6, 5, false}
	case minutes < 90:
		return sessionBudget{7, 5, false}
	default:
		return sessionBudget{8, 5, false}
	}
}

// Targets are the rep range, sets, rest, and effort for the main compound
// work and for accessory or isolation work. Reps is the lower bound the draft
// carries as target_reps; RepsMax is the top of the range named in notes; RIR
// is the reps-in-reserve cue.
type Targets struct {
	MainReps, MainRepsMax, MainSets, MainRest int
	AccReps, AccRepsMax, AccSets, AccRest     int
	RIR                                       int
}

// Source: ACSM 2009 (novice 8–12 reps at 60–70% 1RM, advanced 1–6 reps at
// 80–100%, 2–3 min rest on core lifts); Schoenfeld 2017 load meta-analysis
// (strength favors heavier loads; hypertrophy similar from 5 to 30 reps near
// failure); Robinson 2024 and Refalo 2023/2025 (1–2 reps in reserve for size,
// strength less sensitive to effort); Schoenfeld 2016, Grgic 2018, and the
// 2024 rest network meta-analysis (longer rest on compounds); Garber 2011 and
// WHO 2020 (general health 8–12 reps, 1–3 sets); Donnelly 2009, Willis 2012,
// Clark 2015 (resistance leads for body composition); Milanović 2015 (intervals).
// "heavy" and "moderate" alternate across the week (2022 volume-equated
// periodization meta-analysis: undulating gives a small strength edge).
type goalTargets struct {
	Heavy, Moderate Targets
}

var targets = map[string]map[string]goalTargets{
	model.GoalStrength: {
		"beginner":     {Heavy: Targets{6, 8, 3, 150, 8, 10, 3, 90, 3}, Moderate: Targets{6, 8, 3, 150, 8, 10, 3, 90, 3}},
		"intermediate": {Heavy: Targets{5, 5, 4, 180, 8, 10, 3, 90, 2}, Moderate: Targets{8, 8, 3, 120, 8, 10, 3, 90, 2}},
		"advanced":     {Heavy: Targets{3, 5, 5, 240, 6, 8, 3, 120, 2}, Moderate: Targets{6, 8, 4, 150, 6, 8, 3, 120, 2}},
	},
	model.GoalHypertrophy: {
		"beginner":     {Heavy: Targets{10, 12, 3, 90, 12, 15, 3, 60, 2}, Moderate: Targets{10, 12, 3, 90, 12, 15, 3, 60, 2}},
		"intermediate": {Heavy: Targets{6, 8, 4, 150, 12, 15, 3, 75, 1}, Moderate: Targets{10, 12, 3, 90, 12, 20, 3, 75, 1}},
		"advanced":     {Heavy: Targets{6, 8, 4, 150, 12, 20, 3, 90, 1}, Moderate: Targets{10, 12, 4, 90, 12, 20, 3, 90, 1}},
	},
	model.GoalPower: {
		"beginner":     {Heavy: Targets{6, 8, 3, 150, 8, 10, 3, 90, 3}, Moderate: Targets{6, 8, 3, 150, 8, 10, 3, 90, 3}},
		"intermediate": {Heavy: Targets{3, 5, 4, 180, 6, 8, 3, 120, 2}, Moderate: Targets{6, 8, 3, 150, 6, 8, 3, 120, 2}},
		"advanced":     {Heavy: Targets{3, 5, 5, 240, 5, 8, 3, 150, 2}, Moderate: Targets{5, 6, 4, 180, 5, 8, 3, 150, 2}},
	},
	model.GoalConditioning: {
		"beginner":     {Heavy: Targets{12, 15, 3, 45, 12, 15, 3, 45, 2}, Moderate: Targets{12, 15, 3, 45, 12, 15, 3, 45, 2}},
		"intermediate": {Heavy: Targets{15, 20, 3, 45, 15, 20, 3, 45, 2}, Moderate: Targets{15, 20, 3, 45, 15, 20, 3, 45, 2}},
		"advanced":     {Heavy: Targets{15, 20, 4, 30, 15, 20, 4, 30, 1}, Moderate: Targets{15, 20, 4, 30, 15, 20, 4, 30, 1}},
	},
	model.GoalGeneralHealth: {
		"beginner":     {Heavy: Targets{10, 12, 2, 90, 10, 12, 2, 90, 3}, Moderate: Targets{10, 12, 2, 90, 10, 12, 2, 90, 3}},
		"intermediate": {Heavy: Targets{8, 12, 3, 90, 12, 15, 3, 75, 2}, Moderate: Targets{8, 12, 3, 90, 12, 15, 3, 75, 2}},
		"advanced":     {Heavy: Targets{8, 12, 3, 75, 12, 15, 3, 60, 2}, Moderate: Targets{8, 12, 3, 75, 12, 15, 3, 60, 2}},
	},
	model.GoalBodyComposition: {
		"beginner":     {Heavy: Targets{12, 15, 3, 75, 12, 15, 3, 60, 2}, Moderate: Targets{12, 15, 3, 75, 12, 15, 3, 60, 2}},
		"intermediate": {Heavy: Targets{10, 15, 3, 75, 12, 15, 3, 60, 1}, Moderate: Targets{10, 15, 3, 75, 12, 15, 3, 60, 1}},
		"advanced":     {Heavy: Targets{10, 15, 4, 60, 12, 20, 4, 60, 1}, Moderate: Targets{10, 15, 4, 60, 12, 20, 4, 60, 1}},
	},
}

// Source: Cormie 2011 and NSCA Essentials: explosive work first while fresh,
// 1–5 reps, 3–5 sets, 2–5 minutes rest, after a strength base is established.
var explosiveTargets = map[string]Targets{
	"intermediate": {3, 3, 4, 180, 3, 3, 4, 180, 3},
	"advanced":     {2, 3, 5, 240, 2, 3, 5, 240, 3},
}

// Secondary strength: one heavier compound per session (ACSM 2009 loading).
var secondaryStrength = Targets{5, 5, 4, 180, 5, 5, 4, 180, 2}

// Secondary hypertrophy: one isolation slot at a size-friendly range.
var secondaryHypertrophy = Targets{12, 15, 3, 60, 12, 15, 3, 60, 1}

// Source: Schoenfeld 2017 volume dose-response (about 10 sets per muscle per
// week as the clearest step), Pelland 2025/2026 (gains past 10 with diminishing
// returns; strength saturates sooner), minimum-dose reviews (4 weekly sets for
// size, 1–3 hard sets for strength). Direct sets per muscle group per week.
type setBand struct {
	Min, Max int
}

var weeklySetBands = map[string]setBand{
	model.GoalStrength:        {6, 16},
	model.GoalHypertrophy:     {10, 20},
	model.GoalPower:           {6, 12},
	model.GoalConditioning:    {4, 12},
	model.GoalGeneralHealth:   {6, 12},
	model.GoalBodyComposition: {8, 16},
}

// Source: Garber 2011 and WHO 2020 (150 minutes of moderate work a week in
// 15–30 minute bouts), Milanović 2015 (intervals of 1–4 minutes improve VO2max
// at least as much as continuous work), Wilson 2012 and Schumann 2022 (keep
// cardio short and after lifting when strength or size is the primary goal).
type cardioDose struct {
	Sets, DurationSeconds, RestSeconds int
	Note                               string
}

const (
	noteEasy     = "Easy pace, you can talk."
	noteModerate = "Easy to moderate pace."
)

var (
	conditioningSteady    = cardioDose{1, 1200, 60, noteEasy}
	conditioningSecond    = cardioDose{1, 600, 60, noteEasy}
	conditioningIntervals = cardioDose{4, 240, 120, "Hard efforts: 4 × 4 minutes with 2 easy minutes between."}
	healthFinisher        = cardioDose{1, 900, 60, noteModerate}
	bodyCompIntervals     = cardioDose{4, 180, 60, "Hard efforts: 4 × 3 minutes with 1 easy minute between."}
	secondaryFinisher     = cardioDose{1, 600, 60, noteModerate}
	circuitFinisher       = cardioDose{1, 600, 60, noteModerate}
	maxIntervalDays       = 2
	maxFinisherDays       = 2
	// A finisher costs a strength slot in short sessions (under 45 minutes).
	finisherCostsSlotBelow = 45
)

// Source: Plotkin 2022 (adding load or reps works equally when effort is high);
// the draft stays static (PRODUCT.md) and only says how to progress.
const progressionLine = "Add a rep each week until you reach the top of the range, then add a little load."

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
