package model

import (
	"bytes"
	"encoding/json"
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
	// DurationSeconds is the seconds per set of a hold or stretch.
	DurationSeconds *int    `json:"duration_seconds,omitempty"`
	Intensity       *string `json:"intensity,omitempty"`
	Notes           *string `json:"notes,omitempty"`
}

type WorkoutTemplate struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"-"`
	Name      string    `json:"name" validate:"required,min=1,max=100"`
	TypeID    string    `json:"type_id" validate:"required"`
	SubtypeID string    `json:"subtype_id" validate:"required"`
	// WarmupRoutineID and CooldownRoutineID name the stretching routines the
	// athlete chose for this workout. Null means the app picks by body area.
	WarmupRoutineID   *string    `json:"warmup_routine_id"`
	CooldownRoutineID *string    `json:"cooldown_routine_id"`
	Exercises         []Exercise `json:"exercises"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
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
	Name      string `json:"name" validate:"required,min=1,max=100"`
	TypeID    string `json:"type_id" validate:"required"`
	SubtypeID string `json:"subtype_id" validate:"required"`
	// A routine field that is absent keeps the stored choice on PUT, so a
	// client older than contract 1.13.0 does not clear it. Null or "" clears
	// the choice back to automatic.
	WarmupRoutineID   OptionalText            `json:"warmup_routine_id" swaggertype:"string"`
	CooldownRoutineID OptionalText            `json:"cooldown_routine_id" swaggertype:"string"`
	Exercises         []CreateExerciseRequest `json:"exercises"`
}

// OptionalText is a JSON string field that records whether the body carried
// it at all. Null and "" both decode to a present, empty value.
type OptionalText struct {
	Present bool
	Value   *string
}

// UnmarshalJSON marks the field present; it is only called when the key is in the body.
func (o *OptionalText) UnmarshalJSON(data []byte) error {
	o.Present = true
	o.Value = nil
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	if text != "" {
		o.Value = &text
	}
	return nil
}

// MarshalJSON writes the value, so a request round-trips in tests and logs.
func (o OptionalText) MarshalJSON() ([]byte, error) {
	return json.Marshal(o.Value)
}

// CreateExerciseRequest is a single exercise in a create/update template request.
// An exercise describes strength work (sets+reps), timed work such as a hold
// or stretch (sets+duration_seconds), or cardio work (duration_minutes,
// optional intensity) — exactly one of the three.
type CreateExerciseRequest struct {
	CatalogID       *uuid.UUID `json:"catalog_id,omitempty"`
	Name            string     `json:"name" validate:"required,min=1,max=200"`
	Sets            *int       `json:"sets,omitempty"`
	Reps            *int       `json:"reps,omitempty"`
	Weight          *float64   `json:"weight,omitempty"`
	RestSeconds     *int       `json:"rest_seconds,omitempty"`
	DurationMinutes *int       `json:"duration_minutes,omitempty" validate:"omitempty,gt=0"`
	DurationSeconds *int       `json:"duration_seconds,omitempty" validate:"omitempty,min=1,max=3600"`
	Intensity       *string    `json:"intensity,omitempty" validate:"omitempty,oneof=easy moderate hard"`
	Notes           *string    `json:"notes,omitempty"`
}
