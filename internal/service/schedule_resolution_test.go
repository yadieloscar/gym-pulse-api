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

func sportSessionFixture(t *testing.T, date string, status string) (*scheduleService, *coverageScheduleRepo, *coverageParticipationRepo, model.ScheduledWorkout) {
	t.Helper()
	schedules := newCoverageScheduleRepo()
	participation := &coverageParticipationRepo{}
	profiles := &trainingProfileRepoStub{profile: &model.TrainingProfile{Timezone: "UTC", AvailableDays: []int{1}}}
	svc, ok := NewScheduleService(schedules, nil, profiles, newCoverageSessionRepo(), &coverageSetRepo{}, participation, nil, newCoverageIdempotencyRepo(), validator.New()).(*scheduleService)
	if !ok {
		t.Fatal("NewScheduleService must return *scheduleService")
	}
	svc.now = func() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) }
	ride := model.ScheduledWorkout{Kind: model.UnitKindSport, Date: date, Name: "Long ride", Status: status,
		Sport:        &model.ScheduledSport{SportTarget: model.SportTarget{SportID: "cycling", SportName: "Cycling", TargetDurationMinutes: 90}},
		RequiredSets: []model.ScheduledSet{}, ExtraSets: []model.PerformedSet{}}
	if err := schedules.Create(context.Background(), uuid.New(), []model.ScheduledWorkout{ride}); err != nil {
		t.Fatal(err)
	}
	for _, w := range schedules.workouts {
		ride = w
	}
	return svc, schedules, participation, ride
}

func TestLazyFinalizeMarksUnfulfilledPastSportSessionMissed(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	svc, schedules, participation, ride := sportSessionFixture(t, "2026-10-28", model.WorkoutStatusPlanned)
	listed, err := svc.List(ctx, userID, "2026-10-26", "2026-11-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Status != model.WorkoutStatusMissed || listed[0].FinalizedAt == nil || listed[0].Resolution != nil {
		t.Fatalf("past sport session must finalize as unresolved missed: %+v", listed)
	}
	if len(participation.outcomes) != 1 || !participation.outcomes[0].ScheduledOpportunity || participation.outcomes[0].Participated || participation.outcomes[0].Date != "2026-10-28" {
		t.Fatalf("missed sport session must record an unparticipated opportunity: %+v", participation.outcomes)
	}
	// A future planned session is untouched.
	future := ride
	future.ID, future.Date = uuid.New(), "2026-11-02"
	schedules.workouts[future.ID] = future
	listed, err = svc.List(ctx, userID, "2026-11-02", "2026-11-02")
	if err != nil {
		t.Fatal(err)
	}
	if listed[0].Status != model.WorkoutStatusPlanned || listed[0].FinalizedAt != nil {
		t.Fatalf("future sport session must stay planned: %+v", listed[0])
	}
}

func TestSportSessionCompletedByLinkedActivityStaysCompleted(t *testing.T) {
	activityID := uuid.New()
	done := &model.ScheduledWorkout{Kind: model.UnitKindSport, Status: model.WorkoutStatusPlanned,
		Sport: &model.ScheduledSport{SportTarget: model.SportTarget{SportID: "cycling", SportName: "Cycling", TargetDurationMinutes: 90}, SportActivityID: &activityID}}
	for _, finalized := range []bool{false, true} {
		if got := deriveWorkoutStatus(done, finalized); got != model.WorkoutStatusCompleted {
			t.Fatalf("finalized=%v status=%s want completed", finalized, got)
		}
	}
	open := &model.ScheduledWorkout{Kind: model.UnitKindSport, Status: model.WorkoutStatusPlanned, Sport: &model.ScheduledSport{}}
	if got := deriveWorkoutStatus(open, false); got != model.WorkoutStatusPlanned {
		t.Fatalf("open unfinalized sport session status=%s want planned", got)
	}
	if got := deriveWorkoutStatus(open, true); got != model.WorkoutStatusMissed {
		t.Fatalf("open finalized sport session status=%s want missed", got)
	}
	checked := &model.ScheduledWorkout{RequiredSets: []model.ScheduledSet{{Checked: true}}}
	if got := deriveWorkoutStatus(checked, false); got != model.WorkoutStatusInProgress {
		t.Fatalf("exercise derivation changed: %s", got)
	}
}

func TestSetAndCompletionRoutesRejectSportSessions(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	svc, _, _, ride := sportSessionFixture(t, "2026-11-02", model.WorkoutStatusPlanned)
	sets := []model.ScheduledSet{{ExerciseName: "Push-Up", ExerciseCategory: "push", ExerciseModality: "strength", ExerciseOrder: 1, SetIndex: 1}}
	attempts := map[string]func() error{
		"complete": func() error {
			_, err := svc.Complete(ctx, userID, ride.ID, model.RevisionRequest{OperationKey: "c", ExpectedRevision: ride.Revision})
			return err
		},
		"required set": func() error {
			_, err := svc.PutRequiredSet(ctx, userID, ride.ID, uuid.New(), model.SetMutationRequest{OperationKey: "s", ExpectedRevision: ride.Revision, Completed: true})
			return err
		},
		"extra set": func() error {
			_, err := svc.AddExtraSet(ctx, userID, ride.ID, model.ExtraSetRequest{OperationKey: "e", ExpectedRevision: ride.Revision, ExerciseName: "Push-Up", ExerciseCategory: "push", ExerciseModality: "strength", SetIndex: 1, Completed: true})
			return err
		},
		"set target": func() error {
			_, err := svc.PatchSetTarget(ctx, userID, ride.ID, uuid.New(), model.PatchScheduledSetTargetRequest{OperationKey: "t", ExpectedRevision: ride.Revision})
			return err
		},
		"patch required sets": func() error {
			_, err := svc.PatchWorkout(ctx, userID, ride.ID, model.PatchScheduledWorkoutRequest{OperationKey: "p", ExpectedRevision: ride.Revision, RequiredSets: &sets})
			return err
		},
	}
	for name, attempt := range attempts {
		err := attempt()
		var ve *model.ValidationError
		if !errors.As(err, &ve) || ve.Field != "kind" {
			t.Errorf("%s on a sport session: want 422 on kind, got %v", name, err)
		}
	}
	name := "Weekend ride"
	renamed, err := svc.PatchWorkout(ctx, userID, ride.ID, model.PatchScheduledWorkoutRequest{OperationKey: "n", ExpectedRevision: ride.Revision, Name: &name})
	if err != nil || renamed.Name != name || renamed.Sport == nil || renamed.Sport.SportID != "cycling" || len(renamed.RequiredSets) != 0 {
		t.Fatalf("renaming a sport session must keep its snapshot: %+v err=%v", renamed, err)
	}
	_, err = svc.Complete(ctx, userID, uuid.New(), model.RevisionRequest{OperationKey: "c", ExpectedRevision: 1})
	var notFound *model.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("foreign ids keep the shared 404: %v", err)
	}
}

func TestSkipAndUnskipTreatSportSessionsLikeWorkouts(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	mutations := newScheduleMutationCapture()
	svc := NewScheduleService(newCoverageScheduleRepo(), nil, nil, nil, nil, nil, mutations, newCoverageIdempotencyRepo(), validator.New())
	rideID := uuid.New()
	skipped, err := svc.Skip(ctx, userID, rideID, model.RevisionRequest{OperationKey: "skip-ride", ExpectedRevision: 3})
	if err != nil || skipped.Resolution == nil {
		t.Fatalf("skip = %+v, %v", skipped, err)
	}
	unskipped, err := svc.Unskip(ctx, userID, rideID, model.RevisionRequest{OperationKey: "unskip-ride", ExpectedRevision: 4})
	if err != nil || unskipped.Resolution != nil {
		t.Fatalf("unskip = %+v, %v", unskipped, err)
	}
	if len(mutations.records) != 2 {
		t.Fatalf("skip and unskip must delegate regardless of kind: %d records", len(mutations.records))
	}
}
