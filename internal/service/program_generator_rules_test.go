package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/engine"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// rulesCatalog is a small catalog with programming metadata that covers every
// pattern the engine fills for a barbell-and-dumbbells athlete.
func rulesCatalog() []model.CatalogExercise {
	compound, isolation := "compound", "isolation"
	var out []model.CatalogExercise
	add := func(name, category, modality string, mechanic *string, sort int, pattern, group, equipment, impact string) {
		out = append(out, model.CatalogExercise{ID: uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", len(out)+1)), Name: name, Category: category, Modality: modality,
			Mechanic: mechanic, SortOrder: sort, Pattern: pattern, MuscleGroup: group, EquipmentRequired: []string{equipment}, Impact: impact})
	}
	add("Back Squat", "legs", "strength", &compound, 1, "squat", "quads", "barbell", "low")
	add("Goblet Squat", "legs", "strength", &compound, 3, "squat", "quads", "dumbbells", "low")
	add("Deadlift", "pull", "strength", &compound, 1, "hinge", "hamstrings", "barbell", "low")
	add("Romanian Deadlift", "legs", "strength", &compound, 4, "hinge", "hamstrings", "barbell", "low")
	add("Walking Lunge", "legs", "strength", &compound, 6, "lunge", "quads", "bodyweight", "low")
	add("Barbell Bench Press", "push", "strength", &compound, 1, "horizontal_push", "chest", "barbell", "low")
	add("Push-Up", "push", "strength", &compound, 9, "horizontal_push", "chest", "bodyweight", "low")
	add("Overhead Press", "push", "strength", &compound, 5, "vertical_push", "shoulders", "barbell", "low")
	add("Barbell Row", "pull", "strength", &compound, 5, "horizontal_pull", "back", "barbell", "low")
	add("Dumbbell Row", "pull", "strength", &compound, 6, "horizontal_pull", "back", "dumbbells", "low")
	add("Lat Pulldown", "pull", "strength", &compound, 4, "vertical_pull", "back", "machines", "low")
	add("Lateral Raise", "push", "strength", &isolation, 12, "isolation", "shoulders", "dumbbells", "low")
	add("Dumbbell Curl", "pull", "strength", &isolation, 12, "isolation", "arms", "dumbbells", "low")
	add("Plank", "core", "strength", &isolation, 1, "core", "core", "bodyweight", "low")
	add("Dead Bug", "core", "strength", &isolation, 7, "core", "core", "bodyweight", "low")
	add("Kettlebell Swing", "full", "strength", &compound, 3, "explosive", "glutes", "dumbbells", "low")
	add("Farmer's Carry", "full", "strength", &compound, 6, "carry", "full_body", "dumbbells", "low")
	add("Outdoor Run", "cardio", "cardio", nil, 3, "locomotion", "full_body", "bodyweight", "high")
	add("Jump Rope", "cardio", "cardio", nil, 9, "locomotion", "full_body", "bodyweight", "high")
	return out
}

func rulesInput() GenerationInput {
	request := "ignored by the engine"
	return GenerationInput{PrimaryGoal: "strength", AvailableDays: []int{1, 3, 5}, UsualActivity: "moderate", Experience: "intermediate",
		Equipment: []string{"barbell", "dumbbells"}, SessionDurationMinutes: 60, WorkoutCap: 3, Catalog: rulesCatalog(), Request: &request, Model: "claude-opus-5-5"}
}

func TestRulesGeneratorMapsInputAndIgnoresFreeText(t *testing.T) {
	gen := NewRulesDraftGenerator()
	raw, usage, err := gen.Generate(context.Background(), rulesInput())
	if err != nil || usage != (GenerationUsage{}) {
		t.Fatalf("generate: %v usage %+v", err, usage)
	}
	if raw.Name != "3-day strength plan" || len(raw.Workouts) != 3 || raw.Rationale == "" {
		t.Fatalf("raw draft: %+v", raw)
	}
	for _, w := range raw.Workouts {
		if len(w.Exercises) < 3 {
			t.Fatalf("thin workout %+v", w)
		}
		for _, e := range w.Exercises {
			if (e.TargetReps == nil) == (e.TargetDurationSeconds == nil) || e.RestSeconds == nil || e.TargetSets < 1 {
				t.Fatalf("exercise targets: %+v", e)
			}
		}
	}
	// The engine's draft passes the spec 015 validator without any repair.
	profile := genProfile()
	profile.PrimaryGoal, profile.Equipment = "strength", []string{"barbell", "dumbbells"}
	repaired, err := model.RepairGeneratedDraft(profile, rulesCatalog(), raw, RulesModelID, nil)
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	for i, w := range repaired.Draft.Workouts {
		if len(w.Exercises) != len(raw.Workouts[i].Exercises) {
			t.Fatalf("repair changed workout %d", i)
		}
	}

	// Constraints reach the engine; the too-constrained error passes through.
	in := rulesInput()
	in.Avoid = []string{"hinge"}
	in.Focus = "upper"
	in.Variant = 1
	raw, _, err = gen.Generate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range raw.Workouts {
		for _, e := range w.Exercises {
			if e.CatalogID == "00000000-0000-0000-0000-000000000003" || e.CatalogID == "00000000-0000-0000-0000-000000000004" {
				t.Fatal("avoided hinge appeared")
			}
		}
	}
	in = rulesInput()
	in.Equipment = []string{"bodyweight"}
	in.SessionDurationMinutes = 20
	in.Avoid = []string{"horizontal_push", "lunge", "isolation", "core", "locomotion", "squat"}
	if _, _, err := gen.Generate(context.Background(), in); !errors.Is(err, engine.ErrTooConstrained) {
		t.Fatalf("want ErrTooConstrained, got %v", err)
	}
}

// A committed draft pins determinism across runs and machines (spec 016 SC-003).
func TestRulesGeneratorMatchesTheGoldenDraft(t *testing.T) {
	raw, _, err := NewRulesDraftGenerator().Generate(context.Background(), rulesInput())
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "rules_draft.golden.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(golden, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("golden file missing (run with UPDATE_GOLDEN=1): %v", err)
	}
	if string(want) != string(got)+"\n" {
		t.Fatalf("draft drifted from the golden file; review and re-run with UPDATE_GOLDEN=1\n%s", got)
	}
}
