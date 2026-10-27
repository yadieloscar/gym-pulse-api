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
		       energy_kcal, avg_heart_rate_bpm, perceived_effort, scheduled_workout_id, created_at, updated_at`

type SportActivityDAO interface {
	List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.SportActivity, error)
	Get(ctx context.Context, userID, activityID uuid.UUID) (*model.SportActivity, error)
	// Create records the activity atomically with participation and the
	// idempotency ledger and reports whether it created, replayed, or found an
	// already imported activity.
	Create(ctx context.Context, userID uuid.UUID, activity *model.SportActivity, timezone, operationKey, requestHash string) (*model.SportActivity, model.SportActivityCreateOutcome, error)
	// Link links an owned unlinked activity to an owned open sport session of
	// the same sport and completes the session; replayed reports an idempotent retry.
	Link(ctx context.Context, userID, activityID uuid.UUID, req model.LinkSportActivityRequest, requestHash string) (result *model.SportSessionLinkResult, replayed bool, err error)
	// Unlink clears the activity's session link; the session returns to planned
	// on or after localToday and to missed before it.
	Unlink(ctx context.Context, userID, activityID uuid.UUID, req model.UnlinkSportActivityRequest, localToday, timezone, requestHash string) (result *model.SportSessionLinkResult, replayed bool, err error)
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
			scheduled_workout_id, perceived_effort)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING created_at, updated_at`, activity.ID, userID, activity.Date,
		activity.SportID, activity.SportName, activity.DurationMinutes, activity.Notes,
		activity.Source, activity.ExternalID, activity.DistanceMeters, activity.EnergyKcal, activity.AvgHeartRateBPM,
		activity.ScheduledWorkoutID, activity.PerceivedEffort,
	).Scan(&activity.CreatedAt, &activity.UpdatedAt)
	if err != nil {
		return nil, model.SportActivityCreated, fmt.Errorf("inserting sport activity: %w", err)
	}

	if linked {
		// The linked activity completes the session. A skip is cleared because
		// the outcome is no longer missed; the scheduled date's participation
		// row is left as finalization wrote it.
		if err := completeSportSession(ctx, tx, userID, session.ID); err != nil {
			return nil, model.SportActivityCreated, err
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
		&activity.PerceivedEffort, &activity.ScheduledWorkoutID, &activity.CreatedAt, &activity.UpdatedAt)
}

// completeSportSession marks a linked session completed and clears any skip.
func completeSportSession(ctx context.Context, tx pgx.Tx, userID, sessionID uuid.UUID) error {
	if _, err := tx.Exec(ctx, `
		UPDATE scheduled_workouts SET status='completed', finalized_at=COALESCE(finalized_at, now()),
		       resolution=NULL, resolved_at=NULL, revision=revision+1, updated_at=now()
		WHERE id=$1 AND user_id=$2`, sessionID, userID); err != nil {
		return fmt.Errorf("completing scheduled sport session: %w", err)
	}
	return nil
}

// lockOwnedActivity locks one owned activity row for a link change.
func lockOwnedActivity(ctx context.Context, tx pgx.Tx, userID, activityID uuid.UUID) (*model.SportActivity, error) {
	activity := &model.SportActivity{}
	err := scanSportActivity(tx.QueryRow(ctx, `
		SELECT `+sportActivityColumns+`
		FROM sport_activities WHERE id=$1 AND user_id=$2 FOR UPDATE`, activityID, userID), activity)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "sport activity not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("locking sport activity: %w", err)
	}
	return activity, nil
}

// lockSessionForLinkChange locks an owned scheduled row and checks its revision.
func lockSessionForLinkChange(ctx context.Context, tx pgx.Tx, userID, sessionID uuid.UUID, expectedRevision int64) (*model.ScheduledWorkout, error) {
	var revision int64
	err := tx.QueryRow(ctx, `SELECT revision FROM scheduled_workouts WHERE id=$1 AND user_id=$2 FOR UPDATE`, sessionID, userID).Scan(&revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "scheduled workout not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("locking scheduled sport session: %w", err)
	}
	session, err := loadScheduledWorkout(ctx, tx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	if revision != expectedRevision {
		return nil, &model.ConflictError{Message: "scheduled workout revision conflict", Expected: expectedRevision, Actual: revision, Authoritative: session}
	}
	return session, nil
}

func linkResult(ctx context.Context, tx pgx.Tx, userID, activityID, sessionID uuid.UUID) (*model.SportSessionLinkResult, error) {
	activity := &model.SportActivity{}
	if err := scanSportActivity(tx.QueryRow(ctx, `SELECT `+sportActivityColumns+` FROM sport_activities WHERE id=$1 AND user_id=$2`, activityID, userID), activity); err != nil {
		return nil, fmt.Errorf("reloading sport activity: %w", err)
	}
	session, err := loadScheduledWorkout(ctx, tx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	return &model.SportSessionLinkResult{SportActivity: *activity, ScheduledWorkout: *session}, nil
}

func (r *sportActivityDAO) Link(ctx context.Context, userID, activityID uuid.UUID, req model.LinkSportActivityRequest, requestHash string) (*model.SportSessionLinkResult, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("beginning sport activity link: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, scheduleLockNamespace, userID); err != nil {
		return nil, false, err
	}
	if err := lockUserDomain(ctx, tx, sportActivityLockNamespace, userID); err != nil {
		return nil, false, err
	}
	var replay model.SportSessionLinkResult
	if found, err := findIdempotency(ctx, tx, userID, "sport-activities/link", req.OperationKey, requestHash, &replay); err != nil {
		return nil, false, err
	} else if found {
		return &replay, true, nil
	}
	activity, err := lockOwnedActivity(ctx, tx, userID, activityID)
	if err != nil {
		return nil, false, err
	}
	session, err := lockSessionForLinkChange(ctx, tx, userID, req.ScheduledWorkoutID, req.ExpectedRevision)
	if err != nil {
		return nil, false, err
	}
	if !session.IsSport() || session.Sport == nil {
		return nil, false, &model.ValidationError{Message: "scheduled_workout_id must reference a sport session", Field: "kind"}
	}
	if session.Sport.SportID != activity.SportID {
		return nil, false, &model.ValidationError{Message: "sport_id must match the planned sport", Field: "sport_id"}
	}
	if activity.ScheduledWorkoutID != nil {
		return nil, false, &model.SportSessionConflictError{Message: "sport activity is already linked to a session", Status: session.Status, Reason: "activity_linked", Authoritative: session}
	}
	if session.Status == model.WorkoutStatusCompleted || session.Sport.SportActivityID != nil {
		return nil, false, &model.SportSessionConflictError{Message: "sport session is already completed", Status: session.Status, Reason: "session_completed", Authoritative: session}
	}
	// The activity already credited its own date, so linking writes no participation.
	if _, err := tx.Exec(ctx, `UPDATE sport_activities SET scheduled_workout_id=$3, updated_at=now() WHERE id=$1 AND user_id=$2`, activityID, userID, session.ID); err != nil {
		return nil, false, fmt.Errorf("linking sport activity: %w", err)
	}
	if err := completeSportSession(ctx, tx, userID, session.ID); err != nil {
		return nil, false, err
	}
	result, err := linkResult(ctx, tx, userID, activityID, session.ID)
	if err != nil {
		return nil, false, err
	}
	record := model.IdempotencyRecord{Scope: "sport-activities/link", OperationKey: req.OperationKey, RequestHash: requestHash, ResponseStatus: 200, ResourceType: "scheduled_workout", ResourceID: &session.ID, ResourceRevision: &result.ScheduledWorkout.Revision}
	if err := insertIdempotency(ctx, tx, userID, record, result); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing sport activity link: %w", err)
	}
	return result, false, nil
}

func (r *sportActivityDAO) Unlink(ctx context.Context, userID, activityID uuid.UUID, req model.UnlinkSportActivityRequest, localToday, timezone, requestHash string) (*model.SportSessionLinkResult, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("beginning sport activity unlink: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, scheduleLockNamespace, userID); err != nil {
		return nil, false, err
	}
	if err := lockUserDomain(ctx, tx, sportActivityLockNamespace, userID); err != nil {
		return nil, false, err
	}
	var replay model.SportSessionLinkResult
	if found, err := findIdempotency(ctx, tx, userID, "sport-activities/unlink", req.OperationKey, requestHash, &replay); err != nil {
		return nil, false, err
	} else if found {
		return &replay, true, nil
	}
	activity, err := lockOwnedActivity(ctx, tx, userID, activityID)
	if err != nil {
		return nil, false, err
	}
	if activity.ScheduledWorkoutID == nil {
		return nil, false, &model.SportSessionConflictError{Message: "sport activity is not linked to a session", Reason: "not_linked"}
	}
	session, err := lockSessionForLinkChange(ctx, tx, userID, *activity.ScheduledWorkoutID, req.ExpectedRevision)
	if err != nil {
		return nil, false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE sport_activities SET scheduled_workout_id=NULL, updated_at=now() WHERE id=$1 AND user_id=$2`, activityID, userID); err != nil {
		return nil, false, fmt.Errorf("unlinking sport activity: %w", err)
	}
	// Today or later reopens the session; an earlier date is missed again and
	// stays finalized. The activity and its own date's credit are unchanged.
	var status string
	if err := tx.QueryRow(ctx, `
		UPDATE scheduled_workouts SET
		       status=CASE WHEN date >= $3::date THEN 'planned' ELSE 'missed' END,
		       finalized_at=CASE WHEN date >= $3::date THEN NULL ELSE COALESCE(finalized_at, now()) END,
		       resolution=NULL, resolved_at=NULL, revision=revision+1, updated_at=now()
		WHERE id=$1 AND user_id=$2
		RETURNING status`, session.ID, userID, localToday).Scan(&status); err != nil {
		return nil, false, fmt.Errorf("reopening scheduled sport session: %w", err)
	}
	if status == model.WorkoutStatusMissed {
		if _, err := tx.Exec(ctx, `
			INSERT INTO day_participation (
				user_id, date, scheduled_opportunity, participated, finalized_at, timezone, local_date)
			VALUES ($1,$2,true,false,now(),$3,$2)
			ON CONFLICT (user_id, date) DO UPDATE SET
				scheduled_opportunity=true,
				revision=day_participation.revision+1`, userID, session.Date, timezone); err != nil {
			return nil, false, fmt.Errorf("recording missed sport opportunity: %w", err)
		}
	}
	result, err := linkResult(ctx, tx, userID, activityID, session.ID)
	if err != nil {
		return nil, false, err
	}
	record := model.IdempotencyRecord{Scope: "sport-activities/unlink", OperationKey: req.OperationKey, RequestHash: requestHash, ResponseStatus: 200, ResourceType: "scheduled_workout", ResourceID: &session.ID, ResourceRevision: &result.ScheduledWorkout.Revision}
	if err := insertIdempotency(ctx, tx, userID, record, result); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing sport activity unlink: %w", err)
	}
	return result, false, nil
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
			  -- An import shorter than a third of the plan (a commute) stays unlinked.
			  AND sw.target_duration_minutes <= 3 * $4
			ORDER BY abs(sw.target_duration_minutes - $4), sw.sequence_position NULLS LAST, sw.created_at, sw.id
			LIMIT 1 FOR UPDATE`, userID, activity.Date, activity.SportID, activity.DurationMinutes).Scan(&row.ID, &row.Kind, &row.SportID, &row.Status)
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
