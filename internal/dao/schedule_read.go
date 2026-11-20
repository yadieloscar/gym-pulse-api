package dao

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// The API and the database are in different regions, so each round trip
// costs tens of milliseconds. The reads below send every statement a schedule
// or participation read needs as one batch: one round trip, whatever the
// range holds. Each statement still sees its own snapshot, as separate
// queries did.

// ScheduleRead is what a schedule read needs before it decides whether to
// fill or finalize anything.
type ScheduleRead struct {
	// Profile is nil when ProfileErr is set.
	Profile *model.TrainingProfile
	// ProfileErr is the NotFoundError of an athlete without a training
	// profile; other errors fail the read.
	ProfileErr error
	// ActiveProgram reports whether the athlete has an active program.
	ActiveProgram bool
	// ScheduledThrough is the active program's fill watermark (YYYY-MM-DD),
	// empty when it was never filled.
	ScheduledThrough string
	// Workouts is the range's sessions with their sets, as List returns them.
	Workouts []model.ScheduledWorkout
}

// ScheduleReader reads a schedule range in one round trip. The schedule DAO
// implements it; services reach it by type assertion.
type ScheduleReader interface {
	ReadSchedule(ctx context.Context, userID uuid.UUID, from, to string) (*ScheduleRead, error)
}

// ParticipationRead is what a participation read needs: the sessions it may
// have to finalize first, and the stored outcomes.
type ParticipationRead struct {
	// Profile and ProfileErr are as in ScheduleRead.
	Profile    *model.TrainingProfile
	ProfileErr error
	// Unfinalized is the range's sessions that are not finalized, with ID,
	// Date, and Revision only.
	Unfinalized []model.ScheduledWorkout
	// Days is the range's stored participation, as List returns it.
	Days []model.DayParticipation
}

// ParticipationReader reads a participation range in one round trip. The
// participation DAO implements it; services reach it by type assertion.
type ParticipationReader interface {
	ReadParticipation(ctx context.Context, userID uuid.UUID, from, to string) (*ParticipationRead, error)
}

const selectActiveProgramWatermark = `SELECT to_char(scheduled_through, 'YYYY-MM-DD') FROM programs WHERE user_id=$1 AND active=true`

// ReadSchedule implements ScheduleReader.
func (r *scheduleDAO) ReadSchedule(ctx context.Context, userID uuid.UUID, from, to string) (*ScheduleRead, error) {
	batch := &pgx.Batch{}
	batch.Queue(selectTrainingProfile, userID)
	batch.Queue(selectActiveProgramWatermark, userID)
	batch.Queue(`
		SELECT `+scheduledWorkoutColumns+`
		FROM scheduled_workouts sw
		`+scheduledWorkoutJoin+`
		WHERE sw.user_id=$1 AND sw.date BETWEEN $2 AND $3
		ORDER BY sw.date, sw.created_at`, userID, from, to)
	batch.Queue(listRequiredSetsSelect+`
		WHERE sw.date BETWEEN $2 AND $3
		`+listRequiredSetsOrder, userID, from, to)
	batch.Queue(listExtraSetsSelect+`
		JOIN scheduled_workouts sw ON sw.id=ws.scheduled_workout_id AND sw.user_id=$1
		WHERE sw.date BETWEEN $2 AND $3 AND sl.is_extra=true
		`+listExtraSetsOrder, userID, from, to)
	results := r.pool.SendBatch(ctx, batch)
	read, err := readScheduleResults(results)
	if closeErr := results.Close(); err == nil && closeErr != nil {
		err = fmt.Errorf("reading schedule: %w", closeErr)
	}
	if err != nil {
		return nil, err
	}
	return read, nil
}

func readScheduleResults(results pgx.BatchResults) (*ScheduleRead, error) {
	read := &ScheduleRead{}
	var err error
	read.Profile, read.ProfileErr, err = readProfileResult(results)
	if err != nil {
		return nil, err
	}
	var through *string
	err = results.QueryRow().Scan(&through)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return nil, fmt.Errorf("checking schedule watermark: %w", err)
	default:
		read.ActiveProgram = true
		if through != nil {
			read.ScheduledThrough = *through
		}
	}

	rows, err := results.Query()
	if err != nil {
		return nil, fmt.Errorf("querying scheduled workouts: %w", err)
	}
	read.Workouts = []model.ScheduledWorkout{}
	for rows.Next() {
		var w model.ScheduledWorkout
		if err := scanScheduledWorkout(rows, &w); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scanning scheduled workout: %w", err)
		}
		w.RequiredSets = []model.ScheduledSet{}
		w.ExtraSets = []model.PerformedSet{}
		read.Workouts = append(read.Workouts, w)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating scheduled workouts: %w", err)
	}
	index := make(map[uuid.UUID]int, len(read.Workouts))
	for i := range read.Workouts {
		index[read.Workouts[i].ID] = i
	}

	rows, err = results.Query()
	if err != nil {
		return nil, fmt.Errorf("querying scheduled sets: %w", err)
	}
	err = scanListRequiredSets(rows, read.Workouts, index)
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = results.Query()
	if err != nil {
		return nil, fmt.Errorf("querying extra sets: %w", err)
	}
	err = scanListExtraSets(rows, read.Workouts, index)
	rows.Close()
	if err != nil {
		return nil, err
	}
	return read, nil
}

// readProfileResult reads a queued selectTrainingProfile. A missing profile
// is returned as profileErr, not err.
func readProfileResult(results pgx.BatchResults) (profile *model.TrainingProfile, profileErr, err error) {
	profile, err = scanTrainingProfile(results.QueryRow())
	var notFound *model.NotFoundError
	if errors.As(err, &notFound) {
		return nil, err, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return profile, nil, nil
}

// ReadParticipation implements ParticipationReader.
func (r *participationDAO) ReadParticipation(ctx context.Context, userID uuid.UUID, from, to string) (*ParticipationRead, error) {
	batch := &pgx.Batch{}
	batch.Queue(selectTrainingProfile, userID)
	batch.Queue(`
		SELECT id, to_char(date, 'YYYY-MM-DD'), revision
		FROM scheduled_workouts
		WHERE user_id=$1 AND date BETWEEN $2 AND $3 AND finalized_at IS NULL
		ORDER BY date, created_at`, userID, from, to)
	batch.Queue(selectParticipation, userID, from, to)
	results := r.pool.SendBatch(ctx, batch)
	read, err := readParticipationResults(results)
	if closeErr := results.Close(); err == nil && closeErr != nil {
		err = fmt.Errorf("reading participation: %w", closeErr)
	}
	if err != nil {
		return nil, err
	}
	return read, nil
}

func readParticipationResults(results pgx.BatchResults) (*ParticipationRead, error) {
	read := &ParticipationRead{}
	var err error
	read.Profile, read.ProfileErr, err = readProfileResult(results)
	if err != nil {
		return nil, err
	}
	rows, err := results.Query()
	if err != nil {
		return nil, fmt.Errorf("querying unfinalized workouts: %w", err)
	}
	read.Unfinalized = []model.ScheduledWorkout{}
	for rows.Next() {
		var w model.ScheduledWorkout
		if err := rows.Scan(&w.ID, &w.Date, &w.Revision); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scanning unfinalized workout: %w", err)
		}
		read.Unfinalized = append(read.Unfinalized, w)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating unfinalized workouts: %w", err)
	}
	rows, err = results.Query()
	if err != nil {
		return nil, fmt.Errorf("querying participation: %w", err)
	}
	defer rows.Close()
	read.Days, err = scanParticipation(rows)
	if err != nil {
		return nil, err
	}
	return read, nil
}
