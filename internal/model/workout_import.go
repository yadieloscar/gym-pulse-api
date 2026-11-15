package model

import (
	"time"

	"github.com/google/uuid"
)

// ImportSource is the app a workout-history CSV was exported from.
type ImportSource string

const (
	ImportSourceStrong ImportSource = "strong"
	ImportSourceHevy   ImportSource = "hevy"
)

// Unmatched choices for exercise names that match nothing in the catalog.
const (
	ImportUnmatchedKeep = "keep"
	ImportUnmatchedSkip = "skip"
)

// Import file error codes. All answer 422 with the code and details.
const (
	ImportCodeUnrecognized = "IMPORT_UNRECOGNIZED"
	ImportCodeInvalidRow   = "IMPORT_INVALID_ROW"
	ImportCodeTooLarge     = "IMPORT_TOO_LARGE"
	ImportCodeUnitRequired = "IMPORT_UNIT_REQUIRED"
)

// ImportFileError explains why an uploaded file cannot be imported. Nothing
// has been written when it is returned.
type ImportFileError struct {
	Code    string
	Message string
	Details map[string]any
}

func (e *ImportFileError) Error() string { return e.Message }

// ImportOptions are the athlete's choices for one preview or commit.
type ImportOptions struct {
	// SourceWeightUnit names the file's weight unit when the file does not.
	// Ignored when the file states its unit.
	SourceWeightUnit string
	Unmatched        string
}

// UnmatchedExercise is a source exercise name with no catalog match.
type UnmatchedExercise struct {
	Name     string `json:"name"`
	SetCount int    `json:"set_count"`
}

// ImportTotals counts what an import would write for one unmatched choice.
type ImportTotals struct {
	WorkoutCount int     `json:"workout_count"`
	DayCount     int     `json:"day_count"`
	SetCount     int     `json:"set_count"`
	FirstDate    *string `json:"first_date"`
	LastDate     *string `json:"last_date"`
}

// ImportPreview describes a parsed file without writing anything.
type ImportPreview struct {
	Source ImportSource `json:"source"`
	// SourceWeightUnit is the unit the file states, or null when it states
	// none and the commit must name one.
	SourceWeightUnit *string `json:"source_weight_unit"`
	// WeightUnit is the athlete's unit; imported weights are converted to it.
	WeightUnit string `json:"weight_unit"`
	// FileFirstDate and FileLastDate span every workout in the file.
	FileFirstDate *string `json:"file_first_date"`
	FileLastDate  *string `json:"file_last_date"`
	FileWorkouts  int     `json:"file_workout_count"`
	// Keep and Skip are what the import writes for each unmatched choice.
	Keep               ImportTotals        `json:"keep"`
	Skip               ImportTotals        `json:"skip"`
	DaysAlreadyPresent int                 `json:"days_already_present"`
	DaysSkippedFuture  int                 `json:"days_skipped_future"`
	WarmupSetsSkipped  int                 `json:"warmup_sets_skipped"`
	EmptySetsSkipped   int                 `json:"empty_sets_skipped"`
	SetsLosingDistance int                 `json:"sets_losing_distance"`
	MatchedExercises   int                 `json:"matched_exercise_count"`
	UnmatchedExercises []UnmatchedExercise `json:"unmatched_exercises"`
}

// WorkoutImport is one committed import batch.
type WorkoutImport struct {
	ID               uuid.UUID    `json:"id"`
	Source           ImportSource `json:"source"`
	SourceWeightUnit string       `json:"source_weight_unit"`
	WeightUnit       string       `json:"weight_unit"`
	Unmatched        string       `json:"unmatched"`
	FirstDate        *string      `json:"first_date"`
	LastDate         *string      `json:"last_date"`
	WorkoutCount     int          `json:"workout_count"`
	DayCount         int          `json:"day_count"`
	SetCount         int          `json:"set_count"`
	CreatedAt        time.Time    `json:"created_at"`
}

// ImportSet is one set ready to write.
type ImportSet struct {
	ExerciseID       *uuid.UUID
	ExerciseName     string
	ExerciseCategory string
	ExerciseModality string
	SetIndex         int
	ActualReps       *int
	ActualWeight     *float64
	DurationSeconds  *int
	LoggedAt         time.Time
}

// ImportDay is one day log ready to write.
type ImportDay struct {
	Date         string
	SessionNotes *string
	LoggedAt     time.Time
	Sets         []ImportSet
}

// ImportContext is what the importer needs to know about the athlete.
type ImportContext struct {
	WeightUnit string
	Timezone   string
	// SavedExercises maps each owned saved-workout exercise id to its name.
	SavedExercises map[uuid.UUID]string
	Catalog        []CatalogExercise
}

// ImportCommitOutcome reports whether a commit wrote or replayed.
type ImportCommitOutcome int

const (
	ImportCreated ImportCommitOutcome = iota
	ImportReplayed
)
