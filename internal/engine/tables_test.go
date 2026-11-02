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
	for _, decl := range []string{"var defaultSplit", "var slotOrder", "func budgetFor", "var targets", "var explosiveTargets", "var (\n\tconditioningSteady"} {
		idx := strings.Index(string(src), decl)
		if idx < 0 {
			t.Fatalf("table %q missing", decl)
		}
		before := string(src[max(0, idx-900):idx])
		if !strings.Contains(before, "// Source:") {
			t.Errorf("table %q has no // Source: comment above it", decl)
		}
	}
}

func TestSplitsPerDayCount(t *testing.T) {
	for days := 1; days <= 6; days++ {
		for _, goal := range model.ValidTrainingGoals {
			kinds := splitFor(goal, days)
			if len(kinds) != days {
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
	if kinds := splitFor(model.GoalPower, 3); kinds[0] != KindFull {
		t.Fatalf("power uses the default split: %v", kinds)
	}
}

func TestSessionBudgets(t *testing.T) {
	for _, tc := range []struct{ minutes, slots, cap int }{{20, 3, 3}, {30, 4, 3}, {45, 5, 4}, {60, 6, 5}, {75, 7, 5}, {90, 8, 5}, {120, 8, 5}} {
		if got := budgetFor(tc.minutes); got.Slots != tc.slots || got.BeginnerCap != tc.cap {
			t.Errorf("%d min: got %+v want slots %d cap %d", tc.minutes, got, tc.slots, tc.cap)
		}
	}
}

func TestTargetsPerGoalAndExperience(t *testing.T) {
	want := map[string]map[string]Targets{
		model.GoalStrength:        {"beginner": {6, 3, 150, 8, 3, 90}, "intermediate": {5, 4, 180, 8, 3, 90}, "advanced": {3, 5, 240, 6, 3, 120}},
		model.GoalHypertrophy:     {"beginner": {10, 3, 90, 12, 3, 60}, "intermediate": {8, 4, 120, 15, 3, 75}, "advanced": {6, 4, 150, 20, 3, 90}},
		model.GoalPower:           {"beginner": {6, 3, 150, 8, 3, 90}, "intermediate": {3, 4, 180, 6, 3, 120}, "advanced": {3, 5, 240, 5, 3, 150}},
		model.GoalConditioning:    {"beginner": {12, 3, 45, 12, 3, 45}, "intermediate": {15, 3, 45, 15, 3, 45}, "advanced": {15, 4, 30, 15, 4, 30}},
		model.GoalGeneralHealth:   {"beginner": {10, 2, 90, 10, 2, 90}, "intermediate": {10, 3, 90, 12, 3, 75}, "advanced": {10, 3, 75, 12, 3, 60}},
		model.GoalBodyComposition: {"beginner": {12, 3, 75, 12, 3, 60}, "intermediate": {10, 3, 75, 12, 3, 60}, "advanced": {10, 4, 60, 15, 4, 60}},
	}
	for goal, byExp := range want {
		for exp, w := range byExp {
			if got := targets[goal][exp]; got != w {
				t.Errorf("%s/%s: got %+v want %+v", goal, exp, got, w)
			}
		}
	}
	if explosiveTargets["intermediate"].MainReps != 3 || explosiveTargets["advanced"].MainSets != 5 || explosiveTargets["advanced"].MainRest != 240 {
		t.Fatalf("explosive targets: %+v", explosiveTargets)
	}
	if conditioningIntervals != (cardioDose{4, 240, 120}) || healthFinisher != (cardioDose{1, 900, 60}) || secondaryFinisher != (cardioDose{1, 600, 60}) || bodyCompIntervals != (cardioDose{4, 180, 60}) {
		t.Fatal("cardio doses drifted from research R3")
	}
	if secondaryStrength.MainReps != 5 || secondaryStrength.MainSets != 4 || secondaryStrength.MainRest != 180 {
		t.Fatalf("secondary strength: %+v", secondaryStrength)
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
}
