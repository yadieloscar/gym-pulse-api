package dao

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// ProgramRecapRead is everything a program recap is computed from, read in
// one round trip. Dates are YYYY-MM-DD.
type ProgramRecapRead struct {
	Name   string
	Active bool
	// TrainingDays is how many distinct weekdays the program's units sit
	// on; 0 for a program that was never placed.
	TrainingDays int
	// Timezone is the athlete's saved training timezone, or "".
	Timezone string
	// Sessions are the program's scheduled sessions, oldest first.
	Sessions []ProgramRecapSession
	// TrainedDates are the days the athlete trained between the program's
	// first and last session.
	TrainedDates []string
	// WeeksOff are the Mondays of the weeks off that overlap those dates.
	WeeksOff []string
	// LiftDays holds, per lift and day, the heaviest set lifted in the
	// program's sessions, ordered by lift then date.
	LiftDays []ProgramRecapLiftDay
}

// ProgramRecapSession is one scheduled session of the program.
type ProgramRecapSession struct {
	Date   string
	Status string
}

// ProgramRecapLiftDay is the heaviest set of one lift on one day.
type ProgramRecapLiftDay struct {
	Name   string
	Date   string
	Weight float64
	Reps   int
}

// ProgramRecapDAO reads the facts behind GET /programs/{id}/recap.
type ProgramRecapDAO interface {
	// Read returns the recap facts of a program the athlete owns, in one
	// database round trip. A missing or foreign program is a
	// *model.NotFoundError. No slice in the result is nil.
	Read(ctx context.Context, userID, programID uuid.UUID) (*ProgramRecapRead, error)
}

type programRecapDAO struct{ pool *pgxpool.Pool }

// NewProgramRecapDAO returns a ProgramRecapDAO.
func NewProgramRecapDAO(pool *pgxpool.Pool) ProgramRecapDAO {
	return &programRecapDAO{pool: pool}
}

// programRecapSpan is the first and last date of the program's sessions;
// $1 is the athlete and $2 the program.
const programRecapSpan = `
	SELECT min(date) AS first, max(date) AS last
	FROM scheduled_workouts WHERE user_id=$1 AND program_id=$2`

func (r *programRecapDAO) Read(ctx context.Context, userID, programID uuid.UUID) (*ProgramRecapRead, error) {
	batch := &pgx.Batch{}
	batch.Queue(`
		SELECT p.name, p.active,
		       (SELECT count(DISTINCT weekday)
		        FROM program_workouts pw CROSS JOIN LATERAL unnest(pw.weekdays) AS weekday
		        WHERE pw.program_id=p.id),
		       COALESCE((SELECT timezone FROM training_profiles WHERE user_id=$1), '')
		FROM programs p WHERE p.user_id=$1 AND p.id=$2`, userID, programID)
	batch.Queue(`
		SELECT to_char(date, 'YYYY-MM-DD'), status
		FROM scheduled_workouts WHERE user_id=$1 AND program_id=$2
		ORDER BY date, created_at`, userID, programID)
	batch.Queue(`
		WITH span AS (`+programRecapSpan+`)
		SELECT to_char(dp.local_date, 'YYYY-MM-DD')
		FROM day_participation dp, span
		WHERE dp.user_id=$1 AND dp.participated AND dp.local_date BETWEEN span.first AND span.last
		ORDER BY dp.local_date`, userID, programID)
	batch.Queue(`
		WITH span AS (`+programRecapSpan+`)
		SELECT to_char(wo.week_start, 'YYYY-MM-DD')
		FROM weeks_off wo, span
		WHERE wo.user_id=$1 AND wo.week_start BETWEEN span.first - 6 AND span.last
		ORDER BY wo.week_start`, userID, programID)
	batch.Queue(`
		SELECT DISTINCT ON (lower(btrim(sl.exercise_name)), ws.date)
		       btrim(sl.exercise_name), to_char(ws.date, 'YYYY-MM-DD'), sl.actual_weight, sl.actual_reps
		FROM set_logs sl
		JOIN workout_sessions ws ON ws.id=sl.workout_session_id
		JOIN scheduled_workouts sw ON sw.id=ws.scheduled_workout_id
		WHERE ws.user_id=$1 AND sw.user_id=$1 AND sw.program_id=$2 AND ws.status <> 'discarded'
		  AND sl.completed=true AND sl.actual_weight > 0 AND sl.actual_reps > 0
		ORDER BY lower(btrim(sl.exercise_name)), ws.date, sl.actual_weight DESC, sl.actual_reps DESC`, userID, programID)

	results := r.pool.SendBatch(ctx, batch)
	read, err := readProgramRecap(results)
	if closeErr := results.Close(); err == nil && closeErr != nil {
		err = fmt.Errorf("reading program recap: %w", closeErr)
	}
	if err != nil {
		return nil, err
	}
	return read, nil
}

func readProgramRecap(results pgx.BatchResults) (*ProgramRecapRead, error) {
	read := &ProgramRecapRead{}
	err := results.QueryRow().Scan(&read.Name, &read.Active, &read.TrainingDays, &read.Timezone)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "program not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("querying recap program: %w", err)
	}

	rows, err := results.Query()
	if err != nil {
		return nil, fmt.Errorf("querying recap sessions: %w", err)
	}
	read.Sessions, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (ProgramRecapSession, error) {
		var session ProgramRecapSession
		err := row.Scan(&session.Date, &session.Status)
		return session, err
	})
	if err != nil {
		return nil, fmt.Errorf("reading recap sessions: %w", err)
	}

	if read.TrainedDates, err = collectRecapDates(results, "trained dates"); err != nil {
		return nil, err
	}
	if read.WeeksOff, err = collectRecapDates(results, "weeks off"); err != nil {
		return nil, err
	}

	rows, err = results.Query()
	if err != nil {
		return nil, fmt.Errorf("querying recap lifts: %w", err)
	}
	read.LiftDays, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (ProgramRecapLiftDay, error) {
		var day ProgramRecapLiftDay
		err := row.Scan(&day.Name, &day.Date, &day.Weight, &day.Reps)
		return day, err
	})
	if err != nil {
		return nil, fmt.Errorf("reading recap lifts: %w", err)
	}
	return read, nil
}

// collectRecapDates reads the next batch result as a list of dates.
func collectRecapDates(results pgx.BatchResults, what string) ([]string, error) {
	rows, err := results.Query()
	if err != nil {
		return nil, fmt.Errorf("querying recap %s: %w", what, err)
	}
	dates, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("reading recap %s: %w", what, err)
	}
	return dates, nil
}
