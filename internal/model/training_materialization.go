package model

import (
	"sort"

	"github.com/google/uuid"
)

// SnapshotScheduledSets returns the required sets a session snapshots from
// a unit's exercises: one set per target set, in exercise order.
func SnapshotScheduledSets(exercises []ProgramExercise) []ScheduledSet {
	sets := []ScheduledSet{}
	for _, exercise := range exercises {
		// Bounded here as well as on write, so a stored row can never drive
		// an unbounded allocation.
		for index := 1; index <= min(exercise.TargetSets, MaxProgramTargetSets); index++ {
			exerciseID := exercise.ID
			sets = append(sets, ScheduledSet{ProgramExerciseID: &exerciseID, CatalogID: exercise.CatalogID, ExerciseName: exercise.Name, ExerciseCategory: exercise.Category, ExerciseModality: exercise.Modality, ExerciseOrder: exercise.ExerciseOrder, SetIndex: index, TargetReps: exercise.TargetReps, TargetWeight: exercise.TargetWeight, TargetDurationSeconds: exercise.TargetDurationSeconds, RestSeconds: exercise.RestSeconds, Notes: exercise.Notes})
		}
	}
	return sets
}

// StartedSet is a required set of a session the athlete has started, as far
// as a program edit needs to know it.
type StartedSet struct {
	ID            uuid.UUID
	ExerciseName  string
	ExerciseOrder int
	SetIndex      int
	// Logged is true when a performed set points at this set, done or not.
	// Such a set is the athlete's record and is never removed or retargeted.
	Logged bool
}

// StartedSetPlacement says where a kept set sits after the merge.
type StartedSetPlacement struct {
	ID            uuid.UUID
	ExerciseOrder int
}

// MergeStartedSets folds an edited program workout into a session that is
// already under way. Sets the athlete logged stay, with their index, and move
// to their exercise's place in the edited workout; a logged set whose exercise
// the edit removed goes after the edited exercises, in its earlier order.
// Every other set is replaced by the edited workout's sets. It returns where
// each kept set goes and the sets to add; the caller deletes the unlogged ones.
func MergeStartedSets(current []StartedSet, snapshot []ScheduledSet) (kept []StartedSetPlacement, added []ScheduledSet) {
	// The edited workout's exercises by name; the first of a repeated name wins.
	orderByName := map[string]int{}
	last := 0
	for _, set := range snapshot {
		if _, seen := orderByName[set.ExerciseName]; !seen {
			orderByName[set.ExerciseName] = set.ExerciseOrder
		}
		last = max(last, set.ExerciseOrder)
	}

	// taken holds the (order, index) slots the kept sets occupy.
	type slot struct{ order, index int }
	taken := map[slot]bool{}
	// Exercises the edit removed keep their relative order after the rest.
	removedOrder := map[int]int{}
	logged := make([]StartedSet, 0, len(current))
	for _, set := range current {
		if set.Logged {
			logged = append(logged, set)
		}
	}
	sort.SliceStable(logged, func(i, j int) bool {
		if logged[i].ExerciseOrder != logged[j].ExerciseOrder {
			return logged[i].ExerciseOrder < logged[j].ExerciseOrder
		}
		return logged[i].SetIndex < logged[j].SetIndex
	})
	for _, set := range logged {
		order, ok := orderByName[set.ExerciseName]
		if ok && taken[slot{order, set.SetIndex}] {
			// Two logged sets cannot share a slot; the later one is treated as removed.
			ok = false
		}
		if !ok {
			if _, seen := removedOrder[set.ExerciseOrder]; !seen {
				last++
				removedOrder[set.ExerciseOrder] = last
			}
			order = removedOrder[set.ExerciseOrder]
		}
		taken[slot{order, set.SetIndex}] = true
		kept = append(kept, StartedSetPlacement{ID: set.ID, ExerciseOrder: order})
	}

	for _, set := range snapshot {
		if !taken[slot{set.ExerciseOrder, set.SetIndex}] {
			added = append(added, set)
		}
	}
	return kept, added
}
