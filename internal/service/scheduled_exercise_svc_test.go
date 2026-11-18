package service

import (
	"context"
	"errors"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// exerciseChangeCapture records what the service hands to the mutation store.
type exerciseChangeCapture struct {
	*adoptionMutationStub
	records []model.IdempotencyRecord
	swap    model.SwapScheduledExerciseRequest
	order   int
}

func (c *exerciseChangeCapture) SwapScheduledExercise(_ context.Context, _, workoutID uuid.UUID, order int, request model.SwapScheduledExerciseRequest, record model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error) {
	c.records, c.swap, c.order = append(c.records, record), request, order
	return &model.ScheduledWorkout{ID: workoutID, Revision: request.ExpectedRevision + 1}, false, nil
}

func (c *exerciseChangeCapture) SkipScheduledExercise(_ context.Context, _, workoutID uuid.UUID, order int, expected int64, record model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error) {
	c.records, c.order = append(c.records, record), order
	return &model.ScheduledWorkout{ID: workoutID, Revision: expected + 1}, false, nil
}

func exerciseChangeService(t *testing.T, mutations dao.TrainingMutationDAO, workouts ...model.ScheduledWorkout) ScheduleService {
	t.Helper()
	schedules := newCoverageScheduleRepo()
	for _, workout := range workouts {
		schedules.workouts[workout.ID] = workout
	}
	sessions := newCoverageSessionRepo()
	profile := coverageProfileFixture()
	return NewScheduleService(
		schedules, newCoverageProgramRepo(), &trainingProfileRepoStub{profile: &profile}, sessions,
		&coverageSetRepo{schedules: schedules, sessions: sessions}, &coverageParticipationRepo{},
		mutations, newCoverageIdempotencyRepo(), validator.New(),
	)
}

func TestSwapAndSkipExerciseReachTheMutationStore(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	workout := model.ScheduledWorkout{ID: uuid.New(), Date: "2026-11-17", Name: "Push", Status: model.WorkoutStatusInProgress, Revision: 4}
	capture := &exerciseChangeCapture{adoptionMutationStub: &adoptionMutationStub{}}
	svc := exerciseChangeService(t, capture, workout)

	swapped, err := svc.SwapExercise(ctx, userID, workout.ID, 2, model.SwapScheduledExerciseRequest{
		OperationKey: "swap-1", ExpectedRevision: 4, ExerciseName: "  Push-Up ", ExerciseCategory: " push ",
	})
	if err != nil || swapped.Revision != 5 {
		t.Fatalf("swap: %+v %v", swapped, err)
	}
	if capture.swap.ExerciseName != "Push-Up" || capture.swap.ExerciseCategory != "push" || capture.order != 2 {
		t.Fatalf("the store received %+v at order %d", capture.swap, capture.order)
	}
	skipped, err := svc.SkipExercise(ctx, userID, workout.ID, 3, model.RevisionRequest{OperationKey: "skip-1", ExpectedRevision: 5})
	if err != nil || skipped.Revision != 6 || capture.order != 3 {
		t.Fatalf("skip: %+v %v order=%d", skipped, err, capture.order)
	}
	want := []string{"scheduled-workouts/exercise-swap", "scheduled-workouts/exercise-skip"}
	for i, record := range capture.records {
		if record.Scope != want[i] || record.RequestHash == "" || record.ResponseStatus != 200 || record.ResourceType != "scheduled_workout" {
			t.Fatalf("record %d = %+v", i, record)
		}
	}
}

func TestSwapAndSkipExerciseRefuseBadRequests(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	gym := model.ScheduledWorkout{ID: uuid.New(), Date: "2026-11-17", Name: "Push", Status: model.WorkoutStatusPlanned, Revision: 1}
	sport := model.ScheduledWorkout{ID: uuid.New(), Date: "2026-11-17", Name: "Football", Kind: model.UnitKindSport, Status: model.WorkoutStatusPlanned, Revision: 1}
	capture := &exerciseChangeCapture{adoptionMutationStub: &adoptionMutationStub{}}
	svc := exerciseChangeService(t, capture, gym, sport)
	swap := model.SwapScheduledExerciseRequest{OperationKey: "k", ExpectedRevision: 1, ExerciseName: "Push-Up", ExerciseCategory: "push"}
	revision := model.RevisionRequest{OperationKey: "k", ExpectedRevision: 1}

	var validation *model.ValidationError
	blank := swap
	blank.ExerciseName = "   "
	if _, err := svc.SwapExercise(ctx, userID, gym.ID, 1, blank); !errors.As(err, &validation) {
		t.Fatalf("a blank exercise name: %v", err)
	}
	if _, err := svc.SkipExercise(ctx, userID, gym.ID, 1, model.RevisionRequest{}); !errors.As(err, &validation) {
		t.Fatalf("a skip without a revision: %v", err)
	}
	if _, err := svc.SwapExercise(ctx, userID, gym.ID, 0, swap); !errors.As(err, &validation) || validation.Field != "exercise_order" {
		t.Fatalf("exercise order zero: %v", err)
	}
	if _, err := svc.SkipExercise(ctx, userID, sport.ID, 1, revision); !errors.As(err, &validation) || validation.Field != "kind" {
		t.Fatalf("a sport session: %v", err)
	}
	var notFound *model.NotFoundError
	if _, err := svc.SwapExercise(ctx, userID, uuid.New(), 1, swap); !errors.As(err, &notFound) {
		t.Fatalf("an unknown workout: %v", err)
	}
	if len(capture.records) != 0 {
		t.Fatalf("a refused request reached the store: %+v", capture.records)
	}

	withoutStore := exerciseChangeService(t, nil, gym)
	if _, err := withoutStore.SkipExercise(ctx, userID, gym.ID, 1, revision); err == nil {
		t.Fatal("an exercise change without the mutation store succeeded")
	}
}
