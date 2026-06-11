package model

import (
	"time"

	"github.com/google/uuid"
)

type Exercise struct {
	ID              uuid.UUID  `json:"id"`
	TemplateID      uuid.UUID  `json:"-"`
	CatalogID       *uuid.UUID `json:"catalog_id,omitempty"`
	Name            string     `json:"name" validate:"required,min=1,max=200"`
	SortOrder       int        `json:"sort_order"`
	Sets            *int       `json:"sets,omitempty"`
	Reps            *int       `json:"reps,omitempty"`
	Weight          *float64   `json:"weight,omitempty"`
	RestSeconds     *int       `json:"rest_seconds,omitempty"`
	DurationMinutes *int       `json:"duration_minutes,omitempty"`
	Intensity       *string    `json:"intensity,omitempty"`
	Notes           *string    `json:"notes,omitempty"`
}

type WorkoutTemplate struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"-"`
	Name      string     `json:"name" validate:"required,min=1,max=100"`
	TypeID    string     `json:"type_id" validate:"required"`
	SubtypeID string     `json:"subtype_id" validate:"required"`
	Exercises []Exercise `json:"exercises"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// TemplateSummary is the list-view representation (no full exercises).
type TemplateSummary struct {
	ID               uuid.UUID `json:"id"`
	Name             string    `json:"name"`
	TypeID           string    `json:"type_id"`
	SubtypeID        string    `json:"subtype_id"`
	ExerciseCount    int       `json:"exercise_count"`
	ExercisesPreview []string  `json:"exercises_preview"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// CreateTemplateRequest is the request body for POST/PUT templates.
type CreateTemplateRequest struct {
	Name      string                  `json:"name" validate:"required,min=1,max=100"`
	TypeID    string                  `json:"type_id" validate:"required"`
	SubtypeID string                  `json:"subtype_id" validate:"required"`
	Exercises []CreateExerciseRequest `json:"exercises"`
}

// CreateExerciseRequest is a single exercise in a create/update template request.
// An exercise describes either strength work (sets+reps) or cardio work
// (duration_minutes, optional intensity) — exactly one of the two.
type CreateExerciseRequest struct {
	CatalogID       *uuid.UUID `json:"catalog_id,omitempty"`
	Name            string     `json:"name" validate:"required,min=1,max=200"`
	Sets            *int       `json:"sets,omitempty"`
	Reps            *int       `json:"reps,omitempty"`
	Weight          *float64   `json:"weight,omitempty"`
	RestSeconds     *int       `json:"rest_seconds,omitempty"`
	DurationMinutes *int       `json:"duration_minutes,omitempty" validate:"omitempty,gt=0"`
	Intensity       *string    `json:"intensity,omitempty" validate:"omitempty,oneof=easy moderate hard"`
	Notes           *string    `json:"notes,omitempty"`
}
