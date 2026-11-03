package model

import (
	"time"

	"github.com/google/uuid"
)

// ExportSchemaVersion identifies the shape of DataExport. It increases only
// when a field is removed or changes meaning; new fields and sections are
// additive and keep the version.
const ExportSchemaVersion = 1

// DataExport is everything the athlete authored or performed, read from one
// consistent snapshot. Every collection is an array, never null.
type DataExport struct {
	SchemaVersion   int                    `json:"schema_version"`
	ExportedAt      time.Time              `json:"exported_at"`
	Profile         *ExportProfile         `json:"profile"`
	Settings        *ExportSettings        `json:"settings"`
	TrainingProfile *ExportTrainingProfile `json:"training_profile"`
	Workouts        []ExportWorkout        `json:"workouts"`
	Templates       []ExportTemplate       `json:"templates"`
	SportActivities []ExportSportActivity  `json:"sport_activities"`
	BodyWeights     []ExportBodyWeight     `json:"body_weights"`
	Nutrition       ExportNutrition        `json:"nutrition"`
}

// ExportProfile is the athlete's own profile; absent until it is created.
type ExportProfile struct {
	DisplayName *string   `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
}

// ExportSettings carries the preferences needed to read the export, notably
// the weight unit set weights are recorded in.
type ExportSettings struct {
	WeightUnit string `json:"weight_unit"`
	WeeklyGoal int    `json:"weekly_goal"`
}

// ExportTrainingProfile is the saved goal-based training profile.
type ExportTrainingProfile struct {
	PrimaryGoal            string   `json:"primary_goal"`
	AvailableDays          []int16  `json:"available_days"`
	UsualActivity          string   `json:"usual_activity"`
	Experience             string   `json:"experience"`
	Equipment              []string `json:"equipment"`
	SessionDurationMinutes int      `json:"session_duration_minutes"`
	Timezone               string   `json:"timezone"`
}

// ExportWorkoutSource distinguishes the two places performed workouts live.
type ExportWorkoutSource string

const (
	// ExportWorkoutSession is a goal-based workout session, planned or off-plan.
	ExportWorkoutSession ExportWorkoutSource = "session"
	// ExportWorkoutDayLog is a legacy day log.
	ExportWorkoutDayLog ExportWorkoutSource = "day_log"
)

// ExportWorkout is one workout with its sets in exercise and set order.
type ExportWorkout struct {
	ID          uuid.UUID           `json:"id"`
	Source      ExportWorkoutSource `json:"source"`
	Date        string              `json:"date"`
	Name        string              `json:"name"`
	Status      string              `json:"status"`
	Planned     bool                `json:"planned"`
	Notes       *string             `json:"notes"`
	StartedAt   *time.Time          `json:"started_at"`
	CompletedAt *time.Time          `json:"completed_at"`
	Sets        []ExportSet         `json:"sets"`
	// Exercises carries legacy per-exercise summaries recorded before sets
	// were logged one by one. It is always empty for sessions.
	Exercises []ExportExerciseSummary `json:"exercises"`
}

// ExportSet is one recorded set.
type ExportSet struct {
	ExerciseName     string   `json:"exercise_name"`
	ExerciseCategory string   `json:"exercise_category"`
	ExerciseModality string   `json:"exercise_modality"`
	SetIndex         int      `json:"set_index"`
	TargetReps       *int     `json:"target_reps"`
	TargetWeight     *float64 `json:"target_weight"`
	ActualReps       *int     `json:"actual_reps"`
	ActualWeight     *float64 `json:"actual_weight"`
	DurationSeconds  *int     `json:"duration_seconds"`
	Completed        bool     `json:"completed"`
	Extra            bool     `json:"extra"`
}

// ExportExerciseSummary is a legacy aggregate exercise result.
type ExportExerciseSummary struct {
	ExerciseName string   `json:"exercise_name"`
	ActualSets   *int     `json:"actual_sets"`
	ActualReps   *int     `json:"actual_reps"`
	ActualWeight *float64 `json:"actual_weight"`
	Notes        *string  `json:"notes"`
	Skipped      bool     `json:"skipped"`
}

// ExportTemplate is a saved workout with its exercises in order.
type ExportTemplate struct {
	ID        uuid.UUID                `json:"id"`
	Name      string                   `json:"name"`
	TypeID    string                   `json:"type_id"`
	SubtypeID string                   `json:"subtype_id"`
	CreatedAt time.Time                `json:"created_at"`
	UpdatedAt time.Time                `json:"updated_at"`
	Exercises []ExportTemplateExercise `json:"exercises"`
}

// ExportTemplateExercise is one prescribed exercise in a template.
type ExportTemplateExercise struct {
	Name            string   `json:"name"`
	Sets            *int     `json:"sets"`
	Reps            *int     `json:"reps"`
	Weight          *float64 `json:"weight"`
	RestSeconds     *int     `json:"rest_seconds"`
	DurationMinutes *int     `json:"duration_minutes"`
	Intensity       *string  `json:"intensity"`
	Notes           *string  `json:"notes"`
}

// ExportSportActivity is a recorded sport session.
type ExportSportActivity struct {
	ID              uuid.UUID `json:"id"`
	Date            string    `json:"date"`
	SportID         string    `json:"sport_id"`
	SportName       string    `json:"sport_name"`
	DurationMinutes int       `json:"duration_minutes"`
	PerceivedEffort *int      `json:"perceived_effort"`
	DistanceMeters  *int      `json:"distance_meters"`
	EnergyKcal      *int      `json:"energy_kcal"`
	AvgHeartRateBPM *int      `json:"avg_heart_rate_bpm"`
	Source          string    `json:"source"`
	Notes           *string   `json:"notes"`
}

// ExportBodyWeight is one body-weight measurement in the unit it was entered.
type ExportBodyWeight struct {
	Date   string  `json:"date"`
	Weight float64 `json:"weight"`
	Unit   string  `json:"unit"`
}

// ExportNutrition groups the athlete's daily totals, targets, and food log.
type ExportNutrition struct {
	Days        []ExportNutritionDay    `json:"days"`
	Targets     []ExportNutritionTarget `json:"targets"`
	FoodEntries []ExportFoodEntry       `json:"food_entries"`
	OwnFoods    []ExportOwnFood         `json:"own_foods"`
}

// ExportNutritionDay is one day's totals.
type ExportNutritionDay struct {
	Date       string   `json:"date"`
	EnergyKcal *int     `json:"energy_kcal"`
	ProteinG   *float64 `json:"protein_g"`
	CarbsG     *float64 `json:"carbs_g"`
	FatG       *float64 `json:"fat_g"`
	Source     string   `json:"source"`
}

// ExportNutritionTarget is one target and the first date it applies to.
type ExportNutritionTarget struct {
	EffectiveFrom string   `json:"effective_from"`
	ProteinG      float64  `json:"protein_g"`
	EnergyKcal    *int     `json:"energy_kcal"`
	CarbsG        *float64 `json:"carbs_g"`
	FatG          *float64 `json:"fat_g"`
}

// ExportFoodEntry is one logged food or quick add with its stored snapshot.
type ExportFoodEntry struct {
	Date         string    `json:"date"`
	Kind         string    `json:"kind"`
	FoodName     *string   `json:"food_name"`
	ServingLabel *string   `json:"serving_label"`
	ServingG     *float64  `json:"serving_g"`
	Amount       *float64  `json:"amount"`
	EnergyKcal   *int      `json:"energy_kcal"`
	ProteinG     *float64  `json:"protein_g"`
	CarbsG       *float64  `json:"carbs_g"`
	FatG         *float64  `json:"fat_g"`
	LoggedAt     time.Time `json:"logged_at"`
}

// ExportOwnFood is a food the athlete created, archived ones included.
type ExportOwnFood struct {
	Name         string  `json:"name"`
	Brand        *string `json:"brand"`
	ServingLabel string  `json:"serving_label"`
	ServingG     float64 `json:"serving_g"`
	EnergyKcal   int     `json:"energy_kcal"`
	ProteinG     float64 `json:"protein_g"`
	CarbsG       float64 `json:"carbs_g"`
	FatG         float64 `json:"fat_g"`
	Archived     bool    `json:"archived"`
}

// NewDataExport returns an export whose collections are empty arrays.
func NewDataExport(exportedAt time.Time) *DataExport {
	return &DataExport{
		SchemaVersion:   ExportSchemaVersion,
		ExportedAt:      exportedAt,
		Workouts:        []ExportWorkout{},
		Templates:       []ExportTemplate{},
		SportActivities: []ExportSportActivity{},
		BodyWeights:     []ExportBodyWeight{},
		Nutrition: ExportNutrition{
			Days:        []ExportNutritionDay{},
			Targets:     []ExportNutritionTarget{},
			FoodEntries: []ExportFoodEntry{},
			OwnFoods:    []ExportOwnFood{},
		},
	}
}
