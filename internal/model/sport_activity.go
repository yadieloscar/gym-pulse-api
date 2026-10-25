package model

import (
	"time"

	"github.com/google/uuid"
)

// ActivitySource records where a sport activity came from.
type ActivitySource string

const (
	// ActivitySourceManual is an activity the athlete typed in.
	ActivitySourceManual ActivitySource = "manual"
	// ActivitySourceAppleHealth is a workout imported from Apple Health.
	ActivitySourceAppleHealth ActivitySource = "apple_health"
)

// SportActivity is one completed athlete-owned sport occurrence. Imported
// activities carry the health store's identity and any supplied measurements.
type SportActivity struct {
	ID              uuid.UUID      `json:"id"`
	Date            string         `json:"date"`
	SportID         string         `json:"sport_id"`
	SportName       string         `json:"sport_name"`
	DurationMinutes int            `json:"duration_minutes"`
	Notes           *string        `json:"notes,omitempty"`
	Source          ActivitySource `json:"source"`
	ExternalID      *string        `json:"external_id,omitempty"`
	DistanceMeters  *int           `json:"distance_meters,omitempty"`
	EnergyKcal      *int           `json:"energy_kcal,omitempty"`
	AvgHeartRateBPM *int           `json:"avg_heart_rate_bpm,omitempty"`
	// ScheduledWorkoutID is the scheduled sport session this activity fulfilled.
	ScheduledWorkoutID *uuid.UUID `json:"scheduled_workout_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// CreateSportActivityRequest records a completed sport. Date defaults to the
// athlete's current civil date in their saved training-profile timezone.
// Source defaults to manual; a non-manual source requires external_id.
type CreateSportActivityRequest struct {
	Date            string         `json:"date,omitempty"`
	SportID         string         `json:"sport_id"`
	SportName       string         `json:"sport_name"`
	DurationMinutes int            `json:"duration_minutes"`
	Notes           *string        `json:"notes,omitempty"`
	OperationKey    string         `json:"operation_key"`
	Source          ActivitySource `json:"source,omitempty"`
	ExternalID      *string        `json:"external_id,omitempty"`
	DistanceMeters  *int           `json:"distance_meters,omitempty"`
	EnergyKcal      *int           `json:"energy_kcal,omitempty"`
	AvgHeartRateBPM *int           `json:"avg_heart_rate_bpm,omitempty"`
	// ScheduledWorkoutID records the activity against an owned open scheduled
	// sport session for the same sport.
	ScheduledWorkoutID *uuid.UUID `json:"scheduled_workout_id,omitempty"`
}

// SportActivityCreateOutcome says what a create request did.
type SportActivityCreateOutcome int

const (
	// SportActivityCreated means a new activity was recorded.
	SportActivityCreated SportActivityCreateOutcome = iota
	// SportActivityReplayed means an identical idempotent retry returned the original.
	SportActivityReplayed
	// SportActivityExisting means an imported workout with the same source and
	// external identity already existed; nothing was written.
	SportActivityExisting
)
