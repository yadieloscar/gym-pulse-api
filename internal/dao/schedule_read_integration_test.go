//go:build integration

package dao

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// TestReadScheduleMatchesTheSeparateReads holds the one-round-trip schedule
// read to what the profile read, the watermark check, and List return.
func TestReadScheduleMatchesTheSeparateReads(t *testing.T) {
	f := newReleaseFixture(t)
	f.fill(t, fillToday)
	// The fixture session gets a logged required set and an extra set, so
	// both set reads have rows to place.
	reps := 8
	logged, _, err := f.mutations.PutRequiredSet(f.ctx, f.userID, f.workout.ID, f.workout.RequiredSets[0].ID,
		model.SetMutationRequest{OperationKey: uuid.NewString(), ExpectedRevision: f.workout.Revision, ActualReps: &reps, Completed: true},
		releaseRecord("scheduled-workouts/required-set"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.mutations.AddExtraSet(f.ctx, f.userID, f.workout.ID, model.ExtraSetRequest{
		OperationKey: uuid.NewString(), ExpectedRevision: logged.Revision, ExerciseName: "Lunge",
		ExerciseCategory: "legs", ExerciseModality: "strength", SetIndex: 1, ActualReps: &reps, Completed: true,
	}, releaseRecord("scheduled-workouts/extra-set")); err != nil {
		t.Fatal(err)
	}

	from, to := "2026-10-05", "2026-12-20"
	read, err := f.schedules.ReadSchedule(f.ctx, f.userID, from, to)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := NewTrainingProfileDAO(f.pool).Get(f.ctx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	workouts, err := f.schedules.List(f.ctx, f.userID, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if read.ProfileErr != nil || !reflect.DeepEqual(read.Profile, profile) {
		t.Fatalf("profile = %+v (%v), want %+v", read.Profile, read.ProfileErr, profile)
	}
	if !read.ActiveProgram || read.ScheduledThrough != f.scheduledThrough(t) {
		t.Fatalf("active=%v through=%q, want true and %q", read.ActiveProgram, read.ScheduledThrough, f.scheduledThrough(t))
	}
	if len(workouts) < 2 || !reflect.DeepEqual(read.Workouts, workouts) {
		t.Fatalf("ReadSchedule workouts differ from List:\n got %+v\nwant %+v", read.Workouts, workouts)
	}
	if len(read.Workouts[0].RequiredSets) == 0 || !read.Workouts[0].RequiredSets[0].Checked || len(read.Workouts[0].ExtraSets) != 1 {
		t.Fatalf("the logged session lost its sets: %+v", read.Workouts[0])
	}
}

func TestReadScheduleWithoutAProfileOrActiveProgram(t *testing.T) {
	f := newReleaseFixture(t)
	f.exec(t, `DELETE FROM training_profiles WHERE user_id=$1`, f.userID)
	f.exec(t, `UPDATE programs SET active=false WHERE user_id=$1`, f.userID)

	read, err := f.schedules.ReadSchedule(f.ctx, f.userID, "2026-10-05", "2026-10-18")
	if err != nil {
		t.Fatal(err)
	}
	if read.Profile != nil || read.ProfileErr == nil {
		t.Fatalf("profile = %+v, err = %v; want a not-found error", read.Profile, read.ProfileErr)
	}
	if read.ActiveProgram || read.ScheduledThrough != "" {
		t.Fatalf("active=%v through=%q, want no active program", read.ActiveProgram, read.ScheduledThrough)
	}
	if len(read.Workouts) != 1 || read.Workouts[0].ID != f.workout.ID {
		t.Fatalf("workouts = %+v, want the fixture session", read.Workouts)
	}
}

// TestReadParticipationMatchesTheSeparateReads holds the one-round-trip
// participation read to the unfinalized sessions List shows and to the
// participation List.
func TestReadParticipationMatchesTheSeparateReads(t *testing.T) {
	f := newReleaseFixture(t)
	participation := &participationDAO{pool: f.pool}
	finalized := releaseWorkout(f.program.ID, "2026-10-14")
	pending := []model.ScheduledWorkout{finalized}
	if err := f.schedules.Create(f.ctx, f.userID, pending); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.mutations.FinalizeScheduledWorkout(f.ctx, f.userID, pending[0].ID, pending[0].Revision,
		time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), releaseRecord("scheduled-workouts/lazy-finalize")); err != nil {
		t.Fatal(err)
	}

	from, to := "2026-10-05", "2026-10-18"
	read, err := participation.ReadParticipation(f.ctx, f.userID, from, to)
	if err != nil {
		t.Fatal(err)
	}
	days, err := participation.List(f.ctx, f.userID, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 || !reflect.DeepEqual(read.Days, days) {
		t.Fatalf("days = %+v, want %+v", read.Days, days)
	}
	if read.Profile == nil || read.ProfileErr != nil {
		t.Fatalf("profile = %+v, err = %v", read.Profile, read.ProfileErr)
	}
	if len(read.Unfinalized) != 1 {
		t.Fatalf("unfinalized = %+v, want only the fixture session", read.Unfinalized)
	}
	got := read.Unfinalized[0]
	if got.ID != f.workout.ID || got.Date != f.workout.Date || got.Revision != f.workout.Revision {
		t.Fatalf("unfinalized = %+v, want id %s date %s revision %d", got, f.workout.ID, f.workout.Date, f.workout.Revision)
	}
}
