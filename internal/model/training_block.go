package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	TrainingBlockActive    = "active"
	TrainingBlockCompleted = "completed"
	TrainingBlockArchived  = "archived"

	TrainingLoadEasy      = "easy"
	TrainingLoadDemanding = "demanding"

	SessionCompletedAsPlanned = "completed_as_planned"
	SessionModified           = "modified"
	SessionStopped            = "stopped"

	NextMorningBaseline      = "baseline"
	NextMorningAboveBaseline = "above_baseline"

	TransitionAdvance  = "advance"
	TransitionRegress  = "regress"
	TransitionComplete = "complete"
	TransitionArchive  = "archive"
)

type TrainingStage struct {
	ID                          uuid.UUID `json:"id"`
	StageOrder                  int       `json:"stage_order"`
	Name                        string    `json:"name"`
	Instructions                *string   `json:"instructions,omitempty"`
	LoadLevel                   string    `json:"load_level"`
	TargetCount                 *int      `json:"target_count,omitempty"`
	TargetDurationMinutes       *int      `json:"target_duration_minutes,omitempty"`
	TargetIntensityPercent      *int      `json:"target_intensity_percent,omitempty"`
	RequiredQualifyingExposures int       `json:"required_qualifying_exposures"`
}

type TrainingExposure struct {
	ID                        uuid.UUID  `json:"id"`
	StageID                   uuid.UUID  `json:"stage_id"`
	PerformedOn               string     `json:"performed_on"`
	ActivityLabel             string     `json:"activity_label"`
	LoadLevel                 string     `json:"load_level"`
	PerformedCount            *int       `json:"performed_count,omitempty"`
	DurationMinutes           *int       `json:"duration_minutes,omitempty"`
	PerformedIntensityPercent *int       `json:"performed_intensity_percent,omitempty"`
	SessionOutcome            string     `json:"session_outcome"`
	NextMorningResponse       *string    `json:"next_morning_response,omitempty"`
	Notes                     *string    `json:"notes,omitempty"`
	Qualifies                 bool       `json:"qualifies"`
	CreatedAt                 time.Time  `json:"created_at"`
	NextMorningRecordedAt     *time.Time `json:"next_morning_recorded_at,omitempty"`
}

type TrainingTransition struct {
	ID          uuid.UUID  `json:"id"`
	Action      string     `json:"action"`
	FromStageID *uuid.UUID `json:"from_stage_id,omitempty"`
	ToStageID   *uuid.UUID `json:"to_stage_id,omitempty"`
	Reason      *string    `json:"reason,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type TrainingStageProgress struct {
	RequiredQualifyingExposures int  `json:"required_qualifying_exposures"`
	QualifyingExposures         int  `json:"qualifying_exposures"`
	CriteriaComplete            bool `json:"criteria_complete"`
}

type TrainingBlockSummary struct {
	ID                      uuid.UUID             `json:"id"`
	Name                    string                `json:"name"`
	Purpose                 *string               `json:"purpose,omitempty"`
	ProgramID               *uuid.UUID            `json:"program_id,omitempty"`
	Status                  string                `json:"status"`
	Revision                int64                 `json:"revision"`
	CurrentStage            TrainingStage         `json:"current_stage"`
	CurrentStageProgress    TrainingStageProgress `json:"current_stage_progress"`
	PendingNextMorningCount int                   `json:"pending_next_morning_count"`
	UpdatedAt               time.Time             `json:"updated_at"`
}

type TrainingBlock struct {
	TrainingBlockSummary
	Stages      []TrainingStage      `json:"stages"`
	Exposures   []TrainingExposure   `json:"exposures"`
	Transitions []TrainingTransition `json:"transitions"`
	CreatedAt   time.Time            `json:"created_at"`
}

type TrainingBlockList struct {
	TrainingBlocks []TrainingBlockSummary `json:"training_blocks"`
	NextOffset     *int                   `json:"next_offset"`
}

type CreateTrainingStageRequest struct {
	Name                        string  `json:"name"`
	Instructions                *string `json:"instructions,omitempty"`
	LoadLevel                   string  `json:"load_level"`
	TargetCount                 *int    `json:"target_count,omitempty"`
	TargetDurationMinutes       *int    `json:"target_duration_minutes,omitempty"`
	TargetIntensityPercent      *int    `json:"target_intensity_percent,omitempty"`
	RequiredQualifyingExposures int     `json:"required_qualifying_exposures"`
}

type CreateTrainingBlockRequest struct {
	Name         string                       `json:"name"`
	Purpose      *string                      `json:"purpose,omitempty"`
	ProgramID    *uuid.UUID                   `json:"program_id,omitempty"`
	Stages       []CreateTrainingStageRequest `json:"stages"`
	OperationKey string                       `json:"operation_key"`
}

type CreateTrainingExposureRequest struct {
	PerformedOn               string  `json:"performed_on"`
	ActivityLabel             string  `json:"activity_label"`
	LoadLevel                 string  `json:"load_level"`
	PerformedCount            *int    `json:"performed_count,omitempty"`
	DurationMinutes           *int    `json:"duration_minutes,omitempty"`
	PerformedIntensityPercent *int    `json:"performed_intensity_percent,omitempty"`
	SessionOutcome            string  `json:"session_outcome"`
	Notes                     *string `json:"notes,omitempty"`
	ExpectedRevision          int64   `json:"expected_revision"`
	OperationKey              string  `json:"operation_key"`
}

type RecordNextMorningRequest struct {
	Response         string `json:"response"`
	ExpectedRevision int64  `json:"expected_revision"`
	OperationKey     string `json:"operation_key"`
}

type CreateTrainingTransitionRequest struct {
	Action           string     `json:"action"`
	ToStageID        *uuid.UUID `json:"to_stage_id,omitempty"`
	Reason           *string    `json:"reason,omitempty"`
	ExpectedRevision int64      `json:"expected_revision"`
	OperationKey     string     `json:"operation_key"`
}

func TrainingExposureQualifies(outcome string, response *string) bool {
	return outcome == SessionCompletedAsPlanned && response != nil && *response == NextMorningBaseline
}
