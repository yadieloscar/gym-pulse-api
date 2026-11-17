//go:build integration

package dbmigrate

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TestMigration046RemovesOrphanedProgramSessions stages a database at version
// 45 in the state production was in after upgrading from the older contract:
// future sessions whose program workout was deleted, alongside sessions the
// rolling fill created for the current workout. Applying 046 must remove only
// the untouched future orphans and reset the active program's watermark.
func TestMigration046RemovesOrphanedProgramSessions(t *testing.T) {
	databaseURL := scratchDatabase(t)
	m := newMigrator(t, databaseURL)
	if err := m.Migrate(45); err != nil {
		t.Fatalf("migrating to 45: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.WithoutCancel(ctx))
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}

	userID := uuid.New()
	exec(`INSERT INTO auth.users (id) VALUES ($1)`, userID)
	exec(`INSERT INTO training_profiles (user_id, primary_goal, available_days, usual_activity, experience,
		equipment, session_duration_minutes, timezone)
		VALUES ($1,'strength',ARRAY[3,4,6,7]::smallint[],'moderate','advanced','{barbell}',60,'UTC')`, userID)

	var activeID, savedID, unitID uuid.UUID
	if err := conn.QueryRow(ctx, `INSERT INTO programs (user_id, name, primary_goal, roadmap, active, scheduled_through)
		VALUES ($1,'Active','strength','{}'::jsonb,true, current_date + 56) RETURNING id`, userID).Scan(&activeID); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, `INSERT INTO programs (user_id, name, primary_goal, roadmap, active)
		VALUES ($1,'Saved','strength','{}'::jsonb,false) RETURNING id`, userID).Scan(&savedID); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, `INSERT INTO program_workouts (program_id, name, sequence_position, weekdays)
		VALUES ($1,'Ok',1,ARRAY[4,7]::smallint[]) RETURNING id`, activeID).Scan(&unitID); err != nil {
		t.Fatal(err)
	}

	session := func(name string, programID uuid.UUID, unit *uuid.UUID, dayOffset int, status string, finalized bool) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		err := conn.QueryRow(ctx, `INSERT INTO scheduled_workouts
			(user_id, program_id, program_workout_id, date, slot_date, name, sequence_position, status, finalized_at)
			VALUES ($1,$2,$3, current_date + $4::int, CASE WHEN $3::uuid IS NULL THEN NULL ELSE current_date + $4::int END, $5, 1, $6,
				CASE WHEN $7 THEN now() ELSE NULL END) RETURNING id`,
			userID, programID, unit, dayOffset, name, status, finalized).Scan(&id)
		if err != nil {
			t.Fatalf("inserting %s: %v", name, err)
		}
		return id
	}

	removedA := session("orphan future A", activeID, nil, 2, "planned", false)
	removedB := session("orphan future B", activeID, nil, 9, "planned", false)
	removedSaved := session("orphan of a saved program", savedID, nil, 3, "planned", false)
	keptToday := session("orphan today", activeID, nil, 0, "planned", false)
	keptPast := session("orphan past", activeID, nil, -3, "missed", true)
	keptStarted := session("orphan future, workout started", activeID, nil, 4, "planned", false)
	keptCurrent := session("current workout", activeID, &unitID, 30, "planned", false)
	exec(`INSERT INTO workout_sessions (user_id, scheduled_workout_id, date, name, status)
		VALUES ($1,$2,current_date + 4,'started','active')`, userID, keptStarted)

	if err := m.Migrate(46); err != nil {
		t.Fatalf("migrating to 46: %v", err)
	}

	exists := func(id uuid.UUID) bool {
		t.Helper()
		var found bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM scheduled_workouts WHERE id=$1)`, id).Scan(&found); err != nil {
			t.Fatal(err)
		}
		return found
	}
	for name, id := range map[string]uuid.UUID{"future orphan A": removedA, "future orphan B": removedB, "saved program's orphan": removedSaved} {
		if exists(id) {
			t.Errorf("%s is still scheduled; 046 must remove it", name)
		}
	}
	for name, id := range map[string]uuid.UUID{"today's orphan": keptToday, "past orphan": keptPast, "started orphan": keptStarted, "current workout's session": keptCurrent} {
		if !exists(id) {
			t.Errorf("%s was removed; 046 must keep it", name)
		}
	}

	var activeAtToday, savedUntouched bool
	if err := conn.QueryRow(ctx, `SELECT scheduled_through = current_date FROM programs WHERE id=$1`, activeID).Scan(&activeAtToday); err != nil {
		t.Fatal(err)
	}
	if !activeAtToday {
		t.Error("the active program's watermark must be today, so the next read fills from tomorrow")
	}
	if err := conn.QueryRow(ctx, `SELECT scheduled_through IS NULL FROM programs WHERE id=$1`, savedID).Scan(&savedUntouched); err != nil {
		t.Fatal(err)
	}
	if !savedUntouched {
		t.Error("a saved program's watermark must not change")
	}

	// Running it again changes nothing: there is nothing left to remove.
	if err := m.Force(45); err != nil {
		t.Fatal(err)
	}
	if err := m.Migrate(46); err != nil {
		t.Fatalf("re-applying 46: %v", err)
	}
	if !exists(keptToday) || !exists(keptStarted) || !exists(keptCurrent) {
		t.Error("re-applying 046 removed a session it must keep")
	}
}
