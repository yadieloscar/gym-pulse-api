package service

import (
	"context"
	"errors"
	"slices"
	"testing"

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

// applyCapture records what the service asks the DAO to apply.
type applyCapture struct {
	applied []dao.PlanTransitionApply
}

func (c *applyCapture) Apply(_ context.Context, _ uuid.UUID, apply dao.PlanTransitionApply) (*dao.PlanTransitionResult, error) {
	c.applied = append(c.applied, apply)
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
