package model

import (
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
)

const (
	GoalGeneralHealth   = "general_health"
	GoalStrength        = "strength"
	GoalHypertrophy     = "hypertrophy"
	GoalConditioning    = "conditioning"
	GoalPower           = "power"
	GoalBodyComposition = "body_composition"

	WorkoutStatusPlanned    = "planned"
	WorkoutStatusInProgress = "in_progress"
	WorkoutStatusCompleted  = "completed"
	WorkoutStatusIncomplete = "incomplete"
	WorkoutStatusMissed     = "missed"
)

var (
	ValidTrainingGoals = []string{
		GoalGeneralHealth, GoalStrength, GoalHypertrophy,
		GoalConditioning, GoalPower, GoalBodyComposition,
	}
	ValidActivityLevels   = []string{"sedentary", "light", "moderate", "high"}
	ValidExperienceLevels = []string{"beginner", "intermediate", "advanced"}
	ValidEquipment        = []string{
		"bodyweight", "bands", "dumbbells", "barbell", "machines",
		"cardio_machine", "full_gym",
	}
	ValidWorkoutStatuses = []string{
		WorkoutStatusPlanned, WorkoutStatusInProgress, WorkoutStatusCompleted,
		WorkoutStatusIncomplete, WorkoutStatusMissed,
	}
	ValidSessionStatuses = []string{"draft", "active", "completed", "discarded"}
)

func IsValidTrainingGoal(goal string) bool {
	return slices.Contains(ValidTrainingGoals, goal)
}

func IsValidWorkoutStatus(status string) bool {
	return slices.Contains(ValidWorkoutStatuses, status)
}

// TrainingProfile is the goal-led training contract. It intentionally remains
// separate from UserProfile and UserSettings so legacy profile/settings reads
// retain their existing wire shapes.
type TrainingProfile struct {
	PrimaryGoal            string         `json:"primary_goal"`
	AvailableDays          []int          `json:"available_days"`
	UsualActivity          string         `json:"usual_activity"`
	Experience             string         `json:"experience"`
	Equipment              []string       `json:"equipment"`
	SessionDurationMinutes int            `json:"session_duration_minutes"`
	Timezone               string         `json:"timezone"`
	Preferences            map[string]any `json:"preferences"`
	Revision               int64          `json:"revision"`
	CreatedAt              time.Time      `json:"created_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
}

// Validate rejects values that would make schedule generation ambiguous.
func (p TrainingProfile) Validate() error {
	if !IsValidTrainingGoal(p.PrimaryGoal) {
		return validationError("primary_goal", "unknown training goal")
	}
	if !slices.Contains(ValidActivityLevels, p.UsualActivity) {
		return validationError("usual_activity", "unknown activity level")
	}
	if !slices.Contains(ValidExperienceLevels, p.Experience) {
		return validationError("experience", "unknown experience level")
	}
	if len(p.AvailableDays) == 0 || len(p.AvailableDays) > 7 {
		return validationError("available_days", "available_days must contain 1 to 7 unique ISO weekdays")
	}
	seenDays := make(map[int]struct{}, len(p.AvailableDays))
	for _, day := range p.AvailableDays {
		if day < 1 || day > 7 {
			return validationError("available_days", "available_days must use ISO weekdays 1 through 7")
		}
		if _, exists := seenDays[day]; exists {
			return validationError("available_days", "available_days must be unique")
		}
		seenDays[day] = struct{}{}
	}
	seenEquipment := make(map[string]struct{}, len(p.Equipment))
	for _, equipment := range p.Equipment {
		if !slices.Contains(ValidEquipment, equipment) {
			return validationError("equipment", "unknown equipment value")
		}
		if _, exists := seenEquipment[equipment]; exists {
			return validationError("equipment", "equipment values must be unique")
		}
		seenEquipment[equipment] = struct{}{}
	}
	if p.SessionDurationMinutes < 20 || p.SessionDurationMinutes > 120 {
		return validationError("session_duration_minutes", "session_duration_minutes must be between 20 and 120")
	}
	if p.Timezone == "" {
		return validationError("timezone", "timezone is required")
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return validationError("timezone", "timezone must be a valid IANA timezone")
	}
	return nil
}

// UpdateTrainingProfileRequest is a partial profile update. The service merges
// it onto the current profile and validates the resulting complete profile.
type UpdateTrainingProfileRequest struct {
	PrimaryGoal            *string         `json:"primary_goal,omitempty"`
	AvailableDays          *[]int          `json:"available_days,omitempty"`
	UsualActivity          *string         `json:"usual_activity,omitempty"`
	Experience             *string         `json:"experience,omitempty"`
	Equipment              *[]string       `json:"equipment,omitempty"`
	SessionDurationMinutes *int            `json:"session_duration_minutes,omitempty"`
	Timezone               *string         `json:"timezone,omitempty"`
	Preferences            *map[string]any `json:"preferences,omitempty"`
	ExpectedRevision       int64           `json:"expected_revision" validate:"gte=0"`
}

type StarterProgramFilter struct {
	PrimaryGoal            string
	AvailableDays          int
	Experience             string
	Equipment              []string
	SessionDurationMinutes int
}

type StarterProgram struct {
	ID              uuid.UUID        `json:"id"`
	Slug            string           `json:"slug"`
	Version         int              `json:"version"`
	Name            string           `json:"name"`
	Description     string           `json:"description"`
	PrimaryGoal     string           `json:"primary_goal"`
	MinDays         int              `json:"min_days"`
	MaxDays         int              `json:"max_days"`
	Experience      []string         `json:"experience"`
	Equipment       []string         `json:"equipment"`
	DurationMinutes int              `json:"duration_minutes"`
	Rationale       string           `json:"rationale"`
	Roadmap         map[string]any   `json:"roadmap"`
	Workouts        []ProgramWorkout `json:"workouts"`
}

type Program struct {
	ID               uuid.UUID        `json:"id"`
	StarterProgramID *uuid.UUID       `json:"starter_program_id,omitempty"`
	StarterVersion   *int             `json:"starter_version,omitempty"`
	Name             string           `json:"name"`
	PrimaryGoal      string           `json:"primary_goal"`
	Roadmap          map[string]any   `json:"roadmap"`
	Active           bool             `json:"active"`
	Revision         int64            `json:"revision"`
	Workouts         []ProgramWorkout `json:"workouts"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
}

type ProgramWorkout struct {
	ID               uuid.UUID         `json:"id"`
	Name             string            `json:"name"`
	PreferredWeekday *int              `json:"preferred_weekday,omitempty"`
	SequencePosition int               `json:"sequence_position"`
	Exercises        []ProgramExercise `json:"exercises"`
}

type ProgramExercise struct {
	ID                      uuid.UUID  `json:"id"`
	CatalogID               *uuid.UUID `json:"catalog_id,omitempty"`
	SourceStarterExerciseID *uuid.UUID `json:"source_starter_exercise_id,omitempty"`
	Name                    string     `json:"name"`
	Category                string     `json:"category"`
	Modality                string     `json:"modality"`
	ExerciseOrder           int        `json:"exercise_order"`
	TargetSets              int        `json:"target_sets"`
	TargetReps              *int       `json:"target_reps,omitempty"`
	TargetWeight            *float64   `json:"target_weight,omitempty"`
	TargetDurationSeconds   *int       `json:"target_duration_seconds,omitempty"`
	RestSeconds             *int       `json:"rest_seconds,omitempty"`
	Notes                   *string    `json:"notes,omitempty"`
}

type CreateProgramRequest struct {
	Name        string           `json:"name" validate:"required,min=1,max=200"`
	PrimaryGoal string           `json:"primary_goal" validate:"required"`
	Roadmap     map[string]any   `json:"roadmap"`
	Workouts    []ProgramWorkout `json:"workouts" validate:"required,min=1"`
}

type UpdateProgramRequest struct {
	Name             string           `json:"name" validate:"required,min=1,max=200"`
	PrimaryGoal      string           `json:"primary_goal" validate:"required"`
	Roadmap          map[string]any   `json:"roadmap"`
	Active           bool             `json:"active"`
	Workouts         []ProgramWorkout `json:"workouts" validate:"required,min=1"`
	ExpectedRevision int64            `json:"expected_revision" validate:"required,min=1"`
}

type CloneStarterProgramRequest struct {
	StarterProgramID uuid.UUID `json:"starter_program_id" validate:"required"`
	StarterVersion   int       `json:"starter_version" validate:"required,min=1"`
	Name             *string   `json:"name,omitempty"`
	OperationKey     string    `json:"operation_key" validate:"required"`
}

type ScheduledWorkout struct {
	ID               uuid.UUID      `json:"id"`
	ProgramID        *uuid.UUID     `json:"program_id,omitempty"`
	ProgramWorkoutID *uuid.UUID     `json:"program_workout_id,omitempty"`
	Date             string         `json:"date"`
	Name             string         `json:"name"`
	SequencePosition *int           `json:"sequence_position,omitempty"`
	Status           string         `json:"status"`
	FinalizedAt      *time.Time     `json:"finalized_at,omitempty"`
	Revision         int64          `json:"revision"`
	RequiredSets     []ScheduledSet `json:"required_sets"`
	ExtraSets        []PerformedSet `json:"extra_sets"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

func (w ScheduledWorkout) Validate() error {
	if !IsValidWorkoutStatus(w.Status) {
		return validationError("status", "unknown scheduled workout status")
	}
	for _, set := range w.RequiredSets {
		if err := set.Validate(); err != nil {
			return err
		}
	}
	for _, set := range w.ExtraSets {
		if !set.IsExtra || set.ScheduledSetID != nil {
			return validationError("extra_sets", "extra sets cannot satisfy a required set")
		}
	}
	return nil
}

type ScheduledSet struct {
	ID                    uuid.UUID  `json:"id"`
	ProgramExerciseID     *uuid.UUID `json:"program_exercise_id,omitempty"`
	CatalogID             *uuid.UUID `json:"catalog_id,omitempty"`
	ExerciseName          string     `json:"exercise_name"`
	ExerciseCategory      string     `json:"exercise_category"`
	ExerciseModality      string     `json:"exercise_modality"`
	ExerciseOrder         int        `json:"exercise_order"`
	SetIndex              int        `json:"set_index"`
	TargetReps            *int       `json:"target_reps,omitempty"`
	TargetWeight          *float64   `json:"target_weight,omitempty"`
	TargetDurationSeconds *int       `json:"target_duration_seconds,omitempty"`
	RestSeconds           *int       `json:"rest_seconds,omitempty"`
	Notes                 *string    `json:"notes,omitempty"`
	Checked               bool       `json:"checked"`
	PerformedSetID        *uuid.UUID `json:"performed_set_id,omitempty"`
}

func (s ScheduledSet) Validate() error {
	if s.ExerciseName == "" || s.ExerciseCategory == "" {
		return validationError("required_sets", "scheduled sets require immutable exercise snapshots")
	}
	if s.ExerciseModality != "strength" && s.ExerciseModality != "cardio" {
		return validationError("required_sets", "unknown exercise modality")
	}
	if s.ExerciseOrder < 1 || s.SetIndex < 1 {
		return validationError("required_sets", "exercise_order and set_index must be positive")
	}
	return nil
}

type WorkoutSession struct {
	ID                 uuid.UUID      `json:"id"`
	ScheduledWorkoutID *uuid.UUID     `json:"scheduled_workout_id,omitempty"`
	Date               string         `json:"date"`
	Name               string         `json:"name"`
	Status             string         `json:"status"`
	Notes              *string        `json:"notes,omitempty"`
	StartedAt          *time.Time     `json:"started_at,omitempty"`
	CompletedAt        *time.Time     `json:"completed_at,omitempty"`
	Revision           int64          `json:"revision"`
	Sets               []PerformedSet `json:"sets"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

type PerformedSet struct {
	ID               uuid.UUID  `json:"id"`
	ScheduledSetID   *uuid.UUID `json:"scheduled_set_id,omitempty"`
	ExerciseID       *uuid.UUID `json:"exercise_id,omitempty"`
	IsExtra          bool       `json:"is_extra"`
	ExerciseName     string     `json:"exercise_name"`
	ExerciseCategory string     `json:"exercise_category"`
	ExerciseModality string     `json:"exercise_modality"`
	SetIndex         int        `json:"set_index"`
	TargetReps       *int       `json:"target_reps,omitempty"`
	TargetWeight     *float64   `json:"target_weight,omitempty"`
	ActualReps       *int       `json:"actual_reps,omitempty"`
	ActualWeight     *float64   `json:"actual_weight,omitempty"`
	DurationSeconds  *int       `json:"duration_seconds,omitempty"`
	Completed        bool       `json:"completed"`
	OperationKey     string     `json:"operation_key"`
	Revision         int64      `json:"revision"`
}

func (s PerformedSet) Validate() error {
	if s.IsExtra == (s.ScheduledSetID != nil) {
		return validationError("is_extra", "required results must reference a scheduled set and extra sets must not")
	}
	if s.ExerciseName == "" || s.ExerciseCategory == "" {
		return validationError("exercise_name", "performed sets require immutable exercise snapshots")
	}
	if s.ExerciseModality != "strength" && s.ExerciseModality != "cardio" {
		return validationError("exercise_modality", "unknown exercise modality")
	}
	return nil
}

type SetMutationRequest struct {
	OperationKey     string   `json:"operation_key" validate:"required"`
	ExpectedRevision int64    `json:"expected_revision" validate:"required,min=1"`
	ActualReps       *int     `json:"actual_reps,omitempty"`
	ActualWeight     *float64 `json:"actual_weight,omitempty"`
	DurationSeconds  *int     `json:"duration_seconds,omitempty"`
	Completed        bool     `json:"completed"`
}

type ExtraSetRequest struct {
	OperationKey     string     `json:"operation_key" validate:"required"`
	ExpectedRevision int64      `json:"expected_revision" validate:"required,min=1"`
	ExerciseID       *uuid.UUID `json:"exercise_id,omitempty"`
	ExerciseName     string     `json:"exercise_name" validate:"required"`
	ExerciseCategory string     `json:"exercise_category" validate:"required"`
	ExerciseModality string     `json:"exercise_modality" validate:"required,oneof=strength cardio"`
	SetIndex         int        `json:"set_index" validate:"required,min=1"`
	ActualReps       *int       `json:"actual_reps,omitempty"`
	ActualWeight     *float64   `json:"actual_weight,omitempty"`
	DurationSeconds  *int       `json:"duration_seconds,omitempty"`
	Completed        bool       `json:"completed"`
}

type RevisionRequest struct {
	OperationKey     string `json:"operation_key" validate:"required"`
	ExpectedRevision int64  `json:"expected_revision" validate:"required,min=1"`
}

type MaterializeScheduleRequest struct {
	ProgramID        uuid.UUID `json:"program_id" validate:"required"`
	From             string    `json:"from" validate:"required"`
	To               string    `json:"to" validate:"required"`
	OperationKey     string    `json:"operation_key" validate:"required"`
	ExpectedRevision int64     `json:"expected_revision" validate:"required,min=1"`
}

type RegenerateScheduleRequest struct {
	ProgramID        uuid.UUID `json:"program_id" validate:"required"`
	From             string    `json:"from" validate:"required"`
	To               string    `json:"to" validate:"required"`
	Apply            bool      `json:"apply"`
	PreviewToken     string    `json:"preview_token,omitempty"`
	OperationKey     string    `json:"operation_key" validate:"required"`
	ExpectedRevision int64     `json:"expected_revision" validate:"required,min=1"`
}

type RegenerateScheduleResponse struct {
	PreviewToken      string             `json:"preview_token"`
	RetainedFrom      *string            `json:"retained_from,omitempty"`
	RetainedTo        *string            `json:"retained_to,omitempty"`
	ReplacedFrom      *string            `json:"replaced_from,omitempty"`
	ReplacedTo        *string            `json:"replaced_to,omitempty"`
	ScheduledWorkouts []ScheduledWorkout `json:"scheduled_workouts"`
}

type PatchScheduledWorkoutRequest struct {
	Name             *string         `json:"name,omitempty"`
	RequiredSets     *[]ScheduledSet `json:"required_sets,omitempty"`
	OperationKey     string          `json:"operation_key" validate:"required"`
	ExpectedRevision int64           `json:"expected_revision" validate:"required,min=1"`
}

type CreateWorkoutSessionRequest struct {
	ScheduledWorkoutID *uuid.UUID `json:"scheduled_workout_id,omitempty"`
	Date               string     `json:"date" validate:"required"`
	Name               string     `json:"name" validate:"required"`
	Notes              *string    `json:"notes,omitempty"`
	OperationKey       string     `json:"operation_key" validate:"required"`
	ExpectedRevision   int64      `json:"expected_revision" validate:"gte=0"`
}

type PatchWorkoutSessionRequest struct {
	Name             *string `json:"name,omitempty"`
	Notes            *string `json:"notes,omitempty"`
	Status           *string `json:"status,omitempty"`
	OperationKey     string  `json:"operation_key" validate:"required"`
	ExpectedRevision int64   `json:"expected_revision" validate:"required,min=1"`
}

type DayParticipation struct {
	ID                   uuid.UUID `json:"id"`
	Date                 string    `json:"date"`
	ScheduledOpportunity bool      `json:"scheduled_opportunity"`
	Participated         bool      `json:"participated"`
	FinalizedAt          time.Time `json:"finalized_at"`
	Timezone             string    `json:"timezone"`
	LocalDate            string    `json:"local_date"`
	Revision             int64     `json:"revision"`
}

type IdempotencyRecord struct {
	Scope            string
	OperationKey     string
	RequestHash      string
	ResponseStatus   int
	ResponseBody     []byte
	ResourceType     string
	ResourceID       *uuid.UUID
	ResourceRevision *int64
	CreatedAt        time.Time
	ExpiresAt        *time.Time
}

func validationError(field, message string) error {
	return &ValidationError{Field: field, Message: message}
}

func ValidateDateRange(from, to string) error {
	fromDate, err := ParseDate(from)
	if err != nil {
		return validationError("from", "from must be YYYY-MM-DD")
	}
	toDate, err := ParseDate(to)
	if err != nil {
		return validationError("to", "to must be YYYY-MM-DD")
	}
	if toDate.Before(fromDate) {
		return validationError("to", fmt.Sprintf("to must not be before %s", from))
	}
	return nil
}
