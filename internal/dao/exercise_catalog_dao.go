package dao

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// ExerciseCatalogDAO defines read operations on the exercise_catalog table.
type ExerciseCatalogDAO interface {
	List(ctx context.Context, category string) ([]model.CatalogExercise, error)
}

type exerciseCatalogDAO struct {
	pool *pgxpool.Pool
}

// NewExerciseCatalogDAO creates a new ExerciseCatalogDAO backed by the given connection pool.
func NewExerciseCatalogDAO(pool *pgxpool.Pool) ExerciseCatalogDAO {
	return &exerciseCatalogDAO{pool: pool}
}

func (r *exerciseCatalogDAO) List(ctx context.Context, category string) ([]model.CatalogExercise, error) {
	query := `
		SELECT id, name, category, modality, mechanic, sort_order
		FROM exercise_catalog`
	args := []any{}
	if category != "" {
		query += ` WHERE category = $1`
		args = append(args, category)
	}
	query += ` ORDER BY category, sort_order`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying exercise catalog: %w", err)
	}
	defer rows.Close()

	exercises := []model.CatalogExercise{}
	for rows.Next() {
		var e model.CatalogExercise
		if err := rows.Scan(&e.ID, &e.Name, &e.Category, &e.Modality, &e.Mechanic, &e.SortOrder); err != nil {
			return nil, fmt.Errorf("scanning catalog exercise: %w", err)
		}
		exercises = append(exercises, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating catalog rows: %w", err)
	}
	return exercises, nil
}
