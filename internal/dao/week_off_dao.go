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

// WeekOffDAO owns the weeks_off table: the Monday-to-Sunday weeks an athlete
// took off.
type WeekOffDAO interface {
	// List returns the athlete's weeks off that start within from..to,
	// oldest first. The result is never nil.
	List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.WeekOff, error)
	// Timezone returns the athlete's saved training timezone, or "" when
	// they have no training profile.
	Timezone(ctx context.Context, userID uuid.UUID) (string, error)
	// Put records the week starting on weekStart as off and returns it. A
	// week that is already off is returned unchanged. In the same
	// transaction it marks that week's missed, unresolved workouts skipped,
	// so none of them waits for a choice.
	Put(ctx context.Context, userID uuid.UUID, weekStart string) (*model.WeekOff, error)
	// Delete removes the week off starting on weekStart. A week that is not
	// off is not an error. Workouts already skipped stay skipped.
	Delete(ctx context.Context, userID uuid.UUID, weekStart string) error
}

type weekOffDAO struct{ pool *pgxpool.Pool }

// NewWeekOffDAO returns a WeekOffDAO.
func NewWeekOffDAO(pool *pgxpool.Pool) WeekOffDAO {
	return &weekOffDAO{pool: pool}
}

func (r *weekOffDAO) List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.WeekOff, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT week_start::text, created_at
		FROM weeks_off
		WHERE user_id=$1 AND week_start BETWEEN $2::date AND $3::date
		ORDER BY week_start`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying weeks off: %w", err)
	}
	defer rows.Close()
	weeks := []model.WeekOff{}
	for rows.Next() {
		var week model.WeekOff
		if err := rows.Scan(&week.WeekStart, &week.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning week off: %w", err)
		}
		weeks = append(weeks, week)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading weeks off: %w", err)
	}
	return weeks, nil
}

func (r *weekOffDAO) Timezone(ctx context.Context, userID uuid.UUID) (string, error) {
	var timezone string
	err := r.pool.QueryRow(ctx, `SELECT timezone FROM training_profiles WHERE user_id=$1`, userID).Scan(&timezone)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("querying training timezone: %w", err)
	}
	return timezone, nil
}

func (r *weekOffDAO) Put(ctx context.Context, userID uuid.UUID, weekStart string) (*model.WeekOff, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning week off: %w", err)
	}
	defer tx.Rollback(ctx)

	// The schedule lock orders this against a lazy finalize of the same
	// week: whichever commits second sees the other's rows.
	if err := lockUserDomain(ctx, tx, scheduleLockNamespace, userID); err != nil {
		return nil, err
	}
	week := &model.WeekOff{}
	err = tx.QueryRow(ctx, `
		INSERT INTO weeks_off (user_id, week_start) VALUES ($1, $2::date)
		ON CONFLICT (user_id, week_start) DO UPDATE SET week_start=EXCLUDED.week_start
		RETURNING week_start::text, created_at`, userID, weekStart).Scan(&week.WeekStart, &week.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("saving week off: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE scheduled_workouts
		SET resolution=$4, resolved_at=now(), revision=revision+1, updated_at=now()
		WHERE user_id=$1 AND status=$3 AND resolution IS NULL
		  AND date BETWEEN $2::date AND $2::date + 6`,
		userID, weekStart, model.WorkoutStatusMissed, model.ResolutionSkipped); err != nil {
		return nil, fmt.Errorf("skipping the week's missed workouts: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing week off: %w", err)
	}
	return week, nil
}

func (r *weekOffDAO) Delete(ctx context.Context, userID uuid.UUID, weekStart string) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM weeks_off WHERE user_id=$1 AND week_start=$2::date`, userID, weekStart); err != nil {
		return fmt.Errorf("deleting week off: %w", err)
	}
	return nil
}
