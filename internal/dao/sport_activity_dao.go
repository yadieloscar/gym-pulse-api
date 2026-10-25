package dao

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

const sportActivityLockNamespace = "sport-activity:"

const sportActivityColumns = `id, to_char(date, 'YYYY-MM-DD'), sport_id, sport_name,
		       duration_minutes, notes, source, external_id, distance_meters,
		       energy_kcal, avg_heart_rate_bpm, scheduled_workout_id, created_at, updated_at`

type SportActivityDAO interface {
	List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.SportActivity, error)
	Get(ctx context.Context, userID, activityID uuid.UUID) (*model.SportActivity, error)
	// Create records the activity atomically with participation and the
	// idempotency ledger and reports whether it created, replayed, or found an
	// already imported activity.
	Create(ctx context.Context, userID uuid.UUID, activity *model.SportActivity, timezone, operationKey, requestHash string) (*model.SportActivity, model.SportActivityCreateOutcome, error)
}

type sportActivityDAO struct{ pool *pgxpool.Pool }

func NewSportActivityDAO(pool *pgxpool.Pool) SportActivityDAO {
	return &sportActivityDAO{pool: pool}
}

func (r *sportActivityDAO) List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.SportActivity, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+sportActivityColumns+`
		FROM sport_activities
		WHERE user_id=$1 AND date BETWEEN $2 AND $3
		ORDER BY date DESC, created_at DESC, id DESC`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying sport activities: %w", err)
	}
	defer rows.Close()

	activities := []model.SportActivity{}
	for rows.Next() {
		var activity model.SportActivity
		if err := scanSportActivity(rows, &activity); err != nil {
			return nil, fmt.Errorf("scanning sport activity: %w", err)
		}
		activities = append(activities, activity)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating sport activities: %w", err)
	}
	return activities, nil
}

func (r *sportActivityDAO) Get(ctx context.Context, userID, activityID uuid.UUID) (*model.SportActivity, error) {
	activity := &model.SportActivity{}
	err := scanSportActivity(r.pool.QueryRow(ctx, `
		SELECT `+sportActivityColumns+`
		FROM sport_activities WHERE id=$1 AND user_id=$2`, activityID, userID), activity)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "sport activity not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("querying sport activity: %w", err)
	}
	return activity, nil
}

func (r *sportActivityDAO) Create(ctx context.Context, userID uuid.UUID, activity *model.SportActivity, timezone, operationKey, requestHash string) (*model.SportActivity, model.SportActivityCreateOutcome, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, model.SportActivityCreated, fmt.Errorf("beginning sport activity create: %w", err)
	}
	defer tx.Rollback(ctx)

	// Linking an activity updates scheduled_workouts, so the schedule domain
	// is locked first (program: -> schedule: -> sport-activity:).
	if err := lockUserDomain(ctx, tx, scheduleLockNamespace, userID); err != nil {
		return nil, model.SportActivityCreated, err
	}
	if err := lockUserDomain(ctx, tx, sportActivityLockNamespace, userID); err != nil {
		return nil, model.SportActivityCreated, err
	}
	var replay model.SportActivity
	if found, err := findIdempotency(ctx, tx, userID, "sport-activities/create", operationKey, requestHash, &replay); err != nil {
		return nil, model.SportActivityCreated, err
	} else if found {
		return &replay, model.SportActivityReplayed, nil
	}

	// A re-imported health workout returns the activity it already produced
	// and writes nothing, so participation is never credited twice.
	if activity.ExternalID != nil {
		existing := &model.SportActivity{}
		err := scanSportActivity(tx.QueryRow(ctx, `
			SELECT `+sportActivityColumns+`
			FROM sport_activities
			WHERE user_id=$1 AND source=$2 AND external_id=$3`, userID, activity.Source, *activity.ExternalID), existing)
		if err == nil {
			return existing, model.SportActivityExisting, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, model.SportActivityCreated, fmt.Errorf("querying imported sport activity: %w", err)
		}
	}

	// An explicit scheduled_workout_id records the activity against an owned
	// open sport session; an import without one fulfils the earliest open
	// session for the same sport and date, if any.
	session, linked, err := lockSportSession(ctx, tx, userID, activity)
	if err != nil {
		return nil, model.SportActivityCreated, err
	}

	if activity.ID == uuid.Nil {
		activity.ID = uuid.New()
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO sport_activities (
			id, user_id, date, sport_id, sport_name, duration_minutes, notes,
			source, external_id, distance_meters, energy_kcal, avg_heart_rate_bpm,
			scheduled_workout_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING created_at, updated_at`, activity.ID, userID, activity.Date,
		activity.SportID, activity.SportName, activity.DurationMinutes, activity.Notes,
		activity.Source, activity.ExternalID, activity.DistanceMeters, activity.EnergyKcal, activity.AvgHeartRateBPM,
		activity.ScheduledWorkoutID,
	).Scan(&activity.CreatedAt, &activity.UpdatedAt)
	if err != nil {
		return nil, model.SportActivityCreated, fmt.Errorf("inserting sport activity: %w", err)
	}

	if linked {
		// The linked activity completes the session. A skip is cleared because
		// the outcome is no longer missed; the scheduled date's participation
		// row is left as finalization wrote it.
		if _, err := tx.Exec(ctx, `
			UPDATE scheduled_workouts SET status='completed', finalized_at=COALESCE(finalized_at, now()),
			       resolution=NULL, resolved_at=NULL, revision=revision+1, updated_at=now()
			WHERE id=$1 AND user_id=$2`, session.ID, userID); err != nil {
			return nil, model.SportActivityCreated, fmt.Errorf("completing scheduled sport session: %w", err)
		}
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		INSERT INTO day_participation (
			user_id, date, scheduled_opportunity, participated, finalized_at, timezone, local_date)
		VALUES ($1,$2,false,true,$3,$4,$2)
		ON CONFLICT (user_id, date) DO UPDATE SET
			participated=true,
			scheduled_opportunity=day_participation.scheduled_opportunity OR EXCLUDED.scheduled_opportunity,
			revision=day_participation.revision+1`, userID, activity.Date, now, timezone); err != nil {
		return nil, model.SportActivityCreated, fmt.Errorf("preserving sport participation: %w", err)
	}

	record := model.IdempotencyRecord{
		Scope:          "sport-activities/create",
		OperationKey:   operationKey,
		RequestHash:    requestHash,
		ResponseStatus: 201,
		ResourceType:   "sport_activity",
		ResourceID:     &activity.ID,
	}
	if err := insertIdempotency(ctx, tx, userID, record, activity); err != nil {
		return nil, model.SportActivityCreated, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, model.SportActivityCreated, fmt.Errorf("committing sport activity create: %w", err)
	}
	return activity, model.SportActivityCreated, nil
}

func scanSportActivity(row rowScanner, activity *model.SportActivity) error {
	return row.Scan(&activity.ID, &activity.Date, &activity.SportID, &activity.SportName,
		&activity.DurationMinutes, &activity.Notes, &activity.Source, &activity.ExternalID,
		&activity.DistanceMeters, &activity.EnergyKcal, &activity.AvgHeartRateBPM,
		&activity.ScheduledWorkoutID, &activity.CreatedAt, &activity.UpdatedAt)
}

// sportSessionRow is the locked scheduled sport session an activity fulfils.
type sportSessionRow struct {
	ID      uuid.UUID
	Kind    model.UnitKind
	SportID *string
	Status  string
}

// lockSportSession resolves and locks the scheduled sport session the activity
// should complete. The boolean is false when the activity stays unlinked.
// Ownership, kind, sport, and completion failures map to the documented 404,
// 422, and 409 SPORT_SESSION_CONFLICT responses.
func lockSportSession(ctx context.Context, tx pgx.Tx, userID uuid.UUID, activity *model.SportActivity) (sportSessionRow, bool, error) {
	var row sportSessionRow
	switch {
	case activity.ScheduledWorkoutID != nil:
		err := tx.QueryRow(ctx, `
			SELECT id, kind, sport_id, status FROM scheduled_workouts
			WHERE id=$1 AND user_id=$2 FOR UPDATE`, *activity.ScheduledWorkoutID, userID).Scan(&row.ID, &row.Kind, &row.SportID, &row.Status)
		if errors.Is(err, pgx.ErrNoRows) {
			return row, false, &model.NotFoundError{Message: "scheduled workout not found"}
		}
		if err != nil {
			return row, false, fmt.Errorf("locking scheduled sport session: %w", err)
		}
		if model.NormalizeUnitKind(row.Kind) != model.UnitKindSport {
			return row, false, &model.ValidationError{Message: "scheduled_workout_id must reference a sport session", Field: "scheduled_workout_id"}
		}
		if row.SportID == nil || *row.SportID != activity.SportID {
			return row, false, &model.ValidationError{Message: "sport_id must match the planned sport", Field: "sport_id"}
		}
		if row.Status == model.WorkoutStatusCompleted {
			current, err := loadScheduledWorkout(ctx, tx, userID, row.ID)
			if err != nil {
				return row, false, err
			}
			return row, false, &model.SportSessionConflictError{Message: "sport session is already completed", Status: row.Status, Authoritative: current}
		}
		return row, true, nil
	case activity.Source != model.ActivitySourceManual:
		err := tx.QueryRow(ctx, `
			SELECT sw.id, sw.kind, sw.sport_id, sw.status FROM scheduled_workouts sw
			WHERE sw.user_id=$1 AND sw.date=$2 AND sw.kind='sport' AND sw.sport_id=$3
			  AND sw.status IN ('planned', 'missed')
			  AND NOT EXISTS (SELECT 1 FROM sport_activities sa WHERE sa.scheduled_workout_id = sw.id)
			ORDER BY sw.sequence_position NULLS LAST, sw.created_at, sw.id
			LIMIT 1 FOR UPDATE`, userID, activity.Date, activity.SportID).Scan(&row.ID, &row.Kind, &row.SportID, &row.Status)
		if errors.Is(err, pgx.ErrNoRows) {
			return row, false, nil
		}
		if err != nil {
			return row, false, fmt.Errorf("matching imported activity to a sport session: %w", err)
		}
		activity.ScheduledWorkoutID = &row.ID
		return row, true, nil
	default:
		return row, false, nil
	}
}
