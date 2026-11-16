//go:build integration

package dao

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// fillToday is a Monday. The fixture athlete is available Monday and
// Wednesday and has one active, unplaced program with a single unit.
const fillToday = "2026-11-16"

// programSessions returns "date weekday" for each session of the fixture
// program dated on or after from, in date order.
func (f *releaseFixture) programSessions(t *testing.T, from string) []string {
	t.Helper()
	rows, err := f.pool.Query(f.ctx, `SELECT to_char(date, 'YYYY-MM-DD') FROM scheduled_workouts
		WHERE user_id=$1 AND program_id=$2 AND date >= $3 ORDER BY date, created_at`, f.userID, f.program.ID, from)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	dates := []string{}
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err != nil {
			t.Fatal(err)
		}
		dates = append(dates, date)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return dates
}

func (f *releaseFixture) scheduledThrough(t *testing.T) string {
	t.Helper()
	var through *string
	if err := f.pool.QueryRow(f.ctx, `SELECT to_char(scheduled_through, 'YYYY-MM-DD') FROM programs WHERE id=$1`, f.program.ID).Scan(&through); err != nil {
		t.Fatal(err)
	}
	if through == nil {
		return ""
	}
	return *through
}

func (f *releaseFixture) fill(t *testing.T, today string) {
	t.Helper()
	if err := f.schedules.EnsureFilled(f.ctx, f.userID, today, []int{1, 3}); err != nil {
		t.Fatalf("EnsureFilled(%s) error = %v", today, err)
	}
}

func assertMondaysAndWednesdays(t *testing.T, dates []string, from, through string) {
	t.Helper()
	want := []string{}
	start, _ := model.ParseDate(from)
	end, _ := model.ParseDate(through)
	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		if date.Weekday() == time.Monday || date.Weekday() == time.Wednesday {
			want = append(want, date.Format(time.DateOnly))
		}
	}
	if len(dates) != len(want) {
		t.Fatalf("got %d sessions %v, want %d from %s through %s", len(dates), dates, len(want), from, through)
	}
	for index := range want {
		if dates[index] != want[index] {
			t.Fatalf("session %d on %s, want %s", index, dates[index], want[index])
		}
	}
}

func TestEnsureFilledPlacesAnUnplacedProgramAndFillsTheHorizon(t *testing.T) {
	f := newReleaseFixture(t)
	f.fill(t, fillToday)

	horizon, _ := shiftDate(fillToday, model.ScheduleHorizonDays)
	if got := f.scheduledThrough(t); got != horizon {
		t.Fatalf("scheduled_through = %q, want %q", got, horizon)
	}
	assertMondaysAndWednesdays(t, f.programSessions(t, fillToday), fillToday, horizon)

	program, err := (&programDAO{pool: f.pool}).Get(f.ctx, f.userID, f.program.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := []int(program.Workouts[0].Weekdays); len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("placed weekdays = %v, want [1 3]", got)
	}
	if program.Revision != f.program.Revision {
		t.Fatalf("placement changed the program revision: %d, want %d", program.Revision, f.program.Revision)
	}
	if n := f.count(t, `SELECT count(*) FROM scheduled_workouts WHERE user_id=$1 AND program_id=$2 AND date >= $3 AND (slot_date IS DISTINCT FROM date OR program_workout_id IS NULL)`, f.userID, f.program.ID, fillToday); n != 0 {
		t.Fatalf("%d filled sessions lack their slot date or unit", n)
	}
	if n := f.count(t, `SELECT count(*) FROM scheduled_sets ss JOIN scheduled_workouts sw ON sw.id=ss.scheduled_workout_id WHERE sw.user_id=$1 AND sw.date=$2`, f.userID, fillToday); n != 1 {
		t.Fatalf("today's session has %d required sets, want 1", n)
	}

	// A second fill on the same day writes nothing.
	before := f.snapshot(t)
	f.fill(t, fillToday)
	if after := f.snapshot(t); after != before {
		t.Fatal("a repeated fill changed the database")
	}
}

func TestEnsureFilledNeverCreatesBeforeTodayOrBehindTheWatermark(t *testing.T) {
	f := newReleaseFixture(t)
	f.fill(t, fillToday)
	horizon, _ := shiftDate(fillToday, model.ScheduleHorizonDays)

	// The athlete deletes one future session and the first week passes.
	var deleted uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `SELECT id FROM scheduled_workouts WHERE user_id=$1 AND date='2026-11-25'`, f.userID).Scan(&deleted); err != nil {
		t.Fatal(err)
	}
	if err := f.schedules.Delete(f.ctx, f.userID, deleted); err != nil {
		t.Fatal(err)
	}
	later := "2026-11-23"
	f.fill(t, later)

	dates := f.programSessions(t, fillToday)
	for _, date := range dates {
		if date == "2026-11-25" {
			t.Fatal("a deleted session was created again")
		}
	}
	newHorizon, _ := shiftDate(later, model.ScheduleHorizonDays)
	if got := f.scheduledThrough(t); got != newHorizon {
		t.Fatalf("scheduled_through = %q, want %q", got, newHorizon)
	}
	next, _ := shiftDate(horizon, 1)
	assertMondaysAndWednesdays(t, f.programSessions(t, next), next, newHorizon)
	if n := f.count(t, `SELECT count(*) FROM scheduled_workouts WHERE user_id=$1 AND slot_date < $2`, f.userID, fillToday); n != 0 {
		t.Fatalf("%d sessions were filled before today", n)
	}
}

func TestEnsureFilledSkipsDatesThatPassedWhileAway(t *testing.T) {
	f := newReleaseFixture(t)
	f.fill(t, fillToday)
	// The athlete returns long after the filled range ran out.
	back := "2027-03-01" // a Monday
	f.fill(t, back)
	horizon, _ := shiftDate(fillToday, model.ScheduleHorizonDays)
	gapStart, _ := shiftDate(horizon, 1)
	if n := f.count(t, `SELECT count(*) FROM scheduled_workouts WHERE user_id=$1 AND date >= $2 AND date < $3`, f.userID, gapStart, back); n != 0 {
		t.Fatalf("%d sessions were back-filled into the gap", n)
	}
	newHorizon, _ := shiftDate(back, model.ScheduleHorizonDays)
	assertMondaysAndWednesdays(t, f.programSessions(t, back), back, newHorizon)
}

func TestEnsureFilledContinuesAfterSessionsFromBeforeTheWatermark(t *testing.T) {
	f := newReleaseFixture(t)
	// A session scheduled the old way, with no slot date, ten days out.
	workouts := []model.ScheduledWorkout{releaseWorkout(f.program.ID, "2026-11-26")}
	if err := f.schedules.Create(f.ctx, f.userID, workouts); err != nil {
		t.Fatal(err)
	}
	f.fill(t, fillToday)
	horizon, _ := shiftDate(fillToday, model.ScheduleHorizonDays)
	dates := f.programSessions(t, fillToday)
	if dates[0] != "2026-11-26" {
		t.Fatalf("first session on %s, want the existing 2026-11-26 session", dates[0])
	}
	assertMondaysAndWednesdays(t, dates[1:], "2026-11-27", horizon)
}

// Concurrent reads serialize on the athlete's advisory locks, so this proves
// the fill is safe when called concurrently, not that the unique slot index
// arbitrates a race. TestSlotIndexRejectsASecondSessionForTheSameSlot proves
// the index itself, which is the backstop for a path that forgets the lock.
func TestConcurrentFillsCreateEachSlotOnce(t *testing.T) {
	f := newReleaseFixture(t)
	const readers = 6
	var wg sync.WaitGroup
	errs := make(chan error, readers)
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- f.schedules.EnsureFilled(f.ctx, f.userID, fillToday, []int{1, 3})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent EnsureFilled error = %v", err)
		}
	}
	horizon, _ := shiftDate(fillToday, model.ScheduleHorizonDays)
	assertMondaysAndWednesdays(t, f.programSessions(t, fillToday), fillToday, horizon)
	if n := f.count(t, `SELECT count(*) FROM (SELECT program_workout_id, date FROM scheduled_workouts WHERE user_id=$1 AND date >= $2 GROUP BY 1, 2 HAVING count(*) > 1) d`, f.userID, fillToday); n != 0 {
		t.Fatalf("%d slots hold more than one session", n)
	}
}

func TestSlotIndexRejectsASecondSessionForTheSameSlot(t *testing.T) {
	f := newReleaseFixture(t)
	f.fill(t, fillToday)
	var unitID uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `SELECT id FROM program_workouts WHERE program_id=$1`, f.program.ID).Scan(&unitID); err != nil {
		t.Fatal(err)
	}
	_, err := f.pool.Exec(f.ctx, `INSERT INTO scheduled_workouts (user_id, program_id, program_workout_id, date, slot_date, name, status)
		VALUES ($1,$2,$3,$4,$4,'A','planned')`, f.userID, f.program.ID, unitID, fillToday)
	if err == nil {
		t.Fatal("a second session for an occupied slot was accepted; the unique slot index is missing")
	}
}

func TestRecoverOntoADateThatAlreadyHoldsTheUnitStillWorks(t *testing.T) {
	f := newReleaseFixture(t)
	f.fill(t, fillToday)
	var unitID uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `SELECT id FROM program_workouts WHERE program_id=$1`, f.program.ID).Scan(&unitID); err != nil {
		t.Fatal(err)
	}
	// The fixture's past workout belongs to the same unit and was missed.
	f.exec(t, `UPDATE scheduled_workouts SET program_workout_id=$2 WHERE id=$1`, f.workout.ID, unitID)
	f.markMissed(t, f.workout.ID)
	recovered, _, err := f.schedules.RecoverIdempotent(f.ctx, f.userID, fillToday, releaseRecord("schedule/recover"))
	if err != nil {
		t.Fatalf("RecoverIdempotent() error = %v", err)
	}
	if recovered.Date != fillToday {
		t.Fatalf("recovered onto %s, want %s", recovered.Date, fillToday)
	}
	if n := f.count(t, `SELECT count(*) FROM scheduled_workouts WHERE user_id=$1 AND date=$2 AND status='planned'`, f.userID, fillToday); n != 2 {
		t.Fatalf("%d planned sessions today, want the regular one and the recovered copy", n)
	}
}
