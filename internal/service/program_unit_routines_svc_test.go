package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func chosenRoutine(id string) model.OptionalText {
	return model.OptionalText{Present: true, Value: &id}
}

func sportUnit(sequence int, weekdays ...int) model.ProgramWorkout {
	return model.ProgramWorkout{
		ID: uuid.New(), Kind: model.UnitKindSport, Name: "Run", SequencePosition: sequence, Weekdays: weekdays,
		Sport: &model.SportTarget{SportID: "running", SportName: "Running", TargetDurationMinutes: 30},
	}
}

func routineOf(t *testing.T, field model.OptionalText) string {
	t.Helper()
	if field.Value == nil {
		return ""
	}
	return *field.Value
}

func TestProgramUnitRoutinesAreStoredOnCreateAndUpdate(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	svc := NewProgramService(&coverageStarterRepo{}, newCoverageProgramRepo(), &adoptionMutationStub{}, newCoverageIdempotencyRepo(), validator.New())

	units := []model.ProgramWorkout{placedUnit("Push", 1, 1), placedUnit("Pull", 2, 3)}
	units[0].WarmupRoutineID = chosenRoutine("knee-kit")
	units[0].CooldownRoutineID = chosenRoutine("my-" + strings.Repeat("x", 61))
	created, err := svc.Create(ctx, userID, model.CreateProgramRequest{Name: "Custom", PrimaryGoal: model.GoalStrength, Workouts: units, OperationKey: "create"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got := routineOf(t, created.Workouts[0].WarmupRoutineID); got != "knee-kit" {
		t.Errorf("warmup_routine_id = %q, want knee-kit", got)
	}
	if got := routineOf(t, created.Workouts[0].CooldownRoutineID); len(got) != 64 {
		t.Errorf("cooldown_routine_id = %q, want the 64 character slug", got)
	}
	if created.Workouts[1].WarmupRoutineID.Value != nil || created.Workouts[1].CooldownRoutineID.Value != nil {
		t.Errorf("a unit that chose nothing = %+v, want both routines unset", created.Workouts[1])
	}

	// An update hands the DAO what the body carried: a key that was left out
	// stays absent so the stored choice is kept, and a null stays a clear.
	units = []model.ProgramWorkout{placedUnit("Push", 1, 1), placedUnit("Pull", 2, 3)}
	units[0].CooldownRoutineID = model.OptionalText{Present: true}
	units[1].WarmupRoutineID = chosenRoutine("hips-open")
	updated, err := svc.Update(ctx, userID, uuid.New(), model.UpdateProgramRequest{Name: "Custom", PrimaryGoal: model.GoalStrength, Workouts: units, ExpectedRevision: 1, OperationKey: "update"})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Workouts[0].WarmupRoutineID.Present {
		t.Error("an absent warmup_routine_id must reach the DAO as absent")
	}
	if cooldown := updated.Workouts[0].CooldownRoutineID; !cooldown.Present || cooldown.Value != nil {
		t.Errorf("a null cooldown_routine_id must reach the DAO as a clear, got %+v", cooldown)
	}
	if got := routineOf(t, updated.Workouts[1].WarmupRoutineID); got != "hips-open" {
		t.Errorf("warmup_routine_id = %q, want hips-open", got)
	}
}

func TestProgramUnitRoutinesAreValidated(t *testing.T) {
	ctx, userID := context.Background(), uuid.New()
	svc := NewProgramService(&coverageStarterRepo{}, newCoverageProgramRepo(), &adoptionMutationStub{}, newCoverageIdempotencyRepo(), validator.New())

	for name, mutate := range map[string]func([]model.ProgramWorkout){
		"warm-up is not a slug":   func(u []model.ProgramWorkout) { u[0].WarmupRoutineID = chosenRoutine("Knee Kit") },
		"cool-down is not a slug": func(u []model.ProgramWorkout) { u[0].CooldownRoutineID = chosenRoutine("knee_kit") },
		"warm-up is too long":     func(u []model.ProgramWorkout) { u[0].WarmupRoutineID = chosenRoutine(strings.Repeat("a", 65)) },
		"cool-down has spaces":    func(u []model.ProgramWorkout) { u[0].CooldownRoutineID = chosenRoutine(" knee-kit ") },
		"warm-up on a sport unit": func(u []model.ProgramWorkout) { u[1].WarmupRoutineID = chosenRoutine("knee-kit") },
		"cool-down on a sport unit": func(u []model.ProgramWorkout) {
			u[1].CooldownRoutineID = chosenRoutine("knee-kit")
		},
	} {
		t.Run(name, func(t *testing.T) {
			units := []model.ProgramWorkout{placedUnit("Push", 1, 1), sportUnit(2, 3)}
			mutate(units)
			_, createErr := svc.Create(ctx, userID, model.CreateProgramRequest{Name: "Bad", PrimaryGoal: model.GoalStrength, Workouts: units, OperationKey: "create-" + name})
			_, updateErr := svc.Update(ctx, userID, uuid.New(), model.UpdateProgramRequest{Name: "Bad", PrimaryGoal: model.GoalStrength, Workouts: units, ExpectedRevision: 1, OperationKey: "update-" + name})
			for _, err := range []error{createErr, updateErr} {
				var validation *model.ValidationError
				if !errors.As(err, &validation) || validation.Field != "workouts" {
					t.Fatalf("error = %v, want a workouts validation error", err)
				}
			}
		})
	}
}

// A unit saved as sport drops a routine it stored as an exercises unit, even
// when the request leaves the keys out.
func TestSportUnitClearsItsRoutines(t *testing.T) {
	units := []model.ProgramWorkout{sportUnit(1, 2)}
	if err := validateProgramShape(model.GoalStrength, units); err != nil {
		t.Fatalf("validateProgramShape() error = %v", err)
	}
	for name, field := range map[string]model.OptionalText{"warmup_routine_id": units[0].WarmupRoutineID, "cooldown_routine_id": units[0].CooldownRoutineID} {
		if !field.Present || field.Value != nil {
			t.Errorf("%s = %+v, want a clear", name, field)
		}
	}

	// An exercises unit that left the keys out is not touched.
	units = []model.ProgramWorkout{placedUnit("Push", 1, 1)}
	if err := validateProgramShape(model.GoalStrength, units); err != nil {
		t.Fatalf("validateProgramShape() error = %v", err)
	}
	if units[0].WarmupRoutineID.Present || units[0].CooldownRoutineID.Present {
		t.Errorf("absent keys on an exercises unit became present: %+v", units[0])
	}
}

func TestStarterCopyHasNoRoutines(t *testing.T) {
	clone := cloneStarterWorkouts([]model.ProgramWorkout{placedUnit("Full body", 1, 1)})
	if clone[0].WarmupRoutineID.Value != nil || clone[0].CooldownRoutineID.Value != nil {
		t.Errorf("a unit copied from a starter = %+v, want both routines unset", clone[0])
	}
}
