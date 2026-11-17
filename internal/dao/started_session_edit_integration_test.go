//go:build integration

package dao

import (
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// A session the athlete has started takes a program edit around what they
// logged: logged sets stay, every other set follows the edited workout
// (spec 031 Amendment A4).
func TestSavingTheActiveProgramUpdatesAStartedSessionAroundItsLoggedSets(t *testing.T) {
	f := newReleaseFixture(t)
	program := f.startOwnWeek(t)
	push := f.sessionOn(t, program.ID, fillToday, "Push")
	if len(push.RequiredSets) != 2 {
		t.Fatalf("fixture Push has %d sets, want 2", len(push.RequiredSets))
	}
	logged := push.RequiredSets[0]
	var sessionID uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO workout_sessions (user_id,date,name,status,scheduled_workout_id) VALUES ($1,$2,'Push','active',$3) RETURNING id`, f.userID, fillToday, push.ID).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO set_logs (workout_session_id, scheduled_set_id, is_extra, exercise_name, exercise_category,
		exercise_modality, set_index, actual_reps, actual_weight, completed)
		VALUES ($1,$2,false,$3,'legs','strength',1,8,100,true)`, sessionID, logged.ID, logged.ExerciseName)
	f.exec(t, `UPDATE scheduled_workouts SET status='in_progress' WHERE id=$1`, push.ID)

	// The edit: a stretch goes first, and the lift gets a third set.
	seconds := 30
	edited := &model.Program{ID: program.ID, Name: program.Name, PrimaryGoal: program.PrimaryGoal, Roadmap: program.Roadmap, Active: true}
	for _, unit := range program.Workouts {
		if unit.Name == "Push" {
			lift := unit.Exercises[0]
			lift.ExerciseOrder, lift.TargetSets = 2, 3
			unit.Exercises = []model.ProgramExercise{
				{Name: "Couch stretch", Category: "legs", Modality: "strength", ExerciseOrder: 1, TargetSets: 2, TargetDurationSeconds: &seconds},
				lift,
			}
		}
		edited.Workouts = append(edited.Workouts, unit)
	}
	if _, _, err := f.mutations.ReplaceProgram(f.ctx, f.userID, edited, program.Revision, saveNoon, releaseRecord("programs/update")); err != nil {
		t.Fatal(err)
	}

	after := f.load(t, push.ID)
	var got []string
	for _, set := range after.RequiredSets {
		got = append(got, set.ExerciseName)
	}
	want := []string{"Couch stretch", "Couch stretch", logged.ExerciseName, logged.ExerciseName, logged.ExerciseName}
	if !slices.Equal(got, want) {
		t.Fatalf("today's sets = %v, want %v", got, want)
	}
	if stretch := after.RequiredSets[0]; stretch.TargetReps != nil || stretch.TargetDurationSeconds == nil || *stretch.TargetDurationSeconds != 30 || stretch.Checked {
		t.Fatalf("the added stretch = %+v", stretch)
	}
	kept := after.RequiredSets[2]
	if kept.ID != logged.ID || !kept.Checked || kept.ActualReps == nil || *kept.ActualReps != 8 || kept.ExerciseOrder != 2 || kept.SetIndex != 1 {
		t.Fatalf("the logged set changed: %+v", kept)
	}
	if after.Status != model.WorkoutStatusInProgress || after.Revision <= push.Revision || after.Date != fillToday {
		t.Fatalf("session after the edit: status=%s revision=%d date=%s", after.Status, after.Revision, after.Date)
	}
	if n := f.count(t, `SELECT count(*) FROM set_logs WHERE workout_session_id=$1 AND scheduled_set_id=$2 AND completed`, sessionID, logged.ID); n != 1 {
		t.Fatalf("the performed set lost its set: %d rows", n)
	}

	// Next week's Push is plain: the whole edited workout, nothing logged.
	next := f.sessionOn(t, program.ID, "2026-11-23", "Push")
	if len(next.RequiredSets) != 5 || next.RequiredSets[0].ExerciseName != "Couch stretch" || next.Status != model.WorkoutStatusPlanned {
		t.Fatalf("next week's Push = %d sets, status %s", len(next.RequiredSets), next.Status)
	}

	// Dropping the lift keeps its logged set, after the edited exercises.
	current, err := (&programDAO{pool: f.pool}).Get(f.ctx, f.userID, program.ID)
	if err != nil {
		t.Fatal(err)
	}
	again := &model.Program{ID: current.ID, Name: current.Name, PrimaryGoal: current.PrimaryGoal, Roadmap: current.Roadmap, Active: true}
	for _, unit := range current.Workouts {
		if unit.Name == "Push" {
			unit.Exercises = unit.Exercises[:1]
		}
		again.Workouts = append(again.Workouts, unit)
	}
	if _, _, err := f.mutations.ReplaceProgram(f.ctx, f.userID, again, current.Revision, saveNoon, releaseRecord("programs/update")); err != nil {
		t.Fatal(err)
	}
	final := f.load(t, push.ID)
	got = got[:0]
	for _, set := range final.RequiredSets {
		got = append(got, set.ExerciseName)
	}
	if want := []string{"Couch stretch", "Couch stretch", logged.ExerciseName}; !slices.Equal(got, want) {
		t.Fatalf("after dropping the lift = %v, want %v", got, want)
	}
	if last := final.RequiredSets[2]; last.ID != logged.ID || !last.Checked {
		t.Fatalf("the logged set of a removed exercise = %+v", last)
	}
}

// A finished session never takes an edit.
func TestSavingTheActiveProgramLeavesAFinishedSessionAlone(t *testing.T) {
	f := newReleaseFixture(t)
	program := f.startOwnWeek(t)
	push := f.sessionOn(t, program.ID, fillToday, "Push")
	f.exec(t, `UPDATE scheduled_workouts SET status='missed', finalized_at=now() WHERE id=$1`, push.ID)

	edited := &model.Program{ID: program.ID, Name: program.Name, PrimaryGoal: program.PrimaryGoal, Roadmap: program.Roadmap, Active: true}
	for _, unit := range program.Workouts {
		if unit.Name == "Push" {
			unit.Exercises[0].TargetSets = 4
		}
		edited.Workouts = append(edited.Workouts, unit)
	}
	if _, _, err := f.mutations.ReplaceProgram(f.ctx, f.userID, edited, program.Revision, saveNoon, releaseRecord("programs/update")); err != nil {
		t.Fatal(err)
	}
	if after := f.load(t, push.ID); len(after.RequiredSets) != 2 || after.Revision != push.Revision {
		t.Fatalf("a finished session changed: %d sets, revision %d (was %d)", len(after.RequiredSets), after.Revision, push.Revision)
	}
}
