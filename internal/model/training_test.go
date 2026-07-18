package model

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func validTrainingProfile() TrainingProfile {
	return TrainingProfile{
		PrimaryGoal:            GoalStrength,
		AvailableDays:          []int{1, 3, 5},
		UsualActivity:          "moderate",
		Experience:             "intermediate",
		Equipment:              []string{"barbell", "dumbbells"},
		SessionDurationMinutes: 60,
		Timezone:               "America/New_York",
		Preferences:            map[string]any{},
	}
}

func TestTrainingProfileValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TrainingProfile)
		field  string
	}{
		{name: "valid", mutate: func(*TrainingProfile) {}},
		{name: "unknown goal", mutate: func(p *TrainingProfile) { p.PrimaryGoal = "tone" }, field: "primary_goal"},
		{name: "unknown activity", mutate: func(p *TrainingProfile) { p.UsualActivity = "extreme" }, field: "usual_activity"},
		{name: "unknown experience", mutate: func(p *TrainingProfile) { p.Experience = "expert" }, field: "experience"},
		{name: "empty weekdays", mutate: func(p *TrainingProfile) { p.AvailableDays = nil }, field: "available_days"},
		{name: "weekday out of range", mutate: func(p *TrainingProfile) { p.AvailableDays = []int{0, 2} }, field: "available_days"},
		{name: "duplicate weekday", mutate: func(p *TrainingProfile) { p.AvailableDays = []int{1, 1} }, field: "available_days"},
		{name: "unknown equipment", mutate: func(p *TrainingProfile) { p.Equipment = []string{"kettlebell"} }, field: "equipment"},
		{name: "duplicate equipment", mutate: func(p *TrainingProfile) { p.Equipment = []string{"bands", "bands"} }, field: "equipment"},
		{name: "duration too short", mutate: func(p *TrainingProfile) { p.SessionDurationMinutes = 19 }, field: "session_duration_minutes"},
		{name: "duration too long", mutate: func(p *TrainingProfile) { p.SessionDurationMinutes = 121 }, field: "session_duration_minutes"},
		{name: "invalid IANA timezone", mutate: func(p *TrainingProfile) { p.Timezone = "Eastern" }, field: "timezone"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			profile := validTrainingProfile()
			tc.mutate(&profile)
			err := profile.Validate()
			if tc.field == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("want ValidationError, got %T: %v", err, err)
			}
			if validationErr.Field != tc.field {
				t.Errorf("field=%q want %q", validationErr.Field, tc.field)
			}
		})
	}
}

func TestWorkoutStatusValidation(t *testing.T) {
	for _, status := range ValidWorkoutStatuses {
		if !IsValidWorkoutStatus(status) {
			t.Errorf("canonical status %q was rejected", status)
		}
	}
	if IsValidWorkoutStatus("done") {
		t.Error("unknown status was accepted")
	}
}

func TestRequiredAndExtraSetRepresentations(t *testing.T) {
	requiredID := uuid.New()
	required := PerformedSet{
		ScheduledSetID:   &requiredID,
		IsExtra:          false,
		ExerciseName:     "Back Squat",
		ExerciseCategory: "legs",
		ExerciseModality: "strength",
		SetIndex:         1,
	}
	if err := required.Validate(); err != nil {
		t.Fatalf("required set rejected: %v", err)
	}

	extra := PerformedSet{
		IsExtra:          true,
		ExerciseName:     "Push-Up",
		ExerciseCategory: "push",
		ExerciseModality: "strength",
		SetIndex:         1,
	}
	if err := extra.Validate(); err != nil {
		t.Fatalf("extra set rejected: %v", err)
	}

	extra.ScheduledSetID = &requiredID
	if err := extra.Validate(); err == nil {
		t.Fatal("extra set was allowed to reference a required scheduled set")
	}
}

func TestSetSnapshotsSurviveNullableProvenance(t *testing.T) {
	set := PerformedSet{
		ExerciseID:       nil,
		IsExtra:          true,
		ExerciseName:     "Retired Exercise",
		ExerciseCategory: "other",
		ExerciseModality: "strength",
		SetIndex:         1,
	}
	if err := set.Validate(); err != nil {
		t.Fatalf("snapshot without mutable provenance was rejected: %v", err)
	}
	body, err := json.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["exercise_name"] != "Retired Exercise" || wire["exercise_category"] != "other" {
		t.Fatalf("immutable snapshot missing from JSON: %s", body)
	}
	if _, exists := wire["exercise_id"]; exists {
		t.Fatalf("nil provenance should be omitted: %s", body)
	}
}

func TestScheduledWorkoutRejectsExtraSetAsRequired(t *testing.T) {
	requiredID := uuid.New()
	w := ScheduledWorkout{
		Status: WorkoutStatusPlanned,
		ExtraSets: []PerformedSet{{
			ScheduledSetID:   &requiredID,
			IsExtra:          true,
			ExerciseName:     "Bench Press",
			ExerciseCategory: "push",
			ExerciseModality: "strength",
		}},
	}
	if err := w.Validate(); err == nil {
		t.Fatal("extra set was allowed to satisfy required completion")
	}
}
