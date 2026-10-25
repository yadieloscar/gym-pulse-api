//go:build integration

package dao

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// sportSession inserts a planned sport session on date with the given
// sequence position and returns it as the schedule reads it.
func (f *releaseFixture) sportSession(t *testing.T, date string, sequence int, sportID, sportName string) *model.ScheduledWorkout {
	t.Helper()
	var id uuid.UUID
	err := f.pool.QueryRow(f.ctx, `INSERT INTO scheduled_workouts (user_id, program_id, date, name, sequence_position, status,
		kind, sport_id, sport_name, target_duration_minutes, target_distance_meters)
		VALUES ($1,$2,$3,$4,$5,'planned','sport',$6,$7,90,40000) RETURNING id`,
		f.userID, f.program.ID, date, sportName, sequence, sportID, sportName).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return f.load(t, id)
}

func (f *releaseFixture) participation(t *testing.T, date string) (opportunity, participated bool, found bool) {
	t.Helper()
	err := f.pool.QueryRow(f.ctx, `SELECT scheduled_opportunity, participated FROM day_participation WHERE user_id=$1 AND date=$2`, f.userID, date).Scan(&opportunity, &participated)
	if err != nil {
		return false, false, false
	}
	return opportunity, participated, true
}

func TestRecordAgainstPlannedSportSessionCompletesItOnce(t *testing.T) {
	f := newReleaseFixture(t)
	activities := NewSportActivityDAO(f.pool)
	ride := f.sportSession(t, "2026-10-31", 2, "cycling", "Cycling")
	if ride.Kind != model.UnitKindSport || ride.Sport == nil || ride.Sport.SportActivityID != nil || len(ride.RequiredSets) != 0 {
		t.Fatalf("planned sport session read wrong: %+v", ride)
	}
	meters := 42000
	activity := &model.SportActivity{Date: "2026-10-31", SportID: "cycling", SportName: "Cycling", DurationMinutes: 95, Source: model.ActivitySourceManual, DistanceMeters: &meters, ScheduledWorkoutID: &ride.ID}
	first, outcome, err := activities.Create(f.ctx, f.userID, activity, "UTC", "ride-op", "hash-1")
	if err != nil || outcome != model.SportActivityCreated {
		t.Fatalf("record failed: outcome=%v err=%v", outcome, err)
	}
	if first.ScheduledWorkoutID == nil || *first.ScheduledWorkoutID != ride.ID {
		t.Fatalf("activity must carry the link: %+v", first)
	}
	done := f.load(t, ride.ID)
	if done.Status != model.WorkoutStatusCompleted || done.FinalizedAt == nil || done.Revision != ride.Revision+1 ||
		done.Sport.SportActivityID == nil || *done.Sport.SportActivityID != first.ID || done.Sport.PerformedDate == nil || *done.Sport.PerformedDate != "2026-10-31" {
		t.Fatalf("session must be completed and linked: %+v sport=%+v", done, done.Sport)
	}
	if opportunity, participated, found := f.participation(t, "2026-10-31"); !found || !participated || opportunity {
		t.Fatalf("performed date participation wrong: found=%v opportunity=%v participated=%v", found, opportunity, participated)
	}
	// Same key replays; a fresh key against a completed session conflicts.
	replay, outcome, err := activities.Create(f.ctx, f.userID, &model.SportActivity{Date: "2026-10-31", SportID: "cycling", SportName: "Cycling", DurationMinutes: 95, Source: model.ActivitySourceManual, ScheduledWorkoutID: &ride.ID}, "UTC", "ride-op", "hash-1")
	if err != nil || outcome != model.SportActivityReplayed || replay.ID != first.ID {
		t.Fatalf("replay failed: outcome=%v err=%v", outcome, err)
	}
	_, _, err = activities.Create(f.ctx, f.userID, &model.SportActivity{Date: "2026-10-31", SportID: "cycling", SportName: "Cycling", DurationMinutes: 60, Source: model.ActivitySourceManual, ScheduledWorkoutID: &ride.ID}, "UTC", "ride-op-2", "hash-2")
	var conflict *model.SportSessionConflictError
	if !errors.As(err, &conflict) || conflict.Status != model.WorkoutStatusCompleted {
		t.Fatalf("second record must be a sport session conflict: %v", err)
	}
	authoritative, ok := conflict.Authoritative.(*model.ScheduledWorkout)
	if !ok || authoritative.ID != ride.ID || authoritative.Status != model.WorkoutStatusCompleted {
		t.Fatalf("conflict must carry the authoritative session: %+v", conflict.Authoritative)
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM sport_activities WHERE user_id=$1`, f.userID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("exactly one activity expected, got %d err=%v", count, err)
	}
	// Foreign and mismatched links.
	other := newReleaseFixture(t)
	_, _, err = NewSportActivityDAO(other.pool).Create(other.ctx, other.userID, &model.SportActivity{Date: "2026-10-31", SportID: "cycling", SportName: "Cycling", DurationMinutes: 10, Source: model.ActivitySourceManual, ScheduledWorkoutID: &ride.ID}, "UTC", "foreign", "hash-f")
	var notFound *model.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("foreign session must be 404: %v", err)
	}
	volleyball := f.sportSession(t, "2026-11-01", 3, "volleyball", "Volleyball")
	_, _, err = activities.Create(f.ctx, f.userID, &model.SportActivity{Date: "2026-11-01", SportID: "cycling", SportName: "Cycling", DurationMinutes: 10, Source: model.ActivitySourceManual, ScheduledWorkoutID: &volleyball.ID}, "UTC", "mismatch", "hash-m")
	var validation *model.ValidationError
	if !errors.As(err, &validation) || validation.Field != "sport_id" {
		t.Fatalf("sport mismatch must be 422 on sport_id: %v", err)
	}
	gym := f.load(t, f.workout.ID)
	_, _, err = activities.Create(f.ctx, f.userID, &model.SportActivity{Date: gym.Date, SportID: "cycling", SportName: "Cycling", DurationMinutes: 10, Source: model.ActivitySourceManual, ScheduledWorkoutID: &gym.ID}, "UTC", "kind", "hash-k")
	if !errors.As(err, &validation) || validation.Field != "scheduled_workout_id" {
		t.Fatalf("exercise workout must be 422 on scheduled_workout_id: %v", err)
	}
}

func TestRecordingMissedSportSessionLaterCreditsPerformedDate(t *testing.T) {
	f := newReleaseFixture(t)
	activities := NewSportActivityDAO(f.pool)
	ride := f.sportSession(t, "2026-10-20", 2, "cycling", "Cycling")
	missed := f.markMissed(t, ride.ID)
	f.exec(t, `INSERT INTO day_participation (user_id, date, scheduled_opportunity, participated, finalized_at, timezone, local_date)
		VALUES ($1,'2026-10-20',true,false,now(),'UTC','2026-10-20')`, f.userID)
	skipped, _, err := f.mutations.SkipScheduledWorkout(f.ctx, f.userID, ride.ID, missed.Revision, releaseRecord("scheduled-workouts/skip"))
	if err != nil || skipped.Resolution == nil {
		t.Fatalf("skip failed: %+v err=%v", skipped, err)
	}
	_, _, err = activities.Create(f.ctx, f.userID, &model.SportActivity{Date: "2026-10-22", SportID: "cycling", SportName: "Cycling", DurationMinutes: 80, Source: model.ActivitySourceManual, ScheduledWorkoutID: &ride.ID}, "UTC", "late-ride", "hash-late")
	if err != nil {
		t.Fatal(err)
	}
	done := f.load(t, ride.ID)
	if done.Status != model.WorkoutStatusCompleted || done.Resolution != nil || done.ResolvedAt != nil || done.Date != "2026-10-20" || done.Sport.PerformedDate == nil || *done.Sport.PerformedDate != "2026-10-22" {
		t.Fatalf("late record must complete, clear the skip, keep the scheduled date: %+v sport=%+v", done, done.Sport)
	}
	if opportunity, participated, found := f.participation(t, "2026-10-20"); !found || !opportunity || participated {
		t.Fatalf("scheduled date participation must stay as finalized: found=%v opportunity=%v participated=%v", found, opportunity, participated)
	}
	if _, participated, found := f.participation(t, "2026-10-22"); !found || !participated {
		t.Fatalf("performed date must be credited")
	}
	_, _, err = f.schedules.RecoverIdempotent(f.ctx, f.userID, "2026-10-30", releaseRecord("schedule/recover"))
	var notFound *model.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("recover must ignore sport sessions: %v", err)
	}
}

func TestImportedActivityFulfilsEarliestOpenSportSessionOnce(t *testing.T) {
	f := newReleaseFixture(t)
	activities := NewSportActivityDAO(f.pool)
	second := f.sportSession(t, "2026-10-31", 3, "cycling", "Cycling")
	first := f.sportSession(t, "2026-10-31", 2, "cycling", "Cycling")
	external := "HK-RIDE-1"
	imported, outcome, err := activities.Create(f.ctx, f.userID, &model.SportActivity{Date: "2026-10-31", SportID: "cycling", SportName: "Cycling", DurationMinutes: 88, Source: model.ActivitySourceAppleHealth, ExternalID: &external}, "UTC", "hk:1", "hash-hk1")
	if err != nil || outcome != model.SportActivityCreated {
		t.Fatalf("import failed: outcome=%v err=%v", outcome, err)
	}
	if imported.ScheduledWorkoutID == nil || *imported.ScheduledWorkoutID != first.ID {
		t.Fatalf("import must link the lowest sequence position: got %v want %s", imported.ScheduledWorkoutID, first.ID)
	}
	if f.load(t, first.ID).Status != model.WorkoutStatusCompleted || f.load(t, second.ID).Status != model.WorkoutStatusPlanned {
		t.Fatal("only the earliest open session may be fulfilled")
	}
	// Re-import under a new key: duplicate path, nothing links.
	again, outcome, err := activities.Create(f.ctx, f.userID, &model.SportActivity{Date: "2026-10-31", SportID: "cycling", SportName: "Cycling", DurationMinutes: 88, Source: model.ActivitySourceAppleHealth, ExternalID: &external}, "UTC", "hk:1-again", "hash-hk1b")
	if err != nil || outcome != model.SportActivityExisting || again.ID != imported.ID || f.load(t, second.ID).Status != model.WorkoutStatusPlanned {
		t.Fatalf("re-import must change nothing: outcome=%v err=%v", outcome, err)
	}
	// A different sport on the same date stays unlinked; a manual log never auto-links.
	run := "HK-RUN-1"
	runActivity, _, err := activities.Create(f.ctx, f.userID, &model.SportActivity{Date: "2026-10-31", SportID: "running", SportName: "Running", DurationMinutes: 30, Source: model.ActivitySourceAppleHealth, ExternalID: &run}, "UTC", "hk:2", "hash-hk2")
	if err != nil || runActivity.ScheduledWorkoutID != nil {
		t.Fatalf("non-matching import must stay unlinked: %+v err=%v", runActivity, err)
	}
	manual, _, err := activities.Create(f.ctx, f.userID, &model.SportActivity{Date: "2026-10-31", SportID: "cycling", SportName: "Cycling", DurationMinutes: 30, Source: model.ActivitySourceManual}, "UTC", "manual", "hash-manual")
	if err != nil || manual.ScheduledWorkoutID != nil || f.load(t, second.ID).Status != model.WorkoutStatusPlanned {
		t.Fatalf("manual log must not auto-link: %+v err=%v", manual, err)
	}
	// Finalization never changes a completed session; an unfulfilled past
	// session becomes missed with an unparticipated opportunity.
	stale := f.sportSession(t, "2026-10-01", 1, "volleyball", "Volleyball")
	finalized, _, err := f.mutations.FinalizeScheduledWorkout(f.ctx, f.userID, stale.ID, stale.Revision, time.Now(), releaseRecord("scheduled-workouts/lazy-finalize"))
	if err != nil || finalized.Status != model.WorkoutStatusMissed || finalized.FinalizedAt == nil {
		t.Fatalf("unfulfilled sport session must finalize as missed: %+v err=%v", finalized, err)
	}
	if opportunity, participated, found := f.participation(t, "2026-10-01"); !found || !opportunity || participated {
		t.Fatalf("missed sport session participation wrong: found=%v opportunity=%v participated=%v", found, opportunity, participated)
	}
	done := f.load(t, first.ID)
	refinalized, _, err := f.mutations.FinalizeScheduledWorkout(f.ctx, f.userID, first.ID, done.Revision, time.Now(), releaseRecord("scheduled-workouts/lazy-finalize"))
	if err != nil || refinalized.Status != model.WorkoutStatusCompleted || refinalized.Revision != done.Revision {
		t.Fatalf("a completed sport session must not change on finalize: %+v err=%v", refinalized, err)
	}
}
