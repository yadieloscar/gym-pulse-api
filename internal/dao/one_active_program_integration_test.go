//go:build integration

package dao

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// The scenarios below follow specs/031-one-active-program/quickstart.md.
// fillToday (2026-11-16) is a Monday; starts use the next day.
const startTomorrow = "2026-11-17"

func weekUnit(name string, sequence int, weekdays ...int) model.ProgramWorkout {
	reps := 8
	return model.ProgramWorkout{Name: name, SequencePosition: sequence, Weekdays: weekdays,
		Exercises: []model.ProgramExercise{{Name: name + " lift", Category: "legs", Modality: "strength", ExerciseOrder: 1, TargetSets: 2, TargetReps: &reps}}}
}

func weekSport(name string, sequence int, sportID string, weekdays ...int) model.ProgramWorkout {
	return model.ProgramWorkout{Kind: model.UnitKindSport, Name: name, SequencePosition: sequence, Weekdays: weekdays,
		Sport: &model.SportTarget{SportID: sportID, SportName: name, TargetDurationMinutes: 90}, Exercises: []model.ProgramExercise{}}
}

// pushPullLegsWeek is the owner's example: three lifting days, a bike ride,
// and volleyball, each on its own weekday.
func pushPullLegsWeek() []model.ProgramWorkout {
	return []model.ProgramWorkout{
		weekUnit("Push", 1, 1), weekUnit("Pull", 2, 3), weekSport("Bike ride", 3, "cycling", 4),
		weekUnit("Legs", 4, 5), weekSport("Volleyball", 5, "volleyball", 6),
	}
}

// saveProgram stores a program without starting it, as contract 1.10.0
// clients do with active:false.
func (f *releaseFixture) saveProgram(t *testing.T, name string, units []model.ProgramWorkout) *model.Program {
	t.Helper()
	program := &model.Program{Name: name, PrimaryGoal: "strength", Roadmap: map[string]any{}, Active: false, Workouts: units}
	saved, _, err := f.mutations.CreateProgram(f.ctx, f.userID, program, saveNoon, releaseRecord("programs/create"))
	if err != nil {
		t.Fatalf("saving program %q: %v", name, err)
	}
	return saved
}

func (f *releaseFixture) start(t *testing.T, target *model.Program, start string, profile *model.TrainingProfile, expectedProfileRevision int64) *PlanTransitionResult {
	t.Helper()
	through, _ := shiftDate(fillToday, model.ScheduleHorizonDays)
	to, _ := shiftDate(start, 27)
	result, err := (&planTransitionDAO{pool: f.pool}).Apply(f.ctx, f.userID, PlanTransitionApply{
		Profile: profile, ExpectedProfileRevision: expectedProfileRevision, Target: target, AvailableDays: []int{1, 3},
		Start: start, Through: through, From: start, To: to, Today: fillToday, OperationKey: uuid.NewString(), RequestHash: "hash",
	})
	if err != nil {
		t.Fatalf("starting %q: %v", target.Name, err)
	}
	return result
}

// sessionsOf returns "date name" for a program's sessions from a date on.
func (f *releaseFixture) sessionsOf(t *testing.T, programID uuid.UUID, from string) []string {
	t.Helper()
	rows, err := f.pool.Query(f.ctx, `SELECT to_char(date, 'YYYY-MM-DD') || ' ' || name FROM scheduled_workouts
		WHERE user_id=$1 AND program_id=$2 AND date >= $3 ORDER BY date, sequence_position`, f.userID, programID, from)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	lines := []string{}
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return lines
}

func (f *releaseFixture) activeProgramIDs(t *testing.T) []uuid.UUID {
	t.Helper()
	rows, err := f.pool.Query(f.ctx, `SELECT id FROM programs WHERE user_id=$1 AND active=true`, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func (f *releaseFixture) profileRevision(t *testing.T) int64 {
	t.Helper()
	var revision int64
	if err := f.pool.QueryRow(f.ctx, `SELECT revision FROM training_profiles WHERE user_id=$1`, f.userID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	return revision
}

// Q1: a custom week lands on the weekdays the athlete chose, every week.
func TestStartedProgramFollowsItsWeekdaysEveryWeek(t *testing.T) {
	f := newReleaseFixture(t)
	units := append(pushPullLegsWeek(), weekUnit("Core", 6, 6)) // shares Saturday with volleyball
	program := f.saveProgram(t, "Push pull legs", units)
	f.start(t, program, startTomorrow, nil, 0)

	want := map[string]time.Weekday{"Push": time.Monday, "Pull": time.Wednesday, "Bike ride": time.Thursday, "Legs": time.Friday, "Volleyball": time.Saturday, "Core": time.Saturday}
	rows, err := f.pool.Query(f.ctx, `SELECT to_char(date, 'YYYY-MM-DD'), name, kind FROM scheduled_workouts WHERE user_id=$1 AND program_id=$2 AND date BETWEEN $3 AND $4`, f.userID, program.ID, startTomorrow, "2026-12-14")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	perUnit := map[string]int{}
	for rows.Next() {
		var date, name, kind string
		if err := rows.Scan(&date, &name, &kind); err != nil {
			t.Fatal(err)
		}
		parsed, _ := model.ParseDate(date)
		if parsed.Weekday() != want[name] {
			t.Fatalf("%s scheduled on %s (%s), want %s", name, date, parsed.Weekday(), want[name])
		}
		if (name == "Bike ride" || name == "Volleyball") != (kind == "sport") {
			t.Fatalf("%s has kind %q", name, kind)
		}
		perUnit[name]++
	}
	// 2026-11-17 through 2026-12-14 is exactly four weeks.
	for name := range want {
		if perUnit[name] != 4 {
			t.Fatalf("%s scheduled %d times in four weeks, want 4", name, perUnit[name])
		}
	}
	if n := f.count(t, `SELECT count(*) FROM scheduled_workouts WHERE user_id=$1 AND date='2026-11-21' AND status='planned'`, f.userID); n != 2 {
		t.Fatalf("%d sessions on the shared Saturday, want volleyball and core", n)
	}
}

// Q2: saving a program without starting it changes nothing else.
func TestSavingAProgramDoesNotStartIt(t *testing.T) {
	f := newReleaseFixture(t)
	f.fill(t, fillToday)
	before := f.sessionsOf(t, f.program.ID, fillToday)

	saved := f.saveProgram(t, "Saved for later", pushPullLegsWeek())

	if saved.Active {
		t.Fatal("the saved program is active")
	}
	if active := f.activeProgramIDs(t); len(active) != 1 || active[0] != f.program.ID {
		t.Fatalf("active programs = %v, want only the fixture program", active)
	}
	if after := f.sessionsOf(t, f.program.ID, fillToday); !slices.Equal(after, before) {
		t.Fatal("saving a program changed the schedule")
	}
	if n := f.count(t, `SELECT count(*) FROM scheduled_workouts WHERE user_id=$1 AND program_id=$2`, f.userID, saved.ID); n != 0 {
		t.Fatalf("the saved program has %d sessions, want none", n)
	}
	if got := []int(saved.Workouts[0].Weekdays); !slices.Equal(got, []int{1}) {
		t.Fatalf("saved weekdays = %v, want [1]", got)
	}
}

// Q3: one start makes the target the only active program and replaces the
// other program's unstarted sessions from the start date on, with no end.
func TestStartReplacesTheOtherProgramFromTheStartDateOn(t *testing.T) {
	f := newReleaseFixture(t)
	f.fill(t, fillToday)
	revisionBefore := f.profileRevision(t)
	saved := f.saveProgram(t, "Push pull legs", pushPullLegsWeek())

	result := f.start(t, saved, startTomorrow, nil, 0)

	if active := f.activeProgramIDs(t); len(active) != 1 || active[0] != saved.ID {
		t.Fatalf("active programs = %v, want only the started program", active)
	}
	if left := f.sessionsOf(t, f.program.ID, startTomorrow); len(left) != 0 {
		t.Fatalf("the replaced program still has sessions from the start date on: %v", left)
	}
	if today := f.sessionsOf(t, f.program.ID, fillToday); !slices.Equal(today, []string{fillToday + " A"}) {
		t.Fatalf("sessions before the start date changed: %v", today)
	}
	if got := f.profileRevision(t); got != revisionBefore {
		t.Fatalf("profile revision = %d, want unchanged %d", got, revisionBefore)
	}
	if len(result.Workouts) != 20 || result.Workouts[0].Date != "2026-11-18" || result.Workouts[0].Name != "Pull" || result.Workouts[0].ID == uuid.Nil {
		t.Fatalf("returned %d sessions starting %+v, want 20 stored sessions starting Wed Pull", len(result.Workouts), result.Workouts[0])
	}
	var through string
	if err := f.pool.QueryRow(f.ctx, `SELECT to_char(scheduled_through, 'YYYY-MM-DD') FROM programs WHERE id=$1`, saved.ID).Scan(&through); err != nil || through != "2027-01-11" {
		t.Fatalf("scheduled_through = %q err=%v, want 2027-01-11", through, err)
	}
}

// Q4: a start never touches a session that is in progress, finished, or missed.
func TestStartKeepsStartedFinishedAndMissedSessions(t *testing.T) {
	f := newReleaseFixture(t)
	f.fill(t, fillToday)
	ids := map[string]uuid.UUID{}
	for date, label := range map[string]string{"2026-11-18": "in_progress", "2026-11-23": "completed", "2026-11-25": "missed"} {
		var id uuid.UUID
		if err := f.pool.QueryRow(f.ctx, `SELECT id FROM scheduled_workouts WHERE user_id=$1 AND date=$2`, f.userID, date).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids[label] = id
	}
	f.exec(t, `INSERT INTO workout_sessions (user_id,date,name,status,scheduled_workout_id) VALUES ($1,'2026-11-18','A','active',$2)`, f.userID, ids["in_progress"])
	f.exec(t, `UPDATE scheduled_workouts SET status='in_progress' WHERE id=$1`, ids["in_progress"])
	f.exec(t, `UPDATE scheduled_workouts SET status='completed', finalized_at=now() WHERE id=$1`, ids["completed"])
	f.exec(t, `UPDATE scheduled_workouts SET status='missed', finalized_at=now() WHERE id=$1`, ids["missed"])

	saved := f.saveProgram(t, "Push pull legs", pushPullLegsWeek())
	f.start(t, saved, startTomorrow, nil, 0)

	want := []string{"2026-11-18 A", "2026-11-23 A", "2026-11-25 A"}
	if got := f.sessionsOf(t, f.program.ID, startTomorrow); !slices.Equal(got, want) {
		t.Fatalf("kept sessions of the replaced program = %v, want %v", got, want)
	}
	for label, id := range ids {
		if got := f.load(t, id).Status; got != label {
			t.Fatalf("%s session now has status %q", label, got)
		}
	}
}

// Q5: a profile change that names the active program changes no session.
func TestProfileChangeLeavesTheActiveProgramAndItsSessionsAlone(t *testing.T) {
	f := newReleaseFixture(t)
	saved := f.saveProgram(t, "Push pull legs", pushPullLegsWeek())
	f.start(t, saved, startTomorrow, nil, 0)
	active, err := (&programDAO{pool: f.pool}).Get(f.ctx, f.userID, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	before := f.sessionsOf(t, saved.ID, fillToday)
	sessionRows := f.count(t, `SELECT count(*) FROM scheduled_workouts WHERE user_id=$1`, f.userID)

	profile := &model.TrainingProfile{PrimaryGoal: "hypertrophy", AvailableDays: []int{2, 4, 6}, UsualActivity: "moderate", Experience: "beginner", Equipment: []string{}, SessionDurationMinutes: 45, Timezone: "UTC", Preferences: map[string]any{}}
	result := f.start(t, active, startTomorrow, profile, f.profileRevision(t))

	if result.ProgramID != saved.ID {
		t.Fatalf("result program = %s, want the active program", result.ProgramID)
	}
	if after := f.sessionsOf(t, saved.ID, fillToday); !slices.Equal(after, before) {
		t.Fatal("a profile change moved sessions of the active program")
	}
	if n := f.count(t, `SELECT count(*) FROM scheduled_workouts WHERE user_id=$1`, f.userID); n != sessionRows {
		t.Fatalf("session rows = %d, want unchanged %d", n, sessionRows)
	}
	reloaded, err := (&programDAO{pool: f.pool}).Get(f.ctx, f.userID, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.Active || reloaded.Revision != active.Revision {
		t.Fatalf("active=%v revision=%d, want the program untouched at revision %d", reloaded.Active, reloaded.Revision, active.Revision)
	}
	var goal string
	var days []int32
	if err := f.pool.QueryRow(f.ctx, `SELECT primary_goal, available_days FROM training_profiles WHERE user_id=$1`, f.userID).Scan(&goal, &days); err != nil {
		t.Fatal(err)
	}
	if goal != "hypertrophy" || !slices.Equal(days, []int32{2, 4, 6}) {
		t.Fatalf("profile = %s %v, want the proposed change saved", goal, days)
	}
}

// A released app creates a program (active, without weekdays) and then
// transitions to it. The first week must be the one sequence dealing gave.
func TestStartPlacesAnActiveProgramThatHasNoWeekdaysYet(t *testing.T) {
	f := newReleaseFixture(t)
	legacy := &model.Program{Name: "Knee starter", PrimaryGoal: "strength", Roadmap: map[string]any{}, Active: true, Workouts: []model.ProgramWorkout{
		weekUnit("Tendon A", 1), weekSport("Volleyball", 2, "volleyball"), weekUnit("Tendon B", 3),
	}}
	for i := range legacy.Workouts {
		legacy.Workouts[i].Weekdays = nil
	}
	created, _, err := f.mutations.CreateProgram(f.ctx, f.userID, legacy, saveNoon, releaseRecord("programs/create"))
	if err != nil {
		t.Fatal(err)
	}
	through, _ := shiftDate(fillToday, model.ScheduleHorizonDays)
	if _, err := (&planTransitionDAO{pool: f.pool}).Apply(f.ctx, f.userID, PlanTransitionApply{Target: created, AvailableDays: []int{2, 3, 5},
		Start: startTomorrow, Through: through, From: startTomorrow, To: "2026-11-23", Today: fillToday, OperationKey: uuid.NewString(), RequestHash: "hash"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"2026-11-17 Tendon A", "2026-11-18 Volleyball", "2026-11-20 Tendon B", "2026-11-24 Tendon A"}
	if got := f.sessionsOf(t, created.ID, startTomorrow)[:4]; !slices.Equal(got, want) {
		t.Fatalf("first sessions = %v, want %v", got, want)
	}
	stored, err := (&programDAO{pool: f.pool}).Get(f.ctx, f.userID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.Placed() || stored.Revision != created.Revision {
		t.Fatalf("placed=%v revision=%d, want placed at the client's revision %d", stored.Placed(), stored.Revision, created.Revision)
	}
}

// With no active program a start can create a starter copy, already placed.
func TestStartCreatesAPlacedStarterCopy(t *testing.T) {
	f := newReleaseFixture(t)
	f.exec(t, `UPDATE programs SET active=false WHERE user_id=$1`, f.userID)
	target := &model.Program{Name: "Foundation", PrimaryGoal: "strength", Roadmap: map[string]any{}, Active: true,
		Workouts: []model.ProgramWorkout{weekUnit("Full Body A", 1, 1), weekUnit("Full Body B", 2, 3)}}
	result := f.start(t, target, fillToday, nil, 0)
	if active := f.activeProgramIDs(t); len(active) != 1 || active[0] != result.ProgramID {
		t.Fatalf("active programs = %v, want the new starter copy", active)
	}
	want := []string{"2026-11-16 Full Body A", "2026-11-18 Full Body B"}
	if got := f.sessionsOf(t, result.ProgramID, fillToday)[:2]; !slices.Equal(got, want) {
		t.Fatalf("first sessions = %v, want %v", got, want)
	}
}

func TestStartReplaysAndRejectsAReusedKey(t *testing.T) {
	f := newReleaseFixture(t)
	saved := f.saveProgram(t, "Push pull legs", pushPullLegsWeek())
	through, _ := shiftDate(fillToday, model.ScheduleHorizonDays)
	apply := PlanTransitionApply{Target: saved, AvailableDays: []int{1, 3}, Start: startTomorrow, Through: through, From: startTomorrow, To: "2026-11-23", Today: fillToday, OperationKey: "start-once", RequestHash: "hash"}
	transitions := &planTransitionDAO{pool: f.pool}
	if _, err := transitions.Apply(f.ctx, f.userID, apply); err != nil {
		t.Fatal(err)
	}
	before := f.snapshot(t)
	for range 4 {
		replay, err := transitions.Apply(f.ctx, f.userID, apply)
		if err != nil || !replay.Replay || replay.ProgramID != saved.ID {
			t.Fatalf("replay = %+v err=%v, want a replay of the first start", replay, err)
		}
	}
	if after := f.snapshot(t); after != before {
		t.Fatal("replaying a start changed the database")
	}
	apply.RequestHash = "different"
	var conflict *model.ConflictError
	if _, err := transitions.Apply(f.ctx, f.userID, apply); !errors.As(err, &conflict) {
		t.Fatalf("reusing the key with another payload error = %v, want a conflict", err)
	}
}

func TestStartOfAnotherAthletesProgramIsNotFound(t *testing.T) {
	owner := newReleaseFixture(t)
	other := newReleaseFixture(t)
	foreign := owner.saveProgram(t, "Private", pushPullLegsWeek())
	through, _ := shiftDate(fillToday, model.ScheduleHorizonDays)
	_, err := (&planTransitionDAO{pool: other.pool}).Apply(other.ctx, other.userID, PlanTransitionApply{Target: foreign, AvailableDays: []int{1}, Start: startTomorrow, Through: through, From: startTomorrow, To: "2026-11-23", Today: fillToday, OperationKey: uuid.NewString(), RequestHash: "hash"})
	var notFound *model.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("starting a foreign program error = %v, want not found", err)
	}
	if active := other.activeProgramIDs(t); len(active) != 1 || active[0] != other.program.ID {
		t.Fatal("a refused start changed the other athlete's active program")
	}
}

// saveNoon is midday UTC on fillToday; the fixture athlete's timezone is UTC.
var saveNoon = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

// startOwnWeek replaces the fixture program with the push/pull/legs week,
// started today, and returns it as stored.
func (f *releaseFixture) startOwnWeek(t *testing.T) *model.Program {
	t.Helper()
	saved := f.saveProgram(t, "Push pull legs", pushPullLegsWeek())
	f.start(t, saved, fillToday, nil, 0)
	program, err := (&programDAO{pool: f.pool}).Get(f.ctx, f.userID, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	return program
}

func (f *releaseFixture) sessionOn(t *testing.T, programID uuid.UUID, date, name string) *model.ScheduledWorkout {
	t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `SELECT id FROM scheduled_workouts WHERE user_id=$1 AND program_id=$2 AND date=$3 AND name=$4`, f.userID, programID, date, name).Scan(&id); err != nil {
		t.Fatalf("no %s session on %s: %v", name, date, err)
	}
	return f.load(t, id)
}

func (f *releaseFixture) move(t *testing.T, workout *model.ScheduledWorkout, date string, record model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error) {
	t.Helper()
	return f.schedules.MoveIdempotent(f.ctx, f.userID, workout.ID, date, fillToday, workout.Revision, record)
}

// Q7: one planned session moves; nothing else does, and its slot stays empty.
func TestMoveChangesOneSessionOnly(t *testing.T) {
	f := newReleaseFixture(t)
	program := f.startOwnWeek(t)
	legs := f.sessionOn(t, program.ID, "2026-11-20", "Legs")
	ride := f.sessionOn(t, program.ID, "2026-11-19", "Bike ride")
	before := f.sessionsOf(t, program.ID, fillToday)

	record := releaseRecord("scheduled-workouts/move")
	moved, replay, err := f.move(t, legs, "2026-11-21", record)
	if err != nil || replay {
		t.Fatalf("move error = %v replay = %v", err, replay)
	}
	if moved.Date != "2026-11-21" || moved.Revision != legs.Revision+1 || len(moved.RequiredSets) != len(legs.RequiredSets) {
		t.Fatalf("moved = %+v, want Saturday at revision %d with its sets", moved, legs.Revision+1)
	}
	again, replay, err := f.move(t, legs, "2026-11-21", record)
	if err != nil || !replay || again.Revision != moved.Revision {
		t.Fatalf("replay = %+v replay=%v err=%v, want the stored response", again, replay, err)
	}

	after := f.sessionsOf(t, program.ID, fillToday)
	want := slices.Clone(before)
	index := slices.Index(want, "2026-11-20 Legs")
	want = slices.Delete(want, index, index+1)
	want = slices.Insert(want, slices.Index(want, "2026-11-21 Volleyball"), "2026-11-21 Legs")
	if !slices.Equal(after, want) {
		t.Fatalf("after the move the schedule is %v, want only Legs moved to Saturday", after[:8])
	}

	// A sport session moves the same way.
	if movedRide, _, err := f.move(t, ride, "2026-11-22", releaseRecord("scheduled-workouts/move")); err != nil || movedRide.Date != "2026-11-22" || movedRide.Sport == nil {
		t.Fatalf("moving a sport session: %+v %v", movedRide, err)
	}

	// The vacated Friday is not refilled when the schedule rolls forward.
	f.fill(t, "2026-11-18")
	if n := f.count(t, `SELECT count(*) FROM scheduled_workouts WHERE user_id=$1 AND date='2026-11-20'`, f.userID); n != 0 {
		t.Fatalf("%d sessions on the vacated Friday, want none", n)
	}
	if n := f.count(t, `SELECT count(*) FROM scheduled_workouts WHERE user_id=$1 AND date='2026-11-27' AND name='Legs'`, f.userID); n != 1 {
		t.Fatalf("next Friday holds %d Legs sessions, want the regular one", n)
	}
}

func TestMoveRefusesStaleStartedFinishedAndPastSessions(t *testing.T) {
	f := newReleaseFixture(t)
	program := f.startOwnWeek(t)
	var conflict *model.ConflictError

	pull := f.sessionOn(t, program.ID, "2026-11-18", "Pull")
	stale := *pull
	stale.Revision = pull.Revision + 5
	if _, _, err := f.move(t, &stale, "2026-11-19", releaseRecord("scheduled-workouts/move")); !errors.As(err, &conflict) || conflict.Actual != pull.Revision {
		t.Fatalf("stale revision error = %v, want a revision conflict", err)
	}

	f.exec(t, `UPDATE scheduled_workouts SET status='completed', finalized_at=now() WHERE id=$1`, pull.ID)
	finished := f.load(t, pull.ID)
	_, _, err := f.move(t, finished, "2026-11-19", releaseRecord("scheduled-workouts/move"))
	if !errors.As(err, &conflict) {
		t.Fatalf("moving a finished workout error = %v, want a conflict", err)
	}
	if authoritative, ok := conflict.Authoritative.(*model.ScheduledWorkout); !ok || authoritative.ID != pull.ID || authoritative.Date != "2026-11-18" {
		t.Fatalf("conflict resource = %+v, want the unmoved workout", conflict.Authoritative)
	}

	legs := f.sessionOn(t, program.ID, "2026-11-20", "Legs")
	f.exec(t, `INSERT INTO workout_sessions (user_id,date,name,status,scheduled_workout_id) VALUES ($1,'2026-11-20','Legs','active',$2)`, f.userID, legs.ID)
	if _, _, err := f.move(t, legs, "2026-11-21", releaseRecord("scheduled-workouts/move")); !errors.As(err, &conflict) {
		t.Fatalf("moving a started workout error = %v, want a conflict", err)
	}

	// The fixture's 2026-10-12 workout is in the past.
	past := f.load(t, f.workout.ID)
	if _, _, err := f.move(t, past, "2026-11-19", releaseRecord("scheduled-workouts/move")); !errors.As(err, &conflict) {
		t.Fatalf("moving a past workout error = %v, want a conflict", err)
	}
}

// Q8: saving the active program updates today and its future per unit, in one save.
func TestSavingTheActiveProgramUpdatesItsFuturePerUnit(t *testing.T) {
	f := newReleaseFixture(t)
	program := f.startOwnWeek(t)
	legsMoved, _, err := f.move(t, f.sessionOn(t, program.ID, "2026-11-20", "Legs"), "2026-11-21", releaseRecord("scheduled-workouts/move"))
	if err != nil {
		t.Fatal(err)
	}
	unitID := map[string]uuid.UUID{}
	for _, unit := range program.Workouts {
		unitID[unit.Name] = unit.ID
	}

	edited := &model.Program{ID: program.ID, Name: program.Name, PrimaryGoal: program.PrimaryGoal, Roadmap: program.Roadmap, Active: true}
	for _, unit := range program.Workouts {
		switch unit.Name {
		case "Push":
			unit.Weekdays = model.WeekdayList{2} // Monday to Tuesday
		case "Pull":
			unit.Exercises[0].TargetSets = 3 // one more set
		case "Volleyball":
			continue // removed
		}
		edited.Workouts = append(edited.Workouts, unit)
	}
	core := weekUnit("Core", 9, 1, 7)
	edited.Workouts = append(edited.Workouts, core)
	for index := range edited.Workouts {
		edited.Workouts[index].SequencePosition = index + 1
	}

	saved, replay, err := f.mutations.ReplaceProgram(f.ctx, f.userID, edited, program.Revision, saveNoon, releaseRecord("programs/update"))
	if err != nil || replay {
		t.Fatalf("ReplaceProgram() error = %v replay = %v", err, replay)
	}
	if saved.Revision != program.Revision+1 {
		t.Fatalf("revision = %d, want %d", saved.Revision, program.Revision+1)
	}
	for _, unit := range saved.Workouts {
		if want, kept := unitID[unit.Name]; kept && unit.ID != want {
			t.Fatalf("unit %s changed identity on save", unit.Name)
		}
	}

	got := f.sessionsOf(t, program.ID, fillToday)
	week := []string{
		"2026-11-16 Core",      // the new unit lands today, a Monday
		"2026-11-17 Push",      // Push now on Tuesdays; today's unstarted Push went with it
		"2026-11-18 Pull",      // kept, re-snapshotted
		"2026-11-19 Bike ride", // unchanged
		"2026-11-21 Legs",      // the one-off move survives
		"2026-11-22 Core",
		"2026-11-23 Core",
		"2026-11-24 Push",
		"2026-11-25 Pull",
		"2026-11-26 Bike ride",
		"2026-11-27 Legs",
		"2026-11-29 Core",
	}
	if !slices.Equal(got[:len(week)], week) {
		t.Fatalf("schedule after the save =\n%v\nwant\n%v", got[:len(week)], week)
	}
	if n := f.count(t, `SELECT count(*) FROM scheduled_workouts WHERE user_id=$1 AND name='Volleyball' AND date > $2`, f.userID, fillToday); n != 0 {
		t.Fatalf("%d sessions of the removed unit remain", n)
	}
	if moved := f.load(t, legsMoved.ID); moved.Date != "2026-11-21" || moved.Revision != legsMoved.Revision {
		t.Fatalf("the moved session changed: %+v", moved)
	}
	if pull := f.sessionOn(t, program.ID, "2026-11-18", "Pull"); len(pull.RequiredSets) != 3 {
		t.Fatalf("future Pull has %d required sets, want the saved 3", len(pull.RequiredSets))
	}
	// An unchanged unit keeps its exercise rows, so its sessions keep provenance.
	if n := f.count(t, `SELECT count(*) FROM scheduled_sets ss JOIN scheduled_workouts sw ON sw.id=ss.scheduled_workout_id WHERE sw.user_id=$1 AND sw.name='Legs' AND ss.program_exercise_id IS NULL`, f.userID); n != 0 {
		t.Fatalf("%d sets of the unchanged unit lost their exercise reference", n)
	}
	if n := f.count(t, `SELECT count(*) FROM (SELECT program_workout_id, date FROM scheduled_workouts WHERE user_id=$1 AND date >= $2 GROUP BY 1, 2 HAVING count(*) > 1) d`, f.userID, fillToday); n != 0 {
		t.Fatalf("%d unit-days hold more than one session", n)
	}

	// The same save again is a replay; a stale revision is refused untouched.
	before := f.snapshot(t)
	var conflict *model.ConflictError
	if _, _, err := f.mutations.ReplaceProgram(f.ctx, f.userID, edited, program.Revision, saveNoon, releaseRecord("programs/update")); !errors.As(err, &conflict) {
		t.Fatalf("a save at the old revision error = %v, want a conflict", err)
	}
	if after := f.snapshot(t); after != before {
		t.Fatal("a refused save changed the database")
	}
}

// A session the athlete has started today stays as it is, and its unit does
// not get a second session on the same day.
func TestSavingTheActiveProgramKeepsASessionStartedToday(t *testing.T) {
	f := newReleaseFixture(t)
	program := f.startOwnWeek(t)
	push := f.sessionOn(t, program.ID, fillToday, "Push")
	f.exec(t, `INSERT INTO workout_sessions (user_id,date,name,status,scheduled_workout_id) VALUES ($1,$2,'Push','active',$3)`, f.userID, fillToday, push.ID)

	edited := &model.Program{ID: program.ID, Name: program.Name, PrimaryGoal: program.PrimaryGoal, Roadmap: program.Roadmap, Active: true}
	for _, unit := range program.Workouts {
		if unit.Name == "Push" {
			unit.Weekdays = model.WeekdayList{1, 2} // Monday stays, Tuesday is new
		}
		edited.Workouts = append(edited.Workouts, unit)
	}
	if _, _, err := f.mutations.ReplaceProgram(f.ctx, f.userID, edited, program.Revision, saveNoon, releaseRecord("programs/update")); err != nil {
		t.Fatal(err)
	}

	if kept := f.load(t, push.ID); kept.Date != fillToday || kept.Revision != push.Revision {
		t.Fatalf("the started session changed: %+v", kept)
	}
	if n := f.count(t, `SELECT count(*) FROM scheduled_workouts WHERE user_id=$1 AND name='Push' AND date=$2`, f.userID, fillToday); n != 1 {
		t.Fatalf("today holds %d Push sessions, want the one that was started", n)
	}
	if next := f.sessionOn(t, program.ID, "2026-11-17", "Push"); next.Status != model.WorkoutStatusPlanned {
		t.Fatalf("the new Tuesday session = %+v", next)
	}
}

func TestSavingAnInactiveProgramChangesNoSession(t *testing.T) {
	f := newReleaseFixture(t)
	f.fill(t, fillToday)
	saved := f.saveProgram(t, "Later", pushPullLegsWeek())
	before := f.count(t, `SELECT count(*) FROM scheduled_workouts WHERE user_id=$1`, f.userID)
	saved.Workouts[0].Weekdays = model.WeekdayList{2}
	if _, _, err := f.mutations.ReplaceProgram(f.ctx, f.userID, saved, saved.Revision, saveNoon, releaseRecord("programs/update")); err != nil {
		t.Fatal(err)
	}
	if after := f.count(t, `SELECT count(*) FROM scheduled_workouts WHERE user_id=$1`, f.userID); after != before {
		t.Fatalf("session rows = %d, want unchanged %d", after, before)
	}
	if active := f.activeProgramIDs(t); len(active) != 1 || active[0] != f.program.ID {
		t.Fatal("saving an inactive program changed which program is active")
	}
}

// Q11: a client older than weekdays saves without unit IDs or weekdays and
// then regenerates. The result is one schedule with no duplicates.
func TestReleasedClientSaveThenRegenerateGivesOneSchedule(t *testing.T) {
	f := newReleaseFixture(t)
	program := f.startOwnWeek(t)
	legacy := &model.Program{ID: program.ID, Name: program.Name, PrimaryGoal: program.PrimaryGoal, Roadmap: program.Roadmap, Active: true,
		Workouts: []model.ProgramWorkout{weekUnit("Upper", 1), weekUnit("Lower", 2)}}
	for i := range legacy.Workouts {
		legacy.Workouts[i].Weekdays = nil
	}
	saved, _, err := f.mutations.ReplaceProgram(f.ctx, f.userID, legacy, program.Revision, saveNoon, releaseRecord("programs/update"))
	if err != nil {
		t.Fatal(err)
	}
	// The save already placed the units on the athlete's days (Mon, Wed),
	// dealing from tomorrow: Wednesday first.
	if !saved.Placed() {
		t.Fatal("a save by an older client left the active program without weekdays")
	}
	afterSave := f.sessionsOf(t, program.ID, startTomorrow)
	if want := []string{"2026-11-18 Upper", "2026-11-23 Lower", "2026-11-25 Upper"}; !slices.Equal(afterSave[:3], want) {
		t.Fatalf("after the save = %v, want %v", afterSave[:3], want)
	}
	if today := f.sessionOn(t, program.ID, fillToday, "Push"); today.Status != model.WorkoutStatusPlanned {
		t.Fatalf("today's session changed: %+v", today)
	}

	sessions, err := model.LayoutSessions(saved, startTomorrow, "2026-12-14")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.schedules.RegenerateIdempotent(f.ctx, f.userID, program.ID, saved.Revision, startTomorrow, "2026-12-14", nil, sessions, &model.RegenerateScheduleResponse{}, releaseRecord("schedule/regenerate")); err != nil {
		t.Fatalf("RegenerateIdempotent() error = %v", err)
	}
	if afterRegenerate := f.sessionsOf(t, program.ID, startTomorrow); !slices.Equal(afterRegenerate, afterSave) {
		t.Fatalf("regenerate changed the schedule:\n%v\nwant\n%v", afterRegenerate[:6], afterSave[:6])
	}

	// Materialize twice over the same range, as a retry would.
	for range 2 {
		again, err := model.LayoutSessions(saved, startTomorrow, "2026-11-30")
		if err != nil {
			t.Fatal(err)
		}
		returned, _, err := f.schedules.MaterializeIdempotent(f.ctx, f.userID, program.ID, saved.Revision, "2026-11-30", nil, again, releaseRecord("schedule/materialize"))
		if err != nil {
			t.Fatalf("MaterializeIdempotent() error = %v", err)
		}
		for _, session := range returned {
			if session.ID == uuid.Nil {
				t.Fatalf("materialize returned a session without its stored ID: %+v", session)
			}
		}
	}
	if n := f.count(t, `SELECT count(*) FROM (SELECT program_workout_id, date FROM scheduled_workouts WHERE user_id=$1 AND date >= $2 GROUP BY 1, 2 HAVING count(*) > 1) d`, f.userID, fillToday); n != 0 {
		t.Fatalf("%d unit-days hold more than one session after regenerate and materialize", n)
	}
}

// Q9: a saved program can be removed; the active one cannot; history stays.
func TestRemoveProgramKeepsHistoryAndRefusesTheActiveProgram(t *testing.T) {
	f := newReleaseFixture(t)
	f.fill(t, fillToday)
	f.performWorkout(t, f.workout, model.WorkoutStatusCompleted, true)
	blockID := uuid.New()
	f.exec(t, `INSERT INTO criteria_training_blocks (id, user_id, program_id, name) VALUES ($1,$2,$3,'Block')`, blockID, f.userID, f.program.ID)

	var conflict *model.ConflictError
	if err := f.mutations.DeleteProgram(f.ctx, f.userID, f.program.ID, saveNoon); !errors.As(err, &conflict) {
		t.Fatalf("removing the active program error = %v, want a conflict", err)
	}
	if authoritative, ok := conflict.Authoritative.(*model.Program); !ok || authoritative.ID != f.program.ID {
		t.Fatalf("conflict resource = %+v, want the active program", conflict.Authoritative)
	}

	current := f.startOwnWeek(t)
	// The replaced program keeps one planned session for today; removal takes it.
	if err := f.mutations.DeleteProgram(f.ctx, f.userID, f.program.ID, saveNoon); err != nil {
		t.Fatalf("DeleteProgram() error = %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM programs WHERE id=$1`, f.program.ID); n != 0 {
		t.Fatal("the removed program still exists")
	}
	finished := f.load(t, f.workout.ID)
	if finished.Status != model.WorkoutStatusCompleted || finished.Name != "A" || finished.ProgramID != nil || len(finished.RequiredSets) != 1 {
		t.Fatalf("the finished workout lost its snapshot: %+v", finished)
	}
	if n := f.count(t, `SELECT count(*) FROM scheduled_workouts WHERE user_id=$1 AND program_id IS NULL AND status='planned'`, f.userID); n != 0 {
		t.Fatalf("%d planned sessions of the removed program remain", n)
	}
	if n := f.count(t, `SELECT count(*) FROM criteria_training_blocks WHERE id=$1 AND program_id IS NULL`, blockID); n != 1 {
		t.Fatal("the training block must stay, with its program link cleared")
	}
	if got := len(f.sessionsOf(t, current.ID, fillToday)); got == 0 {
		t.Fatal("removing a saved program touched the active program's sessions")
	}
	var notFound *model.NotFoundError
	if err := f.mutations.DeleteProgram(f.ctx, f.userID, f.program.ID, saveNoon); !errors.As(err, &notFound) {
		t.Fatalf("removing it again error = %v, want not found", err)
	}
}

// Q10: another athlete's programs and sessions are not found.
func TestNewMutationsRefuseAnotherAthletesResources(t *testing.T) {
	owner := newReleaseFixture(t)
	other := newReleaseFixture(t)
	program := owner.startOwnWeek(t)
	session := owner.sessionOn(t, program.ID, "2026-11-18", "Pull")
	before := owner.snapshot(t)
	var notFound *model.NotFoundError

	if _, _, err := other.schedules.MoveIdempotent(other.ctx, other.userID, session.ID, "2026-11-19", fillToday, session.Revision, releaseRecord("scheduled-workouts/move")); !errors.As(err, &notFound) {
		t.Fatalf("moving a foreign session error = %v, want not found", err)
	}
	if err := other.mutations.DeleteProgram(other.ctx, other.userID, program.ID, saveNoon); !errors.As(err, &notFound) {
		t.Fatalf("removing a foreign program error = %v, want not found", err)
	}
	foreign := *program
	if _, _, err := other.mutations.ReplaceProgram(other.ctx, other.userID, &foreign, program.Revision, saveNoon, releaseRecord("programs/update")); !errors.As(err, &notFound) {
		t.Fatalf("saving a foreign program error = %v, want not found", err)
	}
	// A unit ID that belongs to another athlete's program names nothing in
	// the caller's own program: it becomes a new unit and the owner's stays.
	own := other.saveProgram(t, "Mine", []model.ProgramWorkout{weekUnit("Mine", 1, 1)})
	borrowed := weekUnit("Borrowed", 2, 3)
	borrowed.ID = program.Workouts[0].ID
	own.Workouts = append(own.Workouts, borrowed)
	saved, _, err := other.mutations.ReplaceProgram(other.ctx, other.userID, own, own.Revision, saveNoon, releaseRecord("programs/update"))
	if err != nil {
		t.Fatalf("saving with a foreign unit ID error = %v", err)
	}
	if saved.Workouts[1].ID == program.Workouts[0].ID {
		t.Fatal("a foreign unit ID was adopted")
	}
	if after := owner.snapshot(t); after != before {
		t.Fatal("another athlete's requests changed the owner's data")
	}
}

// Review finding 1 and 5: a program started again through a save is laid out
// from tomorrow, not from a stale fill mark and not on top of today.
func TestSaveThatRestartsAProgramFillsItFromTomorrow(t *testing.T) {
	f := newReleaseFixture(t)
	f.fill(t, fillToday) // fixture program A, Mondays and Wednesdays
	other := f.startOwnWeek(t)

	// A week later the athlete switches back to A with a save.
	later := saveNoon.AddDate(0, 0, 7) // Monday 2026-11-23
	again, err := (&programDAO{pool: f.pool}).Get(f.ctx, f.userID, f.program.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.ScheduledThrough != nil {
		t.Fatalf("the replaced program kept its fill mark %q; it must be cleared", *again.ScheduledThrough)
	}
	again.Active = true
	if _, _, err := f.mutations.ReplaceProgram(f.ctx, f.userID, again, again.Revision, later, releaseRecord("programs/update")); err != nil {
		t.Fatalf("ReplaceProgram() error = %v", err)
	}

	if active := f.activeProgramIDs(t); len(active) != 1 || active[0] != f.program.ID {
		t.Fatalf("active programs = %v, want only the restarted program", active)
	}
	horizon, _ := shiftDate("2026-11-23", model.ScheduleHorizonDays)
	assertMondaysAndWednesdays(t, f.programSessions(t, "2026-11-24"), "2026-11-24", horizon)
	if n := f.count(t, `SELECT count(*) FROM scheduled_workouts WHERE user_id=$1 AND program_id=$2 AND date='2026-11-23' AND created_at > (SELECT updated_at - interval '1 minute' FROM programs WHERE id=$2)`, f.userID, f.program.ID); n != 0 {
		t.Fatalf("the restarted program got %d new sessions today; it starts tomorrow", n)
	}
	if left := f.sessionsOf(t, other.ID, "2026-11-24"); len(left) != 0 {
		t.Fatalf("the replaced program still has future sessions: %v", left[:1])
	}
	if today := f.sessionsOf(t, other.ID, "2026-11-23"); !slices.Equal(today, []string{"2026-11-23 Push"}) {
		t.Fatalf("today's session of the replaced program = %v, want it kept", today)
	}
	// The next read adds nothing behind the mark and doubles nothing.
	before := f.snapshot(t)
	if err := f.schedules.EnsureFilled(f.ctx, f.userID, "2026-11-23", []int{1, 3}); err != nil {
		t.Fatal(err)
	}
	if after := f.snapshot(t); after != before {
		t.Fatal("the read after a restart changed the schedule")
	}
}

// Review finding 2: creating an active program, as a client older than
// contract 1.10.0 does, ends the replaced program's future.
func TestCreatingAnActiveProgramRetiresTheReplacedProgramsFuture(t *testing.T) {
	f := newReleaseFixture(t)
	f.fill(t, fillToday)
	legacy := &model.Program{Name: "Created active", PrimaryGoal: "strength", Roadmap: map[string]any{}, Active: true,
		Workouts: []model.ProgramWorkout{weekUnit("Upper", 1), weekUnit("Lower", 2)}}
	for i := range legacy.Workouts {
		legacy.Workouts[i].Weekdays = nil
	}
	created, _, err := f.mutations.CreateProgram(f.ctx, f.userID, legacy, saveNoon, releaseRecord("programs/create"))
	if err != nil {
		t.Fatal(err)
	}
	if left := f.programSessions(t, startTomorrow); len(left) != 0 {
		t.Fatalf("the replaced program keeps %d future sessions", len(left))
	}
	if today := f.programSessions(t, fillToday); !slices.Equal(today, []string{fillToday}) {
		t.Fatalf("today's session of the replaced program = %v, want it kept", today)
	}
	// With no follow-up call from the client, the next read starts the new
	// program tomorrow, beside nothing.
	f.fill(t, fillToday)
	sessions := f.sessionsOf(t, created.ID, fillToday)
	if len(sessions) == 0 || sessions[0] != "2026-11-18 Upper" {
		t.Fatalf("new program sessions start %v, want Wednesday 2026-11-18 Upper", sessions[:min(2, len(sessions))])
	}
	if n := f.count(t, `SELECT count(*) FROM (SELECT date FROM scheduled_workouts WHERE user_id=$1 AND date >= $2 AND status='planned' GROUP BY date HAVING count(DISTINCT program_id) > 1) d`, f.userID, fillToday); n != 0 {
		t.Fatalf("%d dates hold planned sessions of two programs", n)
	}
}

// Review finding 4: removing a program keeps a past session that has not
// been finalized yet; it is history, not future.
func TestRemoveProgramKeepsPastSessionsThatAreStillPlanned(t *testing.T) {
	f := newReleaseFixture(t)
	f.startOwnWeek(t)
	if past := f.load(t, f.workout.ID); past.Status != model.WorkoutStatusPlanned || past.Date != "2026-10-12" {
		t.Fatalf("fixture workout is not a past planned session: %+v", past)
	}
	if err := f.mutations.DeleteProgram(f.ctx, f.userID, f.program.ID, saveNoon); err != nil {
		t.Fatal(err)
	}
	kept := f.load(t, f.workout.ID)
	if kept.ProgramID != nil || kept.Name != "A" {
		t.Fatalf("the past session was not kept as history: %+v", kept)
	}
}

// Review finding 6: a client older than weekdays that echoes unit IDs must
// not re-deal a week the athlete laid out.
func TestSaveWithoutWeekdaysKeepsThePlacedWeek(t *testing.T) {
	f := newReleaseFixture(t)
	program := f.startOwnWeek(t)
	before := f.sessionsOf(t, program.ID, startTomorrow)

	echoed := &model.Program{ID: program.ID, Name: "Renamed by an older client", PrimaryGoal: program.PrimaryGoal, Roadmap: program.Roadmap, Active: true}
	for _, unit := range program.Workouts {
		unit.Weekdays = nil // the older client does not know the field
		echoed.Workouts = append(echoed.Workouts, unit)
	}
	saved, _, err := f.mutations.ReplaceProgram(f.ctx, f.userID, echoed, program.Revision, saveNoon, releaseRecord("programs/update"))
	if err != nil {
		t.Fatal(err)
	}
	for index, unit := range saved.Workouts {
		if !slices.Equal([]int(unit.Weekdays), []int(program.Workouts[index].Weekdays)) {
			t.Fatalf("unit %s weekdays = %v, want the athlete's %v", unit.Name, unit.Weekdays, program.Workouts[index].Weekdays)
		}
	}
	if after := f.sessionsOf(t, program.ID, startTomorrow); !slices.Equal(after, before) {
		t.Fatal("a save that changed only the name moved sessions")
	}
}

func TestStartRecordAnswersARetry(t *testing.T) {
	f := newReleaseFixture(t)
	saved := f.saveProgram(t, "Push pull legs", pushPullLegsWeek())
	through, _ := shiftDate(fillToday, model.ScheduleHorizonDays)
	transitions := &planTransitionDAO{pool: f.pool}
	if _, found, err := transitions.Replayed(f.ctx, f.userID, "start-key", "hash", startTomorrow, "2026-11-23"); err != nil || found {
		t.Fatalf("Replayed() before any start = found %v, err %v", found, err)
	}
	if _, err := transitions.Apply(f.ctx, f.userID, PlanTransitionApply{Target: saved, AvailableDays: []int{1, 3}, Start: startTomorrow, Through: through, From: startTomorrow, To: "2026-11-23", Today: fillToday, OperationKey: "start-key", RequestHash: "hash"}); err != nil {
		t.Fatal(err)
	}
	replay, found, err := transitions.Replayed(f.ctx, f.userID, "start-key", "hash", startTomorrow, "2026-11-23")
	if err != nil || !found || replay.ProgramID != saved.ID || len(replay.Workouts) != 5 || replay.Workouts[0].ID == uuid.Nil {
		t.Fatalf("Replayed() = %+v found=%v err=%v, want the started program with its 5 stored sessions", replay, found, err)
	}
	var conflict *model.ConflictError
	if _, _, err := transitions.Replayed(f.ctx, f.userID, "start-key", "another", startTomorrow, "2026-11-23"); !errors.As(err, &conflict) {
		t.Fatalf("Replayed() with another payload error = %v, want a conflict", err)
	}
}

// Review finding 9: a start that names the already active, placed program
// still returns its sessions when that program was never filled.
func TestStartOfAnUnfilledActiveProgramFillsIt(t *testing.T) {
	f := newReleaseFixture(t)
	program := &model.Program{Name: "Active, never filled", PrimaryGoal: "strength", Roadmap: map[string]any{}, Active: true, Workouts: pushPullLegsWeek()}
	created, _, err := f.mutations.CreateProgram(f.ctx, f.userID, program, saveNoon, releaseRecord("programs/create"))
	if err != nil {
		t.Fatal(err)
	}
	result := f.start(t, created, fillToday, nil, 0)
	if len(result.Workouts) == 0 {
		t.Fatal("the start returned no sessions for an active program that had none")
	}
}
