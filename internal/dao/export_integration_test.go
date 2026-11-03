//go:build integration

package dao

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type exportFixture struct {
	ctx  context.Context
	pool *pgxpool.Pool
}

func newExportFixture(t *testing.T) *exportFixture {
	t.Helper()
	url := os.Getenv("GYMPULSE_TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("integration tests require GYMPULSE_TEST_DATABASE_URL pointing to a disposable migrated database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return &exportFixture{ctx: ctx, pool: pool}
}

// seedAthlete writes one row into every exported table for a new user whose
// athlete-authored text all carries marker, and returns the user's id.
func (f *exportFixture) seedAthlete(t *testing.T, marker string) uuid.UUID {
	t.Helper()
	userID := uuid.New()
	t.Cleanup(func() {
		cleanup := context.WithoutCancel(f.ctx)
		if err := NewAccountDAO(f.pool).DeleteUserData(cleanup, userID); err != nil {
			t.Errorf("cleaning fixture data: %v", err)
		}
		if _, err := f.pool.Exec(cleanup, `DELETE FROM auth.users WHERE id=$1`, userID); err != nil {
			t.Errorf("cleaning fixture identity: %v", err)
		}
	})
	var templateID, exerciseID, dayLogID, sessionID, discardedID, foodID uuid.UUID
	steps := []struct {
		sql  string
		args []any
		out  *uuid.UUID
	}{
		{sql: `INSERT INTO auth.users (id) VALUES ($1)`, args: []any{userID}},
		{sql: `INSERT INTO user_profiles (id, display_name) VALUES ($1, $2)`, args: []any{userID, marker + " athlete"}},
		{sql: `INSERT INTO user_settings (user_id, weight_unit, weekly_goal) VALUES ($1, 'kg', 4)`, args: []any{userID}},
		{sql: `INSERT INTO training_profiles (user_id, primary_goal, available_days, usual_activity, experience,
			equipment, session_duration_minutes, timezone)
			VALUES ($1,'strength',ARRAY[1,3,5]::smallint[],'moderate','beginner','{barbell}',45,'UTC')`, args: []any{userID}},
		{sql: `INSERT INTO workout_templates (user_id, name, type_id) VALUES ($1, $2, 'strength') RETURNING id`,
			args: []any{userID, marker + " template"}, out: &templateID},
		{sql: `INSERT INTO exercises (template_id, name, sort_order, sets, reps, weight) VALUES ($1, $2, 0, 3, 5, 60) RETURNING id`,
			args: []any{&templateID, marker + " squat"}, out: &exerciseID},
		{sql: `INSERT INTO day_logs (user_id, date, type_id, template_id, session_notes) VALUES ($1, '2026-01-02', 'strength', $2, $3) RETURNING id`,
			args: []any{userID, &templateID, marker + " legacy notes"}, out: &dayLogID},
		{sql: `INSERT INTO set_logs (day_log_id, exercise_id, set_index, actual_reps, actual_weight, completed,
			exercise_name, exercise_category, exercise_modality)
			VALUES ($1, $2, 1, 5, 62.5, true, $3, 'legs', 'strength')`, args: []any{&dayLogID, &exerciseID, marker + " squat"}},
		{sql: `INSERT INTO exercise_overrides (day_log_id, exercise_id, actual_sets, actual_reps) VALUES ($1, $2, 3, 5)`,
			args: []any{&dayLogID, &exerciseID}},
		{sql: `INSERT INTO workout_sessions (user_id, date, name, status, completed_at) VALUES ($1, '2026-02-03', $2, 'completed', now()) RETURNING id`,
			args: []any{userID, marker + " session"}, out: &sessionID},
		{sql: `INSERT INTO set_logs (workout_session_id, set_index, actual_reps, actual_weight, completed,
			exercise_name, exercise_category, exercise_modality)
			VALUES ($1, 2, 8, 40, true, $2, 'push', 'strength'), ($1, 1, 8, 40, true, $2, 'push', 'strength')`,
			args: []any{&sessionID, marker + " press"}},
		{sql: `INSERT INTO workout_sessions (user_id, date, name, status) VALUES ($1, '2026-02-04', $2, 'discarded') RETURNING id`,
			args: []any{userID, marker + " discarded"}, out: &discardedID},
		{sql: `INSERT INTO set_logs (workout_session_id, set_index, completed, exercise_name, exercise_category, exercise_modality)
			VALUES ($1, 1, true, 'thrown away', 'push', 'strength')`, args: []any{&discardedID}},
		{sql: `INSERT INTO sport_activities (id, user_id, date, sport_id, sport_name, duration_minutes, notes)
			VALUES ($1, $2, '2026-02-05', 'tennis', 'Tennis', 60, $3)`, args: []any{uuid.New(), userID, marker + " tennis"}},
		{sql: `INSERT INTO body_weights (user_id, date, weight, unit) VALUES ($1, '2026-02-06', 80.4, 'kg')`, args: []any{userID}},
		{sql: `INSERT INTO nutrition_days (id, user_id, date, protein_g, source, logged_at) VALUES ($1, $2, '2026-02-07', 140.5, 'manual', now())`,
			args: []any{uuid.New(), userID}},
		{sql: `INSERT INTO nutrition_targets (id, user_id, effective_from, protein_g) VALUES ($1, $2, '2026-01-01', 150)`,
			args: []any{uuid.New(), userID}},
		{sql: `INSERT INTO user_foods (id, user_id, name, serving_label, serving_g, energy_kcal, protein_g, carbs_g, fat_g, archived_at)
			VALUES ($1, $2, $3, '1 bowl', 250, 400, 30, 50, 8, now()) RETURNING id`,
			args: []any{uuid.New(), userID, marker + " oats"}, out: &foodID},
		{sql: `INSERT INTO food_entries (id, user_id, date, kind, user_food_id, serving_label, serving_g, amount,
			energy_kcal, protein_g, carbs_g, fat_g, logged_at)
			VALUES ($1, $2, '2026-02-07', 'food', $3, '1 bowl', 250, 1, 400, 30, 50, 8, now())`,
			args: []any{uuid.New(), userID, &foodID}},
		{sql: `INSERT INTO food_entries (id, user_id, date, kind, energy_kcal, logged_at) VALUES ($1, $2, '2026-02-07', 'quick_add', 200, now())`,
			args: []any{uuid.New(), userID}},
	}
	for _, step := range steps {
		args := make([]any, len(step.args))
		for i, arg := range step.args {
			if id, ok := arg.(*uuid.UUID); ok {
				arg = *id
			}
			args[i] = arg
		}
		var err error
		if step.out != nil {
			err = f.pool.QueryRow(f.ctx, step.sql, args...).Scan(step.out)
		} else {
			_, err = f.pool.Exec(f.ctx, step.sql, args...)
		}
		if err != nil {
			t.Fatalf("seeding %q: %v", strings.Fields(step.sql)[2], err)
		}
	}
	return userID
}

func TestExportReadsEveryOwnedSectionAndNothingElse(t *testing.T) {
	f := newExportFixture(t)
	mine := f.seedAthlete(t, "mine")
	f.seedAthlete(t, "theirs")

	exportedAt := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	got, err := NewExportDAO(f.pool).Export(f.ctx, mine, exportedAt)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "theirs") {
		t.Fatalf("export leaked another athlete's data: %s", body)
	}
	if strings.Contains(string(body), "discarded") || strings.Contains(string(body), "thrown away") {
		t.Fatalf("export included a discarded session: %s", body)
	}

	if got.Profile == nil || *got.Profile.DisplayName != "mine athlete" {
		t.Errorf("profile = %+v", got.Profile)
	}
	if got.Settings == nil || got.Settings.WeightUnit != "kg" || got.Settings.WeeklyGoal != 4 {
		t.Errorf("settings = %+v", got.Settings)
	}
	if got.TrainingProfile == nil || len(got.TrainingProfile.AvailableDays) != 3 {
		t.Errorf("training profile = %+v", got.TrainingProfile)
	}
	if len(got.Workouts) != 2 {
		t.Fatalf("workouts = %d, want the session and the legacy log", len(got.Workouts))
	}
	for _, w := range got.Workouts {
		switch w.Source {
		case "session":
			if w.Name != "mine session" || len(w.Sets) != 2 || w.Sets[0].SetIndex != 1 || w.Sets[1].SetIndex != 2 {
				t.Errorf("session = %+v", w)
			}
		case "day_log":
			if w.Name != "mine template" || len(w.Sets) != 1 || *w.Sets[0].ActualWeight != 62.5 || len(w.Exercises) != 1 || *w.Notes != "mine legacy notes" {
				t.Errorf("day log = %+v", w)
			}
		}
	}
	if len(got.Templates) != 1 || len(got.Templates[0].Exercises) != 1 || *got.Templates[0].Exercises[0].Weight != 60 {
		t.Errorf("templates = %+v", got.Templates)
	}
	if len(got.SportActivities) != 1 || got.SportActivities[0].SportName != "Tennis" {
		t.Errorf("sport activities = %+v", got.SportActivities)
	}
	if len(got.BodyWeights) != 1 || got.BodyWeights[0].Weight != 80.4 {
		t.Errorf("body weights = %+v", got.BodyWeights)
	}
	n := got.Nutrition
	if len(n.Days) != 1 || *n.Days[0].ProteinG != 140.5 || len(n.Targets) != 1 || len(n.OwnFoods) != 1 || !n.OwnFoods[0].Archived {
		t.Errorf("nutrition = %+v", n)
	}
	if len(n.FoodEntries) != 2 {
		t.Fatalf("food entries = %+v", n.FoodEntries)
	}
	for _, e := range n.FoodEntries {
		if e.Kind == "food" && (e.FoodName == nil || *e.FoodName != "mine oats") {
			t.Errorf("food entry lost its archived food's name: %+v", e)
		}
	}
	if !got.ExportedAt.Equal(exportedAt) {
		t.Errorf("exported_at = %v", got.ExportedAt)
	}
}

func TestExportOfNewAthleteIsEmptyArrays(t *testing.T) {
	f := newExportFixture(t)
	userID := uuid.New()
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO auth.users (id) VALUES ($1)`, userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = f.pool.Exec(context.WithoutCancel(f.ctx), `DELETE FROM auth.users WHERE id=$1`, userID) })

	got, err := NewExportDAO(f.pool).Export(f.ctx, userID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(got)
	for _, want := range []string{`"profile":null`, `"workouts":[]`, `"templates":[]`, `"food_entries":[]`, `"own_foods":[]`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("missing %s in %s", want, body)
		}
	}
}

// The deployed API connects as the least-privilege runtime role, which has no
// owner bypass. The export must read through its grants and RLS policies.
func TestExportWorksAsRuntimeRole(t *testing.T) {
	f := newExportFixture(t)
	var canAssume bool
	if err := f.pool.QueryRow(f.ctx, `SELECT pg_has_role(current_user, 'gym_pulse_api', 'MEMBER')`).Scan(&canAssume); err != nil {
		t.Fatal(err)
	}
	if !canAssume {
		t.Skip("test database user cannot assume gym_pulse_api")
	}
	mine := f.seedAthlete(t, "mine")

	config := f.pool.Config().Copy()
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, `SET ROLE gym_pulse_api`)
		return err
	}
	runtime, err := pgxpool.NewWithConfig(f.ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(runtime.Close)

	got, err := NewExportDAO(runtime).Export(f.ctx, mine, time.Now())
	if err != nil {
		t.Fatalf("export as runtime role: %v", err)
	}
	if len(got.Workouts) != 2 || len(got.Nutrition.FoodEntries) != 2 {
		t.Fatalf("runtime role saw %d workouts and %d food entries", len(got.Workouts), len(got.Nutrition.FoodEntries))
	}
}
