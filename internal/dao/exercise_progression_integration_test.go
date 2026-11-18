//go:build integration

package dao

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// progressionSeed writes set logs straight into the tables the progression
// query reads, for one athlete.
type progressionSeed struct {
	f      *importFixture
	userID uuid.UUID
}

func (s progressionSeed) exercise(t *testing.T, name string) uuid.UUID {
	t.Helper()
	var templateID, exerciseID uuid.UUID
	if err := s.f.pool.QueryRow(s.f.ctx, `INSERT INTO workout_templates (user_id, name, type_id) VALUES ($1, $2, 'strength') RETURNING id`,
		s.userID, name+" template").Scan(&templateID); err != nil {
		t.Fatal(err)
	}
	if err := s.f.pool.QueryRow(s.f.ctx, `INSERT INTO exercises (template_id, name, sort_order, sets, reps) VALUES ($1, $2, 0, 3, 5) RETURNING id`,
		templateID, name).Scan(&exerciseID); err != nil {
		t.Fatal(err)
	}
	return exerciseID
}

// progressionSet is one set log row. A nil Weight stores no weight.
type progressionSet struct {
	exerciseID uuid.UUID
	weight     *float64
	reps       int
	incomplete bool
}

func weighted(exerciseID uuid.UUID, weight float64, reps int) progressionSet {
	return progressionSet{exerciseID: exerciseID, weight: &weight, reps: reps}
}

func (s progressionSeed) insertSets(t *testing.T, parentColumn string, parentID uuid.UUID, sets []progressionSet) {
	t.Helper()
	for i, set := range sets {
		if _, err := s.f.pool.Exec(s.f.ctx, `INSERT INTO set_logs (`+parentColumn+`, exercise_id, set_index, actual_reps, actual_weight, completed,
			exercise_name, exercise_category, exercise_modality)
			VALUES ($1, $2, $3, $4, $5, $6, 'lift', 'legs', 'strength')`,
			parentID, set.exerciseID, i+1, set.reps, set.weight, !set.incomplete); err != nil {
			t.Fatal(err)
		}
	}
}

func (s progressionSeed) dayLog(t *testing.T, date string, sets ...progressionSet) {
	t.Helper()
	var id uuid.UUID
	if err := s.f.pool.QueryRow(s.f.ctx, `INSERT INTO day_logs (user_id, date, type_id) VALUES ($1, $2::date, 'strength') RETURNING id`,
		s.userID, date).Scan(&id); err != nil {
		t.Fatal(err)
	}
	s.insertSets(t, "day_log_id", id, sets)
}

func (s progressionSeed) session(t *testing.T, date, status string, sets ...progressionSet) {
	t.Helper()
	var id uuid.UUID
	if err := s.f.pool.QueryRow(s.f.ctx, `INSERT INTO workout_sessions (user_id, date, name, status) VALUES ($1, $2::date, 'session', $3) RETURNING id`,
		s.userID, date, status).Scan(&id); err != nil {
		t.Fatal(err)
	}
	s.insertSets(t, "workout_session_id", id, sets)
}

// TestWeeklyHeaviestSets covers the progression query against PostgreSQL:
// Monday-start weeks, both tie-breaks, both set sources, the window start,
// and every set the route must ignore.
func TestWeeklyHeaviestSets(t *testing.T) {
	f := newImportFixture(t)
	athlete := progressionSeed{f: f, userID: f.userID}
	other := progressionSeed{f: f, userID: f.newUser(t)}
	for _, userID := range []uuid.UUID{athlete.userID, other.userID} {
		// Registered after newUser's cleanup, so it runs first and the
		// identity row is deleted last.
		t.Cleanup(func() {
			if err := NewAccountDAO(f.pool).DeleteUserData(context.WithoutCancel(f.ctx), userID); err != nil {
				t.Errorf("cleaning fixture data: %v", err)
			}
		})
	}

	squat := athlete.exercise(t, "Back Squat")
	bench := athlete.exercise(t, "Bench Press")
	never := athlete.exercise(t, "Never Lifted")
	since := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC) // a Monday

	// Sunday before the window: the heaviest set of all, and out of range.
	athlete.session(t, "2026-08-16", "completed", weighted(squat, 999, 1))

	// Week of Aug 17. 200 is the top weight three times: 5 reps and 8 reps
	// on Tuesday, 8 reps again on Wednesday. More reps wins, then the
	// earlier date. Two sessions on one day are both read.
	athlete.session(t, "2026-08-18", "completed", weighted(squat, 200, 5), weighted(squat, 185, 12), weighted(bench, 135, 8))
	athlete.session(t, "2026-08-18", "completed", weighted(squat, 200, 8))
	athlete.dayLog(t, "2026-08-19", weighted(squat, 200, 8), weighted(bench, 135, 6))

	// Week of Aug 24: nothing counts, so the week has no entry.
	athlete.session(t, "2026-08-25", "completed",
		progressionSet{exerciseID: squat, weight: ptrFloat(500), reps: 0},                   // no reps
		progressionSet{exerciseID: squat, reps: 10},                                         // no weight
		progressionSet{exerciseID: squat, weight: ptrFloat(300), reps: 1, incomplete: true}) // not completed
	athlete.session(t, "2026-08-26", "discarded", weighted(squat, 400, 1))
	athlete.dayLog(t, "2026-08-27", progressionSet{exerciseID: squat, weight: ptrFloat(350), reps: 1, incomplete: true})

	// Sunday Sep 6 closes the week of Aug 31; Monday Sep 7 opens the next.
	athlete.dayLog(t, "2026-09-06", weighted(squat, 205, 5))
	athlete.session(t, "2026-09-07", "active", weighted(squat, 210.5, 3))

	// Another athlete logs a heavier set against the same exercise id.
	other.dayLog(t, "2026-08-18", weighted(squat, 900, 1))

	want := []model.ExerciseProgression{
		{ExerciseID: squat, Weeks: []model.ProgressionWeek{
			{WeekStart: "2026-08-17", MaxWeight: 200, MaxWeightReps: 8, Date: "2026-08-18"},
			{WeekStart: "2026-08-31", MaxWeight: 205, MaxWeightReps: 5, Date: "2026-09-06"},
			{WeekStart: "2026-09-07", MaxWeight: 210.5, MaxWeightReps: 3, Date: "2026-09-07"},
		}},
		{ExerciseID: bench, Weeks: []model.ProgressionWeek{
			{WeekStart: "2026-08-17", MaxWeight: 135, MaxWeightReps: 8, Date: "2026-08-18"},
		}},
	}
	byExercise := func(progressions []model.ExerciseProgression) map[uuid.UUID][]model.ProgressionWeek {
		m := map[uuid.UUID][]model.ProgressionWeek{}
		for _, p := range progressions {
			m[p.ExerciseID] = p.Weeks
		}
		return m
	}
	ids := []uuid.UUID{squat, bench, never, uuid.New()}

	got, err := f.logs.WeeklyHeaviestSets(f.ctx, athlete.userID, ids, since)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) || !reflect.DeepEqual(byExercise(got), byExercise(want)) {
		t.Errorf("WeeklyHeaviestSets = %+v, want %+v", got, want)
	}

	t.Run("another athlete sees only their own set", func(t *testing.T) {
		got, err := f.logs.WeeklyHeaviestSets(f.ctx, other.userID, ids, since)
		if err != nil {
			t.Fatal(err)
		}
		want := []model.ExerciseProgression{{ExerciseID: squat, Weeks: []model.ProgressionWeek{
			{WeekStart: "2026-08-17", MaxWeight: 900, MaxWeightReps: 1, Date: "2026-08-18"},
		}}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("WeeklyHeaviestSets = %+v, want %+v", got, want)
		}
	})

	t.Run("no ids and a later window are empty lists", func(t *testing.T) {
		for name, call := range map[string]func() ([]model.ExerciseProgression, error){
			"no ids": func() ([]model.ExerciseProgression, error) {
				return f.logs.WeeklyHeaviestSets(f.ctx, athlete.userID, nil, since)
			},
			"later window": func() ([]model.ExerciseProgression, error) {
				return f.logs.WeeklyHeaviestSets(f.ctx, athlete.userID, ids, since.AddDate(1, 0, 0))
			},
		} {
			got, err := call()
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if got == nil || len(got) != 0 {
				t.Errorf("%s: got %+v, want an empty non-nil list", name, got)
			}
		}
	})

	t.Run("the runtime role reads the same rows", func(t *testing.T) {
		_, _, runtime := newRuntimeRoleFixture(t)
		got, err := NewLogDAO(runtime).WeeklyHeaviestSets(f.ctx, athlete.userID, ids, since)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(want) || !reflect.DeepEqual(byExercise(got), byExercise(want)) {
			t.Errorf("WeeklyHeaviestSets = %+v, want %+v", got, want)
		}
	})
}

func ptrFloat(v float64) *float64 { return &v }
