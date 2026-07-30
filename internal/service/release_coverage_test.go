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

func newReleaseScheduleService(t *testing.T) (*scheduleService, uuid.UUID, model.Program, *coverageScheduleRepo, *coverageSessionRepo, *coverageIdempotencyRepo) {
	t.Helper()
	userID := uuid.New()
	program := coverageProgramFixture()
	schedules := newCoverageScheduleRepo()
	sessions := newCoverageSessionRepo()
	idempotency := newCoverageIdempotencyRepo()
	profile := coverageProfileFixture()
	contract := NewScheduleService(
		schedules,
		newCoverageProgramRepo(program),
		&trainingProfileRepoStub{profile: &profile},
		sessions,
		&coverageSetRepo{schedules: schedules, sessions: sessions},
		&coverageParticipationRepo{},
		idempotency,
		validator.New(),
	)
	svc, ok := contract.(*scheduleService)
	if !ok {
		t.Fatal("unexpected schedule service implementation")
	}
	svc.now = func() time.Time { return time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC) }
	return svc, userID, program, schedules, sessions, idempotency
}

func TestScheduleServiceRejectsInvalidAndStaleReleaseMutations(t *testing.T) {
	ctx := context.Background()

	t.Run("list date range", func(t *testing.T) {
		svc, userID, _, _, _, _ := newReleaseScheduleService(t)
		if _, err := svc.List(ctx, userID, "2026-07-21", "2026-07-20"); err == nil {
			t.Fatal("expected invalid date range")
		}
	})

	t.Run("materialize validation range and revision", func(t *testing.T) {
		svc, userID, program, _, _, _ := newReleaseScheduleService(t)
		if _, err := svc.Materialize(ctx, userID, model.MaterializeScheduleRequest{}); err == nil {
			t.Fatal("expected request validation error")
		}
		badRange := model.MaterializeScheduleRequest{
			ProgramID: program.ID, From: "2026-07-21", To: "2026-07-20",
			OperationKey: "bad-range", ExpectedRevision: program.Revision,
		}
		if _, err := svc.Materialize(ctx, userID, badRange); err == nil {
			t.Fatal("expected date range error")
		}
		stale := model.MaterializeScheduleRequest{
			ProgramID: program.ID, From: "2026-07-20", To: "2026-07-26",
			OperationKey: "stale-program", ExpectedRevision: program.Revision + 1,
		}
		if _, err := svc.Materialize(ctx, userID, stale); err == nil {
			t.Fatal("expected program revision conflict")
		}
	})

	t.Run("regenerate validation stale preview and active session", func(t *testing.T) {
		svc, userID, program, _, sessions, _ := newReleaseScheduleService(t)
		if _, err := svc.Regenerate(ctx, userID, model.RegenerateScheduleRequest{}); err == nil {
			t.Fatal("expected request validation error")
		}
		badRange := model.RegenerateScheduleRequest{
			ProgramID: program.ID, From: "2026-07-21", To: "2026-07-20",
			OperationKey: "bad-range", ExpectedRevision: program.Revision,
		}
		if _, err := svc.Regenerate(ctx, userID, badRange); err == nil {
			t.Fatal("expected date range error")
		}
		staleRevision := model.RegenerateScheduleRequest{
			ProgramID: program.ID, From: "2026-07-20", To: "2026-07-26",
			OperationKey: "stale-revision", ExpectedRevision: program.Revision + 1,
		}
		if _, err := svc.Regenerate(ctx, userID, staleRevision); err == nil {
			t.Fatal("expected program revision conflict")
		}
		request := model.RegenerateScheduleRequest{
			ProgramID: program.ID, From: "2026-07-20", To: "2026-07-26",
			OperationKey: "regenerate", ExpectedRevision: program.Revision,
		}
		preview, err := svc.Regenerate(ctx, userID, request)
		if err != nil {
			t.Fatal(err)
		}
		request.Apply = true
		request.PreviewToken = "stale"
		if _, err := svc.Regenerate(ctx, userID, request); err == nil {
			t.Fatal("expected stale preview conflict")
		}
		sessions.sessions[uuid.New()] = model.WorkoutSession{
			ID: uuid.New(), Date: "2026-07-20", Name: "Live", Status: "active", Revision: 1,
		}
		request.PreviewToken = preview.PreviewToken
		if _, err := svc.Regenerate(ctx, userID, request); err == nil {
			t.Fatal("expected active session conflict")
		}
	})

	t.Run("workout patch validation historical and invalid snapshot", func(t *testing.T) {
		svc, userID, _, schedules, _, _ := newReleaseScheduleService(t)
		if _, err := svc.PatchWorkout(ctx, userID, uuid.New(), model.PatchScheduledWorkoutRequest{}); err == nil {
			t.Fatal("expected patch validation error")
		}
		historical := model.ScheduledWorkout{
			ID: uuid.New(), Date: "2026-07-19", Name: "Past", Status: model.WorkoutStatusPlanned, Revision: 1,
		}
		schedules.workouts[historical.ID] = historical
		name := "Changed"
		if _, err := svc.PatchWorkout(ctx, userID, historical.ID, model.PatchScheduledWorkoutRequest{
			Name: &name, OperationKey: "historical", ExpectedRevision: 1,
		}); err == nil {
			t.Fatal("expected historical workout conflict")
		}
		current := historical
		current.ID = uuid.New()
		current.Date = "2026-07-20"
		schedules.workouts[current.ID] = current
		invalidSets := []model.ScheduledSet{{ExerciseName: "Squat"}}
		if _, err := svc.PatchWorkout(ctx, userID, current.ID, model.PatchScheduledWorkoutRequest{
			RequiredSets: &invalidSets, OperationKey: "invalid-snapshot", ExpectedRevision: 1,
		}); err == nil {
			t.Fatal("expected invalid workout snapshot")
		}
		if _, err := svc.PatchSetTarget(ctx, userID, current.ID, uuid.New(), model.PatchScheduledSetTargetRequest{}); err == nil {
			t.Fatal("expected set target validation error")
		}
	})
}

func TestScheduleRecoveryIdempotencyAndMutationGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("recovery request validation", func(t *testing.T) {
		svc, userID, _, _, _, _ := newReleaseScheduleService(t)
		if _, err := svc.RecoverToday(ctx, userID, model.RecoverScheduledWorkoutRequest{}); err == nil {
			t.Fatal("expected request validation error")
		}
		if _, err := svc.RecoverToday(ctx, userID, model.RecoverScheduledWorkoutRequest{
			Date: "not-a-date", OperationKey: "bad-date",
		}); err == nil {
			t.Fatal("expected date validation error")
		}
	})

	t.Run("recovery replay conflicts", func(t *testing.T) {
		svc, userID, _, _, _, idempotency := newReleaseScheduleService(t)
		request := model.RecoverScheduledWorkoutRequest{Date: "2026-07-20", OperationKey: "recover"}
		idempotency.records["schedule/recover|recover"] = model.IdempotencyRecord{RequestHash: "different"}
		if _, err := svc.RecoverToday(ctx, userID, request); err == nil {
			t.Fatal("expected idempotency payload conflict")
		}
		hash, err := hashPayload(request)
		if err != nil {
			t.Fatal(err)
		}
		idempotency.records["schedule/recover|recover"] = model.IdempotencyRecord{RequestHash: hash}
		if _, err := svc.RecoverToday(ctx, userID, request); err == nil {
			t.Fatal("expected missing recovery resource conflict")
		}
	})

	t.Run("no missed workout", func(t *testing.T) {
		svc, userID, _, _, _, _ := newReleaseScheduleService(t)
		if _, err := svc.RecoverToday(ctx, userID, model.RecoverScheduledWorkoutRequest{
			Date: "2026-07-20", OperationKey: "recover-none",
		}); err == nil {
			t.Fatal("expected no missed workout error")
		}
	})

	t.Run("mutation validation", func(t *testing.T) {
		svc, userID, _, _, _, _ := newReleaseScheduleService(t)
		if _, err := svc.PutRequiredSet(ctx, userID, uuid.New(), uuid.New(), model.SetMutationRequest{}); err == nil {
			t.Fatal("expected required-set validation error")
		}
		if _, err := svc.AddExtraSet(ctx, userID, uuid.New(), model.ExtraSetRequest{}); err == nil {
			t.Fatal("expected extra-set validation error")
		}
		if _, err := svc.Complete(ctx, userID, uuid.New(), model.RevisionRequest{}); err == nil {
			t.Fatal("expected completion validation error")
		}
	})

	t.Run("sequence fallback", func(t *testing.T) {
		first := &model.ScheduledWorkout{Date: "2026-07-19"}
		second := &model.ScheduledWorkout{Date: "2026-07-20"}
		if !sequenceBefore(first, second) {
			t.Fatal("expected earlier date to sort first without sequence positions")
		}
	})
}

func TestWorkoutSessionServiceReleaseGuards(t *testing.T) {
	ctx := context.Background()
	scheduleSvc, userID, _, schedules, sessions, idempotency := newReleaseScheduleService(t)
	profile := coverageProfileFixture()
	contract := NewWorkoutSessionService(
		sessions, schedules, &coverageParticipationRepo{}, &trainingProfileRepoStub{profile: &profile},
		idempotency, validator.New(),
	)
	svc, ok := contract.(*workoutSessionService)
	if !ok {
		t.Fatal("unexpected workout session service implementation")
	}
	svc.now = scheduleSvc.now

	if _, err := svc.List(ctx, userID, "2026-07-21", "2026-07-20"); err == nil {
		t.Fatal("expected invalid list date range")
	}
	if _, err := svc.Create(ctx, userID, model.CreateWorkoutSessionRequest{}); err == nil {
		t.Fatal("expected create validation error")
	}
	if _, err := svc.Create(ctx, userID, model.CreateWorkoutSessionRequest{
		Date: "bad-date", Name: "Session", OperationKey: "bad-date",
	}); err == nil {
		t.Fatal("expected create date validation error")
	}
	missingWorkoutID := uuid.New()
	if _, err := svc.Create(ctx, userID, model.CreateWorkoutSessionRequest{
		ScheduledWorkoutID: &missingWorkoutID, Date: "2026-07-20", Name: "Session", OperationKey: "missing-workout",
	}); err == nil {
		t.Fatal("expected missing scheduled workout error")
	}
	if _, err := svc.Patch(ctx, userID, uuid.New(), model.PatchWorkoutSessionRequest{}); err == nil {
		t.Fatal("expected patch validation error")
	}

	historical := model.WorkoutSession{
		ID: uuid.New(), Date: "2026-07-19", Name: "Past", Status: "draft", Revision: 1,
	}
	sessions.sessions[historical.ID] = historical
	discarded := "discarded"
	if _, err := svc.Patch(ctx, userID, historical.ID, model.PatchWorkoutSessionRequest{
		Status: &discarded, OperationKey: "discard-past", ExpectedRevision: 1,
	}); err == nil {
		t.Fatal("expected historical discard conflict")
	}

	current := historical
	current.ID = uuid.New()
	current.Date = "2026-07-20"
	sessions.sessions[current.ID] = current
	unknown := "paused"
	if _, err := svc.Patch(ctx, userID, current.ID, model.PatchWorkoutSessionRequest{
		Status: &unknown, OperationKey: "unknown-status", ExpectedRevision: 1,
	}); err == nil {
		t.Fatal("expected unknown status validation error")
	}
}

func TestProgramServiceReleaseValidationAndReplayGuards(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	program := coverageProgramFixture()
	starter := model.StarterProgram{
		ID: uuid.New(), Version: 1, Name: "Starter", PrimaryGoal: model.GoalStrength,
		Workouts: program.Workouts,
	}
	idempotency := newCoverageIdempotencyRepo()
	svc := NewProgramService(
		&coverageStarterRepo{starters: []model.StarterProgram{starter}},
		newCoverageProgramRepo(program),
		idempotency,
		validator.New(),
	)

	for name, filter := range map[string]model.StarterProgramFilter{
		"days":       {AvailableDays: 8},
		"activity":   {UsualActivity: "extreme"},
		"weekday":    {AvailableWeekdays: []int{0}},
		"duplicates": {AvailableWeekdays: []int{1, 1}},
	} {
		t.Run("starter filter "+name, func(t *testing.T) {
			if _, err := svc.ListStarters(ctx, filter); err == nil {
				t.Fatal("expected invalid starter filter")
			}
		})
	}

	if _, err := svc.Create(ctx, userID, model.CreateProgramRequest{}); err == nil {
		t.Fatal("expected create validation error")
	}
	if _, err := svc.Create(ctx, userID, model.CreateProgramRequest{
		Name: "Invalid", PrimaryGoal: "unknown", Workouts: program.Workouts,
	}); err == nil {
		t.Fatal("expected invalid goal")
	}
	invalidExercise := program.Workouts
	invalidExercise[0].Exercises[0].Modality = "mobility"
	if _, err := svc.Create(ctx, userID, model.CreateProgramRequest{
		Name: "Invalid exercise", PrimaryGoal: model.GoalStrength, Workouts: invalidExercise,
	}); err == nil {
		t.Fatal("expected invalid exercise")
	}
	if _, err := svc.Update(ctx, userID, program.ID, model.UpdateProgramRequest{}); err == nil {
		t.Fatal("expected update validation error")
	}

	cloneRequest := model.CloneStarterProgramRequest{
		StarterProgramID: starter.ID, StarterVersion: starter.Version, OperationKey: "replay",
	}
	hash, err := hashPayload(cloneRequest)
	if err != nil {
		t.Fatal(err)
	}
	idempotency.records["programs/from-starter|replay"] = model.IdempotencyRecord{RequestHash: "different"}
	if _, err := svc.CloneStarter(ctx, userID, cloneRequest); err == nil {
		t.Fatal("expected clone payload conflict")
	}
	idempotency.records["programs/from-starter|replay"] = model.IdempotencyRecord{RequestHash: hash}
	if _, err := svc.CloneStarter(ctx, userID, cloneRequest); err == nil {
		t.Fatal("expected clone replay without resource conflict")
	}
	if _, err := svc.CloneStarter(ctx, userID, model.CloneStarterProgramRequest{}); err == nil {
		t.Fatal("expected clone validation error")
	}
}

func TestTrainingProfileCreationAndMissingRevision(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()

	t.Run("creates a complete profile at revision zero", func(t *testing.T) {
		repo := &trainingProfileRepoStub{}
		svc := NewTrainingProfileService(repo)
		goal := model.GoalStrength
		days := []int{1, 3}
		activity := "light"
		experience := "beginner"
		equipment := []string{"dumbbells"}
		duration := 45
		timezone := "UTC"
		created, err := svc.Update(ctx, userID, model.UpdateTrainingProfileRequest{
			PrimaryGoal: &goal, AvailableDays: &days, UsualActivity: &activity,
			Experience: &experience, Equipment: &equipment, SessionDurationMinutes: &duration,
			Timezone: &timezone, ExpectedRevision: 0,
		})
		if err != nil {
			t.Fatal(err)
		}
		if created.Revision != 1 {
			t.Fatalf("revision=%d want=1", created.Revision)
		}
	})

	t.Run("requires an existing profile for nonzero revision", func(t *testing.T) {
		svc := NewTrainingProfileService(&trainingProfileRepoStub{})
		if _, err := svc.Update(ctx, userID, model.UpdateTrainingProfileRequest{ExpectedRevision: 1}); err == nil {
			t.Fatal("expected missing profile error")
		}
	})
}

type releaseTransitionRepo struct {
	programID uuid.UUID
	workouts  []model.ScheduledWorkout
	replay    bool
	err       error
}

func (r *releaseTransitionRepo) Apply(context.Context, uuid.UUID, int64, *model.TrainingProfile, *model.Program, string, string, string, string) (uuid.UUID, []model.ScheduledWorkout, bool, error) {
	return r.programID, r.workouts, r.replay, r.err
}

func TestPlanTransitionReleaseBranches(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	program := coverageProgramFixture()
	programs := newCoverageProgramRepo(program)
	starter := model.StarterProgram{
		ID: uuid.New(), Version: 2, Name: "Starter", PrimaryGoal: model.GoalStrength,
		Workouts: program.Workouts,
	}
	starters := &coverageStarterRepo{starters: []model.StarterProgram{starter}}
	valid := model.PreviewPlanTransitionRequest{
		ProposedProfile: model.ProposedTrainingProfile{
			PrimaryGoal: model.GoalStrength, AvailableDays: []int{1}, UsualActivity: "light",
			Experience: "beginner", Equipment: []string{"dumbbells"}, SessionDurationMinutes: 45,
			Timezone: "UTC",
		},
		ProgramID: &program.ID, From: "2026-07-20", To: "2026-07-26",
	}

	svc := NewPlanTransitionService(starters, programs, &releaseTransitionRepo{}, validator.New())
	if _, err := svc.Preview(ctx, userID, model.PreviewPlanTransitionRequest{}); err == nil {
		t.Fatal("expected preview validation error")
	}
	badProfile := valid
	badProfile.ProposedProfile.PrimaryGoal = "unknown"
	if _, err := svc.Preview(ctx, userID, badProfile); err == nil {
		t.Fatal("expected proposed profile validation error")
	}
	badRange := valid
	badRange.From, badRange.To = "2026-07-21", "2026-07-20"
	if _, err := svc.Preview(ctx, userID, badRange); err == nil {
		t.Fatal("expected transition date range error")
	}
	preview, err := svc.Preview(ctx, userID, valid)
	if err != nil {
		t.Fatal(err)
	}

	nonReplay := &releaseTransitionRepo{
		programID: uuid.New(),
		workouts:  preview.ScheduledWorkouts,
	}
	applySvc := NewPlanTransitionService(starters, programs, nonReplay, validator.New())
	apply := model.ApplyPlanTransitionRequest{
		PreviewPlanTransitionRequest: valid,
		PreviewToken:                 preview.PreviewToken,
		OperationKey:                 "apply",
		ExpectedProfileRevision:      1,
	}
	applied, err := applySvc.Apply(ctx, userID, apply)
	if err != nil {
		t.Fatal(err)
	}
	if applied.TargetProgram.ID != nonReplay.programID {
		t.Fatalf("target program id=%s want=%s", applied.TargetProgram.ID, nonReplay.programID)
	}
	if _, err := applySvc.Apply(ctx, userID, model.ApplyPlanTransitionRequest{}); err == nil {
		t.Fatal("expected apply validation error")
	}

	failing := NewPlanTransitionService(starters, programs, &releaseTransitionRepo{err: errors.New("transition failed")}, validator.New())
	if _, err := failing.Apply(ctx, userID, apply); err == nil {
		t.Fatal("expected transition persistence failure")
	}

	t.Run("reports no matching starter", func(t *testing.T) {
		request := valid
		request.ProgramID = nil
		empty := NewPlanTransitionService(&coverageStarterRepo{}, programs, &releaseTransitionRepo{}, validator.New())
		if _, err := empty.Preview(ctx, userID, request); err == nil {
			t.Fatal("expected missing starter error")
		}
	})

	t.Run("honors an explicit starter version", func(t *testing.T) {
		request := valid
		request.ProgramID = nil
		request.StarterProgramID = &starter.ID
		request.StarterVersion = &starter.Version
		explicit := NewPlanTransitionService(starters, programs, &releaseTransitionRepo{}, validator.New())
		got, err := explicit.Preview(ctx, userID, request)
		if err != nil {
			t.Fatal(err)
		}
		if got.TargetProgram.StarterVersion == nil || *got.TargetProgram.StarterVersion != starter.Version {
			t.Fatalf("starter version was not preserved: %+v", got.TargetProgram.StarterVersion)
		}
	})
}
