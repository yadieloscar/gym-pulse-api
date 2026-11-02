package engine

import (
	"testing"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// SC-001: every goal × day count × experience × equipment set builds a valid
// draft with no constraints.
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
					if len(d.Workouts) != days {
						t.Fatalf("%s %d days: %d workouts", goal, days, len(d.Workouts))
					}
					for _, w := range d.Workouts {
						if len(w.Exercises) < minExercises || len(w.Exercises) > maxExercises {
							t.Fatalf("%s %d days %s %v: %s has %d exercises", goal, days, exp, equipment, w.Name, len(w.Exercises))
						}
						seen := map[string]bool{}
						for _, e := range w.Exercises {
							if seen[e.Entry.ID.String()] {
								t.Fatalf("duplicate %s", e.Entry.Name)
							}
							seen[e.Entry.ID.String()] = true
							if !e.Entry.EligibleFor(equipment) {
								t.Fatalf("%s not eligible for %v", e.Entry.Name, equipment)
							}
							if e.Entry.Modality == "strength" && (e.Reps < 1 || e.Reps > 50 || e.DurationSeconds != 0) {
								t.Fatalf("bad strength targets %+v", e)
							}
							if e.Entry.Modality == "cardio" && (e.DurationSeconds < 30 || e.DurationSeconds > 3600 || e.Reps != 0) {
								t.Fatalf("bad cardio targets %+v", e)
							}
							if e.Sets < 1 || e.Sets > 10 || e.RestSeconds < 0 || e.RestSeconds > 600 {
								t.Fatalf("bad sets or rest %+v", e)
							}
						}
					}
					built++
				}
			}
		}
	}
	if built != 6*6*3*8 {
		t.Fatalf("built %d drafts", built)
	}
}
