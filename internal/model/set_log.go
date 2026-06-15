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
