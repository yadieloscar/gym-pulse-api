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
	saved, _, err := f.mutations.CreateProgram(f.ctx, f.userID, program, releaseRecord("programs/create"))
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
		Start: start, Through: through, From: start, To: to, OperationKey: uuid.NewString(), RequestHash: "hash",
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
	created, _, err := f.mutations.CreateProgram(f.ctx, f.userID, legacy, releaseRecord("programs/create"))
	if err != nil {
		t.Fatal(err)
	}
	through, _ := shiftDate(fillToday, model.ScheduleHorizonDays)
	if _, err := (&planTransitionDAO{pool: f.pool}).Apply(f.ctx, f.userID, PlanTransitionApply{Target: created, AvailableDays: []int{2, 3, 5},
		Start: startTomorrow, Through: through, From: startTomorrow, To: "2026-11-23", OperationKey: uuid.NewString(), RequestHash: "hash"}); err != nil {
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
	apply := PlanTransitionApply{Target: saved, AvailableDays: []int{1, 3}, Start: startTomorrow, Through: through, From: startTomorrow, To: "2026-11-23", OperationKey: "start-once", RequestHash: "hash"}
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
	_, err := (&planTransitionDAO{pool: other.pool}).Apply(other.ctx, other.userID, PlanTransitionApply{Target: foreign, AvailableDays: []int{1}, Start: startTomorrow, Through: through, From: startTomorrow, To: "2026-11-23", OperationKey: uuid.NewString(), RequestHash: "hash"})
	var notFound *model.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("starting a foreign program error = %v, want not found", err)
	}
	if active := other.activeProgramIDs(t); len(active) != 1 || active[0] != other.program.ID {
		t.Fatal("a refused start changed the other athlete's active program")
	}
}
