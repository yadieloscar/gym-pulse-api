package model

import (
	"time"

	"github.com/google/uuid"
)

type ExerciseOverride struct {
	ID           uuid.UUID `json:"id"`
	DayLogID     uuid.UUID `json:"-"`
	ExerciseID   uuid.UUID `json:"exercise_id" validate:"required"`
	ActualSets   *int      `json:"actual_sets,omitempty"`
	ActualReps   *int      `json:"actual_reps,omitempty"`
	ActualWeight *float64  `json:"actual_weight,omitempty"`
	Notes        *string   `json:"notes,omitempty"`
	Skipped      bool      `json:"skipped"`
}

type DayLog struct {
	ID           uuid.UUID          `json:"id"`
	UserID       uuid.UUID          `json:"-"`
	Date         string             `json:"date"`
	TypeID       string             `json:"type_id" validate:"required"`
	SubtypeID    string             `json:"subtype_id" validate:"required"`
	TemplateID   *uuid.UUID         `json:"template_id,omitempty"`
	TemplateName *string            `json:"template_name,omitempty"`
	Template     *WorkoutTemplate   `json:"template,omitempty"`
	Overrides    []ExerciseOverride `json:"overrides,omitempty"`
	SessionNotes *string            `json:"session_notes,omitempty"`
	LoggedAt     time.Time          `json:"logged_at"`
}

// DayLogSummary is the list-view representation for weekly queries.
type DayLogSummary struct {
	ID           uuid.UUID  `json:"id"`
	Date         string     `json:"date"`
	TypeID       string     `json:"type_id"`
	SubtypeID    string     `json:"subtype_id"`
	TemplateID   *uuid.UUID `json:"template_id,omitempty"`
	TemplateName *string    `json:"template_name,omitempty"`
	SessionNotes *string    `json:"session_notes,omitempty"`
	LoggedAt     time.Time  `json:"logged_at"`
}

// CreateDayLogRequest is the request body for POST /api/v1/logs.
type CreateDayLogRequest struct {
	Date         string                  `json:"date" validate:"required"`
	TypeID       string                  `json:"type_id" validate:"required"`
	SubtypeID    string                  `json:"subtype_id" validate:"required"`
	TemplateID   *uuid.UUID              `json:"template_id,omitempty"`
	Overrides    []CreateOverrideRequest `json:"overrides,omitempty"`
	SessionNotes *string                 `json:"session_notes,omitempty"`
}

// UpdateDayLogRequest is the request body for PUT /api/v1/logs/:date.
type UpdateDayLogRequest struct {
	Overrides    []CreateOverrideRequest `json:"overrides,omitempty"`
	SessionNotes *string                 `json:"session_notes,omitempty"`
}

// CreateOverrideRequest is a single override in a create/update log request.
type CreateOverrideRequest struct {
	ExerciseID   uuid.UUID `json:"exercise_id" validate:"required"`
	ActualSets   *int      `json:"actual_sets,omitempty"`
	ActualReps   *int      `json:"actual_reps,omitempty"`
	ActualWeight *float64  `json:"actual_weight,omitempty"`
	Notes        *string   `json:"notes,omitempty"`
	Skipped      bool      `json:"skipped"`
}
