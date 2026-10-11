//go:build integration

package dao

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func (f *releaseFixture) markMissed(t *testing.T, workoutID uuid.UUID) *model.ScheduledWorkout {
	t.Helper()
	f.exec(t, `UPDATE scheduled_workouts SET status='missed', finalized_at=now() - interval '1 day',
		revision=revision+1 WHERE id=$1 AND user_id=$2`, workoutID, f.userID)
	return f.load(t, workoutID)
}

func (f *releaseFixture) load(t *testing.T, workoutID uuid.UUID) *model.ScheduledWorkout {
	t.Helper()
	workout, err := f.schedules.Get(f.ctx, f.userID, workoutID)
	if err != nil {
		t.Fatal(err)
	}
	return workout
}

// history captures every record skip and unskip must leave untouched.
func (f *releaseFixture) history(t *testing.T) string {
	t.Helper()
	var snapshot string
	err := f.pool.QueryRow(f.ctx, `SELECT jsonb_build_object(
		'statuses', (SELECT jsonb_agg(jsonb_build_object('id', w.id, 'status', w.status, 'finalized_at', w.finalized_at, 'date', w.date) ORDER BY w.id) FROM scheduled_workouts w WHERE w.user_id=$1),
		'sets', (SELECT jsonb_agg(to_jsonb(s) ORDER BY s.id) FROM scheduled_sets s JOIN scheduled_workouts w ON w.id=s.scheduled_workout_id WHERE w.user_id=$1),
		'sessions', (SELECT jsonb_agg(to_jsonb(s) ORDER BY s.id) FROM workout_sessions s WHERE s.user_id=$1),
		'logs', (SELECT jsonb_agg(to_jsonb(l) ORDER BY l.id) FROM set_logs l JOIN workout_sessions s ON s.id=l.workout_session_id WHERE s.user_id=$1),
		'participation', (SELECT jsonb_agg(to_jsonb(p) ORDER BY p.id) FROM day_participation p WHERE p.user_id=$1))::text`, f.userID).Scan(&snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

// stats renders every statistic the stats DAO derives, so a comparison proves
// skip and unskip leave counts, streaks, and volume unchanged.
func (f *releaseFixture) stats(t *testing.T) string {
	t.Helper()
	statsDAO := NewStatsDAO(f.pool)
	weekStart := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	weekly, err := statsDAO.GetWeeklyCount(f.ctx, f.userID, weekStart, weekStart.AddDate(0, 0, 7))
	if err != nil {
		t.Fatal(err)
	}
	total, err := statsDAO.GetTotalWorkouts(f.ctx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	streak, err := statsDAO.GetDayStreak(f.ctx, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	volume, err := statsDAO.GetWeeklyVolume(f.ctx, f.userID, weekStart.AddDate(0, -3, 0))
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("weekly=%d total=%d streak=%d volume=%+v", weekly, total, streak, volume)
}

func TestReleaseSkipAndUnskipResolveWithoutChangingHistory(t *testing.T) {
	f := newReleaseFixture(t)
	missed := f.markMissed(t, f.workout.ID)
	beforeHistory, beforeStats := f.history(t), f.stats(t)

	skipRecord := releaseRecord("scheduled-workouts/skip")
	skipped, replayed, err := f.mutations.SkipScheduledWorkout(f.ctx, f.userID, missed.ID, missed.Revision, skipRecord)
	if err != nil || replayed {
		t.Fatalf("skip = %v replayed=%v", err, replayed)
	}
	if skipped.Status != model.WorkoutStatusMissed || skipped.Resolution == nil || *skipped.Resolution != model.ResolutionSkipped || skipped.ResolvedAt == nil || skipped.Revision != missed.Revision+1 {
		t.Fatalf("skipped workout = %+v", skipped)
	}
	if got := f.history(t); got != beforeHistory {
		t.Fatalf("skip changed history:\nbefore %s\nafter  %s", beforeHistory, got)
	}
	if got := f.stats(t); got != beforeStats {
		t.Fatalf("skip changed stats: %s vs %s", beforeStats, got)
	}

	replay, replayed, err := f.mutations.SkipScheduledWorkout(f.ctx, f.userID, missed.ID, missed.Revision, skipRecord)
	if err != nil || !replayed || replay.Revision != skipped.Revision {
		t.Fatalf("replay = %+v replayed=%v err=%v", replay, replayed, err)
	}
	again, _, err := f.mutations.SkipScheduledWorkout(f.ctx, f.userID, missed.ID, skipped.Revision, releaseRecord("scheduled-workouts/skip"))
	if err != nil || again.Revision != skipped.Revision {
		t.Fatalf("second skip changed revision: %+v %v", again, err)
	}

	unskipped, _, err := f.mutations.UnskipScheduledWorkout(f.ctx, f.userID, missed.ID, skipped.Revision, releaseRecord("scheduled-workouts/unskip"))
	if err != nil || unskipped.Resolution != nil || unskipped.ResolvedAt != nil || unskipped.Revision != skipped.Revision+1 {
		t.Fatalf("unskip = %+v %v", unskipped, err)
	}
	noop, _, err := f.mutations.UnskipScheduledWorkout(f.ctx, f.userID, missed.ID, unskipped.Revision, releaseRecord("scheduled-workouts/unskip"))
	if err != nil || noop.Revision != unskipped.Revision {
		t.Fatalf("unskip of unresolved workout changed it: %+v %v", noop, err)
	}
	if got := f.history(t); got != beforeHistory {
		t.Fatal("unskip changed history")
	}
	if got := f.stats(t); got != beforeStats {
		t.Fatalf("unskip changed stats: %s vs %s", beforeStats, got)
	}

	recovered, _, err := f.schedules.RecoverIdempotent(f.ctx, f.userID, "2026-10-14", releaseRecord("schedule/recover"))
	if err != nil || recovered.Name != missed.Name {
		t.Fatalf("unskipped workout was not recoverable: %+v %v", recovered, err)
	}
}

func TestReleaseSkipRejectsForeignAndNonMissedWorkouts(t *testing.T) {
	f := newReleaseFixture(t)
	other := uuid.New()
	var notFound *model.NotFoundError
	if _, _, err := f.mutations.SkipScheduledWorkout(f.ctx, other, f.workout.ID, f.workout.Revision, releaseRecord("scheduled-workouts/skip")); !errors.As(err, &notFound) {
		t.Fatalf("foreign skip err = %v, want NotFoundError", err)
	}
	var resolution *model.ResolutionConflictError
	if _, _, err := f.mutations.SkipScheduledWorkout(f.ctx, f.userID, f.workout.ID, f.workout.Revision, releaseRecord("scheduled-workouts/skip")); !errors.As(err, &resolution) || resolution.Status != model.WorkoutStatusPlanned {
		t.Fatalf("planned skip err = %v, want ResolutionConflictError", err)
	}
	var conflict *model.ConflictError
	if _, _, err := f.mutations.SkipScheduledWorkout(f.ctx, f.userID, f.workout.ID, f.workout.Revision+5, releaseRecord("scheduled-workouts/skip")); !errors.As(err, &conflict) || conflict.Actual != f.workout.Revision {
		t.Fatalf("stale skip err = %v, want revision conflict", err)
	}
}

func TestReleaseRecoverResolvesSourceOnce(t *testing.T) {
	f := newReleaseFixture(t)
	missed := f.markMissed(t, f.workout.ID)

	copied, _, err := f.schedules.RecoverIdempotent(f.ctx, f.userID, "2026-10-14", releaseRecord("schedule/recover"))
	if err != nil || copied.Status != model.WorkoutStatusPlanned || copied.Resolution != nil {
		t.Fatalf("recover = %+v %v", copied, err)
	}
	source := f.load(t, missed.ID)
	if source.Status != model.WorkoutStatusMissed || source.Resolution == nil || *source.Resolution != model.ResolutionRecovered || source.Revision != missed.Revision+1 {
		t.Fatalf("source after recover = %+v", source)
	}

	var notFound *model.NotFoundError
	if _, _, err := f.schedules.RecoverIdempotent(f.ctx, f.userID, "2026-10-15", releaseRecord("schedule/recover")); !errors.As(err, &notFound) {
		t.Fatalf("second recover err = %v, want NotFoundError", err)
	}

	var conflict *model.ConflictError
	if _, _, err := f.mutations.SkipScheduledWorkout(f.ctx, f.userID, missed.ID, missed.Revision, releaseRecord("scheduled-workouts/skip")); !errors.As(err, &conflict) || conflict.Actual != source.Revision {
		t.Fatalf("skip with pre-recovery revision err = %v, want revision conflict", err)
	}
	var resolution *model.ResolutionConflictError
	if _, _, err := f.mutations.SkipScheduledWorkout(f.ctx, f.userID, missed.ID, source.Revision, releaseRecord("scheduled-workouts/skip")); !errors.As(err, &resolution) {
		t.Fatalf("skip recovered err = %v, want ResolutionConflictError", err)
	}
	if _, _, err := f.mutations.UnskipScheduledWorkout(f.ctx, f.userID, missed.ID, source.Revision, releaseRecord("scheduled-workouts/unskip")); !errors.As(err, &resolution) {
		t.Fatalf("unskip recovered err = %v, want ResolutionConflictError", err)
	}
}

func TestReleaseResolutionSurvivesScheduleReplacement(t *testing.T) {
	f := newReleaseFixture(t)
	missed := f.markMissed(t, f.workout.ID)
	if _, _, err := f.mutations.SkipScheduledWorkout(f.ctx, f.userID, missed.ID, missed.Revision, releaseRecord("scheduled-workouts/skip")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.schedules.DeleteUnstartedRange(f.ctx, f.userID, "2026-10-01", "2026-10-31"); err != nil {
		t.Fatal(err)
	}
	program, err := (&programDAO{pool: f.pool}).Get(f.ctx, f.userID, f.program.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.schedules.RegenerateIdempotent(f.ctx, f.userID, f.program.ID, program.Revision, "2026-10-01", "2026-10-31",
		[]model.ScheduledWorkout{releaseWorkout(f.program.ID, "2026-10-20")}, &model.RegenerateScheduleResponse{}, releaseRecord("schedule/regenerate")); err != nil {
		t.Fatal(err)
	}
	kept := f.load(t, missed.ID)
	if kept.Resolution == nil || *kept.Resolution != model.ResolutionSkipped {
		t.Fatalf("resolution lost after replacement: %+v", kept)
	}
}

func TestReleaseCorrectionClearsResolution(t *testing.T) {
	f := newReleaseFixture(t)
	missed := f.markMissed(t, f.workout.ID)
	skipped, _, err := f.mutations.SkipScheduledWorkout(f.ctx, f.userID, missed.ID, missed.Revision, releaseRecord("scheduled-workouts/skip"))
	if err != nil {
		t.Fatal(err)
	}
	reps := 8
	corrected, _, err := f.mutations.PutRequiredSet(f.ctx, f.userID, missed.ID, missed.RequiredSets[0].ID,
		model.SetMutationRequest{OperationKey: uuid.NewString(), ExpectedRevision: skipped.Revision, ActualReps: &reps, Completed: true},
		releaseRecord("scheduled-workouts/required-set"))
	if err != nil {
		t.Fatal(err)
	}
	if corrected.Status == model.WorkoutStatusMissed || corrected.Resolution != nil || corrected.ResolvedAt != nil {
		t.Fatalf("correction kept resolution: %+v", corrected)
	}
}
