package dao

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// PlanDAO persists the recurring weekly plan and per-date overrides.
type PlanDAO interface {
	GetWeekly(ctx context.Context, userID uuid.UUID) ([]model.WeeklyPlanDay, error)
	GetOverrides(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]model.PlanOverride, error)
	PutWeekly(ctx context.Context, userID uuid.UUID, days []model.WeeklyPlanDay) error
	UpsertOverride(ctx context.Context, userID uuid.UUID, date string, o model.PutPlanOverrideRequest) error
	DeleteOverride(ctx context.Context, userID uuid.UUID, date string) error
}

type planDAO struct {
	pool *pgxpool.Pool
}

// NewPlanDAO creates a new PlanDAO backed by the given connection pool.
func NewPlanDAO(pool *pgxpool.Pool) PlanDAO {
	return &planDAO{pool: pool}
}

func (r *planDAO) GetWeekly(ctx context.Context, userID uuid.UUID) ([]model.WeeklyPlanDay, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT weekday, template_id, rest
		FROM weekly_plans
		WHERE user_id = $1
		ORDER BY weekday`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying weekly plan: %w", err)
	}
	defer rows.Close()

	days := []model.WeeklyPlanDay{}
	for rows.Next() {
		var d model.WeeklyPlanDay
		if err := rows.Scan(&d.Weekday, &d.TemplateID, &d.Rest); err != nil {
			return nil, fmt.Errorf("scanning weekly plan day: %w", err)
		}
		days = append(days, d)
	}
	return days, rows.Err()
}

func (r *planDAO) GetOverrides(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]model.PlanOverride, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT to_char(date, 'YYYY-MM-DD'), template_id, rest
		FROM plan_overrides
		WHERE user_id = $1 AND date BETWEEN $2 AND $3
		ORDER BY date`,
		userID, from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("querying plan overrides: %w", err)
	}
	defer rows.Close()

	overrides := []model.PlanOverride{}
	for rows.Next() {
		var o model.PlanOverride
		if err := rows.Scan(&o.Date, &o.TemplateID, &o.Rest); err != nil {
			return nil, fmt.Errorf("scanning plan override: %w", err)
		}
		overrides = append(overrides, o)
	}
	return overrides, rows.Err()
}

func (r *planDAO) PutWeekly(ctx context.Context, userID uuid.UUID, days []model.WeeklyPlanDay) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM weekly_plans WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("clearing weekly plan: %w", err)
	}

	for _, d := range days {
		if _, err := tx.Exec(ctx, `
			INSERT INTO weekly_plans (user_id, weekday, template_id, rest)
			VALUES ($1, $2, $3, $4)`,
			userID, d.Weekday, d.TemplateID, d.Rest,
		); err != nil {
			return fmt.Errorf("inserting weekly plan day: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (r *planDAO) UpsertOverride(ctx context.Context, userID uuid.UUID, date string, o model.PutPlanOverrideRequest) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO plan_overrides (user_id, date, template_id, rest)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, date) DO UPDATE
		SET template_id = EXCLUDED.template_id, rest = EXCLUDED.rest`,
		userID, date, o.TemplateID, o.Rest,
	)
	if err != nil {
		return fmt.Errorf("upserting plan override: %w", err)
	}
	return nil
}

func (r *planDAO) DeleteOverride(ctx context.Context, userID uuid.UUID, date string) error {
	result, err := r.pool.Exec(ctx, `
		DELETE FROM plan_overrides WHERE user_id = $1 AND date = $2`,
		userID, date,
	)
	if err != nil {
		return fmt.Errorf("deleting plan override: %w", err)
	}
	if result.RowsAffected() == 0 {
		return &model.NotFoundError{Message: "plan override not found"}
	}
	return nil
}
