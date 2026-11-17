package model

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
