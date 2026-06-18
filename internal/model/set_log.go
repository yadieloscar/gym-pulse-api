package model

import "github.com/google/uuid"

// SetLog is one performed (or planned-but-skipped) set within a day's workout.
// It is the per-set source of truth behind the active workout player and the
// "last time you did X" history. exercise_overrides keeps per-exercise notes.
type SetLog struct {
	ID              uuid.UUID `json:"id"`
	DayLogID        uuid.UUID `json:"-"`
	ExerciseID      uuid.UUID `json:"exercise_id" validate:"required"`
	SetIndex        int       `json:"set_index" validate:"required,min=1"`
	TargetReps      *int      `json:"target_reps,omitempty"`
	TargetWeight    *float64  `json:"target_weight,omitempty"`
	ActualReps      *int      `json:"actual_reps,omitempty"`
	ActualWeight    *float64  `json:"actual_weight,omitempty"`
	DurationSeconds *int      `json:"duration_seconds,omitempty"`
	Completed       bool      `json:"completed"`
}

// CreateSetLogRequest is a single set in a create/update day-log request. A
// completed set must record what was done — actual_reps or duration_seconds.
type CreateSetLogRequest struct {
	ExerciseID      uuid.UUID `json:"exercise_id" validate:"required"`
	SetIndex        int       `json:"set_index" validate:"required,min=1"`
	TargetReps      *int      `json:"target_reps,omitempty"`
	TargetWeight    *float64  `json:"target_weight,omitempty"`
	ActualReps      *int      `json:"actual_reps,omitempty"`
	ActualWeight    *float64  `json:"actual_weight,omitempty"`
	DurationSeconds *int      `json:"duration_seconds,omitempty"`
	Completed       bool      `json:"completed"`
}

// ExerciseHistory is the most recent completed sets for one exercise, used to
// surface "last time" hints in the player. Date is the day those sets were done.
type ExerciseHistory struct {
	ExerciseID uuid.UUID `json:"exercise_id"`
	Date       string    `json:"date"`
	Sets       []SetLog  `json:"sets"`
}

// SetPerf is one completed strength set's load, used to compute records. The
// DAO returns these raw; the service reduces them to ExerciseRecord.
type SetPerf struct {
	ExerciseID uuid.UUID
	Weight     float64
	Reps       int
	Date       string
}

// ExerciseRecord is a user's all-time bests for one exercise: heaviest weight
// lifted, and best estimated 1RM (Epley). Pointers are nil when the exercise
// has no completed weighted sets yet.
type ExerciseRecord struct {
	ExerciseID    uuid.UUID `json:"exercise_id"`
	MaxWeight     *float64  `json:"max_weight"`
	MaxWeightReps *int      `json:"max_weight_reps"`
	MaxWeightDate *string   `json:"max_weight_date,omitempty"`
	BestE1RM      *float64  `json:"best_e1rm"`
	E1RMWeight    *float64  `json:"e1rm_weight"`
	E1RMReps      *int      `json:"e1rm_reps"`
	E1RMDate      *string   `json:"e1rm_date,omitempty"`
}
