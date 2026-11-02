package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
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
	profileCopy := *r.profile
	return &profileCopy, nil
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
	svc := NewTrainingProfileService(repo, nil, validator.New())
	goal := model.GoalPower
	got, err := svc.Update(context.Background(), uuid.New(), model.UpdateTrainingProfileRequest{PrimaryGoal: &goal, ExpectedRevision: 4, OperationKey: "profile-power"})
	if err != nil {
		t.Fatal(err)
	}
	if got.PrimaryGoal != model.GoalPower || got.AvailableDays[0] != 1 || got.Revision != 5 {
		t.Fatalf("partial update did not preserve profile: %+v", got)
	}

	unknown := "tone"
	_, err = svc.Update(context.Background(), uuid.New(), model.UpdateTrainingProfileRequest{PrimaryGoal: &unknown, ExpectedRevision: 5, OperationKey: "profile-invalid"})
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

type adoptionMutationStub struct {
	response *model.AdoptLegacyProgramResponse
	record   model.IdempotencyRecord
	records  []model.IdempotencyRecord
	now      time.Time
	calls    int
}

func (s *adoptionMutationStub) AdoptLegacy(_ context.Context, _ uuid.UUID, now time.Time, record model.IdempotencyRecord) (*model.AdoptLegacyProgramResponse, bool, error) {
	s.calls++
	s.now = now
	s.record = record
	return s.response, false, nil
}

func (s *adoptionMutationStub) PutTrainingProfile(_ context.Context, _ uuid.UUID, profile *model.TrainingProfile, expected int64, record model.IdempotencyRecord) (*model.TrainingProfile, bool, error) {
	s.records = append(s.records, record)
	profile.Revision = expected + 1
	return profile, false, nil
}

func (s *adoptionMutationStub) CreateProgram(_ context.Context, _ uuid.UUID, program *model.Program, record model.IdempotencyRecord) (*model.Program, bool, error) {
	s.records = append(s.records, record)
	program.ID = uuid.New()
	program.Revision = 1
	return program, false, nil
}

func (s *adoptionMutationStub) ReplaceProgram(_ context.Context, _ uuid.UUID, program *model.Program, expected int64, record model.IdempotencyRecord) (*model.Program, bool, error) {
	s.records = append(s.records, record)
	program.Revision = expected + 1
	return program, false, nil
}

func (*adoptionMutationStub) ReplaceScheduledWorkout(context.Context, uuid.UUID, *model.ScheduledWorkout, int64, model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error) {
	return nil, false, nil
}

func (*adoptionMutationStub) UpdateScheduledSetTarget(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, model.PatchScheduledSetTargetRequest, model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error) {
	return nil, false, nil
}

func (*adoptionMutationStub) PutRequiredSet(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, model.SetMutationRequest, model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error) {
	return nil, false, nil
}

func (*adoptionMutationStub) AddExtraSet(context.Context, uuid.UUID, uuid.UUID, model.ExtraSetRequest, model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error) {
	return nil, false, nil
}

func (*adoptionMutationStub) FinalizeScheduledWorkout(context.Context, uuid.UUID, uuid.UUID, int64, time.Time, model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error) {
	return nil, false, nil
}

func (*adoptionMutationStub) SkipScheduledWorkout(context.Context, uuid.UUID, uuid.UUID, int64, model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error) {
	return nil, false, nil
}

func (*adoptionMutationStub) UnskipScheduledWorkout(context.Context, uuid.UUID, uuid.UUID, int64, model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error) {
	return nil, false, nil
}

func (*adoptionMutationStub) CreateWorkoutSession(context.Context, uuid.UUID, *model.WorkoutSession, model.IdempotencyRecord) (*model.WorkoutSession, bool, error) {
	return nil, false, nil
}

func (*adoptionMutationStub) ReplaceWorkoutSession(context.Context, uuid.UUID, *model.WorkoutSession, int64, time.Time, model.IdempotencyRecord) (*model.WorkoutSession, bool, error) {
	return nil, false, nil
}

func TestProfileAndCustomProgramMutationsUseAtomicReplayBoundary(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	mutations := &adoptionMutationStub{}
	profile := validServiceTrainingProfile()
	profile.Revision = 2
	profileRepo := &trainingProfileRepoStub{
		profile: &profile,
		put: func(uuid.UUID, *model.TrainingProfile, int64) error {
			t.Fatal("legacy profile DAO write was used")
			return nil
		},
	}
	profileService := NewTrainingProfileService(profileRepo, mutations, validator.New())
	goal := model.GoalPower
	updated, err := profileService.Update(ctx, userID, model.UpdateTrainingProfileRequest{
		PrimaryGoal: &goal, ExpectedRevision: 2, OperationKey: "profile-op",
	})
	if err != nil || updated.Revision != 3 {
		t.Fatalf("profile update = %+v, %v", updated, err)
	}

	program := coverageProgramFixture()
	programs := newCoverageProgramRepo(program)
	programService := NewProgramService(&coverageStarterRepo{}, programs, mutations, newCoverageIdempotencyRepo(), validator.New())
	created, err := programService.Create(ctx, userID, model.CreateProgramRequest{
		Name: "Custom", PrimaryGoal: model.GoalStrength, Workouts: program.Workouts,
		OperationKey: "program-create-op",
	})
	if err != nil || created.Revision != 1 {
		t.Fatalf("program create = %+v, %v", created, err)
	}
	if _, err := programService.Update(ctx, userID, program.ID, model.UpdateProgramRequest{
		Name: "Updated", PrimaryGoal: model.GoalStrength, Active: true,
		Workouts: program.Workouts, ExpectedRevision: program.Revision,
		OperationKey: "program-update-op",
	}); err != nil {
		t.Fatal(err)
	}

	if len(mutations.records) != 3 {
		t.Fatalf("mutation records = %+v", mutations.records)
	}
	wantScopes := []string{"training-profile/put", "programs/create", "programs/update"}
	for i, want := range wantScopes {
		if got := mutations.records[i]; got.Scope != want || got.RequestHash == "" || got.OperationKey == "" {
			t.Fatalf("record %d = %+v, want scope %q", i, got, want)
		}
	}
}

func validServiceTrainingProfile() model.TrainingProfile {
	return model.TrainingProfile{
		PrimaryGoal: model.GoalStrength, AvailableDays: []int{1, 3},
		UsualActivity: "moderate", Experience: "intermediate",
		Equipment: []string{"barbell"}, SessionDurationMinutes: 60,
		Timezone: "America/New_York", Preferences: map[string]any{},
	}
}

var _ dao.TrainingMutationDAO = (*adoptionMutationStub)(nil)

func TestProgramServiceAdoptsLegacyPlanThroughAtomicMutation(t *testing.T) {
	userID := uuid.New()
	wantNow := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	response := &model.AdoptLegacyProgramResponse{
		Program:  model.Program{ID: uuid.New(), Name: model.LegacyProgramName, Revision: 1},
		Schedule: []model.ScheduledWorkout{}, Adopted: true,
	}
	mutations := &adoptionMutationStub{response: response}
	contract := NewProgramService(&coverageStarterRepo{}, newCoverageProgramRepo(), mutations, newCoverageIdempotencyRepo(), validator.New())
	svc, ok := contract.(*programService)
	if !ok {
		t.Fatal("unexpected program service implementation")
	}
	svc.now = func() time.Time { return wantNow }

	request := model.AdoptLegacyProgramRequest{OperationKey: "adopt-once", ExpectedRevision: 0}
	got, err := svc.AdoptLegacy(context.Background(), userID, request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Program.ID != response.Program.ID || mutations.calls != 1 || mutations.now != wantNow {
		t.Fatalf("unexpected adoption result=%+v mutation=%+v", got, mutations)
	}
	if mutations.record.Scope != "programs/adopt-legacy" || mutations.record.OperationKey != request.OperationKey || mutations.record.RequestHash == "" || mutations.record.ResponseStatus != 200 {
		t.Fatalf("unexpected adoption idempotency record: %+v", mutations.record)
	}
	if _, err := svc.AdoptLegacy(context.Background(), userID, model.AdoptLegacyProgramRequest{}); err == nil {
		t.Fatal("adoption without an operation key was accepted")
	}
}

func TestValidateProgramShapeSportUnits(t *testing.T) {
	distance := 40000
	sportUnit := func(mutate func(*model.ProgramWorkout)) []model.ProgramWorkout {
		unit := model.ProgramWorkout{Kind: model.UnitKindSport, Name: "Long ride", SequencePosition: 1, Sport: &model.SportTarget{SportID: "cycling", SportName: "Cycling", TargetDurationMinutes: 90, TargetDistanceMeters: &distance}}
		mutate(&unit)
		return []model.ProgramWorkout{unit}
	}
	exercise := model.ProgramExercise{Name: "Back Squat", Category: "legs", Modality: "strength", ExerciseOrder: 1, TargetSets: 3}
	longNotes := strings.Repeat("n", 2001)
	bad := 0
	huge := 1000001
	cases := []struct {
		name     string
		workouts []model.ProgramWorkout
		wantErr  bool
	}{
		{name: "sport unit accepted", workouts: sportUnit(func(*model.ProgramWorkout) {})},
		{name: "kind omitted defaults to exercises", workouts: []model.ProgramWorkout{{Name: "A", SequencePosition: 1, Exercises: []model.ProgramExercise{exercise}}}},
		{name: "exercises unit without exercises", workouts: []model.ProgramWorkout{{Kind: model.UnitKindExercises, Name: "A", SequencePosition: 1}}, wantErr: true},
		{name: "exercises unit with sport target", workouts: []model.ProgramWorkout{{Name: "A", SequencePosition: 1, Exercises: []model.ProgramExercise{exercise}, Sport: &model.SportTarget{SportID: "cycling", SportName: "Cycling", TargetDurationMinutes: 1}}}, wantErr: true},
		{name: "sport unit with exercises", workouts: sportUnit(func(w *model.ProgramWorkout) { w.Exercises = []model.ProgramExercise{exercise} }), wantErr: true},
		{name: "sport unit without target", workouts: sportUnit(func(w *model.ProgramWorkout) { w.Sport = nil }), wantErr: true},
		{name: "unknown kind", workouts: sportUnit(func(w *model.ProgramWorkout) { w.Kind = "rest" }), wantErr: true},
		{name: "bad slug", workouts: sportUnit(func(w *model.ProgramWorkout) { w.Sport.SportID = "Road Cycling" }), wantErr: true},
		{name: "empty name", workouts: sportUnit(func(w *model.ProgramWorkout) { w.Sport.SportName = "  " }), wantErr: true},
		{name: "other needs custom name", workouts: sportUnit(func(w *model.ProgramWorkout) { w.Sport.SportID, w.Sport.SportName = "other", "Other" }), wantErr: true},
		{name: "duration zero", workouts: sportUnit(func(w *model.ProgramWorkout) { w.Sport.TargetDurationMinutes = 0 }), wantErr: true},
		{name: "duration 1441", workouts: sportUnit(func(w *model.ProgramWorkout) { w.Sport.TargetDurationMinutes = 1441 }), wantErr: true},
		{name: "distance zero", workouts: sportUnit(func(w *model.ProgramWorkout) { w.Sport.TargetDistanceMeters = &bad }), wantErr: true},
		{name: "distance too far", workouts: sportUnit(func(w *model.ProgramWorkout) { w.Sport.TargetDistanceMeters = &huge }), wantErr: true},
		{name: "distance on volleyball", workouts: sportUnit(func(w *model.ProgramWorkout) { w.Sport.SportID, w.Sport.SportName = "volleyball", "Volleyball" }), wantErr: true},
		{name: "volleyball without distance", workouts: sportUnit(func(w *model.ProgramWorkout) {
			w.Sport.SportID, w.Sport.SportName, w.Sport.TargetDistanceMeters = "volleyball", "Volleyball", nil
		})},
		{name: "notes too long", workouts: sportUnit(func(w *model.ProgramWorkout) { w.Sport.Notes = &longNotes }), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateProgramShape(model.GoalStrength, tc.workouts)
			if tc.wantErr {
				var ve *model.ValidationError
				if !errors.As(err, &ve) || ve.Field != "workouts" {
					t.Fatalf("want validation error on workouts, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.workouts[0].Kind == "" {
				t.Fatal("kind must be normalized in place")
			}
		})
	}
	blank := "   "
	trimmed := sportUnit(func(w *model.ProgramWorkout) {
		w.Sport.SportID, w.Sport.SportName, w.Sport.Notes = " cycling ", " Cycling ", &blank
	})
	if err := validateProgramShape(model.GoalStrength, trimmed); err != nil {
		t.Fatal(err)
	}
	if trimmed[0].Sport.SportID != "cycling" || trimmed[0].Sport.SportName != "Cycling" || trimmed[0].Sport.Notes != nil || trimmed[0].Exercises == nil {
		t.Fatalf("sport target must be trimmed and exercises non-nil: %+v", trimmed[0])
	}
}

func TestMaterializeSportUnitSnapshotsTargetWithoutSets(t *testing.T) {
	distance := 40000
	note := "Easy pace"
	program := &model.Program{
		ID: uuid.New(), Name: "Mixed", PrimaryGoal: model.GoalStrength,
		Workouts: []model.ProgramWorkout{
			{ID: uuid.New(), Name: "Full Body", SequencePosition: 1, Exercises: []model.ProgramExercise{{ID: uuid.New(), Name: "Back Squat", Category: "legs", Modality: "strength", ExerciseOrder: 1, TargetSets: 2}}},
			{ID: uuid.New(), Kind: model.UnitKindSport, Name: "Long ride", SequencePosition: 2, Sport: &model.SportTarget{SportID: "cycling", SportName: "Cycling", TargetDurationMinutes: 90, TargetDistanceMeters: &distance, Notes: &note}},
		},
	}
	workouts, err := model.MaterializeProgramForWeekdays(program, []int{1, 3}, "2026-10-26", "2026-11-01")
	if err != nil {
		t.Fatal(err)
	}
	if len(workouts) != 2 {
		t.Fatalf("want 2 scheduled units, got %d", len(workouts))
	}
	gym, ride := workouts[0], workouts[1]
	if gym.Kind != model.UnitKindExercises || gym.Sport != nil || len(gym.RequiredSets) != 2 {
		t.Fatalf("exercise unit changed: %+v", gym)
	}
	if ride.Kind != model.UnitKindSport || ride.Status != model.WorkoutStatusPlanned || len(ride.RequiredSets) != 0 || ride.Sport == nil {
		t.Fatalf("sport unit not materialized as a planned session: %+v", ride)
	}
	if ride.Sport.SportID != "cycling" || ride.Sport.TargetDurationMinutes != 90 || *ride.Sport.TargetDistanceMeters != 40000 || *ride.Sport.Notes != "Easy pace" || ride.Sport.SportActivityID != nil {
		t.Fatalf("sport snapshot wrong: %+v", ride.Sport)
	}
	if ride.ProgramWorkoutID == nil || *ride.ProgramWorkoutID != program.Workouts[1].ID || ride.Date != "2026-10-28" {
		t.Fatalf("provenance or date wrong: %+v", ride)
	}
	program.Workouts[1].Sport.SportName = "Renamed"
	*program.Workouts[1].Sport.TargetDistanceMeters = 1
	if ride.Sport.SportName != "Cycling" || *ride.Sport.TargetDistanceMeters != 40000 {
		t.Fatal("scheduled sport snapshot must not alias the program target")
	}
	if err := ride.Validate(); err != nil {
		t.Fatal(err)
	}
}
