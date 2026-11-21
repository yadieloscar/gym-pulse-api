//go:build integration

package dao

import (
	"testing"
	"time"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// weekOffWorkoutState reads the columns a week off changes.
func weekOffWorkoutState(t *testing.T, f *releaseFixture, workout model.ScheduledWorkout) (status string, resolution *string, resolved bool) {
	t.Helper()
	var resolvedAt *time.Time
	if err := f.pool.QueryRow(f.ctx, `SELECT status, resolution, resolved_at FROM scheduled_workouts WHERE id=$1 AND user_id=$2`,
		workout.ID, f.userID).Scan(&status, &resolution, &resolvedAt); err != nil {
		t.Fatal(err)
	}
	return status, resolution, resolvedAt != nil
}

func weekOffParticipationRows(t *testing.T, f *releaseFixture, date string) int {
	t.Helper()
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM day_participation WHERE user_id=$1 AND date=$2::date`, f.userID, date).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestWeekOffPutListDelete(t *testing.T) {
	f := newReleaseFixture(t)
	weeks := NewWeekOffDAO(f.pool)

	empty, err := weeks.List(f.ctx, f.userID, "2026-10-01", "2026-12-31")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty list = %#v, err = %v; want a non-nil empty list", empty, err)
	}
	first, err := weeks.Put(f.ctx, f.userID, "2026-11-23")
	if err != nil {
		t.Fatal(err)
	}
	again, err := weeks.Put(f.ctx, f.userID, "2026-11-23")
	if err != nil {
		t.Fatal(err)
	}
	if first.WeekStart != "2026-11-23" || !first.CreatedAt.Equal(again.CreatedAt) {
		t.Fatalf("first = %+v, again = %+v; want the same week unchanged", first, again)
	}
	if _, err := weeks.Put(f.ctx, f.userID, "2026-11-09"); err != nil {
		t.Fatal(err)
	}
	listed, err := weeks.List(f.ctx, f.userID, "2026-11-10", "2026-12-31")
	if err != nil || len(listed) != 1 || listed[0].WeekStart != "2026-11-23" {
		t.Fatalf("listed = %+v, err = %v; want only the week of 11-23", listed, err)
	}
	if _, err := weeks.Put(f.ctx, f.userID, "2026-11-24"); err == nil {
		t.Fatal("a Tuesday was stored; want the Monday check to refuse it")
	}
	for range 2 {
		if err := weeks.Delete(f.ctx, f.userID, "2026-11-23"); err != nil {
			t.Fatal(err)
		}
	}
	left, err := weeks.List(f.ctx, f.userID, "2026-10-01", "2026-12-31")
	if err != nil || len(left) != 1 || left[0].WeekStart != "2026-11-09" {
		t.Fatalf("left = %+v, err = %v; want only the week of 11-09", left, err)
	}
}

// A workout left untouched in a week off is finalized as skipped and writes
// no participation row; the same workout in an ordinary week is an unresolved
// miss that counts as a scheduled opportunity.
func TestWeekOffFinalizesUntouchedWorkoutsAsSkipped(t *testing.T) {
	f := newReleaseFixture(t)
	weeks := NewWeekOffDAO(f.pool)
	if _, err := weeks.Put(f.ctx, f.userID, "2026-10-12"); err != nil {
		t.Fatal(err)
	}
	// Wednesday of the week off, the Sunday that ends it, and the Monday after.
	created := []model.ScheduledWorkout{
		releaseWorkout(f.program.ID, "2026-10-14"),
		releaseWorkout(f.program.ID, "2026-10-18"),
		releaseWorkout(f.program.ID, "2026-10-19"),
	}
	if err := f.schedules.Create(f.ctx, f.userID, created); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	for _, workout := range created {
		if _, _, err := f.mutations.FinalizeScheduledWorkout(f.ctx, f.userID, workout.ID, workout.Revision, now, releaseRecord("scheduled-workouts/lazy-finalize")); err != nil {
			t.Fatal(err)
		}
	}
	for _, workout := range created[:2] {
		status, resolution, resolved := weekOffWorkoutState(t, f, workout)
		if status != model.WorkoutStatusMissed || resolution == nil || *resolution != model.ResolutionSkipped || !resolved {
			t.Errorf("%s in the week off: status=%s resolution=%v resolved=%v; want missed and skipped", workout.Date, status, resolution, resolved)
		}
		if rows := weekOffParticipationRows(t, f, workout.Date); rows != 0 {
			t.Errorf("%s in the week off wrote %d participation rows, want 0", workout.Date, rows)
		}
	}
	status, resolution, _ := weekOffWorkoutState(t, f, created[2])
	if status != model.WorkoutStatusMissed || resolution != nil {
		t.Errorf("the Monday after: status=%s resolution=%v; want an unresolved miss", status, resolution)
	}
	if rows := weekOffParticipationRows(t, f, created[2].Date); rows != 1 {
		t.Errorf("the Monday after wrote %d participation rows, want 1", rows)
	}
}

// Taking the current week off after a day was already missed resolves that
// miss, and leaves a miss outside the week alone.
func TestWeekOffPutSkipsTheWeeksUnresolvedMisses(t *testing.T) {
	f := newReleaseFixture(t)
	created := []model.ScheduledWorkout{
		releaseWorkout(f.program.ID, "2026-10-13"),
		releaseWorkout(f.program.ID, "2026-10-20"),
	}
	if err := f.schedules.Create(f.ctx, f.userID, created); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	for _, workout := range created {
		if _, _, err := f.mutations.FinalizeScheduledWorkout(f.ctx, f.userID, workout.ID, workout.Revision, now, releaseRecord("scheduled-workouts/lazy-finalize")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := NewWeekOffDAO(f.pool).Put(f.ctx, f.userID, "2026-10-12"); err != nil {
		t.Fatal(err)
	}
	if _, resolution, resolved := weekOffWorkoutState(t, f, created[0]); resolution == nil || *resolution != model.ResolutionSkipped || !resolved {
		t.Errorf("the miss inside the week: resolution=%v resolved=%v; want skipped", resolution, resolved)
	}
	if _, resolution, _ := weekOffWorkoutState(t, f, created[1]); resolution != nil {
		t.Errorf("the miss outside the week: resolution=%v; want unresolved", *resolution)
	}
}
