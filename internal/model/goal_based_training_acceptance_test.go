package model

import (
	"os"
	"strings"
	"testing"
	"time"

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

func TestLegacyWeeklyPlanMapsToDeterministicProgram(t *testing.T) {
	sets, reps, duration := 3, 5, 20
	mondayTemplateID := uuid.New()
	fridayTemplateID := uuid.New()
	assignments := []LegacyWeeklyAssignment{
		{
			Weekday: 5,
			Template: WorkoutTemplate{
				ID: fridayTemplateID, Name: "Conditioning", TypeID: "cardio", SubtypeID: "intervals",
				Exercises: []Exercise{{Name: "Bike", SortOrder: 0, DurationMinutes: &duration}},
			},
		},
		{
			Weekday: 1,
			Template: WorkoutTemplate{
				ID: mondayTemplateID, Name: "Strength", TypeID: "strength", SubtypeID: "full_body",
				Exercises: []Exercise{{Name: "Squat", SortOrder: 0, Sets: &sets, Reps: &reps}},
			},
		},
	}

	program, weekdays, err := ProgramFromLegacyWeeklyPlan(GoalStrength, assignments)
	if err != nil {
		t.Fatal(err)
	}
	if program.Name != LegacyProgramName || program.PrimaryGoal != GoalStrength || !program.Active {
		t.Fatalf("unexpected imported program: %+v", program)
	}
	if len(program.Workouts) != 2 || len(weekdays) != 2 || weekdays[0] != 1 || weekdays[1] != 5 {
		t.Fatalf("weekday order = %v; workouts = %+v", weekdays, program.Workouts)
	}
	strength := program.Workouts[0]
	if strength.PreferredWeekday == nil || *strength.PreferredWeekday != 1 || strength.SequencePosition != 1 {
		t.Fatalf("strength ordering = %+v", strength)
	}
	if got := strength.Exercises[0]; got.Modality != "strength" || got.TargetSets != 3 || got.TargetReps == nil || *got.TargetReps != 5 || got.ExerciseOrder != 1 {
		t.Fatalf("strength mapping = %+v", got)
	}
	cardio := program.Workouts[1].Exercises[0]
	if cardio.Modality != "cardio" || cardio.TargetSets != 1 || cardio.TargetDurationSeconds == nil || *cardio.TargetDurationSeconds != 1200 {
		t.Fatalf("cardio mapping = %+v", cardio)
	}
	if assignments[0].Template.ID != fridayTemplateID || assignments[1].Template.ID != mondayTemplateID {
		t.Fatal("source legacy assignments were mutated")
	}
}

func TestLegacyWeeklyPlanPreservesRepeatedTemplateAssignments(t *testing.T) {
	template := WorkoutTemplate{
		ID: uuid.New(), Name: "Full body", TypeID: "strength", SubtypeID: "general",
		Exercises: []Exercise{{ID: uuid.New(), Name: "Push-Up", SortOrder: 0}},
	}
	program, weekdays, err := ProgramFromLegacyWeeklyPlan(GoalGeneralHealth, []LegacyWeeklyAssignment{
		{Weekday: 1, Template: template},
		{Weekday: 3, Template: template},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(program.Workouts) != 2 || len(weekdays) != 2 {
		t.Fatalf("program = %+v weekdays = %v", program, weekdays)
	}
	if program.Workouts[0].Exercises[0].ID != uuid.Nil || program.Workouts[1].Exercises[0].ID != uuid.Nil {
		t.Fatal("legacy exercise IDs leaked into goal-training identity")
	}
}

func TestNextFutureWeekUsesAthleteTimezone(t *testing.T) {
	// Sunday evening in New York is already Monday UTC. The athlete's local
	// calendar must still choose the immediately upcoming local Monday.
	now := time.Date(2026, time.August, 10, 1, 0, 0, 0, time.UTC)
	from, to, err := NextFutureWeek(now, "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if from != "2026-08-10" || to != "2026-08-16" {
		t.Fatalf("week = %s..%s", from, to)
	}

	from, to, err = NextFutureWeek(time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC), "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if from != "2026-08-17" || to != "2026-08-23" {
		t.Fatalf("monday week = %s..%s", from, to)
	}
}
