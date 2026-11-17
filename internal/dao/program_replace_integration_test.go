//go:build integration

package dao

import (
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// TestProgramReplaceLeavesNoSessionWithoutAUnit covers the unit-replacing
// save that has no clock of its own (programDAO.Replace). It deletes every
// unit, which clears the unit reference on the program's sessions, so the
// untouched future must go with the units. Dates are relative to the
// database's today because that is the clock Replace reads.
func TestProgramReplaceLeavesNoSessionWithoutAUnit(t *testing.T) {
	f := newReleaseFixture(t)
	programs := &programDAO{pool: f.pool}

	program := &model.Program{Name: "Two days", PrimaryGoal: "strength", Roadmap: map[string]any{}, Active: false,
		Workouts: []model.ProgramWorkout{weekUnit("Ok", 1, 4, 7)}}
	if err := programs.Create(f.ctx, f.userID, program); err != nil {
		t.Fatal(err)
	}
	unitID := program.Workouts[0].ID
	f.exec(t, `UPDATE programs SET scheduled_through = current_date + 56 WHERE id=$1`, program.ID)

	session := func(name string, dayOffset int, status string, finalized bool) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		err := f.pool.QueryRow(f.ctx, `INSERT INTO scheduled_workouts
			(user_id, program_id, program_workout_id, date, slot_date, name, sequence_position, status, finalized_at)
			VALUES ($1,$2,$3, current_date + $4::int, current_date + $4::int, $5, 1, $6,
				CASE WHEN $7 THEN now() ELSE NULL END) RETURNING id`,
			f.userID, program.ID, unitID, dayOffset, name, status, finalized).Scan(&id)
		if err != nil {
			t.Fatalf("inserting %s: %v", name, err)
		}
		return id
	}
	removedNear := session("future near", 2, "planned", false)
	removedFar := session("future far", 30, "planned", false)
	keptToday := session("today", 0, "planned", false)
	keptPast := session("past", -3, "missed", true)
	keptStarted := session("future, workout started", 4, "planned", false)
	f.exec(t, `INSERT INTO workout_sessions (user_id, scheduled_workout_id, date, name, status)
		VALUES ($1,$2,current_date + 4,'started','active')`, f.userID, keptStarted)

	current, err := programs.Get(f.ctx, f.userID, program.ID)
	if err != nil {
		t.Fatal(err)
	}
	replacement := &model.Program{ID: program.ID, Name: "Two days", PrimaryGoal: "strength", Roadmap: map[string]any{},
		Active: false, Workouts: []model.ProgramWorkout{weekUnit("Jump taper A", 1, 3, 6)}}
	if err := programs.Replace(f.ctx, f.userID, replacement, current.Revision); err != nil {
		t.Fatal(err)
	}

	exists := func(id uuid.UUID) bool {
		t.Helper()
		var found bool
		if err := f.pool.QueryRow(f.ctx, `SELECT EXISTS (SELECT 1 FROM scheduled_workouts WHERE id=$1)`, id).Scan(&found); err != nil {
			t.Fatal(err)
		}
		return found
	}
	for name, id := range map[string]uuid.UUID{"near future session": removedNear, "far future session": removedFar} {
		if exists(id) {
			t.Errorf("%s of a replaced unit is still scheduled", name)
		}
	}
	for name, id := range map[string]uuid.UUID{"today's session": keptToday, "past session": keptPast, "started session": keptStarted} {
		if !exists(id) {
			t.Errorf("%s was removed; Replace must keep it", name)
		}
	}

	var strays int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM scheduled_workouts sw
		WHERE sw.user_id=$1 AND sw.program_id=$2 AND sw.program_workout_id IS NULL AND sw.date > current_date AND `+unstartedPlanned,
		f.userID, program.ID).Scan(&strays); err != nil {
		t.Fatal(err)
	}
	if strays != 0 {
		t.Errorf("%d untouched future sessions have no unit after Replace, want 0", strays)
	}

	var refillsFromTomorrow bool
	if err := f.pool.QueryRow(f.ctx, `SELECT scheduled_through = current_date FROM programs WHERE id=$1`, program.ID).Scan(&refillsFromTomorrow); err != nil {
		t.Fatal(err)
	}
	if !refillsFromTomorrow {
		t.Error("the fill mark must move back to today, so the next read lays the new units out from tomorrow")
	}
}
