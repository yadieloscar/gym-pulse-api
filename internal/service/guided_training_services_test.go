package service

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type atomicProgramRepo struct {
	*coverageProgramRepo
	called bool
}

func (r *atomicProgramRepo) CreateIdempotent(_ context.Context, _ uuid.UUID, program *model.Program, record model.IdempotencyRecord) (*model.Program, bool, error) {
	r.called = true
	if record.Scope != "programs/from-starter" || record.ResponseStatus != 201 || record.RequestHash == "" {
		return nil, false, fmt.Errorf("invalid idempotency record: %+v", record)
	}
	program.ID = uuid.New()
	program.Revision = 1
	r.programs[program.ID] = *program
	return program, false, nil
}

var _ dao.IdempotentProgramDAO = (*atomicProgramRepo)(nil)

type coverageStarterRepo struct{ starters []model.StarterProgram }

func (r *coverageStarterRepo) List(context.Context, model.StarterProgramFilter) ([]model.StarterProgram, error) {
	return append([]model.StarterProgram(nil), r.starters...), nil
}
func (r *coverageStarterRepo) Get(_ context.Context, id uuid.UUID, version int) (*model.StarterProgram, error) {
	for i := range r.starters {
		if r.starters[i].ID == id && r.starters[i].Version == version {
			value := r.starters[i]
			return &value, nil
		}
	}
	return nil, &model.NotFoundError{Message: "starter not found"}
}

type coverageProgramRepo struct{ programs map[uuid.UUID]model.Program }

func newCoverageProgramRepo(programs ...model.Program) *coverageProgramRepo {
	repo := &coverageProgramRepo{programs: map[uuid.UUID]model.Program{}}
	for _, program := range programs {
		repo.programs[program.ID] = program
	}
	return repo
}
func (r *coverageProgramRepo) List(context.Context, uuid.UUID) ([]model.Program, error) {
	values := make([]model.Program, 0, len(r.programs))
	for _, program := range r.programs {
		values = append(values, program)
	}
	return values, nil
}
func (r *coverageProgramRepo) Get(_ context.Context, _ uuid.UUID, id uuid.UUID) (*model.Program, error) {
	program, ok := r.programs[id]
	if !ok {
		return nil, &model.NotFoundError{Message: "program not found"}
	}
	value := program
	return &value, nil
}
func (r *coverageProgramRepo) Create(_ context.Context, _ uuid.UUID, program *model.Program) error {
	if program.ID == uuid.Nil {
		program.ID = uuid.New()
	}
	if program.Revision == 0 {
		program.Revision = 1
	}
	r.programs[program.ID] = *program
	return nil
}
func (r *coverageProgramRepo) Replace(_ context.Context, _ uuid.UUID, program *model.Program, expected int64) error {
	program.Revision = expected + 1
	r.programs[program.ID] = *program
	return nil
}
func (*coverageProgramRepo) RecordLegacyAdoption(context.Context, uuid.UUID, uuid.UUID, string) error {
	return nil
}

type coverageIdempotencyRepo struct {
	records map[string]model.IdempotencyRecord
}

func newCoverageIdempotencyRepo() *coverageIdempotencyRepo {
	return &coverageIdempotencyRepo{records: map[string]model.IdempotencyRecord{}}
}
func (r *coverageIdempotencyRepo) Get(_ context.Context, _ uuid.UUID, scope, operationKey string) (*model.IdempotencyRecord, error) {
	record, ok := r.records[scope+"|"+operationKey]
	if !ok {
		return nil, &model.NotFoundError{Message: "idempotency record not found"}
	}
	value := record
	return &value, nil
}
func (r *coverageIdempotencyRepo) Create(_ context.Context, _ uuid.UUID, record *model.IdempotencyRecord) error {
	r.records[record.Scope+"|"+record.OperationKey] = *record
	return nil
}

type coverageScheduleRepo struct {
	workouts map[uuid.UUID]model.ScheduledWorkout
}

func newCoverageScheduleRepo() *coverageScheduleRepo {
	return &coverageScheduleRepo{workouts: map[uuid.UUID]model.ScheduledWorkout{}}
}
func (r *coverageScheduleRepo) List(_ context.Context, _ uuid.UUID, from, to string) ([]model.ScheduledWorkout, error) {
	values := []model.ScheduledWorkout{}
	for _, workout := range r.workouts {
		if workout.Date >= from && workout.Date <= to {
			values = append(values, workout)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Date < values[j].Date })
	return values, nil
}
func (r *coverageScheduleRepo) Get(_ context.Context, _ uuid.UUID, id uuid.UUID) (*model.ScheduledWorkout, error) {
	workout, ok := r.workouts[id]
	if !ok {
		return nil, &model.NotFoundError{Message: "scheduled workout not found"}
	}
	value := workout
	return &value, nil
}
func (r *coverageScheduleRepo) Create(_ context.Context, _ uuid.UUID, workouts []model.ScheduledWorkout) error {
	for i := range workouts {
		if workouts[i].ID == uuid.Nil {
			workouts[i].ID = uuid.New()
		}
		if workouts[i].Revision == 0 {
			workouts[i].Revision = 1
		}
		for j := range workouts[i].RequiredSets {
			if workouts[i].RequiredSets[j].ID == uuid.Nil {
				workouts[i].RequiredSets[j].ID = uuid.New()
			}
		}
		r.workouts[workouts[i].ID] = workouts[i]
	}
	return nil
}
func (r *coverageScheduleRepo) ReplaceSnapshot(_ context.Context, _ uuid.UUID, workout *model.ScheduledWorkout, expected int64) error {
	workout.Revision = expected + 1
	r.workouts[workout.ID] = *workout
	return nil
}
func (r *coverageScheduleRepo) UpdateOutcome(_ context.Context, _ uuid.UUID, workout *model.ScheduledWorkout, expected int64) error {
	workout.Revision = expected + 1
	r.workouts[workout.ID] = *workout
	return nil
}
func (r *coverageScheduleRepo) UpdateSetTarget(_ context.Context, _ uuid.UUID, workoutID, setID uuid.UUID, target model.PatchScheduledSetTargetRequest) error {
	workout := r.workouts[workoutID]
	for i := range workout.RequiredSets {
		if workout.RequiredSets[i].ID == setID {
			workout.RequiredSets[i].TargetReps = target.TargetReps
			workout.RequiredSets[i].TargetWeight = target.TargetWeight
			workout.RequiredSets[i].TargetDurationSeconds = target.TargetDurationSeconds
			workout.RequiredSets[i].RestSeconds = target.RestSeconds
			workout.RequiredSets[i].Notes = target.Notes
		}
	}
	workout.Revision++
	r.workouts[workoutID] = workout
	return nil
}
func (r *coverageScheduleRepo) DeleteUnstartedRange(_ context.Context, _ uuid.UUID, from, to string) ([]uuid.UUID, error) {
	deleted := []uuid.UUID{}
	for id, workout := range r.workouts {
		if workout.Date >= from && workout.Date <= to && workout.FinalizedAt == nil {
			deleted = append(deleted, id)
			delete(r.workouts, id)
		}
	}
	return deleted, nil
}

type coverageSessionRepo struct {
	sessions map[uuid.UUID]model.WorkoutSession
}

func newCoverageSessionRepo() *coverageSessionRepo {
	return &coverageSessionRepo{sessions: map[uuid.UUID]model.WorkoutSession{}}
}
func (r *coverageSessionRepo) List(_ context.Context, _ uuid.UUID, from, to string) ([]model.WorkoutSession, error) {
	values := []model.WorkoutSession{}
	for _, session := range r.sessions {
		if session.Date >= from && session.Date <= to {
			values = append(values, session)
		}
	}
	return values, nil
}
func (r *coverageSessionRepo) Get(_ context.Context, _ uuid.UUID, id uuid.UUID) (*model.WorkoutSession, error) {
	session, ok := r.sessions[id]
	if !ok {
		return nil, &model.NotFoundError{Message: "session not found"}
	}
	value := session
	return &value, nil
}
func (r *coverageSessionRepo) Create(_ context.Context, _ uuid.UUID, session *model.WorkoutSession) error {
	if session.ID == uuid.Nil {
		session.ID = uuid.New()
	}
	if session.Revision == 0 {
		session.Revision = 1
	}
	r.sessions[session.ID] = *session
	return nil
}
func (r *coverageSessionRepo) Update(_ context.Context, _ uuid.UUID, session *model.WorkoutSession, expected int64) error {
	session.Revision = expected + 1
	r.sessions[session.ID] = *session
	return nil
}

type coverageSetRepo struct {
	schedules *coverageScheduleRepo
	sessions  *coverageSessionRepo
}

func (r *coverageSetRepo) PutRequired(_ context.Context, _ uuid.UUID, sessionID, scheduledSetID uuid.UUID, set *model.PerformedSet, _ int64) (*model.WorkoutSession, error) {
	session := r.sessions.sessions[sessionID]
	if session.ScheduledWorkoutID != nil {
		workout := r.schedules.workouts[*session.ScheduledWorkoutID]
		for i := range workout.RequiredSets {
			if workout.RequiredSets[i].ID == scheduledSetID {
				performedID := uuid.New()
				workout.RequiredSets[i].Checked = set.Completed
				workout.RequiredSets[i].PerformedSetID = &performedID
				workout.RequiredSets[i].ActualReps = set.ActualReps
				workout.RequiredSets[i].ActualWeight = set.ActualWeight
				workout.RequiredSets[i].ActualDurationSeconds = set.DurationSeconds
			}
		}
		r.schedules.workouts[workout.ID] = workout
	}
	return &session, nil
}
func (r *coverageSetRepo) AddExtra(_ context.Context, _ uuid.UUID, sessionID uuid.UUID, set *model.PerformedSet, _ int64) (*model.WorkoutSession, error) {
	session := r.sessions.sessions[sessionID]
	if session.ScheduledWorkoutID != nil {
		workout := r.schedules.workouts[*session.ScheduledWorkoutID]
		workout.ExtraSets = append(workout.ExtraSets, *set)
		r.schedules.workouts[workout.ID] = workout
	}
	return &session, nil
}

type coverageParticipationRepo struct{ outcomes []model.DayParticipation }

func (r *coverageParticipationRepo) List(_ context.Context, _ uuid.UUID, from, to string) ([]model.DayParticipation, error) {
	values := []model.DayParticipation{}
	for _, outcome := range r.outcomes {
		if outcome.Date >= from && outcome.Date <= to {
			values = append(values, outcome)
		}
	}
	return values, nil
}
func (r *coverageParticipationRepo) Finalize(_ context.Context, _ uuid.UUID, outcome *model.DayParticipation) error {
	r.outcomes = append(r.outcomes, *outcome)
	return nil
}
func (r *coverageParticipationRepo) Preserve(_ context.Context, _ uuid.UUID, outcome *model.DayParticipation) error {
	r.outcomes = append(r.outcomes, *outcome)
	return nil
}

type coverageTransitionRepo struct {
	programID uuid.UUID
	workouts  []model.ScheduledWorkout
	replay    bool
}

func (r *coverageTransitionRepo) Apply(context.Context, uuid.UUID, int64, *model.TrainingProfile, *model.Program, string, string, string, string) (uuid.UUID, []model.ScheduledWorkout, bool, error) {
	return r.programID, r.workouts, r.replay, nil
}

func coverageProgramFixture() model.Program {
	weekday, reps := 1, 5
	return model.Program{
		ID: uuid.New(), Name: "Strength", PrimaryGoal: model.GoalStrength, Active: true, Revision: 1,
		Workouts: []model.ProgramWorkout{{
			ID: uuid.New(), Name: "Full Body", PreferredWeekday: &weekday, SequencePosition: 1,
			Exercises: []model.ProgramExercise{{ID: uuid.New(), Name: "Squat", Category: "legs", Modality: "strength", ExerciseOrder: 1, TargetSets: 1, TargetReps: &reps}},
		}},
	}
}

func coverageProfileFixture() model.TrainingProfile {
	return model.TrainingProfile{PrimaryGoal: model.GoalStrength, AvailableDays: []int{1}, UsualActivity: "light", Experience: "beginner", Equipment: []string{"dumbbells"}, SessionDurationMinutes: 45, Timezone: "UTC", Preferences: map[string]any{}, Revision: 1}
}

func TestGuidedProgramAndProfileServices(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	profileRepo := &trainingProfileRepoStub{profile: func() *model.TrainingProfile { value := coverageProfileFixture(); return &value }()}
	profileService := NewTrainingProfileService(profileRepo)
	if _, err := profileService.Get(ctx, userID); err != nil {
		t.Fatal(err)
	}
	goal, days, activity, experience := model.GoalPower, []int{2, 4}, "moderate", "intermediate"
	equipment, duration, timezone, preferences := []string{"barbell"}, 60, "America/New_York", map[string]any{"deload": true}
	if _, err := profileService.Update(ctx, userID, model.UpdateTrainingProfileRequest{PrimaryGoal: &goal, AvailableDays: &days, UsualActivity: &activity, Experience: &experience, Equipment: &equipment, SessionDurationMinutes: &duration, Timezone: &timezone, Preferences: &preferences, ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}

	program := coverageProgramFixture()
	starter := model.StarterProgram{ID: uuid.New(), Version: 1, Name: "Starter", PrimaryGoal: model.GoalStrength, MinDays: 1, MaxDays: 4, Experience: []string{"beginner"}, Equipment: []string{"dumbbells"}, DurationMinutes: 45, Workouts: program.Workouts}
	starters := &coverageStarterRepo{starters: []model.StarterProgram{starter, {ID: uuid.New(), Version: 1, Name: "Other", PrimaryGoal: model.GoalConditioning, MinDays: 4, MaxDays: 6, Experience: []string{"advanced"}, Equipment: []string{"full_gym"}, DurationMinutes: 90, Workouts: program.Workouts}}}
	programs := newCoverageProgramRepo(program)
	idempotency := newCoverageIdempotencyRepo()
	service := NewProgramService(starters, programs, idempotency, validator.New())
	filter := model.StarterProgramFilter{PrimaryGoal: model.GoalStrength, AvailableDays: 3, AvailableWeekdays: []int{1, 3, 5}, UsualActivity: "light", Experience: "beginner", Equipment: []string{"dumbbells"}, SessionDurationMinutes: 50}
	ranked, err := service.ListStarters(ctx, filter)
	if err != nil || len(ranked) != 2 || ranked[0].ID != starter.ID {
		t.Fatalf("starter ranking failed: %+v %v", ranked, err)
	}
	if allEquipmentAvailable([]string{"barbell"}, []string{"dumbbells"}) || !allEquipmentAvailable([]string{"barbell"}, []string{"full_gym"}) {
		t.Fatal("equipment matching failed")
	}
	if _, err := service.List(ctx, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Get(ctx, userID, program.ID); err != nil {
		t.Fatal(err)
	}
	create := model.CreateProgramRequest{Name: "Custom", PrimaryGoal: model.GoalStrength, Workouts: program.Workouts}
	created, err := service.Create(ctx, userID, create)
	if err != nil || created.ID == uuid.Nil {
		t.Fatalf("create failed: %+v %v", created, err)
	}
	cloneRequest := model.CloneStarterProgramRequest{StarterProgramID: starter.ID, StarterVersion: 1, OperationKey: "clone-program"}
	cloned, err := service.CloneStarter(ctx, userID, cloneRequest)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.CloneStarter(ctx, userID, cloneRequest)
	if err != nil || replayed.ID != cloned.ID {
		t.Fatalf("clone replay failed: %+v %v", replayed, err)
	}
	update := model.UpdateProgramRequest{Name: "Updated", PrimaryGoal: model.GoalStrength, Active: true, Workouts: program.Workouts, ExpectedRevision: 1}
	if _, err := service.Update(ctx, userID, program.ID, update); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ListStarters(ctx, model.StarterProgramFilter{PrimaryGoal: "invalid"}); err == nil {
		t.Fatal("invalid starter filter was accepted")
	}
	if err := validateProgramShape(model.GoalStrength, []model.ProgramWorkout{{}}); err == nil {
		t.Fatal("invalid program shape was accepted")
	}
}

func TestGuidedScheduleSessionAndParticipationServices(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	program := coverageProgramFixture()
	programs := newCoverageProgramRepo(program)
	profile := coverageProfileFixture()
	profiles := &trainingProfileRepoStub{profile: &profile}
	schedules := newCoverageScheduleRepo()
	sessions := newCoverageSessionRepo()
	participation := &coverageParticipationRepo{}
	idempotency := newCoverageIdempotencyRepo()
	sets := &coverageSetRepo{schedules: schedules, sessions: sessions}
	scheduleContract := NewScheduleService(schedules, programs, profiles, sessions, sets, participation, idempotency, validator.New())
	service, ok := scheduleContract.(*scheduleService)
	if !ok {
		t.Fatal("unexpected schedule service implementation")
	}
	service.now = func() time.Time { return time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC) }

	materialize := model.MaterializeScheduleRequest{ProgramID: program.ID, From: "2026-07-20", To: "2026-07-26", OperationKey: "materialize", ExpectedRevision: 1}
	workouts, err := service.Materialize(ctx, userID, materialize)
	if err != nil || len(workouts) != 1 {
		t.Fatalf("materialize failed: %+v %v", workouts, err)
	}
	if replay, err := service.Materialize(ctx, userID, materialize); err != nil || len(replay) != 1 {
		t.Fatalf("materialize replay failed: %+v %v", replay, err)
	}
	workoutID := workouts[0].ID
	if listed, err := service.List(ctx, userID, materialize.From, materialize.To); err != nil || len(listed) != 1 {
		t.Fatalf("list failed: %+v %v", listed, err)
	}
	name := "Full Body Edited"
	current, _ := schedules.Get(ctx, userID, workoutID)
	patched, err := service.PatchWorkout(ctx, userID, workoutID, model.PatchScheduledWorkoutRequest{Name: &name, OperationKey: "patch", ExpectedRevision: current.Revision})
	if err != nil || patched.Name != name {
		t.Fatalf("patch failed: %+v %v", patched, err)
	}
	setID := patched.RequiredSets[0].ID
	reps := 8
	patched, err = service.PatchSetTarget(ctx, userID, workoutID, setID, model.PatchScheduledSetTargetRequest{TargetReps: &reps, OperationKey: "target", ExpectedRevision: patched.Revision})
	if err != nil || *patched.RequiredSets[0].TargetReps != reps {
		t.Fatalf("set target patch failed: %+v %v", patched, err)
	}
	actualReps := 8
	patched, err = service.PutRequiredSet(ctx, userID, workoutID, setID, model.SetMutationRequest{OperationKey: "required", ExpectedRevision: patched.Revision, ActualReps: &actualReps, Completed: true})
	if err != nil || patched.Status != model.WorkoutStatusInProgress {
		t.Fatalf("required set failed: %+v %v", patched, err)
	}
	patched, err = service.AddExtraSet(ctx, userID, workoutID, model.ExtraSetRequest{OperationKey: "extra", ExpectedRevision: patched.Revision, ExerciseName: "Curl", ExerciseCategory: "pull", ExerciseModality: "strength", SetIndex: 1, ActualReps: &actualReps, Completed: true})
	if err != nil || len(patched.ExtraSets) != 1 {
		t.Fatalf("extra set failed: %+v %v", patched, err)
	}
	completed, err := service.Complete(ctx, userID, workoutID, model.RevisionRequest{OperationKey: "complete", ExpectedRevision: patched.Revision})
	if err != nil || completed.Status != model.WorkoutStatusCompleted || completed.FinalizedAt == nil {
		t.Fatalf("completion failed: %+v %v", completed, err)
	}
	if _, err := service.Complete(ctx, userID, workoutID, model.RevisionRequest{OperationKey: "stale", ExpectedRevision: 1}); err == nil {
		t.Fatal("stale revision was accepted")
	}

	first, second := 2, 1
	missed := []model.ScheduledWorkout{
		{ID: uuid.New(), Date: "2026-07-18", Name: "Later", SequencePosition: &first, Status: model.WorkoutStatusMissed, Revision: 1, RequiredSets: completed.RequiredSets},
		{ID: uuid.New(), Date: "2026-07-19", Name: "Next", SequencePosition: &second, Status: model.WorkoutStatusMissed, Revision: 1, RequiredSets: completed.RequiredSets},
	}
	if err := schedules.Create(ctx, userID, missed); err != nil {
		t.Fatal(err)
	}
	recovered, err := service.RecoverToday(ctx, userID, model.RecoverScheduledWorkoutRequest{Date: "2026-07-20", OperationKey: "recover"})
	if err != nil || recovered.Status != model.WorkoutStatusPlanned || recovered.Name != "Next" {
		t.Fatalf("recovery failed: %+v %v", recovered, err)
	}

	previewRequest := model.RegenerateScheduleRequest{ProgramID: program.ID, From: "2026-07-27", To: "2026-08-02", OperationKey: "regenerate", ExpectedRevision: 1}
	preview, err := service.Regenerate(ctx, userID, previewRequest)
	if err != nil || preview.PreviewToken == "" {
		t.Fatalf("regenerate preview failed: %+v %v", preview, err)
	}
	previewRequest.Apply = true
	previewRequest.PreviewToken = preview.PreviewToken
	if _, err := service.Regenerate(ctx, userID, previewRequest); err != nil {
		t.Fatal(err)
	}

	past := model.ScheduledWorkout{ID: uuid.New(), Date: "2026-07-17", Name: "Past", Status: model.WorkoutStatusPlanned, Revision: 1, RequiredSets: completed.RequiredSets}
	for i := range past.RequiredSets {
		past.RequiredSets[i].Checked = false
	}
	if err := schedules.Create(ctx, userID, []model.ScheduledWorkout{past}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.List(ctx, userID, past.Date, past.Date); err != nil {
		t.Fatal(err)
	}
	finalizedPast, _ := schedules.Get(ctx, userID, past.ID)
	if finalizedPast.FinalizedAt == nil || finalizedPast.Status != model.WorkoutStatusMissed {
		t.Fatalf("lazy finalization failed: %+v", finalizedPast)
	}

	sessionContract := NewWorkoutSessionService(sessions, schedules, participation, profiles, idempotency, validator.New())
	sessionService, ok := sessionContract.(*workoutSessionService)
	if !ok {
		t.Fatal("unexpected workout session service implementation")
	}
	sessionService.now = service.now
	created, err := sessionService.Create(ctx, userID, model.CreateWorkoutSessionRequest{Date: "2026-07-20", Name: "Off plan", OperationKey: "session"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sessionService.List(ctx, userID, "2026-07-20", "2026-07-20"); err != nil {
		t.Fatal(err)
	}
	if _, err := sessionService.Get(ctx, userID, created.ID); err != nil {
		t.Fatal(err)
	}
	updatedName, notes, status := "Completed off plan", "felt good", "completed"
	updated, err := sessionService.Patch(ctx, userID, created.ID, model.PatchWorkoutSessionRequest{Name: &updatedName, Notes: &notes, Status: &status, OperationKey: "session-patch", ExpectedRevision: created.Revision})
	if err != nil || updated.CompletedAt == nil {
		t.Fatalf("session completion failed: %+v %v", updated, err)
	}

	participationService := NewParticipationService(service, participation)
	if values, err := participationService.List(ctx, userID, past.Date, "2026-07-20"); err != nil || len(values) == 0 {
		t.Fatalf("participation list failed: %+v %v", values, err)
	}
	if _, err := hashPayload(make(chan int)); err == nil {
		t.Fatal("unencodable idempotency payload was accepted")
	}
	if !isConflict(fmt.Errorf("wrapped: %w", &model.ConflictError{Message: "conflict"})) {
		t.Fatal("wrapped conflict was not detected")
	}
}

func TestGuidedPlanTransitionStarterReplay(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	program := coverageProgramFixture()
	starter := model.StarterProgram{ID: uuid.New(), Version: 1, Name: "Starter", PrimaryGoal: model.GoalStrength, Workouts: program.Workouts}
	starters := &coverageStarterRepo{starters: []model.StarterProgram{starter}}
	existing := program
	programs := newCoverageProgramRepo(existing)
	transitions := &coverageTransitionRepo{programID: existing.ID, replay: true}
	service := NewPlanTransitionService(starters, programs, transitions, validator.New())
	request := model.PreviewPlanTransitionRequest{ProposedProfile: model.ProposedTrainingProfile{PrimaryGoal: model.GoalStrength, AvailableDays: []int{1}, UsualActivity: "light", Experience: "beginner", Equipment: []string{"dumbbells"}, SessionDurationMinutes: 45, Timezone: "UTC", Preferences: map[string]any{}}, StarterProgramID: &starter.ID, From: "2026-07-20", To: "2026-07-26"}
	preview, err := service.Preview(ctx, userID, request)
	if err != nil || preview.RecommendedStarterProgram == nil || preview.FirstAffectedDate == nil {
		t.Fatalf("starter preview failed: %+v %v", preview, err)
	}
	apply := model.ApplyPlanTransitionRequest{PreviewPlanTransitionRequest: request, PreviewToken: preview.PreviewToken, OperationKey: "transition", ExpectedProfileRevision: 1}
	applied, err := service.Apply(ctx, userID, apply)
	if err != nil || applied.TargetProgram.ID != existing.ID {
		t.Fatalf("transition replay failed: %+v %v", applied, err)
	}
	apply.PreviewToken = "stale"
	if _, err := service.Apply(ctx, userID, apply); err == nil {
		t.Fatal("stale transition preview was accepted")
	}
}

func TestCloneStarterUsesAtomicProgramWorkflow(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	starter := model.StarterProgram{
		ID: uuid.New(), Version: 1, Name: "Starter", PrimaryGoal: model.GoalStrength,
		Workouts: coverageProgramFixture().Workouts,
	}
	programs := &atomicProgramRepo{coverageProgramRepo: newCoverageProgramRepo()}
	idempotency := newCoverageIdempotencyRepo()
	service := NewProgramService(&coverageStarterRepo{starters: []model.StarterProgram{starter}}, programs, idempotency, validator.New())

	result, err := service.CloneStarter(ctx, userID, model.CloneStarterProgramRequest{
		StarterProgramID: starter.ID, StarterVersion: starter.Version, OperationKey: "atomic-clone",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !programs.called || result.ID == uuid.Nil {
		t.Fatalf("atomic program workflow was not used: %+v", result)
	}
	if len(idempotency.records) != 0 {
		t.Fatal("service separately recorded idempotency after atomic create")
	}
}
