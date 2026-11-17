//go:build integration

package dao

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// roundTripCounter counts the statements a connection sends on their own and
// the batches it sends as one round trip.
type roundTripCounter struct {
	queries atomic.Int64
	batches atomic.Int64
}

func (c *roundTripCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.queries.Add(1)
	return ctx
}

func (c *roundTripCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (c *roundTripCounter) TraceBatchStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceBatchStartData) context.Context {
	c.batches.Add(1)
	return ctx
}

func (c *roundTripCounter) TraceBatchQuery(context.Context, *pgx.Conn, pgx.TraceBatchQueryData) {}

func (c *roundTripCounter) TraceBatchEnd(context.Context, *pgx.Conn, pgx.TraceBatchEndData) {}

// TestSavingTheActiveProgramStaysWithinARoundTripBudget guards the save that
// timed out in production on 2026-11-17. Replacing a workout with a
// six-exercise one and adding a sport session lays about two dozen sessions
// and almost three hundred sets out to the horizon. With one statement per
// row that was about 360 round trips, more than the app's 15-second timeout
// allows when the database is in another region; the rows now travel in
// batches.
func TestSavingTheActiveProgramStaysWithinARoundTripBudget(t *testing.T) {
	f := newReleaseFixture(t)
	program := f.startOwnWeek(t)

	config, err := pgxpool.ParseConfig(os.Getenv("GYMPULSE_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	counter := &roundTripCounter{}
	config.ConnConfig.Tracer = counter
	config.MaxConns = 2
	counted, err := pgxpool.NewWithConfig(f.ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(counted.Close)

	legDay := weekUnit("Leg day", 1, 3, 7)
	legDay.Exercises = nil
	for order := 1; order <= 6; order++ {
		reps := 10
		legDay.Exercises = append(legDay.Exercises, model.ProgramExercise{
			Name: fmt.Sprintf("Exercise %d", order), Category: "strength", Modality: "strength",
			ExerciseOrder: order, TargetSets: 3, TargetReps: &reps,
		})
	}
	edited := &model.Program{ID: program.ID, Name: program.Name, PrimaryGoal: program.PrimaryGoal, Roadmap: program.Roadmap, Active: true,
		Workouts: []model.ProgramWorkout{legDay, weekSport("Volleyball", 2, "volleyball", 5)}}

	counter.queries.Store(0)
	counter.batches.Store(0)
	saved, _, err := (&trainingMutationDAO{pool: counted}).ReplaceProgram(f.ctx, f.userID, edited, program.Revision, saveNoon, releaseRecord("programs/update"))
	if err != nil {
		t.Fatal(err)
	}

	var sessions, sets int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(DISTINCT sw.id), count(ss.id) FROM scheduled_workouts sw
		LEFT JOIN scheduled_sets ss ON ss.scheduled_workout_id = sw.id
		WHERE sw.user_id=$1 AND sw.program_id=$2 AND sw.date >= $3`, f.userID, saved.ID, startTomorrow).Scan(&sessions, &sets); err != nil {
		t.Fatal(err)
	}
	if sessions < 20 || sets < 250 {
		t.Fatalf("the save laid out %d sessions and %d sets; the scenario needs at least 20 and 250 to mean anything", sessions, sets)
	}

	const budget = 60
	roundTrips := counter.queries.Load() + counter.batches.Load()
	if roundTrips > budget {
		t.Errorf("the save took %d round trips (%d statements, %d batches) for %d sessions and %d sets, want at most %d",
			roundTrips, counter.queries.Load(), counter.batches.Load(), sessions, sets, budget)
	}
	t.Logf("%d sessions, %d sets, %d statements, %d batches", sessions, sets, counter.queries.Load(), counter.batches.Load())
}
