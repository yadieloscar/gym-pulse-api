package model

import (
	"encoding/json"
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
	SetLogs      []SetLog           `json:"set_logs,omitempty"`
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
	SetLogs      []CreateSetLogRequest   `json:"set_logs,omitempty"`
	SessionNotes *string                 `json:"session_notes,omitempty"`
}

// UpdateDayLogRequest is the request body for PUT /api/v1/logs/:date.
// TypeID/SubtypeID/TemplateID, when present, REPLACE the day's workout
// (e.g. "logged Push but actually did Legs"). Replacing always rewrites the
// detail supplied by the caller. Omitted detail is preserved for an ordinary
// partial update and cleared for a workout replacement.
type UpdateDayLogRequest struct {
	TypeID          *string                 `json:"type_id,omitempty"`
	SubtypeID       *string                 `json:"subtype_id,omitempty"`
	TemplateID      *uuid.UUID              `json:"template_id,omitempty"`
	Overrides       []CreateOverrideRequest `json:"overrides,omitempty"`
	SetLogs         []CreateSetLogRequest   `json:"set_logs,omitempty"`
	SessionNotes    *string                 `json:"session_notes,omitempty"`
	OverridesSet    bool                    `json:"-"`
	SetLogsSet      bool                    `json:"-"`
	SessionNotesSet bool                    `json:"-"`
}

// UnmarshalJSON records field presence so omission can mean "preserve" while
// an explicit empty array (or null) can mean "clear".
func (r *UpdateDayLogRequest) UnmarshalJSON(data []byte) error {
	type requestAlias UpdateDayLogRequest
	var decoded requestAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*r = UpdateDayLogRequest(decoded)
	_, r.OverridesSet = fields["overrides"]
	_, r.SetLogsSet = fields["set_logs"]
	_, r.SessionNotesSet = fields["session_notes"]
	return nil
}

// DayLogUpdate is the persistence intent resolved by the service. Replace
// flags distinguish preservation from replacement even when a replacement
// collection is empty.
type DayLogUpdate struct {
	Overrides        []ExerciseOverride
	ReplaceOverrides bool
	SetLogs          []SetLog
	ReplaceSetLogs   bool
	SessionNotes     *string
	ReplaceNotes     bool
	Replacement      *LogReplacement
}

// LogReplacement is the resolved "this day was actually a different workout"
// change applied during an update. TemplateID nil means a template-less log.
type LogReplacement struct {
	TypeID     string
	SubtypeID  string
	TemplateID *uuid.UUID
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
