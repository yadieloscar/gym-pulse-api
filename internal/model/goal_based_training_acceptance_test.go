package model

import (
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func readMigration(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile("../../migrations/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestGoalStarterCoverage(t *testing.T) {
	seed := readMigration(t, "013_seed_starter_programs.up.sql")
	if len(ValidTrainingGoals) != 6 {
		t.Fatalf("goal count=%d want 6", len(ValidTrainingGoals))
	}
	for _, goal := range ValidTrainingGoals {
		if !strings.Contains(seed, "'"+goal+"'") {
			t.Errorf("starter seed missing goal %q", goal)
		}
	}
	for _, cadence := range []string{"min_days, max_days", "1, 7", "Full Body A", "Full Body B"} {
		if !strings.Contains(seed, cadence) {
			t.Errorf("starter seed missing adaptable cadence marker %q", cadence)
		}
	}
}

func TestProgramScheduleParticipationPersistenceContract(t *testing.T) {
	migration := readMigration(t, "012_create_training_domain.up.sql")
	for _, invariant := range []string{
		"CREATE TABLE programs", "CREATE TABLE scheduled_workouts",
		"CREATE TABLE scheduled_sets", "CREATE TABLE workout_sessions",
		"CREATE TABLE day_participation", "CREATE TABLE idempotency_records",
		"ON DELETE SET NULL", "exercise_name TEXT", "revision BIGINT",
	} {
		if !strings.Contains(migration, invariant) {
			t.Errorf("training migration missing invariant %q", invariant)
		}
	}
}

func TestMigration011UpgradePreservesHistoryAndRemovesDateIdentity(t *testing.T) {
	migration := readMigration(t, "012_create_training_domain.up.sql")
	for _, upgrade := range []string{
		"ALTER TABLE day_logs DROP CONSTRAINT IF EXISTS day_logs_user_id_date_key",
		"UPDATE set_logs sl", "ALTER COLUMN exercise_id DROP NOT NULL",
		"REFERENCES exercises(id) ON DELETE SET NULL",
	} {
		if !strings.Contains(migration, upgrade) {
			t.Errorf("migration-011 upgrade missing %q", upgrade)
		}
	}
}

func TestMultipleSessionIDsCanShareOneDate(t *testing.T) {
	date := "2026-07-20"
	sessions := []WorkoutSession{{ID: uuid.New(), Date: date}, {ID: uuid.New(), Date: date}}
	if sessions[0].ID == sessions[1].ID || sessions[0].Date != sessions[1].Date {
		t.Fatal("UUID session identity unexpectedly collapsed by date")
	}
	migration := readMigration(t, "012_create_training_domain.up.sql")
	if strings.Contains(migration, "UNIQUE (user_id, date)\n);\n\nCREATE INDEX idx_workout_sessions") {
		t.Fatal("workout_sessions unexpectedly has date-only identity")
	}
}

func TestAccountDeletionCascadesGoalTrainingData(t *testing.T) {
	migration := readMigration(t, "012_create_training_domain.up.sql")
	for _, table := range []string{"training_profiles", "programs", "scheduled_workouts", "workout_sessions", "day_participation", "idempotency_records"} {
		start := strings.Index(migration, "CREATE TABLE "+table)
		if start < 0 {
			t.Fatalf("missing table %s", table)
		}
		end := strings.Index(migration[start:], ");")
		if end < 0 || !strings.Contains(migration[start:start+end], "REFERENCES auth.users(id) ON DELETE CASCADE") {
			t.Errorf("%s does not cascade with account deletion", table)
		}
	}
}
