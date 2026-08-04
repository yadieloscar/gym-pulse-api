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

type SportActivityDAO interface {
	List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.SportActivity, error)
	Get(ctx context.Context, userID, activityID uuid.UUID) (*model.SportActivity, error)
	Create(ctx context.Context, userID uuid.UUID, activity *model.SportActivity, timezone, operationKey, requestHash string) (*model.SportActivity, bool, error)
}

type sportActivityDAO struct{ pool *pgxpool.Pool }

func NewSportActivityDAO(pool *pgxpool.Pool) SportActivityDAO {
	return &sportActivityDAO{pool: pool}
}

func (r *sportActivityDAO) List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.SportActivity, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, to_char(date, 'YYYY-MM-DD'), sport_id, sport_name,
		       duration_minutes, notes, created_at, updated_at
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
		SELECT id, to_char(date, 'YYYY-MM-DD'), sport_id, sport_name,
		       duration_minutes, notes, created_at, updated_at
		FROM sport_activities WHERE id=$1 AND user_id=$2`, activityID, userID), activity)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "sport activity not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("querying sport activity: %w", err)
	}
	return activity, nil
}

func (r *sportActivityDAO) Create(ctx context.Context, userID uuid.UUID, activity *model.SportActivity, timezone, operationKey, requestHash string) (*model.SportActivity, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("beginning sport activity create: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := lockUserDomain(ctx, tx, sportActivityLockNamespace, userID); err != nil {
		return nil, false, err
	}
	var replay model.SportActivity
	if found, err := findIdempotency(ctx, tx, userID, "sport-activities/create", operationKey, requestHash, &replay); err != nil {
		return nil, false, err
	} else if found {
		return &replay, true, nil
	}

	if activity.ID == uuid.Nil {
		activity.ID = uuid.New()
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO sport_activities (
			id, user_id, date, sport_id, sport_name, duration_minutes, notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING created_at, updated_at`, activity.ID, userID, activity.Date,
		activity.SportID, activity.SportName, activity.DurationMinutes, activity.Notes,
	).Scan(&activity.CreatedAt, &activity.UpdatedAt)
	if err != nil {
		return nil, false, fmt.Errorf("inserting sport activity: %w", err)
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
		return nil, false, fmt.Errorf("preserving sport participation: %w", err)
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
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing sport activity create: %w", err)
	}
	return activity, false, nil
}

func scanSportActivity(row rowScanner, activity *model.SportActivity) error {
	return row.Scan(&activity.ID, &activity.Date, &activity.SportID, &activity.SportName,
		&activity.DurationMinutes, &activity.Notes, &activity.CreatedAt, &activity.UpdatedAt)
}
