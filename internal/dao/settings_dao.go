package dao

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type SettingsDAO interface {
	Get(ctx context.Context, userID uuid.UUID) (*model.UserSettings, error)
	Upsert(ctx context.Context, userID uuid.UUID, settings *model.UserSettings) error
}

type settingsDAO struct {
	pool *pgxpool.Pool
}

func NewSettingsDAO(pool *pgxpool.Pool) SettingsDAO {
	return &settingsDAO{pool: pool}
}

func (r *settingsDAO) Get(ctx context.Context, userID uuid.UUID) (*model.UserSettings, error) {
	s := &model.UserSettings{}
	err := r.pool.QueryRow(ctx, `
		SELECT weight_unit, weekly_goal
		FROM user_settings
		WHERE user_id = $1`,
		userID,
	).Scan(&s.WeightUnit, &s.WeeklyGoal)
	if err != nil {
		if err == pgx.ErrNoRows {
			defaults := model.DefaultUserSettings()
			return &defaults, nil
		}
		return nil, fmt.Errorf("querying user settings: %w", err)
	}
	return s, nil
}

func (r *settingsDAO) Upsert(ctx context.Context, userID uuid.UUID, settings *model.UserSettings) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_settings (user_id, weight_unit, weekly_goal)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE
		SET weight_unit = EXCLUDED.weight_unit,
		    weekly_goal = EXCLUDED.weekly_goal,
		    updated_at = now()`,
		userID, settings.WeightUnit, settings.WeeklyGoal,
	)
	if err != nil {
		return fmt.Errorf("upserting user settings: %w", err)
	}
	return nil
}
