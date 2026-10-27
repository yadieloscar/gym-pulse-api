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

// sportSessionTarget inserts a planned sport session with a given planned duration.
func (f *releaseFixture) sportSessionTarget(t *testing.T, date string, sequence int, sportID, sportName string, minutes int) *model.ScheduledWorkout {
	t.Helper()
	var id uuid.UUID
	err := f.pool.QueryRow(f.ctx, `INSERT INTO scheduled_workouts (user_id, program_id, date, name, sequence_position, status,
		kind, sport_id, sport_name, target_duration_minutes)
		VALUES ($1,$2,$3,$4,$5,'planned','sport',$6,$7,$8) RETURNING id`,
		f.userID, f.program.ID, date, sportName, sequence, sportID, sportName, minutes).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return f.load(t, id)
}

func (f *releaseFixture) participationRevision(t *testing.T, date string) int64 {
	t.Helper()
	var revision int64
	if err := f.pool.QueryRow(f.ctx, `SELECT revision FROM day_participation WHERE user_id=$1 AND date=$2`, f.userID, date).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	return revision
}

func TestUnlinkAndLinkExistingRepairAWrongMatch(t *testing.T) {
	f := newReleaseFixture(t)
	activities := NewSportActivityDAO(f.pool)
	ride := f.sportSession(t, "2026-10-01", 2, "cycling", "Cycling")
	commute, _, err := activities.Create(f.ctx, f.userID, &model.SportActivity{Date: "2026-10-01", SportID: "cycling", SportName: "Cycling", DurationMinutes: 40, Source: model.ActivitySourceManual, ScheduledWorkoutID: &ride.ID}, "UTC", "commute", "h-commute")
	if err != nil {
		t.Fatal(err)
	}
	done := f.load(t, ride.ID)

	// Unlinking a past session makes it missed again and counts the scheduled date as an opportunity.
	result, replayed, err := activities.Unlink(f.ctx, f.userID, commute.ID, model.UnlinkSportActivityRequest{ExpectedRevision: done.Revision, OperationKey: "unlink-1"}, "2026-10-26", "UTC", "h-unlink-1")
	if err != nil || replayed {
		t.Fatalf("unlink failed: replayed=%v err=%v", replayed, err)
	}
	if result.SportActivity.ScheduledWorkoutID != nil || result.ScheduledWorkout.Status != model.WorkoutStatusMissed || result.ScheduledWorkout.FinalizedAt == nil ||
		result.ScheduledWorkout.Sport.SportActivityID != nil || result.ScheduledWorkout.Revision != done.Revision+1 {
		t.Fatalf("unlink must clear the link and miss the past session: %+v sport=%+v", result.ScheduledWorkout, result.ScheduledWorkout.Sport)
	}
	if result.SportActivity.DurationMinutes != 40 || result.SportActivity.Date != "2026-10-01" {
		t.Fatalf("unlink must leave the activity's facts: %+v", result.SportActivity)
	}
	if opportunity, participated, found := f.participation(t, "2026-10-01"); !found || !opportunity || !participated {
		t.Fatalf("the activity's own credit stays and the date becomes an opportunity: found=%v opportunity=%v participated=%v", found, opportunity, participated)
	}
	again, replayed, err := activities.Unlink(f.ctx, f.userID, commute.ID, model.UnlinkSportActivityRequest{ExpectedRevision: done.Revision, OperationKey: "unlink-1"}, "2026-10-26", "UTC", "h-unlink-1")
	if err != nil || !replayed || again.ScheduledWorkout.Revision != result.ScheduledWorkout.Revision {
		t.Fatalf("unlink replay must return the stored result: replayed=%v err=%v", replayed, err)
	}
	_, _, err = activities.Unlink(f.ctx, f.userID, commute.ID, model.UnlinkSportActivityRequest{ExpectedRevision: result.ScheduledWorkout.Revision, OperationKey: "unlink-2"}, "2026-10-26", "UTC", "h-unlink-2")
	var conflict *model.SportSessionConflictError
	if !errors.As(err, &conflict) || conflict.Reason != "not_linked" {
		t.Fatalf("unlinking an unlinked activity must conflict with not_linked: %v", err)
	}

	// The real ride arrives unlinked and is linked by hand without a second credit.
	real, _, err := activities.Create(f.ctx, f.userID, &model.SportActivity{Date: "2026-10-01", SportID: "cycling", SportName: "Cycling", DurationMinutes: 95, Source: model.ActivitySourceManual}, "UTC", "real-ride", "h-real")
	if err != nil || real.ScheduledWorkoutID != nil {
		t.Fatalf("manual create must stay unlinked: %+v err=%v", real, err)
	}
	creditRevision := f.participationRevision(t, "2026-10-01")
	missed := f.load(t, ride.ID)
	linked, _, err := activities.Link(f.ctx, f.userID, real.ID, model.LinkSportActivityRequest{ScheduledWorkoutID: ride.ID, ExpectedRevision: missed.Revision, OperationKey: "link-1"}, "h-link-1")
	if err != nil {
		t.Fatal(err)
	}
	if linked.ScheduledWorkout.Status != model.WorkoutStatusCompleted || linked.ScheduledWorkout.Sport.SportActivityID == nil || *linked.ScheduledWorkout.Sport.SportActivityID != real.ID ||
		linked.SportActivity.ScheduledWorkoutID == nil || *linked.SportActivity.ScheduledWorkoutID != ride.ID {
		t.Fatalf("link must complete the session by the chosen activity: %+v", linked)
	}
	if f.participationRevision(t, "2026-10-01") != creditRevision {
		t.Fatal("linking must not credit participation again")
	}

	// Conflicts and validation.
	other := f.sportSession(t, "2026-10-02", 3, "cycling", "Cycling")
	_, _, err = activities.Link(f.ctx, f.userID, real.ID, model.LinkSportActivityRequest{ScheduledWorkoutID: other.ID, ExpectedRevision: other.Revision, OperationKey: "link-2"}, "h-link-2")
	if !errors.As(err, &conflict) || conflict.Reason != "activity_linked" {
		t.Fatalf("an already linked activity must conflict: %v", err)
	}
	_, _, err = activities.Link(f.ctx, f.userID, commute.ID, model.LinkSportActivityRequest{ScheduledWorkoutID: ride.ID, ExpectedRevision: linked.ScheduledWorkout.Revision, OperationKey: "link-3"}, "h-link-3")
	if !errors.As(err, &conflict) || conflict.Reason != "session_completed" {
		t.Fatalf("a completed session must conflict: %v", err)
	}
	_, _, err = activities.Link(f.ctx, f.userID, commute.ID, model.LinkSportActivityRequest{ScheduledWorkoutID: other.ID, ExpectedRevision: other.Revision + 5, OperationKey: "link-4"}, "h-link-4")
	var revision *model.ConflictError
	if !errors.As(err, &revision) || revision.Actual != other.Revision {
		t.Fatalf("a stale session revision must be a revision conflict: %v", err)
	}
	game := f.sportSession(t, "2026-10-03", 4, "volleyball", "Volleyball")
	_, _, err = activities.Link(f.ctx, f.userID, commute.ID, model.LinkSportActivityRequest{ScheduledWorkoutID: game.ID, ExpectedRevision: game.Revision, OperationKey: "link-5"}, "h-link-5")
	var validation *model.ValidationError
	if !errors.As(err, &validation) || validation.Field != "sport_id" {
		t.Fatalf("a sport mismatch must be 422 sport_id: %v", err)
	}
	gym := f.load(t, f.workout.ID)
	_, _, err = activities.Link(f.ctx, f.userID, commute.ID, model.LinkSportActivityRequest{ScheduledWorkoutID: gym.ID, ExpectedRevision: gym.Revision, OperationKey: "link-6"}, "h-link-6")
	if !errors.As(err, &validation) || validation.Field != "kind" {
		t.Fatalf("an exercise workout must be 422 kind: %v", err)
	}
	foreign := newReleaseFixture(t)
	_, _, err = NewSportActivityDAO(foreign.pool).Link(foreign.ctx, foreign.userID, commute.ID, model.LinkSportActivityRequest{ScheduledWorkoutID: other.ID, ExpectedRevision: 1, OperationKey: "link-7"}, "h-link-7")
	var notFound *model.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("a foreign activity must be 404: %v", err)
	}

	// A future session unlinked reopens as planned with no finalization.
	future := f.sportSession(t, "2099-01-10", 5, "cycling", "Cycling")
	early, _, err := activities.Create(f.ctx, f.userID, &model.SportActivity{Date: "2026-10-04", SportID: "cycling", SportName: "Cycling", DurationMinutes: 60, Source: model.ActivitySourceManual, ScheduledWorkoutID: &future.ID}, "UTC", "early", "h-early")
	if err != nil {
		t.Fatal(err)
	}
	completed := f.load(t, future.ID)
	reopened, _, err := activities.Unlink(f.ctx, f.userID, early.ID, model.UnlinkSportActivityRequest{ExpectedRevision: completed.Revision, OperationKey: "unlink-future"}, "2026-10-26", "UTC", "h-unlink-future")
	if err != nil || reopened.ScheduledWorkout.Status != model.WorkoutStatusPlanned || reopened.ScheduledWorkout.FinalizedAt != nil {
		t.Fatalf("a future session must reopen as planned: %+v err=%v", reopened, err)
	}
}

func TestImportAutoMatchPrefersClosestDurationAndSkipsShortImports(t *testing.T) {
	f := newReleaseFixture(t)
	activities := NewSportActivityDAO(f.pool)
	short := f.sportSessionTarget(t, "2026-10-05", 2, "cycling", "Cycling", 30)
	long := f.sportSessionTarget(t, "2026-10-05", 1, "cycling", "Cycling", 90)
	external := "HK-LONG"
	imported, _, err := activities.Create(f.ctx, f.userID, &model.SportActivity{Date: "2026-10-05", SportID: "cycling", SportName: "Cycling", DurationMinutes: 85, Source: model.ActivitySourceAppleHealth, ExternalID: &external}, "UTC", "hk:long", "h-long")
	if err != nil || imported.ScheduledWorkoutID == nil || *imported.ScheduledWorkoutID != long.ID {
		t.Fatalf("an 85-minute ride must fulfil the 90-minute plan, not the earlier-sequence 30-minute one: %+v err=%v", imported, err)
	}
	if f.load(t, short.ID).Status != model.WorkoutStatusPlanned {
		t.Fatal("the other session must stay open")
	}

	ride := f.sportSessionTarget(t, "2026-10-06", 1, "cycling", "Cycling", 90)
	commuteID := "HK-COMMUTE"
	commute, _, err := activities.Create(f.ctx, f.userID, &model.SportActivity{Date: "2026-10-06", SportID: "cycling", SportName: "Cycling", DurationMinutes: 15, Source: model.ActivitySourceAppleHealth, ExternalID: &commuteID}, "UTC", "hk:commute", "h-commute")
	if err != nil || commute.ScheduledWorkoutID != nil {
		t.Fatalf("a 15-minute commute must not fulfil a 90-minute plan: %+v err=%v", commute, err)
	}
	thirdID := "HK-THIRD"
	third, _, err := activities.Create(f.ctx, f.userID, &model.SportActivity{Date: "2026-10-06", SportID: "cycling", SportName: "Cycling", DurationMinutes: 30, Source: model.ActivitySourceAppleHealth, ExternalID: &thirdID}, "UTC", "hk:third", "h-third")
	if err != nil || third.ScheduledWorkoutID == nil || *third.ScheduledWorkoutID != ride.ID {
		t.Fatalf("a ride of exactly a third of the plan must link: %+v err=%v", third, err)
	}

	effort := 7
	rated, _, err := activities.Create(f.ctx, f.userID, &model.SportActivity{Date: "2026-10-07", SportID: "volleyball", SportName: "Volleyball", DurationMinutes: 90, Source: model.ActivitySourceManual, PerceivedEffort: &effort}, "UTC", "rated", "h-rated")
	if err != nil {
		t.Fatal(err)
	}
	read, err := activities.Get(f.ctx, f.userID, rated.ID)
	if err != nil || read.PerceivedEffort == nil || *read.PerceivedEffort != 7 {
		t.Fatalf("effort must round-trip: %+v err=%v", read, err)
	}
}
