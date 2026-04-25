package dao

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type StatsDAO interface {
	GetWeeklyCount(ctx context.Context, userID uuid.UUID, weekStart, weekEnd time.Time) (int, error)
	GetTotalWorkouts(ctx context.Context, userID uuid.UUID) (int, error)
	GetDistribution(ctx context.Context, userID uuid.UUID) ([]model.TypeDistribution, error)
}

type statsDAO struct {
	pool *pgxpool.Pool
}

func NewStatsDAO(pool *pgxpool.Pool) StatsDAO {
	return &statsDAO{pool: pool}
}

func (r *statsDAO) GetWeeklyCount(ctx context.Context, userID uuid.UUID, weekStart, weekEnd time.Time) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM day_logs
		WHERE user_id = $1 AND date BETWEEN $2 AND $3`,
		userID, weekStart, weekEnd,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting weekly logs: %w", err)
	}
	return count, nil
}

func (r *statsDAO) GetTotalWorkouts(ctx context.Context, userID uuid.UUID) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM day_logs WHERE user_id = $1`,
		userID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting total workouts: %w", err)
	}
	return count, nil
}

func (r *statsDAO) GetDistribution(ctx context.Context, userID uuid.UUID) ([]model.TypeDistribution, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT type_id, subtype_id, COUNT(*) AS count
		FROM day_logs
		WHERE user_id = $1
		GROUP BY type_id, subtype_id
		ORDER BY type_id, subtype_id`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying distribution: %w", err)
	}
	defer rows.Close()

	typeMap := make(map[string]*model.TypeDistribution)
	var typeOrder []string

	for rows.Next() {
		var typeID, subtypeID string
		var count int
		if err := rows.Scan(&typeID, &subtypeID, &count); err != nil {
			return nil, fmt.Errorf("scanning distribution row: %w", err)
		}

		td, ok := typeMap[typeID]
		if !ok {
			td = &model.TypeDistribution{
				TypeID:   typeID,
				Subtypes: []model.SubtypeDistribution{},
			}
			typeMap[typeID] = td
			typeOrder = append(typeOrder, typeID)
		}
		td.Count += count
		td.Subtypes = append(td.Subtypes, model.SubtypeDistribution{
			SubtypeID: subtypeID,
			Count:     count,
		})
	}

	result := make([]model.TypeDistribution, 0, len(typeOrder))
	for _, typeID := range typeOrder {
		result = append(result, *typeMap[typeID])
	}
	return result, rows.Err()
}
