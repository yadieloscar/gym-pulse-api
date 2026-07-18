package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type transitionStarterStub struct{ starter model.StarterProgram }

func (s transitionStarterStub) List(context.Context, model.StarterProgramFilter) ([]model.StarterProgram, error) {
	return []model.StarterProgram{s.starter}, nil
}
func (s transitionStarterStub) Get(context.Context, uuid.UUID, int) (*model.StarterProgram, error) {
	value := s.starter
	return &value, nil
}

type transitionProgramStub struct{ program model.Program }

func (s transitionProgramStub) List(context.Context, uuid.UUID) ([]model.Program, error) {
	return []model.Program{s.program}, nil
}
func (s transitionProgramStub) Get(context.Context, uuid.UUID, uuid.UUID) (*model.Program, error) {
	value := s.program
	return &value, nil
}
func (s transitionProgramStub) Create(context.Context, uuid.UUID, *model.Program) error { return nil }
func (s transitionProgramStub) Replace(context.Context, uuid.UUID, *model.Program, int64) error {
	return nil
}
func (s transitionProgramStub) RecordLegacyAdoption(context.Context, uuid.UUID, uuid.UUID, string) error {
	return nil
}

type transitionApplyStub struct {
	calls    int
	workouts []model.ScheduledWorkout
}

func (s *transitionApplyStub) Apply(_ context.Context, _ uuid.UUID, _ int64, _ *model.TrainingProfile, target *model.Program, _, _, _, _ string) (uuid.UUID, []model.ScheduledWorkout, bool, error) {
	s.calls++
	return target.ID, s.workouts, false, nil
}

func TestGuidedTransitionPreviewIsNonPersistentAndApplyUsesExactWeekdays(t *testing.T) {
	reps, monday, thursday := 5, 1, 4
	program := model.Program{ID: uuid.New(), Name: "Alternating", PrimaryGoal: model.GoalStrength, Active: true, Revision: 3, Workouts: []model.ProgramWorkout{
		{ID: uuid.New(), Name: "A", PreferredWeekday: &monday, SequencePosition: 1, Exercises: []model.ProgramExercise{{ID: uuid.New(), Name: "Squat", Category: "legs", Modality: "strength", ExerciseOrder: 1, TargetSets: 1, TargetReps: &reps}}},
		{ID: uuid.New(), Name: "B", PreferredWeekday: &thursday, SequencePosition: 2, Exercises: []model.ProgramExercise{{ID: uuid.New(), Name: "Press", Category: "push", Modality: "strength", ExerciseOrder: 1, TargetSets: 1, TargetReps: &reps}}},
	}}
	starter := model.StarterProgram{ID: uuid.New(), Name: "Starter", PrimaryGoal: model.GoalStrength}
	apply := &transitionApplyStub{}
	svc := NewPlanTransitionService(transitionStarterStub{starter}, transitionProgramStub{program}, apply, validator.New())
	userID := uuid.New()
	req := model.PreviewPlanTransitionRequest{ProposedProfile: model.ProposedTrainingProfile{PrimaryGoal: model.GoalStrength, AvailableDays: []int{2}, UsualActivity: "light", Experience: "intermediate", Equipment: []string{"barbell"}, SessionDurationMinutes: 60, Timezone: "America/New_York", Preferences: map[string]any{}}, ProgramID: &program.ID, From: "2026-07-20", To: "2026-08-03"}
	preview, err := svc.Preview(context.Background(), userID, req)
	if err != nil {
		t.Fatal(err)
	}
	if apply.calls != 0 {
		t.Fatal("preview persisted changes")
	}
	if len(preview.ScheduledWorkouts) != 2 || preview.ScheduledWorkouts[0].Date != "2026-07-21" || preview.ScheduledWorkouts[1].Date != "2026-07-28" {
		t.Fatalf("exact weekday mapping failed: %+v", preview.ScheduledWorkouts)
	}
	apply.workouts = preview.ScheduledWorkouts
	_, err = svc.Apply(context.Background(), userID, model.ApplyPlanTransitionRequest{PreviewPlanTransitionRequest: req, PreviewToken: preview.PreviewToken, OperationKey: "apply-1", ExpectedProfileRevision: 2})
	if err != nil || apply.calls != 1 {
		t.Fatalf("confirmed transition was not applied exactly once: calls=%d err=%v", apply.calls, err)
	}
}

func TestGuidedFinalizedCorrectionsNeverReturnLiveStatus(t *testing.T) {
	sets := []model.ScheduledSet{{Checked: false}, {Checked: false}}
	if got := deriveScheduledStatus(sets, true); got != model.WorkoutStatusMissed {
		t.Fatalf("got %s", got)
	}
	sets[0].Checked = true
	if got := deriveScheduledStatus(sets, true); got != model.WorkoutStatusIncomplete {
		t.Fatalf("got %s", got)
	}
	sets[1].Checked = true
	if got := deriveScheduledStatus(sets, true); got != model.WorkoutStatusCompleted {
		t.Fatalf("got %s", got)
	}
}

func TestGuidedScheduledSetWireShapeIncludesNullableActuals(t *testing.T) {
	body, err := json.Marshal(model.ScheduledSet{})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"actual_reps", "actual_weight", "actual_duration_seconds"} {
		if !json.Valid(body) || !containsJSONKey(body, key) {
			t.Fatalf("scheduled set missing %s: %s", key, body)
		}
	}
}

func containsJSONKey(body []byte, key string) bool {
	var value map[string]any
	_ = json.Unmarshal(body, &value)
	_, ok := value[key]
	return ok
}
