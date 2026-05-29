package dao

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// BodyWeightDAO defines operations on the body_weights table.
type BodyWeightDAO interface {
	Upsert(ctx context.Context, userID uuid.UUID, w *model.BodyWeight) (*model.BodyWeight, error)
	List(ctx context.Context, userID uuid.UUID) ([]model.BodyWeight, error)
	Delete(ctx context.Context, userID uuid.UUID, entryID uuid.UUID) error
}

type bodyWeightDAO struct {
	pool *pgxpool.Pool
}

// NewBodyWeightDAO creates a new BodyWeightDAO backed by the given connection pool.
func NewBodyWeightDAO(pool *pgxpool.Pool) BodyWeightDAO {
	return &bodyWeightDAO{pool: pool}
}

func (r *bodyWeightDAO) Upsert(ctx context.Context, userID uuid.UUID, w *model.BodyWeight) (*model.BodyWeight, error) {
	result := &model.BodyWeight{}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO body_weights (user_id, date, weight, unit)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, date) DO UPDATE
		SET weight = EXCLUDED.weight,
		    unit = EXCLUDED.unit,
		    logged_at = now()
		RETURNING id, user_id, date, weight, unit, logged_at`,
		userID, w.Date, w.Weight, w.Unit,
	).Scan(&result.ID, &result.UserID, &result.Date, &result.Weight, &result.Unit, &result.LoggedAt)
	if err != nil {
		return nil, fmt.Errorf("upserting body weight: %w", err)
	}
	return result, nil
}

func (r *bodyWeightDAO) List(ctx context.Context, userID uuid.UUID) ([]model.BodyWeight, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, date, weight, unit, logged_at
		FROM body_weights
		WHERE user_id = $1
		ORDER BY date DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying body weights: %w", err)
	}
	defer rows.Close()

	results := []model.BodyWeight{}
	for rows.Next() {
		var bw model.BodyWeight
		if err := rows.Scan(&bw.ID, &bw.UserID, &bw.Date, &bw.Weight, &bw.Unit, &bw.LoggedAt); err != nil {
			return nil, fmt.Errorf("scanning body weight row: %w", err)
		}
		results = append(results, bw)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating body weight rows: %w", err)
	}
	return results, nil
}

func (r *bodyWeightDAO) Delete(ctx context.Context, userID uuid.UUID, entryID uuid.UUID) error {
	ct, err := r.pool.Exec(ctx, `
		DELETE FROM body_weights
		WHERE id = $1 AND user_id = $2`,
		entryID, userID,
	)
	if err != nil {
		return fmt.Errorf("deleting body weight: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return &model.NotFoundError{Message: "body weight entry not found"}
	}
	return nil
}
