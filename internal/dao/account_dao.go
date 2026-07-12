package dao

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AccountDAO interface {
	// DeleteUserData removes every row belonging to the user in one
	// transaction. Child tables (exercises, set_logs, exercise_overrides,
	// user_profiles) cascade from these roots.
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
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	// day_logs before workout_templates: set_logs cascade from both sides.
	// auth.users last: user_profiles cascades from it.
	statements := []string{
		`DELETE FROM day_logs WHERE user_id = $1`,
		`DELETE FROM plan_overrides WHERE user_id = $1`,
		`DELETE FROM weekly_plans WHERE user_id = $1`,
		`DELETE FROM workout_templates WHERE user_id = $1`,
		`DELETE FROM user_settings WHERE user_id = $1`,
		`DELETE FROM body_weights WHERE user_id = $1`,
		`DELETE FROM auth.users WHERE id = $1`,
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
