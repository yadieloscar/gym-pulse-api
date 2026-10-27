//go:build integration

package dao

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// isolatedMigrationDB creates a throwaway database on the integration server so
// migration tests can move the schema up and down without touching the shared,
// already-migrated test database.
func isolatedMigrationDB(t *testing.T) (*migrate.Migrate, *pgxpool.Pool) {
	t.Helper()
	base := os.Getenv("GYMPULSE_TEST_DATABASE_URL")
	if base == "" {
		t.Fatal("integration tests require GYMPULSE_TEST_DATABASE_URL pointing to a disposable migrated database")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	name := "gympulse_migration_" + uuid.NewString()[:8]
	if _, err := admin.Exec(ctx, `CREATE DATABASE `+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.WithoutCancel(ctx), `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`); err != nil {
			t.Errorf("dropping %s: %v", name, err)
		}
	})

	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + name
	target := parsed.String()

	db, err := sql.Open("pgx", target)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://../../migrations", "postgres", driver)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return m, pool
}

func TestReleaseMigration020BackfillsOnlyRecoveredSources(t *testing.T) {
	m, pool := isolatedMigrationDB(t)
	ctx := t.Context()
	if err := m.Migrate(19); err != nil {
		t.Fatal(err)
	}

	userID := uuid.New()
	mustExec := func(query string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	mustExec(`INSERT INTO auth.users (id) VALUES ($1)`, userID)
	program := releaseProgram()
	reps := 8
	program.Workouts = append(program.Workouts, model.ProgramWorkout{Name: "B", SequencePosition: 2,
		Exercises: []model.ProgramExercise{{Name: "Bench", Category: "push", Modality: "strength", ExerciseOrder: 1, TargetSets: 1, TargetReps: &reps}}})
	// The schema is at version 19 here, before the program unit kind columns,
	// so the fixture program is inserted with the columns of that era rather
	// than through the current DAO.
	if err := pool.QueryRow(ctx, `INSERT INTO programs (user_id, name, primary_goal, roadmap, active)
		VALUES ($1,$2,$3,'{}'::jsonb,true) RETURNING id`, userID, program.Name, program.PrimaryGoal).Scan(&program.ID); err != nil {
		t.Fatal(err)
	}
	for i := range program.Workouts {
		if err := pool.QueryRow(ctx, `INSERT INTO program_workouts (program_id, name, sequence_position)
			VALUES ($1,$2,$3) RETURNING id`, program.ID, program.Workouts[i].Name, program.Workouts[i].SequencePosition).Scan(&program.Workouts[i].ID); err != nil {
			t.Fatal(err)
		}
	}
	workoutA, workoutB := program.Workouts[0].ID, program.Workouts[1].ID

	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	insert := func(programWorkoutID *uuid.UUID, sequence int, date, status string, finalizedAt *time.Time, createdAt time.Time) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		err := pool.QueryRow(ctx, `INSERT INTO scheduled_workouts
			(user_id, program_id, program_workout_id, date, name, sequence_position, status, finalized_at, created_at)
			VALUES ($1,$2,$3,$4,'W',$5,$6,$7,$8) RETURNING id`,
			userID, program.ID, programWorkoutID, date, sequence, status, finalizedAt, createdAt).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	recordRecovery := func(copyID uuid.UUID, at time.Time) {
		t.Helper()
		mustExec(`INSERT INTO idempotency_records (user_id, scope, operation_key, request_hash, response_status, response_body, resource_type, resource_id, created_at)
			VALUES ($1,'schedule/recover',$2,'hash',201,'{}','scheduled_workout',$3,$4)`, userID, uuid.NewString(), copyID, at)
	}
	at := func(days int) *time.Time { v := base.AddDate(0, 0, days); return &v }

	// A missed on day 0, recovered on day 2 into a copy dated day 2.
	missedA := insert(&workoutA, 1, "2026-09-01", "missed", at(1), base)
	copyA := insert(&workoutA, 1, "2026-09-03", "completed", at(3), *at(2))
	recordRecovery(copyA, *at(2))
	// The same program workout a cycle later, missed after that recovery: not recovered.
	laterA := insert(&workoutA, 1, "2026-09-08", "missed", at(8), *at(5))
	// B missed and recovered the same day it was finalized.
	missedB := insert(&workoutB, 2, "2026-09-10", "missed", at(10), *at(7))
	copyB := insert(&workoutB, 2, "2026-09-10", "planned", nil, *at(10))
	recordRecovery(copyB, *at(10))
	// B missed again with no recovery record.
	unrecorded := insert(&workoutB, 2, "2026-09-15", "missed", at(15), *at(12))
	// A pair whose program workout was later deleted (provenance SET NULL).
	missedOrphan := insert(nil, 3, "2026-09-20", "missed", at(20), *at(18))
	copyOrphan := insert(nil, 3, "2026-09-22", "planned", nil, *at(21))
	recordRecovery(copyOrphan, *at(21))

	if err := m.Migrate(20); err != nil {
		t.Fatal(err)
	}
	resolution := func(id uuid.UUID) (*string, *time.Time) {
		t.Helper()
		var value *string
		var resolvedAt *time.Time
		if err := pool.QueryRow(ctx, `SELECT resolution, resolved_at FROM scheduled_workouts WHERE id=$1`, id).Scan(&value, &resolvedAt); err != nil {
			t.Fatal(err)
		}
		return value, resolvedAt
	}
	for _, want := range []struct {
		name string
		id   uuid.UUID
		at   *time.Time
	}{{"recovered A", missedA, at(2)}, {"same-day B", missedB, at(10)}, {"orphaned provenance", missedOrphan, at(21)}} {
		value, resolvedAt := resolution(want.id)
		if value == nil || *value != model.ResolutionRecovered || resolvedAt == nil || !resolvedAt.Equal(*want.at) {
			t.Fatalf("%s: resolution=%v resolved_at=%v", want.name, value, resolvedAt)
		}
	}
	for name, id := range map[string]uuid.UUID{"later cycle": laterA, "unrecorded": unrecorded, "copy A": copyA, "copy B": copyB} {
		if value, _ := resolution(id); value != nil {
			t.Fatalf("%s was marked %s", name, *value)
		}
	}

	if err := m.Migrate(19); err != nil {
		t.Fatal(err)
	}
	var columns int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_name='scheduled_workouts' AND column_name IN ('resolution','resolved_at')`).Scan(&columns); err != nil || columns != 0 {
		t.Fatalf("down migration left %d columns (%v)", columns, err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatal(err)
	}
	if value, _ := resolution(missedA); value == nil || *value != model.ResolutionRecovered {
		t.Fatalf("backfill not reapplied after down/up: %v", value)
	}
}
