package model

import (
	"testing"

	"github.com/google/uuid"
)

func TestMergeStartedSetsKeepsLoggedSetsAndTakesTheRestFromTheEdit(t *testing.T) {
	squat1, squat2, curl1, plank1 := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	current := []StartedSet{
		{ID: squat1, ExerciseName: "Squat", ExerciseOrder: 1, SetIndex: 1, Logged: true},
		{ID: squat2, ExerciseName: "Squat", ExerciseOrder: 1, SetIndex: 2},
		{ID: curl1, ExerciseName: "Curl", ExerciseOrder: 2, SetIndex: 1, Logged: true},
		{ID: plank1, ExerciseName: "Plank", ExerciseOrder: 3, SetIndex: 1},
	}
	// The edit puts a stretch first, gives Squat three sets, and drops Curl and Plank.
	set := func(name string, order, index int) ScheduledSet {
		return ScheduledSet{ExerciseName: name, ExerciseOrder: order, SetIndex: index}
	}
	snapshot := []ScheduledSet{set("Couch stretch", 1, 1), set("Couch stretch", 1, 2), set("Squat", 2, 1), set("Squat", 2, 2), set("Squat", 2, 3)}

	kept, added := MergeStartedSets(current, snapshot)

	wantKept := []StartedSetPlacement{{ID: squat1, ExerciseOrder: 2}, {ID: curl1, ExerciseOrder: 3}}
	if len(kept) != len(wantKept) || kept[0] != wantKept[0] || kept[1] != wantKept[1] {
		t.Fatalf("kept = %+v, want %+v", kept, wantKept)
	}
	var names []string
	for _, a := range added {
		names = append(names, a.ExerciseName+" "+string(rune('0'+a.SetIndex)))
	}
	want := []string{"Couch stretch 1", "Couch stretch 2", "Squat 2", "Squat 3"}
	if len(names) != len(want) {
		t.Fatalf("added = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("added = %v, want %v", names, want)
		}
	}
}

func TestMergeStartedSetsWithNothingLoggedIsTheEditedWorkout(t *testing.T) {
	current := []StartedSet{{ID: uuid.New(), ExerciseName: "Squat", ExerciseOrder: 1, SetIndex: 1}}
	snapshot := []ScheduledSet{{ExerciseName: "Lunge", ExerciseOrder: 1, SetIndex: 1}}
	kept, added := MergeStartedSets(current, snapshot)
	if len(kept) != 0 || len(added) != 1 || added[0].ExerciseName != "Lunge" {
		t.Fatalf("kept = %+v added = %+v", kept, added)
	}
}

func TestMergeStartedSetsPlacesRemovedExercisesAfterTheEditInTheirOrder(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	current := []StartedSet{
		{ID: b, ExerciseName: "Row", ExerciseOrder: 3, SetIndex: 1, Logged: true},
		{ID: a, ExerciseName: "Press", ExerciseOrder: 1, SetIndex: 1, Logged: true},
		{ID: c, ExerciseName: "Press", ExerciseOrder: 1, SetIndex: 2, Logged: true},
	}
	kept, added := MergeStartedSets(current, []ScheduledSet{{ExerciseName: "Squat", ExerciseOrder: 1, SetIndex: 1}})
	got := map[uuid.UUID]int{}
	for _, placement := range kept {
		got[placement.ID] = placement.ExerciseOrder
	}
	if got[a] != 2 || got[c] != 2 || got[b] != 3 || len(added) != 1 {
		t.Fatalf("placements = %v added = %+v", got, added)
	}
}

func TestMergeStartedSetsNeverPutsTwoSetsInOneSlot(t *testing.T) {
	// Two logged sets that would land on the same exercise and index: the
	// second stays as its own group instead of colliding.
	first, second := uuid.New(), uuid.New()
	current := []StartedSet{
		{ID: first, ExerciseName: "Press", ExerciseOrder: 1, SetIndex: 1, Logged: true},
		{ID: second, ExerciseName: "Press", ExerciseOrder: 2, SetIndex: 1, Logged: true},
	}
	kept, _ := MergeStartedSets(current, []ScheduledSet{{ExerciseName: "Press", ExerciseOrder: 1, SetIndex: 1}})
	if kept[0].ExerciseOrder == kept[1].ExerciseOrder {
		t.Fatalf("two logged sets share a slot: %+v", kept)
	}
}
