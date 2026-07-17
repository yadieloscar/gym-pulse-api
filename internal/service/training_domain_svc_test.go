package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type trainingProfileRepoStub struct {
	profile *model.TrainingProfile
	put     func(uuid.UUID, *model.TrainingProfile, int64) error
}

func (r *trainingProfileRepoStub) Get(context.Context, uuid.UUID) (*model.TrainingProfile, error) {
	if r.profile == nil {
		return nil, &model.NotFoundError{Message: "not found"}
	}
	copy := *r.profile
	return &copy, nil
}

func (r *trainingProfileRepoStub) Put(_ context.Context, userID uuid.UUID, profile *model.TrainingProfile, revision int64) error {
	if r.put != nil {
		return r.put(userID, profile, revision)
	}
	profile.Revision = revision + 1
	r.profile = profile
	return nil
}

func TestTrainingProfileServiceMergeAndValidation(t *testing.T) {
	repo := &trainingProfileRepoStub{profile: &model.TrainingProfile{
		PrimaryGoal: model.GoalStrength, AvailableDays: []int{1, 3},
		UsualActivity: "moderate", Experience: "intermediate",
		Equipment: []string{"barbell"}, SessionDurationMinutes: 60,
		Timezone: "America/New_York", Preferences: map[string]any{}, Revision: 4,
	}}
	svc := NewTrainingProfileService(repo)
	goal := model.GoalPower
	got, err := svc.Update(context.Background(), uuid.New(), model.UpdateTrainingProfileRequest{PrimaryGoal: &goal, ExpectedRevision: 4})
	if err != nil {
		t.Fatal(err)
	}
	if got.PrimaryGoal != model.GoalPower || got.AvailableDays[0] != 1 || got.Revision != 5 {
		t.Fatalf("partial update did not preserve profile: %+v", got)
	}

	unknown := "tone"
	_, err = svc.Update(context.Background(), uuid.New(), model.UpdateTrainingProfileRequest{PrimaryGoal: &unknown, ExpectedRevision: 5})
	var validationErr *model.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "primary_goal" {
		t.Fatalf("want primary_goal validation error, got %v", err)
	}
}

func TestMaterializeSnapshotsAndRequiredCompletionRules(t *testing.T) {
	weekday := 1
	reps := 5
	program := &model.Program{
		ID: uuid.New(), Name: "Strength", PrimaryGoal: model.GoalStrength,
		Workouts: []model.ProgramWorkout{{
			ID: uuid.New(), Name: "Full Body", PreferredWeekday: &weekday, SequencePosition: 1,
			Exercises: []model.ProgramExercise{{
				ID: uuid.New(), Name: "Back Squat", Category: "legs", Modality: "strength",
				ExerciseOrder: 1, TargetSets: 2, TargetReps: &reps,
			}},
		}},
	}
	workouts, err := materializeProgram(program, "2026-07-20", "2026-07-26")
	if err != nil {
		t.Fatal(err)
	}
	if len(workouts) != 1 || len(workouts[0].RequiredSets) != 2 {
		t.Fatalf("unexpected materialization: %+v", workouts)
	}
	program.Workouts[0].Exercises[0].Name = "Renamed Later"
	if workouts[0].RequiredSets[0].ExerciseName != "Back Squat" {
		t.Fatal("dated scheduled set did not retain its immutable name snapshot")
	}
	workouts[0].RequiredSets[0].Checked = true
	workouts[0].ExtraSets = []model.PerformedSet{{IsExtra: true, Completed: true}}
	if got := checkedCount(workouts[0].RequiredSets); got != 1 {
		t.Fatalf("extra set affected required completion count: %d", got)
	}
}

func TestCloneStarterDetachesMutableIDs(t *testing.T) {
	starterExerciseID := uuid.New()
	source := []model.ProgramWorkout{{ID: uuid.New(), Exercises: []model.ProgramExercise{{ID: starterExerciseID, Name: "Press"}}}}
	clone := cloneStarterWorkouts(source)
	if clone[0].ID != uuid.Nil || clone[0].Exercises[0].ID != uuid.Nil {
		t.Fatal("starter mutable ids leaked into owned copy")
	}
	if clone[0].Exercises[0].SourceStarterExerciseID == nil || *clone[0].Exercises[0].SourceStarterExerciseID != starterExerciseID {
		t.Fatal("starter provenance was not retained")
	}
}
