package dao

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AccountDAO interface {
	// DeleteUserData removes every row belonging to the user in one
	// transaction. Global catalog and starter-program rows remain. Auth
	// identity deletion is coordinated separately through the provider API.
	DeleteUserData(ctx context.Context, userID uuid.UUID) error
}

type accountDAO struct {
	pool *pgxpool.Pool
}

func NewAccountDAO(pool *pgxpool.Pool) AccountDAO {
	return &accountDAO{pool: pool}
}

func (r *accountDAO) DeleteUserData(ctx context.Context, userID uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning account delete: %w", err)
	}
	// Rollback after a successful commit is a no-op.
	defer tx.Rollback(ctx)

	// Delete relationship roots in dependency order. Child exercise, set,
	// workout, and program rows cascade from these roots.
	statements := []string{
		`DELETE FROM day_logs WHERE user_id = $1`,
		`DELETE FROM sport_activities WHERE user_id = $1`,
		`DELETE FROM workout_sessions WHERE user_id = $1`,
		`DELETE FROM scheduled_workouts WHERE user_id = $1`,
		`DELETE FROM legacy_adoptions WHERE user_id = $1`,
		`DELETE FROM programs WHERE user_id = $1`,
		`DELETE FROM training_profiles WHERE user_id = $1`,
		`DELETE FROM day_participation WHERE user_id = $1`,
		`DELETE FROM idempotency_records WHERE user_id = $1`,
		`DELETE FROM plan_overrides WHERE user_id = $1`,
		`DELETE FROM weekly_plans WHERE user_id = $1`,
		`DELETE FROM workout_templates WHERE user_id = $1`,
		`DELETE FROM user_settings WHERE user_id = $1`,
		`DELETE FROM body_weights WHERE user_id = $1`,
		`DELETE FROM user_profiles WHERE id = $1`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(ctx, stmt, userID); err != nil {
			return fmt.Errorf("deleting account data: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing account delete: %w", err)
	}
	return nil
}
