package model

import (
	"time"

	"github.com/google/uuid"
)

// HealthExportKind names which finished-workout table an export refers to.
type HealthExportKind string

const (
	// HealthExportKindScheduledWorkout is a finalized planned workout.
	HealthExportKindScheduledWorkout HealthExportKind = "scheduled_workout"
	// HealthExportKindDayLog is an off-plan workout log.
	HealthExportKindDayLog HealthExportKind = "day_log"
)

// HealthExportSource names the health store a workout was written to.
type HealthExportSource string

// HealthExportSourceAppleHealth is Apple Health on iOS.
const HealthExportSourceAppleHealth HealthExportSource = "apple_health"

// HealthExport remembers the health-store identity of one exported finished
// workout. It is write-once per workout and source.
type HealthExport struct {
	ID          uuid.UUID          `json:"id"`
	WorkoutKind HealthExportKind   `json:"workout_kind"`
	WorkoutID   uuid.UUID          `json:"workout_id"`
	Date        string             `json:"date"`
	Source      HealthExportSource `json:"source"`
	ExternalID  string             `json:"external_id"`
	ExportedAt  time.Time          `json:"exported_at"`
	CreatedAt   time.Time          `json:"created_at"`
}

// RecordHealthExportRequest records the identity the device received from the
// health store. ExportedAt defaults to now.
type RecordHealthExportRequest struct {
	Source       HealthExportSource `json:"source"`
	ExternalID   string             `json:"external_id"`
	ExportedAt   *time.Time         `json:"exported_at,omitempty"`
	OperationKey string             `json:"operation_key"`
}

// HealthExportOutcome says what a record request did.
type HealthExportOutcome int

const (
	// HealthExportRecorded means a new export was stored.
	HealthExportRecorded HealthExportOutcome = iota
	// HealthExportReplayed means an identical idempotent retry returned the original.
	HealthExportReplayed
	// HealthExportExisting means the same identity was already recorded; nothing was written.
	HealthExportExisting
)
