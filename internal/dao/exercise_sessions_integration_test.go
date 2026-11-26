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

// liftSet is one set log row for the lift-history query. A zero weight or
// reps stores NULL; seconds stores a duration.
type liftSet struct {
	name          string
	exerciseID    *uuid.UUID
	scheduledSet  *uuid.UUID
	weight        float64
	reps, seconds int
	incomplete    bool
}

func (s progressionSeed) insertLiftSets(t *testing.T, parentColumn string, parentID uuid.UUID, sets []liftSet) {
	t.Helper()
	for i, set := range sets {
		var weight *float64
		var reps, seconds *int
		if set.weight != 0 {
			weight = &set.weight
		}
		if set.reps != 0 {
			reps = &set.reps
		}
		if set.seconds != 0 {
			seconds = &set.seconds
		}
		if _, err := s.f.pool.Exec(s.f.ctx, `INSERT INTO set_logs (`+parentColumn+`, exercise_id, scheduled_set_id, is_extra, set_index,
			actual_reps, actual_weight, duration_seconds, completed, exercise_name, exercise_category, exercise_modality)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'legs', 'strength')`,
			parentID, set.exerciseID, set.scheduledSet, set.scheduledSet == nil, i+1, reps, weight, seconds, !set.incomplete, set.name); err != nil {
			t.Fatal(err)
		}
	}
}

func (s progressionSeed) liftDayLog(t *testing.T, date string, sets ...liftSet) {
	t.Helper()
	var id uuid.UUID
	if err := s.f.pool.QueryRow(s.f.ctx, `INSERT INTO day_logs (user_id, date, type_id) VALUES ($1, $2::date, 'strength') RETURNING id`,
		s.userID, date).Scan(&id); err != nil {
		t.Fatal(err)
	}
	s.insertLiftSets(t, "day_log_id", id, sets)
}

func (s progressionSeed) liftSession(t *testing.T, date, status string, sets ...liftSet) {
	t.Helper()
	var id uuid.UUID
	if err := s.f.pool.QueryRow(s.f.ctx, `INSERT INTO workout_sessions (user_id, date, name, status) VALUES ($1, $2::date, 'session', $3) RETURNING id`,
		s.userID, date, status).Scan(&id); err != nil {
		t.Fatal(err)
	}
	s.insertLiftSets(t, "workout_session_id", id, sets)
}

// plannedSet stores a scheduled set for a catalog entry and returns its id.
func (s progressionSeed) plannedSet(t *testing.T, date string, catalogID uuid.UUID, name string, setIndex int) uuid.UUID {
	t.Helper()
	var workoutID, setID uuid.UUID
	if err := s.f.pool.QueryRow(s.f.ctx, `INSERT INTO scheduled_workouts (user_id, date, name) VALUES ($1, $2::date, 'planned') RETURNING id`,
		s.userID, date).Scan(&workoutID); err != nil {
		t.Fatal(err)
	}
	if err := s.f.pool.QueryRow(s.f.ctx, `INSERT INTO scheduled_sets (scheduled_workout_id, catalog_id, exercise_name, exercise_category, exercise_modality,
		exercise_order, set_index, target_reps) VALUES ($1, $2, $3, 'legs', 'strength', 1, $4, 5) RETURNING id`,
		workoutID, catalogID, name, setIndex).Scan(&setID); err != nil {
		t.Fatal(err)
	}
	return setID
}

func set(reps int, weight float64) model.ExerciseSessionSet {
	s := model.ExerciseSessionSet{}
	if reps != 0 {
		s.ActualReps = &reps
	}
	if weight != 0 {
		s.ActualWeight = &weight
	}
	return s
}

// TestExerciseSessions covers the lift-history query against PostgreSQL: the
// three ways a set belongs to a lift, both set sources merged by day, paging,
// the all-time heaviest set, and every set the route must leave out.
func TestExerciseSessions(t *testing.T) {
	f := newImportFixture(t)
	athlete := progressionSeed{f: f, userID: f.userID}
	other := progressionSeed{f: f, userID: f.newUser(t)}
	for _, userID := range []uuid.UUID{athlete.userID, other.userID} {
		t.Cleanup(func() {
			if err := NewAccountDAO(f.pool).DeleteUserData(context.WithoutCancel(f.ctx), userID); err != nil {
				t.Errorf("cleaning fixture data: %v", err)
			}
		})
	}

	var catalogID uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `SELECT id FROM exercise_catalog WHERE modality = 'strength' ORDER BY name LIMIT 1`).Scan(&catalogID); err != nil {
		t.Fatal(err)
	}
	squat := athlete.exercise(t, "Back Squat")
	linked := athlete.exercise(t, "Squat variation")
	if _, err := f.pool.Exec(f.ctx, `UPDATE exercises SET catalog_id = $1 WHERE id = $2`, catalogID, linked); err != nil {
		t.Fatal(err)
	}
	bench := athlete.exercise(t, "Bench Press")

	// Sep 1: a day log by exercise id, three sets in order, one not completed.
	athlete.liftDayLog(t, "2026-09-01",
		liftSet{name: "Back Squat", exerciseID: &squat, weight: 185, reps: 5},
		liftSet{name: "Back Squat", exerciseID: &squat, weight: 195, reps: 5},
		liftSet{name: "Back Squat", exerciseID: &squat, weight: 300, reps: 1, incomplete: true},
		liftSet{name: "Bench Press", exerciseID: &bench, weight: 400, reps: 1})
	// Sep 3: an imported day with no exercise id, matched by name without case.
	athlete.liftDayLog(t, "2026-09-03", liftSet{name: " back squat ", weight: 205, reps: 3})
	// Sep 5: a planned workout matched by catalog id only, under another name.
	planned := athlete.plannedSet(t, "2026-09-05", catalogID, "Barbell Squat", 1)
	athlete.liftSession(t, "2026-09-05", "completed", liftSet{name: "Barbell Squat", scheduledSet: &planned, weight: 205, reps: 5})
	// Sep 7: a saved-workout exercise linked to the catalog entry, a set
	// with reps only, and a timed set.
	athlete.liftDayLog(t, "2026-09-07",
		liftSet{name: "Squat variation", exerciseID: &linked, reps: 12},
		liftSet{name: "Squat variation", exerciseID: &linked, seconds: 45})
	// Sep 8: a discarded session never counts.
	athlete.liftSession(t, "2026-09-08", "discarded", liftSet{name: "Back Squat", exerciseID: &squat, weight: 500, reps: 1})
	// Another athlete's heavier set under the same name and catalog entry.
	other.liftDayLog(t, "2026-09-04", liftSet{name: "Back Squat", weight: 900, reps: 1})

	lift := model.ExerciseSessionsFilter{ExerciseID: &squat, CatalogID: &catalogID, Name: "back squat", Limit: 20}
	seconds := 45
	all := []model.ExerciseSession{
		{Date: "2026-09-07", Sets: []model.ExerciseSessionSet{set(12, 0), {DurationSeconds: &seconds}}},
		{Date: "2026-09-05", Sets: []model.ExerciseSessionSet{set(5, 205)}},
		{Date: "2026-09-03", Sets: []model.ExerciseSessionSet{set(3, 205)}},
		{Date: "2026-09-01", Sets: []model.ExerciseSessionSet{set(5, 185), set(5, 195)}},
	}
	// 205 was lifted twice; more reps wins.
	heaviest := &model.HeaviestSet{Date: "2026-09-05", Weight: 205, Reps: 5}

	t.Run("every identity together returns the whole history, newest first", func(t *testing.T) {
		got, err := f.logs.ExerciseSessions(f.ctx, athlete.userID, lift)
		if err != nil {
			t.Fatal(err)
		}
		want := &model.ExerciseSessionsPage{Sessions: all, Heaviest: heaviest}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ExerciseSessions = %+v, want %+v", got, want)
		}
	})

	t.Run("each identity alone matches only its own sets", func(t *testing.T) {
		tests := []struct {
			name   string
			filter model.ExerciseSessionsFilter
			dates  []string
		}{
			{name: "exercise id", filter: model.ExerciseSessionsFilter{ExerciseID: &squat, Limit: 20}, dates: []string{"2026-09-01"}},
			{name: "catalog id", filter: model.ExerciseSessionsFilter{CatalogID: &catalogID, Limit: 20}, dates: []string{"2026-09-07", "2026-09-05"}},
			{name: "name", filter: model.ExerciseSessionsFilter{Name: "back squat", Limit: 20}, dates: []string{"2026-09-03", "2026-09-01"}},
			{name: "unknown lift", filter: model.ExerciseSessionsFilter{Name: "never lifted", Limit: 20}, dates: []string{}},
		}
		for _, tc := range tests {
			got, err := f.logs.ExerciseSessions(f.ctx, athlete.userID, tc.filter)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			dates := []string{}
			for _, session := range got.Sessions {
				dates = append(dates, session.Date)
			}
			if !reflect.DeepEqual(dates, tc.dates) {
				t.Errorf("%s: dates = %v, want %v", tc.name, dates, tc.dates)
			}
			if got.Sessions == nil {
				t.Errorf("%s: sessions is nil, want an empty list", tc.name)
			}
			if len(tc.dates) == 0 && (got.Heaviest != nil || got.NextBefore != nil) {
				t.Errorf("%s: got heaviest %+v and next %v, want neither", tc.name, got.Heaviest, got.NextBefore)
			}
		}
	})

	t.Run("pages walk the history and keep the all-time heaviest set", func(t *testing.T) {
		paged := lift
		paged.Limit = 2
		var seen []model.ExerciseSession
		for page := 1; ; page++ {
			got, err := f.logs.ExerciseSessions(f.ctx, athlete.userID, paged)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Heaviest, heaviest) {
				t.Errorf("page %d: heaviest = %+v, want %+v", page, got.Heaviest, heaviest)
			}
			seen = append(seen, got.Sessions...)
			if got.NextBefore == nil {
				if page != 2 {
					t.Errorf("history ended on page %d, want 2", page)
				}
				break
			}
			if page == 1 && *got.NextBefore != "2026-09-05" {
				t.Errorf("next_before = %s, want 2026-09-05", *got.NextBefore)
			}
			before, err := time.Parse("2006-01-02", *got.NextBefore)
			if err != nil {
				t.Fatal(err)
			}
			paged.Before = &before
		}
		if !reflect.DeepEqual(seen, all) {
			t.Errorf("pages = %+v, want %+v", seen, all)
		}
	})

	t.Run("a page that ends exactly on the last day has no next page", func(t *testing.T) {
		exact := lift
		exact.Limit = len(all)
		got, err := f.logs.ExerciseSessions(f.ctx, athlete.userID, exact)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Sessions) != len(all) || got.NextBefore != nil {
			t.Errorf("got %d sessions and next %v, want %d and none", len(got.Sessions), got.NextBefore, len(all))
		}
	})

	t.Run("another athlete sees only their own set", func(t *testing.T) {
		got, err := f.logs.ExerciseSessions(f.ctx, other.userID, lift)
		if err != nil {
			t.Fatal(err)
		}
		want := &model.ExerciseSessionsPage{
			Sessions: []model.ExerciseSession{{Date: "2026-09-04", Sets: []model.ExerciseSessionSet{set(1, 900)}}},
			Heaviest: &model.HeaviestSet{Date: "2026-09-04", Weight: 900, Reps: 1},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ExerciseSessions = %+v, want %+v", got, want)
		}
	})

	t.Run("the runtime role reads the same rows", func(t *testing.T) {
		_, _, runtime := newRuntimeRoleFixture(t)
		got, err := NewLogDAO(runtime).ExerciseSessions(f.ctx, athlete.userID, lift)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Sessions, all) {
			t.Errorf("ExerciseSessions = %+v, want %+v", got.Sessions, all)
		}
	})
}
