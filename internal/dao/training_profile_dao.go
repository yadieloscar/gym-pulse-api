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

type TrainingProfileDAO interface {
	Get(ctx context.Context, userID uuid.UUID) (*model.TrainingProfile, error)
	Put(ctx context.Context, userID uuid.UUID, profile *model.TrainingProfile, expectedRevision int64) error
}

type trainingProfileDAO struct {
	pool *pgxpool.Pool
}

func NewTrainingProfileDAO(pool *pgxpool.Pool) TrainingProfileDAO {
	return &trainingProfileDAO{pool: pool}
}

func (r *trainingProfileDAO) Get(ctx context.Context, userID uuid.UUID) (*model.TrainingProfile, error) {
	p := &model.TrainingProfile{}
	err := r.pool.QueryRow(ctx, `
		SELECT primary_goal, available_days, usual_activity, experience,
		       equipment, session_duration_minutes, timezone, preferences,
		       revision, created_at, updated_at
		FROM training_profiles
		WHERE user_id = $1`, userID).Scan(
		&p.PrimaryGoal, &p.AvailableDays, &p.UsualActivity, &p.Experience,
		&p.Equipment, &p.SessionDurationMinutes, &p.Timezone, &p.Preferences,
		&p.Revision, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "training profile not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("querying training profile: %w", err)
	}
	return p, nil
}

// Put creates at expectedRevision=0 or performs a compare-and-swap update. The
// revision increment is part of the SQL write, so concurrent clients cannot
// silently overwrite one another.
func (r *trainingProfileDAO) Put(ctx context.Context, userID uuid.UUID, p *model.TrainingProfile, expectedRevision int64) error {
	if expectedRevision == 0 {
		err := r.pool.QueryRow(ctx, `
			INSERT INTO training_profiles (
				user_id, primary_goal, available_days, usual_activity, experience,
				equipment, session_duration_minutes, timezone, preferences)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			RETURNING revision, created_at, updated_at`,
			userID, p.PrimaryGoal, p.AvailableDays, p.UsualActivity, p.Experience,
			p.Equipment, p.SessionDurationMinutes, p.Timezone, p.Preferences,
		).Scan(&p.Revision, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			return fmt.Errorf("creating training profile: %w", err)
		}
		return nil
	}

	err := r.pool.QueryRow(ctx, `
		UPDATE training_profiles
		SET primary_goal=$3, available_days=$4, usual_activity=$5, experience=$6,
		    equipment=$7, session_duration_minutes=$8, timezone=$9,
		    preferences=$10, revision=revision+1, updated_at=now()
		WHERE user_id=$1 AND revision=$2
		RETURNING revision, created_at, updated_at`,
		userID, expectedRevision, p.PrimaryGoal, p.AvailableDays, p.UsualActivity,
		p.Experience, p.Equipment, p.SessionDurationMinutes, p.Timezone, p.Preferences,
	).Scan(&p.Revision, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		current, getErr := r.Get(ctx, userID)
		if getErr != nil {
			return getErr
		}
		return &model.ConflictError{
			Message:       "training profile revision conflict",
			Expected:      expectedRevision,
			Actual:        current.Revision,
			Authoritative: current,
		}
	}
	if err != nil {
		return fmt.Errorf("updating training profile: %w", err)
	}
	return nil
}
