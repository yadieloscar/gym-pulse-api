package dao

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// RollingScheduleDAO keeps the active program filled with sessions and moves
// single sessions. The schedule DAO implements it; services reach it by type
// assertion, like IdempotentScheduleDAO.
type RollingScheduleDAO interface {
	// EnsureFilled makes sure the athlete's active program has sessions from
	// today through today plus model.ScheduleHorizonDays. availableDays is
	// used only to place a program that has no weekdays yet.
	EnsureFilled(ctx context.Context, userID uuid.UUID, today string, availableDays []int) error
	// MoveIdempotent moves one planned, unstarted session to another date.
	MoveIdempotent(ctx context.Context, userID, workoutID uuid.UUID, date, today string, expectedRevision int64, record model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error)
}

// unstartedPlanned selects sessions that no operation has touched: planned,
// not finalized, and without an active or completed workout session. Starts,
// program saves, and removals may replace only these. The table alias is sw.
const unstartedPlanned = `sw.status='planned' AND sw.finalized_at IS NULL AND NOT EXISTS (SELECT 1 FROM workout_sessions ws WHERE ws.scheduled_workout_id=sw.id AND ws.status IN ('active','completed'))`

// shiftDate returns the YYYY-MM-DD date days after date.
func shiftDate(date string, days int) (string, error) {
	parsed, err := model.ParseDate(date)
	if err != nil {
		return "", fmt.Errorf("parsing date %q: %w", date, err)
	}
	return parsed.AddDate(0, 0, days).Format(time.DateOnly), nil
}

// scheduleHorizon returns the last date the fill covers when run on today.
func scheduleHorizon(today string) (string, error) {
	return shiftDate(today, model.ScheduleHorizonDays)
}

// insertLayoutSessions inserts program sessions that carry a slot date. A
// slot that already holds a session is left alone: the unique slot index makes
// the insert a no-op, so fills are idempotent and safe under concurrency. When
// loadExisting is set, the skipped entries are replaced by the stored session
// so the caller can return the authoritative rows.
func insertLayoutSessions(ctx context.Context, tx pgx.Tx, userID uuid.UUID, sessions []model.ScheduledWorkout, loadExisting bool) error {
	for i := range sessions {
		w := &sessions[i]
		w.Kind = model.NormalizeUnitKind(w.Kind)
		var sport scannedSport
		if w.Sport != nil {
			sport = sportColumns(&w.Sport.SportTarget)
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO scheduled_workouts (user_id, program_id, program_workout_id, date, slot_date, name, sequence_position, status,
				kind, sport_id, sport_name, target_duration_minutes, target_distance_meters, sport_notes)
			VALUES ($1,$2,$3,$4,$5,$6,$7,'planned',$8,$9,$10,$11,$12,$13)
			ON CONFLICT (program_workout_id, slot_date) WHERE program_workout_id IS NOT NULL AND slot_date IS NOT NULL DO NOTHING
			RETURNING id, revision, created_at, updated_at`, userID, w.ProgramID, w.ProgramWorkoutID,
			w.Date, w.SlotDate, w.Name, w.SequencePosition,
			w.Kind, sport.ID, sport.Name, sport.DurationMinutes, sport.DistanceMeters, sport.Notes).Scan(&w.ID, &w.Revision, &w.CreatedAt, &w.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			if !loadExisting {
				continue
			}
			var existingID uuid.UUID
			if err := tx.QueryRow(ctx, `SELECT id FROM scheduled_workouts WHERE user_id=$1 AND program_workout_id=$2 AND slot_date=$3`,
				userID, w.ProgramWorkoutID, w.SlotDate).Scan(&existingID); err != nil {
				return fmt.Errorf("finding the session that holds a slot: %w", err)
			}
			existing, err := loadScheduledWorkout(ctx, tx, userID, existingID)
			if err != nil {
				return err
			}
			sessions[i] = *existing
			continue
		}
		if err != nil {
			return fmt.Errorf("inserting program session: %w", err)
		}
		if err := insertScheduledSets(ctx, tx, w.ID, w.RequiredSets); err != nil {
			return err
		}
	}
	return nil
}

// storePlacement persists the weekdays a placement gave each unit. It does
// not change the program revision: placement is the server finishing a save,
// not an edit the client must reconcile.
func storePlacement(ctx context.Context, tx pgx.Tx, programID uuid.UUID, units []model.ProgramWorkout) error {
	for i := range units {
		unit := &units[i]
		if len(unit.Weekdays) == 0 {
			return fmt.Errorf("storing placement: unit %s has no weekdays", unit.ID)
		}
		if _, err := tx.Exec(ctx, `UPDATE program_workouts SET weekdays=$3, preferred_weekday=$4 WHERE id=$1 AND program_id=$2`,
			unit.ID, programID, weekdayColumn(unit.Weekdays), unit.Weekdays[0]); err != nil {
			return fmt.Errorf("storing unit weekdays: %w", err)
		}
	}
	return nil
}

// advanceScheduledThrough moves the program's fill watermark forward to
// through. The watermark never moves back.
func advanceScheduledThrough(ctx context.Context, tx pgx.Tx, programID uuid.UUID, through string) error {
	if _, err := tx.Exec(ctx, `UPDATE programs SET scheduled_through=GREATEST(COALESCE(scheduled_through, $2::date), $2::date) WHERE id=$1`, programID, through); err != nil {
		return fmt.Errorf("advancing schedule watermark: %w", err)
	}
	return nil
}

// fillProgram inserts the placed program's sessions for every slot from from
// through through and advances the watermark. Occupied slots are kept.
func fillProgram(ctx context.Context, tx pgx.Tx, userID uuid.UUID, p *model.Program, from, through string) error {
	sessions, err := model.LayoutSessions(p, from, through)
	if err != nil {
		return err
	}
	if err := insertLayoutSessions(ctx, tx, userID, sessions, false); err != nil {
		return err
	}
	return advanceScheduledThrough(ctx, tx, p.ID, through)
}

// insertPlacedSessions serves the materialize and regenerate operations of a
// released app. It stores a pending placement, inserts the layout sessions
// while keeping any slot that already holds one, replaces skipped entries of
// sessions with the stored row, and advances the watermark to through.
func insertPlacedSessions(ctx context.Context, tx pgx.Tx, userID, programID uuid.UUID, through string, placement []model.ProgramWorkout, sessions []model.ScheduledWorkout) error {
	if placement != nil {
		if err := storePlacement(ctx, tx, programID, placement); err != nil {
			return err
		}
	}
	if err := insertLayoutSessions(ctx, tx, userID, sessions, true); err != nil {
		return err
	}
	return advanceScheduledThrough(ctx, tx, programID, through)
}

// loadActiveProgram returns the athlete's active program, or nil when there
// is none.
func loadActiveProgram(ctx context.Context, tx pgx.Tx, userID uuid.UUID) (*model.Program, error) {
	var programID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM programs WHERE user_id=$1 AND active=true`, userID).Scan(&programID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finding active program: %w", err)
	}
	return loadProgram(ctx, tx, userID, programID)
}

// fillActiveProgram extends the active program's sessions through the
// horizon. It only ever adds dates after the watermark and never before
// today, so a session the athlete skipped, moved, or deleted is not created
// again and dates that passed unfilled stay empty. The caller holds the
// program and schedule locks.
func fillActiveProgram(ctx context.Context, tx pgx.Tx, userID uuid.UUID, today string, availableDays []int) error {
	p, err := loadActiveProgram(ctx, tx, userID)
	if err != nil || p == nil || len(p.Workouts) == 0 {
		return err
	}
	horizon, err := scheduleHorizon(today)
	if err != nil {
		return err
	}
	from := today
	if p.ScheduledThrough != nil {
		next, err := shiftDate(*p.ScheduledThrough, 1)
		if err != nil {
			return err
		}
		from = max(from, next)
	} else {
		// A program scheduled before the watermark existed continues after
		// its latest session, so nothing already on the calendar is doubled.
		var latest *string
		if err := tx.QueryRow(ctx, `SELECT to_char(max(date), 'YYYY-MM-DD') FROM scheduled_workouts WHERE user_id=$1 AND program_id=$2 AND date >= $3`,
			userID, p.ID, today).Scan(&latest); err != nil {
			return fmt.Errorf("finding the latest program session: %w", err)
		}
		if latest != nil {
			next, err := shiftDate(*latest, 1)
			if err != nil {
				return err
			}
			from = max(from, next)
		}
	}
	if from > horizon {
		return advanceScheduledThrough(ctx, tx, p.ID, horizon)
	}
	if !p.Placed() {
		if len(availableDays) == 0 {
			// Nothing to place on yet; the next start or read tries again.
			return nil
		}
		if err := model.PlaceProgram(p, availableDays, from); err != nil {
			return err
		}
		if err := storePlacement(ctx, tx, p.ID, p.Workouts); err != nil {
			return err
		}
	}
	return fillProgram(ctx, tx, userID, p, from, horizon)
}

// EnsureFilled implements RollingScheduleDAO. The common case, a program
// already filled through the horizon, costs one indexed single-row read.
func (r *scheduleDAO) EnsureFilled(ctx context.Context, userID uuid.UUID, today string, availableDays []int) error {
	horizon, err := scheduleHorizon(today)
	if err != nil {
		return err
	}
	var filled bool
	err = r.pool.QueryRow(ctx, `SELECT COALESCE(scheduled_through >= $2::date, false) FROM programs WHERE user_id=$1 AND active=true`, userID, horizon).Scan(&filled)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && filled) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking schedule watermark: %w", err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning schedule fill: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, programLockNamespace, userID); err != nil {
		return err
	}
	if err := lockUserDomain(ctx, tx, scheduleLockNamespace, userID); err != nil {
		return err
	}
	if err := fillActiveProgram(ctx, tx, userID, today, availableDays); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing schedule fill: %w", err)
	}
	return nil
}

// MoveIdempotent implements RollingScheduleDAO. Only the session's date and
// revision change; its slot date stays, so its original slot is never
// refilled. The day's participation is recomputed for both dates.
func (r *scheduleDAO) MoveIdempotent(ctx context.Context, userID, workoutID uuid.UUID, date, today string, expectedRevision int64, record model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("beginning session move: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, scheduleLockNamespace, userID); err != nil {
		return nil, false, err
	}
	var replay model.ScheduledWorkout
	if found, err := findIdempotency(ctx, tx, userID, record.Scope, record.OperationKey, record.RequestHash, &replay); err != nil {
		return nil, false, err
	} else if found {
		return &replay, true, nil
	}
	var (
		status, currentDate string
		finalized, started  bool
		revision            int64
	)
	err = tx.QueryRow(ctx, `
		SELECT sw.status, to_char(sw.date, 'YYYY-MM-DD'), sw.finalized_at IS NOT NULL, sw.revision,
		       EXISTS (SELECT 1 FROM workout_sessions ws WHERE ws.scheduled_workout_id=sw.id AND ws.status IN ('active','completed'))
		FROM scheduled_workouts sw WHERE sw.id=$1 AND sw.user_id=$2 FOR UPDATE OF sw`, workoutID, userID).Scan(&status, &currentDate, &finalized, &revision, &started)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, &model.NotFoundError{Message: "scheduled workout not found"}
	}
	if err != nil {
		return nil, false, fmt.Errorf("locking scheduled workout: %w", err)
	}
	if revision != expectedRevision || status != model.WorkoutStatusPlanned || finalized || started || currentDate < today {
		current, err := loadScheduledWorkout(ctx, tx, userID, workoutID)
		if err != nil {
			return nil, false, err
		}
		if revision != expectedRevision {
			return nil, false, &model.ConflictError{Message: "scheduled workout revision conflict", Expected: expectedRevision, Actual: revision, Authoritative: current}
		}
		return nil, false, &model.ConflictError{Message: "only a planned workout that has not started can be moved", Authoritative: current}
	}
	if _, err := tx.Exec(ctx, `UPDATE scheduled_workouts SET date=$3, revision=revision+1, updated_at=now() WHERE id=$1 AND user_id=$2`, workoutID, userID, date); err != nil {
		return nil, false, fmt.Errorf("moving scheduled workout: %w", err)
	}
	for _, day := range []string{currentDate, date} {
		if err := recomputeParticipation(ctx, tx, userID, day); err != nil {
			return nil, false, err
		}
	}
	moved, err := loadScheduledWorkout(ctx, tx, userID, workoutID)
	if err != nil {
		return nil, false, err
	}
	record.ResourceID, record.ResourceRevision = &moved.ID, &moved.Revision
	if err := insertIdempotency(ctx, tx, userID, record, moved); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing session move: %w", err)
	}
	return moved, false, nil
}
