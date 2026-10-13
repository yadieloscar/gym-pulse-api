package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func TestRecoverFallbackIgnoresResolvedMisses(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	schedules := newCoverageScheduleRepo()
	idempotency := newCoverageIdempotencyRepo()
	svc := NewScheduleService(schedules, nil, nil, nil, nil, nil, nil, idempotency, validator.New())

	skipped := model.ResolutionSkipped
	resolvedAt := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	first, second := 1, 2
	workouts := []model.ScheduledWorkout{
		{ID: uuid.New(), Date: "2026-07-17", Name: "Skipped", SequencePosition: &first, Status: model.WorkoutStatusMissed, Resolution: &skipped, ResolvedAt: &resolvedAt, Revision: 2, RequiredSets: []model.ScheduledSet{}},
		{ID: uuid.New(), Date: "2026-07-18", Name: "Unresolved", SequencePosition: &second, Status: model.WorkoutStatusMissed, Revision: 1, RequiredSets: []model.ScheduledSet{}},
	}
	if err := schedules.Create(ctx, userID, workouts); err != nil {
		t.Fatal(err)
	}

	recovered, err := svc.RecoverToday(ctx, userID, model.RecoverScheduledWorkoutRequest{Date: "2026-07-20", OperationKey: "recover-1"})
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Name != "Unresolved" {
		t.Fatalf("recovered %q, want the earliest unresolved miss", recovered.Name)
	}
	if recovered.Resolution != nil || recovered.ResolvedAt != nil {
		t.Fatalf("recovered copy carries a resolution: %+v", recovered)
	}
}

func TestRecoverFallbackReportsNotFoundWhenEveryMissIsResolved(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	schedules := newCoverageScheduleRepo()
	svc := NewScheduleService(schedules, nil, nil, nil, nil, nil, nil, newCoverageIdempotencyRepo(), validator.New())

	recovered := model.ResolutionRecovered
	resolvedAt := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	if err := schedules.Create(ctx, userID, []model.ScheduledWorkout{
		{ID: uuid.New(), Date: "2026-07-17", Name: "Recovered", Status: model.WorkoutStatusMissed, Resolution: &recovered, ResolvedAt: &resolvedAt, Revision: 2, RequiredSets: []model.ScheduledSet{}},
	}); err != nil {
		t.Fatal(err)
	}

	_, err := svc.RecoverToday(ctx, userID, model.RecoverScheduledWorkoutRequest{Date: "2026-07-20", OperationKey: "recover-2"})
	var notFound *model.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("err = %v, want NotFoundError", err)
	}
}

func TestSkipAndUnskipDelegateWithScopedIdempotency(t *testing.T) {
	ctx := context.Background()
	userID, workoutID := uuid.New(), uuid.New()
	mutations := newScheduleMutationCapture()
	svc := NewScheduleService(newCoverageScheduleRepo(), nil, nil, nil, nil, nil, mutations, newCoverageIdempotencyRepo(), validator.New())

	skipped, err := svc.Skip(ctx, userID, workoutID, model.RevisionRequest{OperationKey: "skip-1", ExpectedRevision: 1})
	if err != nil || skipped.Resolution == nil || *skipped.Resolution != model.ResolutionSkipped {
		t.Fatalf("skip = %+v, %v", skipped, err)
	}
	unskipped, err := svc.Unskip(ctx, userID, workoutID, model.RevisionRequest{OperationKey: "unskip-1", ExpectedRevision: 2})
	if err != nil || unskipped.Resolution != nil {
		t.Fatalf("unskip = %+v, %v", unskipped, err)
	}

	if len(mutations.records) != 2 {
		t.Fatalf("records = %d, want 2", len(mutations.records))
	}
	for i, want := range []string{"scheduled-workouts/skip", "scheduled-workouts/unskip"} {
		got := mutations.records[i]
		if got.Scope != want || got.ResponseStatus != 200 || got.ResourceType != "scheduled_workout" || got.RequestHash == "" {
			t.Fatalf("record %d = %+v, want scope %s", i, got, want)
		}
	}

	// The hash binds the workout, so the same key and body for another workout differ.
	if _, err := svc.Skip(ctx, userID, uuid.New(), model.RevisionRequest{OperationKey: "skip-1", ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	if mutations.records[0].RequestHash == mutations.records[2].RequestHash {
		t.Fatal("request hash does not include the workout id")
	}
}

func TestSkipAndUnskipValidateRequests(t *testing.T) {
	ctx := context.Background()
	svc := NewScheduleService(newCoverageScheduleRepo(), nil, nil, nil, nil, nil, newScheduleMutationCapture(), newCoverageIdempotencyRepo(), validator.New())
	var validation *model.ValidationError
	if _, err := svc.Skip(ctx, uuid.New(), uuid.New(), model.RevisionRequest{}); !errors.As(err, &validation) {
		t.Fatalf("skip err = %v, want ValidationError", err)
	}
	if _, err := svc.Unskip(ctx, uuid.New(), uuid.New(), model.RevisionRequest{}); !errors.As(err, &validation) {
		t.Fatalf("unskip err = %v, want ValidationError", err)
	}

	legacy := NewScheduleService(newCoverageScheduleRepo(), nil, nil, nil, nil, nil, nil, newCoverageIdempotencyRepo(), validator.New())
	if _, err := legacy.Skip(ctx, uuid.New(), uuid.New(), model.RevisionRequest{OperationKey: "skip", ExpectedRevision: 1}); !errors.Is(err, ErrResolutionUnavailable) {
		t.Fatalf("skip without a mutation DAO err = %v", err)
	}
}
