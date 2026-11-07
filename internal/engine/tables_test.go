package engine

import (
	"os"
	"strings"
	"testing"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// Every table carries its source (spec 016 FR-006).
func TestTablesNameTheirSources(t *testing.T) {
	src, err := os.ReadFile("tables.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range []string{"var defaultSplit", "var slotOrder", "func budgetFor", "var targets", "var explosiveTargets", "var weeklySetBands", "var (\n\tconditioningSteady", "const progressionLine"} {
		idx := strings.Index(string(src), decl)
		if idx < 0 {
			t.Fatalf("table %q missing", decl)
		}
		before := string(src[max(0, idx-1200):idx])
		if !strings.Contains(before, "// Source:") {
			t.Errorf("table %q has no // Source: comment above it", decl)
		}
	}
}

func TestSplitsPerDayCount(t *testing.T) {
	for days := 1; days <= 6; days++ {
		for _, goal := range model.ValidTrainingGoals {
			if kinds := splitFor(goal, days); len(kinds) != days {
				t.Fatalf("%s %d days: %d kinds", goal, days, len(kinds))
			}
		}
	}
	if kinds := splitFor(model.GoalHypertrophy, 6); kinds[0] != KindPush || kinds[2] != KindLegs || kinds[5] != KindLegs {
		t.Fatalf("hypertrophy 6 days must be push/pull/legs twice: %v", kinds)
	}
	if kinds := splitFor(model.GoalStrength, 4); kinds[0] != KindUpper || kinds[1] != KindLower {
		t.Fatalf("4 days must be upper/lower: %v", kinds)
	}
	if kinds := splitFor(model.GoalConditioning, 3); kinds[0] != KindCardio || kinds[1] != KindCircuit || kinds[2] != KindCardio {
		t.Fatalf("conditioning alternates cardio and circuit: %v", kinds)
	}
}

func TestSessionBudgets(t *testing.T) {
	for _, tc := range []struct {
		minutes, slots, cap int
		minimal             bool
	}{{15, 3, 3, true}, {20, 3, 3, false}, {30, 4, 3, false}, {45, 5, 4, false}, {60, 6, 5, false}, {75, 7, 5, false}, {90, 8, 5, false}, {120, 8, 5, false}} {
		if got := budgetFor(tc.minutes); got.Slots != tc.slots || got.BeginnerCap != tc.cap || got.Minimal != tc.minimal {
			t.Errorf("%d min: got %+v want slots %d cap %d minimal %v", tc.minutes, got, tc.slots, tc.cap, tc.minimal)
		}
	}
}

// Every goal × experience row, heavy and moderate (research R3 and the
// 2026-11-07 evidence amendment).
func TestTargetsPerGoalAndExperience(t *testing.T) {
	want := map[string]map[string]goalTargets{
		model.GoalStrength: {
			"beginner":     {Targets{6, 8, 3, 150, 8, 10, 3, 90, 3}, Targets{6, 8, 3, 150, 8, 10, 3, 90, 3}},
			"intermediate": {Targets{5, 5, 4, 180, 8, 10, 3, 90, 2}, Targets{8, 8, 3, 120, 8, 10, 3, 90, 2}},
			"advanced":     {Targets{3, 5, 5, 240, 6, 8, 3, 120, 2}, Targets{6, 8, 4, 150, 6, 8, 3, 120, 2}},
		},
		model.GoalHypertrophy: {
			"beginner":     {Targets{10, 12, 3, 90, 12, 15, 3, 60, 2}, Targets{10, 12, 3, 90, 12, 15, 3, 60, 2}},
			"intermediate": {Targets{6, 8, 4, 150, 12, 15, 3, 75, 1}, Targets{10, 12, 3, 90, 12, 20, 3, 75, 1}},
			"advanced":     {Targets{6, 8, 4, 150, 12, 20, 3, 90, 1}, Targets{10, 12, 4, 90, 12, 20, 3, 90, 1}},
		},
		model.GoalPower: {
			"beginner":     {Targets{6, 8, 3, 150, 8, 10, 3, 90, 3}, Targets{6, 8, 3, 150, 8, 10, 3, 90, 3}},
			"intermediate": {Targets{3, 5, 4, 180, 6, 8, 3, 120, 2}, Targets{6, 8, 3, 150, 6, 8, 3, 120, 2}},
			"advanced":     {Targets{3, 5, 5, 240, 5, 8, 3, 150, 2}, Targets{5, 6, 4, 180, 5, 8, 3, 150, 2}},
		},
		model.GoalConditioning: {
			"beginner":     {Targets{12, 15, 3, 45, 12, 15, 3, 45, 2}, Targets{12, 15, 3, 45, 12, 15, 3, 45, 2}},
			"intermediate": {Targets{15, 20, 3, 45, 15, 20, 3, 45, 2}, Targets{15, 20, 3, 45, 15, 20, 3, 45, 2}},
			"advanced":     {Targets{15, 20, 4, 30, 15, 20, 4, 30, 1}, Targets{15, 20, 4, 30, 15, 20, 4, 30, 1}},
		},
		model.GoalGeneralHealth: {
			"beginner":     {Targets{10, 12, 2, 90, 10, 12, 2, 90, 3}, Targets{10, 12, 2, 90, 10, 12, 2, 90, 3}},
			"intermediate": {Targets{8, 12, 3, 90, 12, 15, 3, 75, 2}, Targets{8, 12, 3, 90, 12, 15, 3, 75, 2}},
			"advanced":     {Targets{8, 12, 3, 75, 12, 15, 3, 60, 2}, Targets{8, 12, 3, 75, 12, 15, 3, 60, 2}},
		},
		model.GoalBodyComposition: {
			"beginner":     {Targets{12, 15, 3, 75, 12, 15, 3, 60, 2}, Targets{12, 15, 3, 75, 12, 15, 3, 60, 2}},
			"intermediate": {Targets{10, 15, 3, 75, 12, 15, 3, 60, 1}, Targets{10, 15, 3, 75, 12, 15, 3, 60, 1}},
			"advanced":     {Targets{10, 15, 4, 60, 12, 20, 4, 60, 1}, Targets{10, 15, 4, 60, 12, 20, 4, 60, 1}},
		},
	}
	for goal, byExp := range want {
		for exp, w := range byExp {
			if got := targets[goal][exp]; got != w {
				t.Errorf("%s/%s: got %+v want %+v", goal, exp, got, w)
			}
			// The range never runs backwards and the lower bound is what the draft carries.
			for _, tg := range []Targets{w.Heavy, w.Moderate} {
				if tg.MainRepsMax < tg.MainReps || tg.AccRepsMax < tg.AccReps || tg.RIR < 1 || tg.RIR > 3 {
					t.Errorf("%s/%s targets malformed: %+v", goal, exp, tg)
				}
			}
		}
	}
	if explosiveTargets["intermediate"].MainReps != 3 || explosiveTargets["advanced"].MainSets != 5 || explosiveTargets["advanced"].MainRest != 240 {
		t.Fatalf("explosive targets: %+v", explosiveTargets)
	}
	if conditioningIntervals.Sets != 4 || conditioningIntervals.DurationSeconds != 240 || healthFinisher.DurationSeconds != 900 || secondaryFinisher.DurationSeconds != 600 || bodyCompIntervals.Sets != 4 || bodyCompIntervals.DurationSeconds != 180 {
		t.Fatal("cardio doses drifted from research R3")
	}
	for _, dose := range []cardioDose{conditioningSteady, conditioningSecond, conditioningIntervals, healthFinisher, bodyCompIntervals, secondaryFinisher, circuitFinisher} {
		if dose.Note == "" {
			t.Fatalf("every cardio dose names its intensity: %+v", dose)
		}
	}
	if secondaryStrength.MainReps != 5 || secondaryStrength.MainSets != 4 || secondaryStrength.MainRest != 180 {
		t.Fatalf("secondary strength: %+v", secondaryStrength)
	}
	want2 := map[string]setBand{model.GoalStrength: {6, 16}, model.GoalHypertrophy: {10, 20}, model.GoalPower: {6, 12}, model.GoalConditioning: {4, 12}, model.GoalGeneralHealth: {6, 12}, model.GoalBodyComposition: {8, 16}}
	for goal, band := range want2 {
		if weeklySetBands[goal] != band {
			t.Errorf("%s band: got %+v want %+v", goal, weeklySetBands[goal], band)
		}
	}
}

func TestSlotOrderCoversEveryKind(t *testing.T) {
	for _, kind := range []string{KindFull, KindUpper, KindLower, KindPush, KindPull, KindLegs, KindCardio, KindCircuit} {
		if len(slotOrder[kind]) < maxExercises-2 {
			t.Errorf("%s has only %d slots", kind, len(slotOrder[kind]))
		}
	}
	if slotOrder[KindFull][0].Pattern != model.PatternSquat || slotOrder[KindUpper][0].Pattern != model.PatternHorizontalPush || slotOrder[KindPull][0].Pattern != model.PatternHinge {
		t.Fatal("compound slots must come first")
	}
	// Isolation slots select by sub_pattern, never by name (FR-019).
	triceps, biceps := model.SubPatternTriceps, model.SubPatternBiceps
	if !slotTriceps.Match(model.CatalogExercise{Pattern: "isolation", SubPattern: &triceps}) || slotTriceps.Match(model.CatalogExercise{Pattern: "isolation", SubPattern: &biceps, Name: "Triceps Something"}) {
		t.Fatal("triceps slot must match the sub_pattern, not the name")
	}
	if slotBiceps.Match(model.CatalogExercise{Pattern: "isolation", MuscleGroup: "arms", Category: "pull"}) {
		t.Fatal("an unseeded sub_pattern must not match by category")
	}
}
