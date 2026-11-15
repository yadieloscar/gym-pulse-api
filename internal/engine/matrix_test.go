package engine

import (
	"errors"
	"testing"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func assertValid(t *testing.T, label string, d Draft, equipment []string, days int, goal string, focus string) {
	t.Helper()
	if len(d.Workouts) != days {
		t.Fatalf("%s: %d workouts", label, len(d.Workouts))
	}
	for _, w := range d.Workouts {
		if len(w.Exercises) < minExercises || len(w.Exercises) > maxExercises {
			t.Fatalf("%s: %s has %d exercises", label, w.Name, len(w.Exercises))
		}
		seen := map[string]bool{}
		for _, e := range w.Exercises {
			if seen[e.Entry.ID.String()] {
				t.Fatalf("%s: duplicate %s", label, e.Entry.Name)
			}
			seen[e.Entry.ID.String()] = true
			if !e.Entry.EligibleFor(equipment) {
				t.Fatalf("%s: %s not eligible for %v", label, e.Entry.Name, equipment)
			}
			if e.Entry.Modality == "strength" && (e.Reps < 1 || e.Reps > 50 || e.DurationSeconds != 0) {
				t.Fatalf("%s: bad strength targets %+v", label, e)
			}
			if e.Entry.Modality == "cardio" && (e.DurationSeconds < 30 || e.DurationSeconds > 3600 || e.Reps != 0) {
				t.Fatalf("%s: bad cardio targets %+v", label, e)
			}
			if e.Sets < 1 || e.Sets > 10 || e.RestSeconds < 0 || e.RestSeconds > 600 || e.Note == "" || len([]rune(e.Note)) > 200 {
				t.Fatalf("%s: bad sets, rest, or note %+v", label, e)
			}
		}
	}
	if band, ok := weeklySetBands[goal]; ok && goal != model.GoalConditioning {
		for group, sets := range weeklySets(d) {
			limit := band.Max
			if focus != "" && focusAreaOf(group) == focus {
				limit = band.Max + band.Max/2
			}
			if sets > limit {
				t.Fatalf("%s: %s has %d weekly sets, above %d", label, group, sets, limit)
			}
		}
	}
}

// SC-001: every goal × day count × experience × equipment set builds a valid
// draft with no constraints, and every muscle group stays inside its band.
func TestExhaustiveMatrix(t *testing.T) {
	equipmentSets := [][]string{{"bodyweight"}, {"bands"}, {"dumbbells"}, {"barbell"}, {"machines"}, {"cardio_machine"}, {"full_gym"}, {"dumbbells", "cardio_machine"}}
	minutesByExperience := map[string]int{"beginner": 30, "intermediate": 60, "advanced": 90}
	built := 0
	for _, goal := range model.ValidTrainingGoals {
		for days := 1; days <= 6; days++ {
			available := make([]int, 0, days)
			for d := 1; d <= days; d++ {
				available = append(available, d)
			}
			for exp, minutes := range minutesByExperience {
				for _, equipment := range equipmentSets {
					in := input(goal, available, exp, equipment, minutes)
					d, _, err := Build(in)
					if err != nil {
						t.Fatalf("%s %d days %s %v: %v", goal, days, exp, equipment, err)
					}
					assertValid(t, goal+"/"+exp, d, equipment, days, goal, "")
					built++
				}
			}
		}
	}
	if built != 6*6*3*8 {
		t.Fatalf("built %d drafts", built)
	}
}

// FR-020: the matrix also runs over fixed avoid and focus combinations; a
// draft or the documented too-constrained error are the only outcomes.
func TestConstrainedMatrix(t *testing.T) {
	avoidSets := [][]string{
		{model.PatternHinge},
		{model.PatternVerticalPush, model.PatternExplosive},
		{model.PatternSquat, model.PatternLunge, model.PatternLocomotion},
		{model.PatternHorizontalPush, model.PatternHorizontalPull, model.PatternVerticalPull, model.PatternIsolation},
		{model.PatternCore, model.PatternCarry, model.PatternHinge, model.PatternSquat, model.PatternLunge, model.PatternExplosive},
	}
	equipmentSets := [][]string{{"bodyweight"}, {"dumbbells"}, {"full_gym"}}
	built, refused := 0, 0
	for _, goal := range model.ValidTrainingGoals {
		for _, days := range []int{1, 3, 5} {
			available := make([]int, 0, days)
			for d := 1; d <= days; d++ {
				available = append(available, d)
			}
			for _, avoid := range avoidSets {
				for _, focus := range []string{"", AreaUpper, AreaLower, AreaCore} {
					for _, equipment := range equipmentSets {
						for variant := range 2 {
							in := input(goal, available, "intermediate", equipment, 45)
							in.Avoid, in.Focus, in.Variant = avoid, focus, variant
							d, _, err := Build(in)
							if err != nil {
								if errors.Is(err, ErrTooConstrained) {
									refused++
									continue
								}
								t.Fatalf("%s %v %s %v: %v", goal, avoid, focus, equipment, err)
							}
							assertValid(t, goal+"/"+focus, d, equipment, days, goal, focus)
							for _, w := range d.Workouts {
								for _, e := range w.Exercises {
									for _, p := range avoid {
										if e.Entry.Pattern == p {
											t.Fatalf("%s: avoided %s appeared as %s", goal, p, e.Entry.Name)
										}
									}
								}
							}
							built++
						}
					}
				}
			}
		}
	}
	if built == 0 || refused > built/4 {
		t.Fatalf("built %d, refused %d", built, refused)
	}
}
