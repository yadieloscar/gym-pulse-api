package model

import (
	"encoding/json"
	"errors"
	"strings"
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

func TestValidateDateRangeInclusiveLimit(t *testing.T) {
	tests := []struct {
		name      string
		from      string
		to        string
		wantField string
	}{
		{name: "one day", from: "2026-01-01", to: "2026-01-01"},
		{name: "366 inclusive days", from: "2026-01-01", to: "2027-01-01"},
		{name: "leap day within limit", from: "2028-01-01", to: "2028-12-31"},
		{name: "367 inclusive days", from: "2026-01-01", to: "2027-01-02", wantField: "range"},
		{name: "reversed", from: "2026-01-02", to: "2026-01-01", wantField: "to"},
		{name: "bad from", from: "01/01/2026", to: "2026-01-02", wantField: "from"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateDateRange(tc.from, tc.to)
			if tc.wantField == "" {
				if err != nil {
					t.Fatalf("ValidateDateRange() error = %v", err)
				}
				return
			}
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("want ValidationError, got %T: %v", err, err)
			}
			if validationErr.Field != tc.wantField {
				t.Fatalf("field = %q, want %q", validationErr.Field, tc.wantField)
			}
		})
	}
}

func TestUnitKindDefaultsToExercisesForLegacyJSON(t *testing.T) {
	var workout ProgramWorkout
	if err := json.Unmarshal([]byte(`{"name":"Full Body A","sequence_position":1,"exercises":[]}`), &workout); err != nil {
		t.Fatal(err)
	}
	if workout.Kind != "" || NormalizeUnitKind(workout.Kind) != UnitKindExercises || workout.IsSport() {
		t.Fatalf("legacy program workout must normalize to exercises: %+v", workout)
	}
	var scheduled ScheduledWorkout
	if err := json.Unmarshal([]byte(`{"date":"2026-10-31","name":"Full Body A","status":"planned","revision":1,"required_sets":[],"extra_sets":[]}`), &scheduled); err != nil {
		t.Fatal(err)
	}
	if scheduled.IsSport() || scheduled.Sport != nil {
		t.Fatalf("legacy scheduled workout must not be a sport: %+v", scheduled)
	}
	encoded, err := json.Marshal(ProgramWorkout{Kind: UnitKindExercises, Name: "Full Body A", SequencePosition: 1, Exercises: []ProgramExercise{}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"kind":"exercises"`) || strings.Contains(string(encoded), `"sport"`) {
		t.Fatalf("exercises unit must encode kind and omit sport: %s", encoded)
	}
}

func TestSportUnitJSONShape(t *testing.T) {
	distance := 40000
	unit := ProgramWorkout{Kind: UnitKindSport, Name: "Long ride", SequencePosition: 3, Sport: &SportTarget{SportID: "cycling", SportName: "Cycling", TargetDurationMinutes: 90, TargetDistanceMeters: &distance}, Exercises: []ProgramExercise{}}
	encoded, err := json.Marshal(unit)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"kind":"sport"`, `"sport":{"sport_id":"cycling","sport_name":"Cycling","target_duration_minutes":90,"target_distance_meters":40000}`, `"exercises":[]`} {
		if !strings.Contains(string(encoded), want) {
			t.Errorf("sport unit JSON missing %s: %s", want, encoded)
		}
	}
	open := ScheduledWorkout{Kind: UnitKindSport, Status: WorkoutStatusPlanned, Sport: &ScheduledSport{SportTarget: unit.Sport.Copy()}, RequiredSets: []ScheduledSet{}, ExtraSets: []PerformedSet{}}
	if err := open.Validate(); err != nil {
		t.Fatalf("open sport session must validate: %v", err)
	}
	encodedOpen, err := json.Marshal(open)
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"sport_activity_id", "performed_date"} {
		if strings.Contains(string(encodedOpen), absent) {
			t.Errorf("open sport session must omit %s: %s", absent, encodedOpen)
		}
	}
	activityID, performed := uuid.New(), "2026-10-31"
	done := open
	done.Status = WorkoutStatusCompleted
	done.Sport = &ScheduledSport{SportTarget: unit.Sport.Copy(), SportActivityID: &activityID, PerformedDate: &performed}
	encodedDone, err := json.Marshal(done)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encodedDone), `"performed_date":"2026-10-31"`) || !strings.Contains(string(encodedDone), `"sport_activity_id":"`+activityID.String()+`"`) {
		t.Fatalf("completed sport session must carry the link: %s", encodedDone)
	}
}

func TestSportSessionValidationRejectsSetsAndLiveStatuses(t *testing.T) {
	sport := &ScheduledSport{SportTarget: SportTarget{SportID: "volleyball", SportName: "Volleyball", TargetDurationMinutes: 120}}
	cases := []struct {
		name  string
		w     ScheduledWorkout
		field string
	}{
		{name: "missing snapshot", w: ScheduledWorkout{Kind: UnitKindSport, Status: WorkoutStatusPlanned}, field: "sport"},
		{name: "required sets", w: ScheduledWorkout{Kind: UnitKindSport, Status: WorkoutStatusPlanned, Sport: sport, RequiredSets: []ScheduledSet{{ExerciseName: "x", ExerciseCategory: "y", ExerciseModality: "strength", ExerciseOrder: 1, SetIndex: 1}}}, field: "kind"},
		{name: "in progress", w: ScheduledWorkout{Kind: UnitKindSport, Status: WorkoutStatusInProgress, Sport: sport}, field: "status"},
		{name: "incomplete", w: ScheduledWorkout{Kind: UnitKindSport, Status: WorkoutStatusIncomplete, Sport: sport}, field: "status"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.w.Validate()
			var ve *ValidationError
			if !errors.As(err, &ve) || ve.Field != tc.field {
				t.Fatalf("want validation error on %s, got %v", tc.field, err)
			}
		})
	}
	for _, status := range []string{WorkoutStatusPlanned, WorkoutStatusCompleted, WorkoutStatusMissed} {
		if err := (ScheduledWorkout{Kind: UnitKindSport, Status: status, Sport: sport}).Validate(); err != nil {
			t.Errorf("status %s must validate: %v", status, err)
		}
	}
}
