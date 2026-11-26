//go:build integration

package dao

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// recapSeed writes one athlete's program history straight into the tables
// the recap batch reads.
type recapSeed struct {
	f      *importFixture
	userID uuid.UUID
}

func (s recapSeed) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := s.f.pool.Exec(s.f.ctx, sql, args...); err != nil {
		t.Fatal(err)
	}
}

// program creates a program whose units sit on the given weekdays.
func (s recapSeed) program(t *testing.T, name string, active bool, weekdays ...[]int16) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := s.f.pool.QueryRow(s.f.ctx, `INSERT INTO programs (user_id, name, primary_goal, active) VALUES ($1,$2,'strength',$3) RETURNING id`,
		s.userID, name, active).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for i, days := range weekdays {
		s.exec(t, `INSERT INTO program_workouts (program_id, name, sequence_position, weekdays) VALUES ($1,$2,$3,$4)`, id, name, i+1, days)
	}
	return id
}

// session creates a scheduled session of the program and returns its id.
func (s recapSeed) session(t *testing.T, programID uuid.UUID, date, status string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := s.f.pool.QueryRow(s.f.ctx, `INSERT INTO scheduled_workouts (user_id, program_id, date, name, status) VALUES ($1,$2,$3::date,'A',$4) RETURNING id`,
		s.userID, programID, date, status).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

type recapSet struct {
	name      string
	weight    *float64
	reps      int
	completed bool
}

func lifted(name string, weight float64, reps int) recapSet {
	return recapSet{name: name, weight: &weight, reps: reps, completed: true}
}

// performed logs sets in a workout session. scheduledID is nil for an
// off-plan session.
func (s recapSeed) performed(t *testing.T, scheduledID *uuid.UUID, date, status string, sets ...recapSet) {
	t.Helper()
	var id uuid.UUID
	if err := s.f.pool.QueryRow(s.f.ctx, `INSERT INTO workout_sessions (user_id, scheduled_workout_id, date, name, status) VALUES ($1,$2,$3::date,'A',$4) RETURNING id`,
		s.userID, scheduledID, date, status).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for i, set := range sets {
		s.exec(t, `INSERT INTO set_logs (workout_session_id, set_index, actual_reps, actual_weight, completed, exercise_name, exercise_category, exercise_modality)
			VALUES ($1,$2,$3,$4,$5,$6,'legs','strength')`, id, i+1, set.reps, set.weight, set.completed, set.name)
	}
}

func (s recapSeed) trained(t *testing.T, date string, participated bool) {
	t.Helper()
	s.exec(t, `INSERT INTO day_participation (user_id, date, scheduled_opportunity, participated, finalized_at, timezone, local_date)
		VALUES ($1,$2::date,true,$3,now(),'America/New_York',$2::date)`, s.userID, date, participated)
}

func newRecapSeeds(t *testing.T) (athlete, other recapSeed) {
	t.Helper()
	f := newImportFixture(t)
	athlete = recapSeed{f: f, userID: f.userID}
	other = recapSeed{f: f, userID: f.newUser(t)}
	for _, userID := range []uuid.UUID{athlete.userID, other.userID} {
		t.Cleanup(func() {
			if err := NewAccountDAO(f.pool).DeleteUserData(context.WithoutCancel(f.ctx), userID); err != nil {
				t.Errorf("cleaning fixture data: %v", err)
			}
		})
	}
	return athlete, other
}

// TestProgramRecapRead covers the recap batch against PostgreSQL: the header,
// the sessions, the trained dates and weeks off inside the program's span,
// and the heaviest set per lift and day with every set it must ignore.
func TestProgramRecapRead(t *testing.T) {
	athlete, _ := newRecapSeeds(t)
	athlete.exec(t, `INSERT INTO training_profiles (user_id, primary_goal, available_days, usual_activity, experience, equipment, session_duration_minutes, timezone)
		VALUES ($1,'strength',ARRAY[1,3]::smallint[],'moderate','beginner','{}',45,'America/New_York')`, athlete.userID)

	// Units on Monday and Friday, and on Wednesday and Friday: three days.
	programID := athlete.program(t, "Strength", false, []int16{1, 5}, []int16{3, 5})
	otherProgram := athlete.program(t, "Other", true, []int16{2})

	monday := athlete.session(t, programID, "2026-09-07", "completed")
	wednesday := athlete.session(t, programID, "2026-09-09", "incomplete")
	athlete.session(t, programID, "2026-09-11", "missed")
	lastMonday := athlete.session(t, programID, "2026-09-21", "completed")
	foreign := athlete.session(t, otherProgram, "2026-09-08", "completed")

	// Monday: 185 is the top weight twice; more reps wins. Trimmed and
	// case-folded names are one lift.
	athlete.performed(t, &monday, "2026-09-07", "completed",
		lifted("Back Squat", 185, 5), lifted(" back squat ", 185, 8), lifted("Back Squat", 135, 12), lifted("Bench Press", 135, 5),
		recapSet{name: "Back Squat", weight: ptrFloat(300), reps: 1},           // not completed
		recapSet{name: "Back Squat", weight: ptrFloat(400), completed: true},   // no reps
		recapSet{name: "Pull-up", reps: 8, completed: true},                    // no weight
		recapSet{name: "Plank", weight: ptrFloat(0), reps: 1, completed: true}) // zero weight
	athlete.performed(t, &wednesday, "2026-09-09", "discarded", lifted("Back Squat", 500, 1))
	athlete.performed(t, &lastMonday, "2026-09-21", "completed", lifted("Back Squat", 195, 5))
	// Another program's session and an off-plan session are not this program's lifts.
	athlete.performed(t, &foreign, "2026-09-08", "completed", lifted("Back Squat", 600, 1))
	athlete.performed(t, nil, "2026-09-10", "completed", lifted("Back Squat", 700, 1))

	// Trained dates: only participated days inside 09-07..09-21.
	athlete.trained(t, "2026-09-06", true)
	athlete.trained(t, "2026-09-07", true)
	athlete.trained(t, "2026-09-10", true)
	athlete.trained(t, "2026-09-11", false)
	athlete.trained(t, "2026-09-21", true)
	athlete.trained(t, "2026-09-22", true)

	// Weeks off: the week of 09-07 overlaps even when the program started
	// midweek; the weeks before and after the span do not.
	for _, week := range []string{"2026-08-31", "2026-09-14", "2026-09-28"} {
		athlete.exec(t, `INSERT INTO weeks_off (user_id, week_start) VALUES ($1,$2::date)`, athlete.userID, week)
	}

	got, err := NewProgramRecapDAO(athlete.f.pool).Read(athlete.f.ctx, athlete.userID, programID)
	if err != nil {
		t.Fatal(err)
	}
	want := &ProgramRecapRead{
		Name: "Strength", Active: false, TrainingDays: 3, Timezone: "America/New_York",
		Sessions: []ProgramRecapSession{
			{Date: "2026-09-07", Status: "completed"}, {Date: "2026-09-09", Status: "incomplete"},
			{Date: "2026-09-11", Status: "missed"}, {Date: "2026-09-21", Status: "completed"},
		},
		TrainedDates: []string{"2026-09-07", "2026-09-10", "2026-09-21"},
		WeeksOff:     []string{"2026-09-14"},
		LiftDays: []ProgramRecapLiftDay{
			{Name: "back squat", Date: "2026-09-07", Weight: 185, Reps: 8},
			{Name: "Back Squat", Date: "2026-09-21", Weight: 195, Reps: 5},
			{Name: "Bench Press", Date: "2026-09-07", Weight: 135, Reps: 5},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Read() = %+v\nwant %+v", got, want)
	}
}

// A program with nothing scheduled and no profile reads as empty lists, and
// a program started midweek still sees that week's week off.
func TestProgramRecapReadEmptyAndMidweek(t *testing.T) {
	athlete, _ := newRecapSeeds(t)
	empty := athlete.program(t, "Empty", true)
	got, err := NewProgramRecapDAO(athlete.f.pool).Read(athlete.f.ctx, athlete.userID, empty)
	if err != nil {
		t.Fatal(err)
	}
	want := &ProgramRecapRead{Name: "Empty", Active: true, Sessions: []ProgramRecapSession{}, TrainedDates: []string{}, WeeksOff: []string{}, LiftDays: []ProgramRecapLiftDay{}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Read() = %+v\nwant %+v", got, want)
	}

	midweek := athlete.program(t, "Midweek", false, []int16{4})
	athlete.session(t, midweek, "2026-09-10", "missed") // a Thursday
	athlete.exec(t, `INSERT INTO weeks_off (user_id, week_start) VALUES ($1,'2026-09-07')`, athlete.userID)
	got, err = NewProgramRecapDAO(athlete.f.pool).Read(athlete.f.ctx, athlete.userID, midweek)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.WeeksOff, []string{"2026-09-07"}) {
		t.Errorf("WeeksOff = %v, want the week of 09-07", got.WeeksOff)
	}
}

// Another athlete's program is not found, and their history never appears.
func TestProgramRecapReadIsOwned(t *testing.T) {
	athlete, other := newRecapSeeds(t)
	theirs := other.program(t, "Theirs", true, []int16{1})
	session := other.session(t, theirs, "2026-09-07", "completed")
	other.performed(t, &session, "2026-09-07", "completed", lifted("Back Squat", 315, 5))
	other.trained(t, "2026-09-07", true)
	other.exec(t, `INSERT INTO weeks_off (user_id, week_start) VALUES ($1,'2026-09-07')`, other.userID)

	recaps := NewProgramRecapDAO(athlete.f.pool)
	var notFound *model.NotFoundError
	if _, err := recaps.Read(athlete.f.ctx, athlete.userID, theirs); !errors.As(err, &notFound) {
		t.Fatalf("foreign program err = %v, want not found", err)
	}
	if _, err := recaps.Read(athlete.f.ctx, athlete.userID, uuid.New()); !errors.As(err, &notFound) {
		t.Fatalf("missing program err = %v, want not found", err)
	}

	// The athlete's own program over the same dates sees none of it.
	mine := athlete.program(t, "Mine", true, []int16{1})
	athlete.session(t, mine, "2026-09-07", "missed")
	got, err := recaps.Read(athlete.f.ctx, athlete.userID, mine)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.TrainedDates) != 0 || len(got.WeeksOff) != 0 || len(got.LiftDays) != 0 {
		t.Errorf("Read() = %+v; want nothing from the other athlete", got)
	}
}
