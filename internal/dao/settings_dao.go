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

type SettingsDAO interface {
	Get(ctx context.Context, userID uuid.UUID) (*model.UserSettings, error)
	Upsert(ctx context.Context, userID uuid.UUID, settings *model.UserSettings) error
	Patch(ctx context.Context, userID uuid.UUID, req model.UpdateUserSettingsRequest) (*model.UserSettings, error)
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
		SELECT weight_unit, weekly_goal, palette
		FROM user_settings
		WHERE user_id = $1`,
		userID,
	).Scan(&s.WeightUnit, &s.WeeklyGoal, &s.Palette)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			defaults := model.DefaultUserSettings()
			return &defaults, nil
		}
		return nil, fmt.Errorf("querying user settings: %w", err)
	}
	return s, nil
}

func (r *settingsDAO) Upsert(ctx context.Context, userID uuid.UUID, settings *model.UserSettings) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_settings (user_id, weight_unit, weekly_goal, palette)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE
		SET weight_unit = EXCLUDED.weight_unit,
		    weekly_goal = EXCLUDED.weekly_goal,
		    palette = EXCLUDED.palette,
		    updated_at = now()`,
		userID, settings.WeightUnit, settings.WeeklyGoal, settings.Palette,
	)
	if err != nil {
		return fmt.Errorf("upserting user settings: %w", err)
	}
	return nil
}

// Patch applies only the fields present in req in one database statement. This
// prevents concurrent partial updates from reverting fields changed by another
// client between a read and a write.
func (r *settingsDAO) Patch(ctx context.Context, userID uuid.UUID, req model.UpdateUserSettingsRequest) (*model.UserSettings, error) {
	defaults := model.DefaultUserSettings()
	settings := &model.UserSettings{}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO user_settings (user_id, weight_unit, weekly_goal, palette)
		VALUES (
		    $1,
		    COALESCE($2::text, $5::text),
		    COALESCE($3::integer, $6::integer),
		    COALESCE($4::text, $7::text)
		)
		ON CONFLICT (user_id) DO UPDATE
		SET weight_unit = COALESCE($2::text, user_settings.weight_unit),
		    weekly_goal = COALESCE($3::integer, user_settings.weekly_goal),
		    palette = COALESCE($4::text, user_settings.palette),
		    updated_at = now()
		RETURNING weight_unit, weekly_goal, palette`,
		userID, req.WeightUnit, req.WeeklyGoal, req.Palette,
		defaults.WeightUnit, defaults.WeeklyGoal, defaults.Palette,
	).Scan(&settings.WeightUnit, &settings.WeeklyGoal, &settings.Palette)
	if err != nil {
		return nil, fmt.Errorf("patching user settings: %w", err)
	}
	return settings, nil
}
