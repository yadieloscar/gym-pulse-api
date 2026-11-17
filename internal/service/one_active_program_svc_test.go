package service

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// placedUnit is a program unit with weekdays, as contract 1.10.0 clients send.
func placedUnit(name string, sequence int, weekdays ...int) model.ProgramWorkout {
	reps := 8
	return model.ProgramWorkout{
		ID: uuid.New(), Kind: model.UnitKindExercises, Name: name, SequencePosition: sequence, Weekdays: weekdays,
		Exercises: []model.ProgramExercise{{ID: uuid.New(), Name: name + " lift", Category: "legs", Modality: "strength", ExerciseOrder: 1, TargetSets: 3, TargetReps: &reps}},
	}
}

func customProgram(active bool) model.Program {
	return model.Program{ID: uuid.New(), Name: "My push pull legs", PrimaryGoal: model.GoalStrength, Active: active, Revision: 4,
		Workouts: []model.ProgramWorkout{placedUnit("Push", 1, 1), placedUnit("Pull", 2, 3), placedUnit("Legs", 3, 5)}}
}

func proposedProfile(days ...int) *model.ProposedTrainingProfile {
	return &model.ProposedTrainingProfile{PrimaryGoal: model.GoalStrength, AvailableDays: days, UsualActivity: "light", Experience: "beginner", Equipment: []string{"dumbbells"}, SessionDurationMinutes: 45, Timezone: "UTC", Preferences: map[string]any{}}
}

// applyCapture records what the service asks the DAO to apply. Once a start
// is applied it remembers the operation key, as the idempotency ledger does.
type applyCapture struct {
	applied  []dao.PlanTransitionApply
	replayed map[string]dao.PlanTransitionApply
	stored   []model.ScheduledWorkout
}

func (c *applyCapture) Replayed(_ context.Context, _ uuid.UUID, operationKey, requestHash, _, _ string) (*dao.PlanTransitionResult, bool, error) {
	prior, ok := c.replayed[operationKey]
	if !ok {
		return nil, false, nil
	}
	if prior.RequestHash != requestHash {
		return nil, false, &model.ConflictError{Message: "idempotency key was already used with a different payload"}
	}
	return &dao.PlanTransitionResult{ProgramID: prior.Target.ID, Workouts: c.stored, Replay: true}, true, nil
}

func (c *applyCapture) Apply(_ context.Context, _ uuid.UUID, apply dao.PlanTransitionApply) (*dao.PlanTransitionResult, error) {
	c.applied = append(c.applied, apply)
	if c.replayed == nil {
		c.replayed = map[string]dao.PlanTransitionApply{}
	}
	c.replayed[apply.OperationKey] = apply
	return &dao.PlanTransitionResult{ProgramID: apply.Target.ID, Workouts: []model.ScheduledWorkout{}}, nil
}

func sessionLines(workouts []model.ScheduledWorkout) []string {
	lines := []string{}
	for _, workout := range workouts {
		lines = append(lines, workout.Date+" "+workout.Name)
	}
	return lines
}

func TestCreateProgramSavesWithoutStartingAndValidatesWeekdays(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	svc := NewProgramService(&coverageStarterRepo{}, newCoverageProgramRepo(), nil, newCoverageIdempotencyRepo(), validator.New())
	units := func() []model.ProgramWorkout {
		return []model.ProgramWorkout{placedUnit("Push", 1, 5, 1), placedUnit("Bike", 2, 4)}
	}
	inactive := false

	saved, err := svc.Create(ctx, userID, model.CreateProgramRequest{Name: "Saved", PrimaryGoal: model.GoalStrength, Workouts: units(), Active: &inactive, OperationKey: "save"})
	if err != nil {
		t.Fatalf("Create(active=false) error = %v", err)
	}
	if saved.Active {
		t.Fatal("Create(active=false) started the program")
	}
	if saved.Roadmap == nil {
		t.Fatal("an omitted roadmap must be stored as an empty object, not null")
	}
	if got := []int(saved.Workouts[0].Weekdays); !slices.Equal(got, []int{1, 5}) {
		t.Fatalf("weekdays = %v, want sorted [1 5]", got)
	}
	if saved.Workouts[0].PreferredWeekday == nil || *saved.Workouts[0].PreferredWeekday != 1 {
		t.Fatalf("preferred_weekday = %v, want the first weekday 1", saved.Workouts[0].PreferredWeekday)
	}

	started, err := svc.Create(ctx, userID, model.CreateProgramRequest{Name: "Legacy client", PrimaryGoal: model.GoalStrength, Workouts: units(), OperationKey: "legacy"})
	if err != nil || !started.Active {
		t.Fatalf("Create without active must keep starting the program: active=%v err=%v", started != nil && started.Active, err)
	}

	for name, mutate := range map[string]func([]model.ProgramWorkout){
		"weekday out of range":      func(u []model.ProgramWorkout) { u[0].Weekdays = model.WeekdayList{8} },
		"duplicate weekday":         func(u []model.ProgramWorkout) { u[0].Weekdays = model.WeekdayList{2, 2} },
		"present but empty":         func(u []model.ProgramWorkout) { u[0].Weekdays = model.WeekdayList{} },
		"mixed placed and unplaced": func(u []model.ProgramWorkout) { u[1].Weekdays = nil },
	} {
		t.Run(name, func(t *testing.T) {
			bad := units()
			mutate(bad)
			_, createErr := svc.Create(ctx, userID, model.CreateProgramRequest{Name: "Bad", PrimaryGoal: model.GoalStrength, Workouts: bad, OperationKey: "bad-" + name})
			_, updateErr := svc.Update(ctx, userID, uuid.New(), model.UpdateProgramRequest{Name: "Bad", PrimaryGoal: model.GoalStrength, Workouts: bad, ExpectedRevision: 1, OperationKey: "bad-update-" + name})
			for _, err := range []error{createErr, updateErr} {
				var validation *model.ValidationError
				if !errors.As(err, &validation) || validation.Field != "workouts" {
					t.Fatalf("error = %v, want workouts validation error", err)
				}
			}
		})
	}
}

func TestProfileChangeKeepsTheActiveProgram(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	program := customProgram(true)
	starter := model.StarterProgram{ID: uuid.New(), Version: 1, Name: "Recommended starter", PrimaryGoal: model.GoalStrength, Workouts: []model.ProgramWorkout{placedUnit("Full body", 1)}}
	capture := &applyCapture{}
	svc := newTransitionServiceAt("2026-11-16", &coverageStarterRepo{starters: []model.StarterProgram{starter}}, newCoverageProgramRepo(program), &trainingProfileRepoStub{}, capture)

	// The athlete moves their available days to Tue, Thu, Sat and names no program.
	req := model.PreviewPlanTransitionRequest{ProposedProfile: proposedProfile(2, 4, 6), From: "2026-11-17", To: "2026-11-23"}
	preview, err := svc.Preview(ctx, userID, req)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if preview.TargetProgram.ID != program.ID {
		t.Fatalf("target = %q, want the active custom program", preview.TargetProgram.Name)
	}
	if preview.ReplacesProgram != nil {
		t.Fatalf("replaces_program = %+v, want nil", preview.ReplacesProgram)
	}
	if preview.FirstAffectedDate != nil {
		t.Fatalf("first_affected_date = %v, want nil because no session changes", *preview.FirstAffectedDate)
	}
	if preview.RecommendedStarterProgram == nil || preview.RecommendedStarterProgram.ID != starter.ID {
		t.Fatal("the recommended starter must still be returned as a suggestion")
	}
	want := []string{"2026-11-18 Pull", "2026-11-20 Legs", "2026-11-23 Push"}
	if got := sessionLines(preview.ScheduledWorkouts); !slices.Equal(got, want) {
		t.Fatalf("sessions = %v, want the program's own weekdays %v", got, want)
	}
	if len(preview.OffAvailability) != 3 {
		t.Fatalf("off_availability has %d units, want all 3", len(preview.OffAvailability))
	}
	for index, unit := range preview.OffAvailability {
		if unit.ProgramWorkoutID == nil || *unit.ProgramWorkoutID != program.Workouts[index].ID || !slices.Equal(unit.Weekdays, []int(program.Workouts[index].Weekdays)) {
			t.Fatalf("off_availability[%d] = %+v, want unit %s on %v", index, unit, program.Workouts[index].Name, program.Workouts[index].Weekdays)
		}
	}

	if _, err := svc.Apply(ctx, userID, model.ApplyPlanTransitionRequest{PreviewPlanTransitionRequest: req, PreviewToken: preview.PreviewToken, OperationKey: "profile-change", ExpectedProfileRevision: 3}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	applied := capture.applied[0]
	if applied.Target.ID != program.ID || applied.Profile == nil || !slices.Equal(applied.Profile.AvailableDays, []int{2, 4, 6}) || applied.ExpectedProfileRevision != 3 {
		t.Fatalf("applied = %+v, want the active program and the new profile", applied)
	}
}

func TestStartKeepsTheProfileWhenNoneIsProposed(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	active, saved := customProgram(true), customProgram(false)
	saved.Name = "Saved program"
	stored := coverageProfileFixture()
	stored.AvailableDays = []int{1, 3, 5}
	capture := &applyCapture{}
	svc := newTransitionServiceAt("2026-11-16", &coverageStarterRepo{}, newCoverageProgramRepo(active, saved), &trainingProfileRepoStub{profile: &stored}, capture)

	req := model.PreviewPlanTransitionRequest{ProgramID: &saved.ID, From: "2026-11-17"}
	preview, err := svc.Preview(ctx, userID, req)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if preview.ProposedProfile != nil {
		t.Fatal("proposed_profile must be null when the request carries none")
	}
	if preview.StartDate != "2026-11-17" || preview.FirstAffectedDate == nil || *preview.FirstAffectedDate != "2026-11-18" {
		t.Fatalf("start_date = %s first_affected_date = %v", preview.StartDate, preview.FirstAffectedDate)
	}
	if preview.ReplacesProgram == nil || preview.ReplacesProgram.ID != active.ID || preview.ReplacesProgram.Name != active.Name {
		t.Fatalf("replaces_program = %+v, want the active program", preview.ReplacesProgram)
	}
	if len(preview.OffAvailability) != 0 || preview.OffAvailability == nil {
		t.Fatalf("off_availability = %v, want an empty array", preview.OffAvailability)
	}
	// With no end date the preview returns four weeks: 3 sessions a week.
	if got := len(preview.ScheduledWorkouts); got != 12 {
		t.Fatalf("default preview returned %d sessions, want 12", got)
	}
	if last := preview.ScheduledWorkouts[11].Date; last != "2026-12-14" {
		t.Fatalf("last previewed session on %s, want 2026-12-14", last)
	}

	if _, err := svc.Apply(ctx, userID, model.ApplyPlanTransitionRequest{PreviewPlanTransitionRequest: req, PreviewToken: preview.PreviewToken, OperationKey: "start"}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	applied := capture.applied[0]
	if applied.Profile != nil {
		t.Fatal("a start without a proposed profile must not write the profile")
	}
	if applied.Start != "2026-11-17" || applied.Through != "2027-01-11" || applied.To != "2026-12-14" {
		t.Fatalf("applied start=%s through=%s to=%s, want 2026-11-17, today+56 2027-01-11, 2026-12-14", applied.Start, applied.Through, applied.To)
	}
	if !slices.Equal(applied.AvailableDays, []int{1, 3, 5}) {
		t.Fatalf("applied available days = %v, want the stored profile's", applied.AvailableDays)
	}
}

func TestStartNeverBeginsBeforeToday(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	saved := customProgram(false)
	stored := coverageProfileFixture()
	svc := newTransitionServiceAt("2026-11-18", &coverageStarterRepo{}, newCoverageProgramRepo(saved), &trainingProfileRepoStub{profile: &stored}, &applyCapture{})

	preview, err := svc.Preview(ctx, userID, model.PreviewPlanTransitionRequest{ProgramID: &saved.ID, From: "2026-11-16", To: "2026-11-22"})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if preview.StartDate != "2026-11-18" {
		t.Fatalf("start_date = %s, want today 2026-11-18", preview.StartDate)
	}
	want := []string{"2026-11-18 Pull", "2026-11-20 Legs"}
	if got := sessionLines(preview.ScheduledWorkouts); !slices.Equal(got, want) {
		t.Fatalf("sessions = %v, want %v with nothing before today", got, want)
	}
}

func TestStartPreviewGoesStaleWhenTheDayChanges(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	saved := customProgram(false)
	stored := coverageProfileFixture()
	programs, profiles := newCoverageProgramRepo(saved), &trainingProfileRepoStub{profile: &stored}
	req := model.PreviewPlanTransitionRequest{ProgramID: &saved.ID, From: "2026-11-16"}
	preview, err := newTransitionServiceAt("2026-11-16", &coverageStarterRepo{}, programs, profiles, &applyCapture{}).Preview(ctx, userID, req)
	if err != nil {
		t.Fatal(err)
	}
	nextDay := newTransitionServiceAt("2026-11-17", &coverageStarterRepo{}, programs, profiles, &applyCapture{})
	_, err = nextDay.Apply(ctx, userID, model.ApplyPlanTransitionRequest{PreviewPlanTransitionRequest: req, PreviewToken: preview.PreviewToken, OperationKey: "late"})
	var conflict *model.ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("Apply() after midnight error = %v, want a stale-preview conflict", err)
	}
}

func TestStartWithoutAnActiveProgramPlacesTheTopStarter(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	reps := 5
	starter := model.StarterProgram{ID: uuid.New(), Version: 2, Name: "Foundation", PrimaryGoal: model.GoalStrength, Workouts: []model.ProgramWorkout{
		{ID: uuid.New(), Name: "Full Body A", SequencePosition: 1, Exercises: []model.ProgramExercise{{Name: "Squat", Category: "legs", Modality: "strength", ExerciseOrder: 1, TargetSets: 1, TargetReps: &reps}}},
		{ID: uuid.New(), Name: "Full Body B", SequencePosition: 2, Exercises: []model.ProgramExercise{{Name: "Press", Category: "push", Modality: "strength", ExerciseOrder: 1, TargetSets: 1, TargetReps: &reps}}},
	}}
	capture := &applyCapture{}
	svc := newTransitionServiceAt("2026-11-16", &coverageStarterRepo{starters: []model.StarterProgram{starter}}, newCoverageProgramRepo(), &trainingProfileRepoStub{}, capture)

	req := model.PreviewPlanTransitionRequest{ProposedProfile: proposedProfile(1, 3, 5), From: "2026-11-16", To: "2026-11-22"}
	preview, err := svc.Preview(ctx, userID, req)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if preview.TargetProgram.StarterProgramID == nil || *preview.TargetProgram.StarterProgramID != starter.ID || preview.TargetProgram.ID != uuid.Nil {
		t.Fatalf("target = %+v, want an unsaved copy of the top starter", preview.TargetProgram)
	}
	// Two units over three days: A on Mon and Fri, B on Wed, every week.
	want := []string{"2026-11-16 Full Body A", "2026-11-18 Full Body B", "2026-11-20 Full Body A"}
	if got := sessionLines(preview.ScheduledWorkouts); !slices.Equal(got, want) {
		t.Fatalf("sessions = %v, want %v", got, want)
	}
	if !preview.TargetProgram.Placed() {
		t.Fatal("the starter copy must be placed before it is created")
	}
}

func TestStartNeedsAProfileForAnUnplacedProgram(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	unplaced := model.Program{ID: uuid.New(), Name: "Legacy", PrimaryGoal: model.GoalStrength, Workouts: []model.ProgramWorkout{placedUnit("A", 1)}}
	svc := newTransitionServiceAt("2026-11-16", &coverageStarterRepo{}, newCoverageProgramRepo(unplaced), &trainingProfileRepoStub{}, &applyCapture{})
	_, err := svc.Preview(ctx, userID, model.PreviewPlanTransitionRequest{ProgramID: &unplaced.ID, From: "2026-11-16"})
	var validation *model.ValidationError
	if !errors.As(err, &validation) || validation.Field != "available_days" {
		t.Fatalf("Preview() without any profile error = %v, want available_days validation error", err)
	}
}

// rollingScheduleRepo is a schedule repository that can fill and move, and
// records what the service asked of it.
type rollingScheduleRepo struct {
	*coverageScheduleRepo
	fills    []string
	fillDays []int
	moves    []string
	moveErr  error
}

func (r *rollingScheduleRepo) EnsureFilled(_ context.Context, _ uuid.UUID, today string, availableDays []int) error {
	r.fills = append(r.fills, today)
	r.fillDays = availableDays
	return nil
}

func (r *rollingScheduleRepo) MoveIdempotent(_ context.Context, _, workoutID uuid.UUID, date, today string, expected int64, record model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error) {
	r.moves = append(r.moves, date+" today="+today+" scope="+record.Scope)
	if r.moveErr != nil {
		return nil, false, r.moveErr
	}
	return &model.ScheduledWorkout{ID: workoutID, Date: date, Revision: expected + 1}, false, nil
}

func newRollingScheduleService(t *testing.T, profile *model.TrainingProfile, now string) (*scheduleService, *rollingScheduleRepo) {
	t.Helper()
	repo := &rollingScheduleRepo{coverageScheduleRepo: newCoverageScheduleRepo()}
	sessions := newCoverageSessionRepo()
	contract := NewScheduleService(repo, newCoverageProgramRepo(), &trainingProfileRepoStub{profile: profile}, sessions,
		&coverageSetRepo{schedules: repo.coverageScheduleRepo, sessions: sessions}, &coverageParticipationRepo{}, nil, newCoverageIdempotencyRepo(), validator.New())
	svc, ok := contract.(*scheduleService)
	if !ok {
		t.Fatal("unexpected schedule service implementation")
	}
	fixed, err := time.Parse(time.RFC3339, now)
	if err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return fixed }
	return svc, repo
}

func TestScheduleReadKeepsTheActiveProgramFilled(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	profile := coverageProfileFixture()
	profile.Timezone, profile.AvailableDays = "America/New_York", []int{1, 3, 5}
	// 02:30 UTC on the 17th is still the 16th in New York.
	svc, repo := newRollingScheduleService(t, &profile, "2026-11-17T02:30:00Z")

	if _, err := svc.List(ctx, userID, "2026-11-16", "2026-11-22"); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if !slices.Equal(repo.fills, []string{"2026-11-16"}) || !slices.Equal(repo.fillDays, []int{1, 3, 5}) {
		t.Fatalf("fills = %v days = %v, want one fill for the athlete's local today with their available days", repo.fills, repo.fillDays)
	}
	// A read that ends before today is history; it fills nothing.
	if _, err := svc.List(ctx, userID, "2026-10-01", "2026-10-31"); err != nil {
		t.Fatalf("List(history) error = %v", err)
	}
	if len(repo.fills) != 1 {
		t.Fatalf("a history read triggered a fill: %v", repo.fills)
	}
}

func TestScheduleReadWithoutAProfileFillsNothing(t *testing.T) {
	svc, repo := newRollingScheduleService(t, nil, "2026-11-16T12:00:00Z")
	if _, err := svc.List(context.Background(), uuid.New(), "2026-11-16", "2026-11-22"); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(repo.fills) != 0 {
		t.Fatalf("fills = %v, want none without a training profile", repo.fills)
	}
}

func TestMoveValidatesTheDateAgainstTheAthletesToday(t *testing.T) {
	ctx, userID, workoutID := context.Background(), uuid.New(), uuid.New()
	profile := coverageProfileFixture()
	svc, repo := newRollingScheduleService(t, &profile, "2026-11-16T12:00:00Z")
	request := func(date string) model.MoveScheduledWorkoutRequest {
		return model.MoveScheduledWorkoutRequest{Date: date, ExpectedRevision: 2, OperationKey: "move-" + date}
	}

	for _, date := range []string{"2026-11-15", "2027-01-12", "next friday"} {
		_, err := svc.Move(ctx, userID, workoutID, request(date))
		var validation *model.ValidationError
		if !errors.As(err, &validation) || validation.Field != "date" {
			t.Fatalf("Move(%q) error = %v, want date validation error", date, err)
		}
	}
	if _, err := svc.Move(ctx, userID, workoutID, model.MoveScheduledWorkoutRequest{Date: "2026-11-17"}); err == nil {
		t.Fatal("Move() without a revision and key was accepted")
	}
	if len(repo.moves) != 0 {
		t.Fatalf("invalid moves reached the repository: %v", repo.moves)
	}

	for _, date := range []string{"2026-11-16", "2027-01-11"} { // today and today + 56
		moved, err := svc.Move(ctx, userID, workoutID, request(date))
		if err != nil || moved.Date != date || moved.Revision != 3 {
			t.Fatalf("Move(%q) = %+v, %v", date, moved, err)
		}
	}
	want := []string{"2026-11-16 today=2026-11-16 scope=scheduled-workouts/move", "2027-01-11 today=2026-11-16 scope=scheduled-workouts/move"}
	if !slices.Equal(repo.moves, want) {
		t.Fatalf("moves = %v, want %v", repo.moves, want)
	}

	repo.moveErr = &model.ConflictError{Message: "only a planned workout that has not started can be moved"}
	var conflict *model.ConflictError
	if _, err := svc.Move(ctx, userID, workoutID, request("2026-11-18")); !errors.As(err, &conflict) {
		t.Fatalf("Move() of a started workout error = %v, want the repository's conflict", err)
	}
}

func TestMoveNeedsARepositoryThatCanMove(t *testing.T) {
	sessions := newCoverageSessionRepo()
	plain := newCoverageScheduleRepo()
	svc := NewScheduleService(plain, newCoverageProgramRepo(), &trainingProfileRepoStub{}, sessions, &coverageSetRepo{schedules: plain, sessions: sessions}, &coverageParticipationRepo{}, nil, newCoverageIdempotencyRepo(), validator.New())
	today := time.Now().UTC().Format(time.DateOnly)
	if _, err := svc.Move(context.Background(), uuid.New(), uuid.New(), model.MoveScheduledWorkoutRequest{Date: today, ExpectedRevision: 1, OperationKey: "k"}); !errors.Is(err, errTrainingMutationsUnavailable) {
		t.Fatalf("Move() error = %v, want the unavailable error", err)
	}
}

func TestRemoveProgramDelegatesAndPassesTheConflictThrough(t *testing.T) {
	ctx, userID, programID := context.Background(), uuid.New(), uuid.New()
	mutations := &adoptionMutationStub{}
	svc := NewProgramService(&coverageStarterRepo{}, newCoverageProgramRepo(), mutations, newCoverageIdempotencyRepo(), validator.New())
	if err := svc.Delete(ctx, userID, programID); err != nil || !slices.Equal(mutations.deleted, []uuid.UUID{programID}) {
		t.Fatalf("Delete() err=%v deleted=%v", err, mutations.deleted)
	}
	mutations.deleteErr = &model.ConflictError{Message: "the active program cannot be removed"}
	var conflict *model.ConflictError
	if err := svc.Delete(ctx, userID, programID); !errors.As(err, &conflict) {
		t.Fatalf("Delete(active) error = %v, want conflict", err)
	}
	without := NewProgramService(&coverageStarterRepo{}, newCoverageProgramRepo(), nil, newCoverageIdempotencyRepo(), validator.New())
	if err := without.Delete(ctx, userID, programID); !errors.Is(err, errTrainingMutationsUnavailable) {
		t.Fatalf("Delete() without mutations error = %v, want the unavailable error", err)
	}
}

// A start whose response was lost is retried with the same key. It must be
// answered from its record even though the first attempt changed which
// program is active and the date has moved on.
func TestRetriedStartIsReplayedNotJudgedStale(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	active, saved := customProgram(true), customProgram(false)
	stored := coverageProfileFixture()
	programs, profiles := newCoverageProgramRepo(active, saved), &trainingProfileRepoStub{profile: &stored}
	capture := &applyCapture{stored: []model.ScheduledWorkout{{ID: uuid.New(), Date: "2026-11-18", Name: "Pull"}}}

	req := model.PreviewPlanTransitionRequest{ProgramID: &saved.ID, From: "2026-11-16"}
	first := newTransitionServiceAt("2026-11-16", &coverageStarterRepo{}, programs, profiles, capture)
	preview, err := first.Preview(ctx, userID, req)
	if err != nil {
		t.Fatal(err)
	}
	apply := model.ApplyPlanTransitionRequest{PreviewPlanTransitionRequest: req, PreviewToken: preview.PreviewToken, OperationKey: "start-once"}
	if _, err := first.Apply(ctx, userID, apply); err != nil {
		t.Fatalf("first Apply() error = %v", err)
	}

	// The retry arrives after midnight, when a fresh preview would differ.
	retry := newTransitionServiceAt("2026-11-17", &coverageStarterRepo{}, programs, profiles, capture)
	replayed, err := retry.Apply(ctx, userID, apply)
	if err != nil {
		t.Fatalf("retried Apply() error = %v, want the stored result", err)
	}
	if len(capture.applied) != 1 {
		t.Fatalf("the start was applied %d times, want once", len(capture.applied))
	}
	if replayed.TargetProgram.ID != saved.ID || len(replayed.ScheduledWorkouts) != 1 || replayed.ScheduledWorkouts[0].ID == uuid.Nil {
		t.Fatalf("replay = %+v, want the started program and its stored sessions", replayed)
	}
	if replayed.OffAvailability == nil {
		t.Fatal("off_availability must be an array on a replay too")
	}

	// The same key with another payload is a conflict, not a replay.
	other := apply
	other.From = "2026-11-18"
	var conflict *model.ConflictError
	if _, err := retry.Apply(ctx, userID, other); !errors.As(err, &conflict) {
		t.Fatalf("reusing the key with another payload error = %v, want a conflict", err)
	}
}

func TestStartPreviewGoesStaleWhenTheProgramIsEdited(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	saved := customProgram(false)
	stored := coverageProfileFixture()
	programs := newCoverageProgramRepo(saved)
	svc := newTransitionServiceAt("2026-11-16", &coverageStarterRepo{}, programs, &trainingProfileRepoStub{profile: &stored}, &applyCapture{})
	req := model.PreviewPlanTransitionRequest{ProgramID: &saved.ID, From: "2026-11-16"}
	preview, err := svc.Preview(ctx, userID, req)
	if err != nil {
		t.Fatal(err)
	}
	edited := saved
	edited.Revision++
	edited.Workouts = []model.ProgramWorkout{placedUnit("Push", 1, 2)}
	programs.programs[saved.ID] = edited
	var conflict *model.ConflictError
	_, err = svc.Apply(ctx, userID, model.ApplyPlanTransitionRequest{PreviewPlanTransitionRequest: req, PreviewToken: preview.PreviewToken, OperationKey: "after-edit"})
	if !errors.As(err, &conflict) {
		t.Fatalf("Apply() after the program changed error = %v, want a stale-preview conflict", err)
	}
}
