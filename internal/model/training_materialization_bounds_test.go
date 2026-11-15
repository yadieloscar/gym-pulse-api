package model

import (
	"math"
	"testing"
)

// Rows written before the write-side limit existed must not drive an
// unbounded allocation when a schedule is materialized.
func TestSnapshotScheduledSetsCapsTargetSets(t *testing.T) {
	sets := SnapshotScheduledSets([]ProgramExercise{{Name: "Back Squat", Modality: "strength", ExerciseOrder: 1, TargetSets: math.MaxInt32}})
	if len(sets) != MaxProgramTargetSets {
		t.Fatalf("len(SnapshotScheduledSets(target_sets=MaxInt32)) = %d, want %d", len(sets), MaxProgramTargetSets)
	}
}
