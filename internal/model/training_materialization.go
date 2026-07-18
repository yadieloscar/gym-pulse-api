package model

import (
	"sort"
	"time"
)

// MaterializeProgramForWeekdays maps successive program sequence items onto
// the user's exact ISO weekdays. It supports one-day schedules and arbitrary
// non-default weekdays without mutating the source program.
func MaterializeProgramForWeekdays(p *Program, weekdays []int, from, to string) ([]ScheduledWorkout, error) {
	start, err := ParseDate(from)
	if err != nil {
		return nil, validationError("from", "from must be YYYY-MM-DD")
	}
	end, err := ParseDate(to)
	if err != nil {
		return nil, validationError("to", "to must be YYYY-MM-DD")
	}
	allowed := make(map[int]bool, len(weekdays))
	for _, day := range weekdays {
		if day < 1 || day > 7 {
			return nil, validationError("available_days", "available_days must use ISO weekdays 1 through 7")
		}
		allowed[day] = true
	}
	if len(allowed) == 0 {
		return nil, validationError("available_days", "at least one available day is required")
	}
	workouts := append([]ProgramWorkout(nil), p.Workouts...)
	sort.SliceStable(workouts, func(i, j int) bool { return workouts[i].SequencePosition < workouts[j].SequencePosition })
	if len(workouts) == 0 {
		return []ScheduledWorkout{}, nil
	}
	result := []ScheduledWorkout{}
	occurrence := 0
	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		isoDay := int(date.Weekday())
		if isoDay == 0 {
			isoDay = 7
		}
		if !allowed[isoDay] {
			continue
		}
		pw := &workouts[occurrence%len(workouts)]
		occurrence++
		programID, workoutID, sequence := p.ID, pw.ID, pw.SequencePosition
		result = append(result, ScheduledWorkout{ProgramID: &programID, ProgramWorkoutID: &workoutID, Date: date.Format(time.DateOnly), Name: pw.Name, SequencePosition: &sequence, Status: WorkoutStatusPlanned, RequiredSets: SnapshotScheduledSets(pw.Exercises), ExtraSets: []PerformedSet{}})
	}
	return result, nil
}

func SnapshotScheduledSets(exercises []ProgramExercise) []ScheduledSet {
	sets := []ScheduledSet{}
	for _, exercise := range exercises {
		for index := 1; index <= exercise.TargetSets; index++ {
			exerciseID := exercise.ID
			sets = append(sets, ScheduledSet{ProgramExerciseID: &exerciseID, CatalogID: exercise.CatalogID, ExerciseName: exercise.Name, ExerciseCategory: exercise.Category, ExerciseModality: exercise.Modality, ExerciseOrder: exercise.ExerciseOrder, SetIndex: index, TargetReps: exercise.TargetReps, TargetWeight: exercise.TargetWeight, TargetDurationSeconds: exercise.TargetDurationSeconds, RestSeconds: exercise.RestSeconds, Notes: exercise.Notes})
		}
	}
	return sets
}
