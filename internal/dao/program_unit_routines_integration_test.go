//go:build integration

package dao

import (
	"testing"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func chosenRoutine(id string) model.OptionalText {
	return model.OptionalText{Present: true, Value: &id}
}

func routineValue(field model.OptionalText) string {
	if field.Value == nil {
		return ""
	}
	return *field.Value
}

// unitNamed returns the program's unit with the given name.
func unitNamed(t *testing.T, program *model.Program, name string) model.ProgramWorkout {
	t.Helper()
	for _, unit := range program.Workouts {
		if unit.Name == name {
			return unit
		}
	}
	t.Fatalf("program has no unit named %q", name)
	return model.ProgramWorkout{}
}

// sessionSnapshot is every dated session and set of the athlete as stored,
// so a save that must leave them alone can be shown to.
func (f *releaseFixture) sessionSnapshot(t *testing.T) string {
	t.Helper()
	var snapshot string
	err := f.pool.QueryRow(f.ctx, `SELECT jsonb_build_object(
		'workouts', (SELECT jsonb_agg(to_jsonb(w) ORDER BY w.id) FROM scheduled_workouts w WHERE w.user_id=$1),
		'sets', (SELECT jsonb_agg(to_jsonb(s) ORDER BY s.id) FROM scheduled_sets s JOIN scheduled_workouts w ON w.id=s.scheduled_workout_id WHERE w.user_id=$1)
	)::text`, f.userID).Scan(&snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

// A program unit's chosen stretching routines survive a create and a read; an
// update that leaves a key out keeps the stored choice, null clears it, and
// none of it touches a dated session (spec 036, User Story 2).
func TestProgramUnitKeepsItsChosenRoutines(t *testing.T) {
	f := newReleaseFixture(t)
	programs := &programDAO{pool: f.pool}

	units := pushPullLegsWeek()
	units[0].WarmupRoutineID = chosenRoutine("knee-kit")
	units[0].CooldownRoutineID = chosenRoutine("my-evening")
	created := f.saveProgram(t, "Push pull legs", units)
	if push := unitNamed(t, created, "Push"); routineValue(push.WarmupRoutineID) != "knee-kit" || routineValue(push.CooldownRoutineID) != "my-evening" {
		t.Fatalf("created Push routines = %q %q", routineValue(push.WarmupRoutineID), routineValue(push.CooldownRoutineID))
	}
	f.start(t, created, fillToday, nil, 0)

	stored, err := programs.Get(f.ctx, f.userID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if push := unitNamed(t, stored, "Push"); routineValue(push.WarmupRoutineID) != "knee-kit" || routineValue(push.CooldownRoutineID) != "my-evening" {
		t.Fatalf("stored Push routines = %q %q", routineValue(push.WarmupRoutineID), routineValue(push.CooldownRoutineID))
	}
	for _, name := range []string{"Pull", "Bike ride", "Legs", "Volleyball"} {
		if unit := unitNamed(t, stored, name); unit.WarmupRoutineID.Value != nil || unit.CooldownRoutineID.Value != nil {
			t.Fatalf("%s gained a routine it never chose: %+v", name, unit)
		}
	}
	listed, err := programs.List(f.ctx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for i := range listed {
		if listed[i].ID == created.ID {
			found = true
			if push := unitNamed(t, &listed[i], "Push"); routineValue(push.WarmupRoutineID) != "knee-kit" {
				t.Fatalf("listed Push warm-up = %q", routineValue(push.WarmupRoutineID))
			}
		}
	}
	if !found {
		t.Fatal("the program is missing from the list")
	}

	sessions := f.sessionSnapshot(t)
	edit := func(mutate func(*model.ProgramWorkout)) *model.Program {
		t.Helper()
		current, err := programs.Get(f.ctx, f.userID, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		request := &model.Program{ID: current.ID, Name: current.Name, PrimaryGoal: current.PrimaryGoal, Roadmap: current.Roadmap, Active: true}
		for _, unit := range current.Workouts {
			// A request starts with both keys absent, as an older client sends it.
			unit.WarmupRoutineID, unit.CooldownRoutineID = model.OptionalText{}, model.OptionalText{}
			mutate(&unit)
			request.Workouts = append(request.Workouts, unit)
		}
		saved, _, err := f.mutations.ReplaceProgram(f.ctx, f.userID, request, current.Revision, saveNoon, releaseRecord("programs/update"))
		if err != nil {
			t.Fatal(err)
		}
		if after := f.sessionSnapshot(t); after != sessions {
			t.Fatal("a save that only touched routines changed a dated session")
		}
		reread, err := programs.Get(f.ctx, f.userID, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, unit := range saved.Workouts {
			row := unitNamed(t, reread, unit.Name)
			if routineValue(unit.WarmupRoutineID) != routineValue(row.WarmupRoutineID) || routineValue(unit.CooldownRoutineID) != routineValue(row.CooldownRoutineID) {
				t.Fatalf("the response for %s (%q %q) does not match the row (%q %q)", unit.Name,
					routineValue(unit.WarmupRoutineID), routineValue(unit.CooldownRoutineID), routineValue(row.WarmupRoutineID), routineValue(row.CooldownRoutineID))
			}
		}
		return reread
	}

	// Both keys left out: the stored choices stay.
	kept := edit(func(*model.ProgramWorkout) {})
	if push := unitNamed(t, kept, "Push"); routineValue(push.WarmupRoutineID) != "knee-kit" || routineValue(push.CooldownRoutineID) != "my-evening" {
		t.Fatalf("absent keys did not keep the choices: %q %q", routineValue(push.WarmupRoutineID), routineValue(push.CooldownRoutineID))
	}

	// Null clears one; the key left out keeps the other; another unit sets one.
	cleared := edit(func(unit *model.ProgramWorkout) {
		switch unit.Name {
		case "Push":
			unit.WarmupRoutineID = model.OptionalText{Present: true}
		case "Legs":
			unit.CooldownRoutineID = chosenRoutine("hips-and-hamstrings")
		}
	})
	if push := unitNamed(t, cleared, "Push"); push.WarmupRoutineID.Value != nil || routineValue(push.CooldownRoutineID) != "my-evening" {
		t.Fatalf("after clearing the warm-up: %q %q", routineValue(push.WarmupRoutineID), routineValue(push.CooldownRoutineID))
	}
	if legs := unitNamed(t, cleared, "Legs"); routineValue(legs.CooldownRoutineID) != "hips-and-hamstrings" || legs.WarmupRoutineID.Value != nil {
		t.Fatalf("Legs routines = %q %q", routineValue(legs.WarmupRoutineID), routineValue(legs.CooldownRoutineID))
	}

	// A unit the request adds (no id the program owns) stores what it carries.
	current, err := programs.Get(f.ctx, f.userID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	added := weekUnit("Arms", 6, 7)
	added.WarmupRoutineID = chosenRoutine("warmup-upper")
	request := &model.Program{ID: current.ID, Name: current.Name, PrimaryGoal: current.PrimaryGoal, Roadmap: current.Roadmap, Active: true,
		Workouts: append(append([]model.ProgramWorkout{}, current.Workouts...), added)}
	withArms, _, err := f.mutations.ReplaceProgram(f.ctx, f.userID, request, current.Revision, saveNoon, releaseRecord("programs/update"))
	if err != nil {
		t.Fatal(err)
	}
	if arms := unitNamed(t, withArms, "Arms"); routineValue(arms.WarmupRoutineID) != "warmup-upper" || arms.CooldownRoutineID.Value != nil {
		t.Fatalf("added unit routines = %q %q", routineValue(arms.WarmupRoutineID), routineValue(arms.CooldownRoutineID))
	}
}

// The column check is the backstop for a value the service would refuse.
func TestProgramUnitRoutineColumnsRejectValuesThatAreNotSlugs(t *testing.T) {
	f := newReleaseFixture(t)
	for _, column := range []string{"warmup_routine_id", "cooldown_routine_id"} {
		for _, value := range []string{"", "Knee Kit", "knee_kit"} {
			if _, err := f.pool.Exec(f.ctx, `UPDATE program_workouts SET `+column+`=$2 WHERE program_id=$1`, f.program.ID, value); err == nil {
				t.Errorf("%s accepted %q", column, value)
			}
		}
	}
}

// Starter units have no routine columns and always read as unset.
func TestStarterUnitsHaveNoRoutines(t *testing.T) {
	f := newReleaseFixture(t)
	starters, err := NewStarterProgramDAO(f.pool).List(f.ctx, model.StarterProgramFilter{Equipment: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(starters) == 0 {
		t.Skip("the test database has no starter programs")
	}
	starter, err := NewStarterProgramDAO(f.pool).Get(f.ctx, starters[0].ID, starters[0].Version)
	if err != nil {
		t.Fatal(err)
	}
	for _, unit := range starter.Workouts {
		if unit.WarmupRoutineID.Value != nil || unit.CooldownRoutineID.Value != nil {
			t.Fatalf("starter unit %q carries a routine: %+v", unit.Name, unit)
		}
	}
}
