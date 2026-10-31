package dao

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

const programGenerationLockNamespace = "program-generation:"

// ProgramGenerationReservation is one reserved slot in the athlete's day.
type ProgramGenerationReservation struct {
	ID   uuid.UUID
	Used int // succeeded or live pending rows for the day, including this one
}

// ProgramGenerationDAO owns the daily-limit ledger. It never stores drafts.
type ProgramGenerationDAO interface {
	// Reserve counts the day's live attempts under a per-athlete advisory
	// lock and inserts a pending row when below the limit; otherwise it
	// returns GenerationLimitError (ResetsAt left for the caller).
	Reserve(ctx context.Context, userID uuid.UUID, localDate string, limit int, modelID string) (*ProgramGenerationReservation, error)
	// Complete marks a reservation succeeded or failed.
	Complete(ctx context.Context, id uuid.UUID, status string) error
	// CountToday reports the day's live attempts for the status read.
	CountToday(ctx context.Context, userID uuid.UUID, localDate string) (int, error)
}

type programGenerationDAO struct {
	pool *pgxpool.Pool
	// pendingWindow is how long a pending reservation counts. It must exceed
	// the provider deadline so an in-flight call always holds its slot; a
	// reservation whose call never completed stops counting after it.
	pendingWindow string
}

// NewProgramGenerationDAO builds the ledger. pendingWindow should be the
// generation timeout plus a margin.
func NewProgramGenerationDAO(pool *pgxpool.Pool, pendingWindow time.Duration) ProgramGenerationDAO {
	return &programGenerationDAO{pool: pool, pendingWindow: fmt.Sprintf("%d seconds", int(pendingWindow.Seconds()))}
}

// liveGenerationCountQuery counts succeeded rows plus pending rows younger
// than the pending window; failed and stale pending rows never charge the
// athlete.
const liveGenerationCountQuery = `
		SELECT count(*) FROM program_generations
		WHERE user_id=$1 AND local_date=$2
		  AND (status = 'succeeded' OR (status = 'pending' AND created_at > now() - $3::interval))`

func (r *programGenerationDAO) Reserve(ctx context.Context, userID uuid.UUID, localDate string, limit int, modelID string) (*ProgramGenerationReservation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning program generation reservation: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := lockUserDomain(ctx, tx, programGenerationLockNamespace, userID); err != nil {
		return nil, err
	}
	var used int
	if err := tx.QueryRow(ctx, liveGenerationCountQuery, userID, localDate, r.pendingWindow).Scan(&used); err != nil {
		return nil, fmt.Errorf("counting program generations: %w", err)
	}
	if used >= limit {
		return nil, &model.GenerationLimitError{Limit: limit}
	}
	reservation := &ProgramGenerationReservation{ID: uuid.New(), Used: used + 1}
	if _, err := tx.Exec(ctx, `
		INSERT INTO program_generations (id, user_id, local_date, status, model)
		VALUES ($1, $2, $3, 'pending', $4)`, reservation.ID, userID, localDate, modelID); err != nil {
		return nil, fmt.Errorf("inserting program generation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing program generation reservation: %w", err)
	}
	return reservation, nil
}

func (r *programGenerationDAO) Complete(ctx context.Context, id uuid.UUID, status string) error {
	if _, err := r.pool.Exec(ctx, `
		UPDATE program_generations SET status=$2, completed_at=now()
		WHERE id=$1 AND status = 'pending'`, id, status); err != nil {
		return fmt.Errorf("completing program generation: %w", err)
	}
	return nil
}

func (r *programGenerationDAO) CountToday(ctx context.Context, userID uuid.UUID, localDate string) (int, error) {
	var used int
	if err := r.pool.QueryRow(ctx, liveGenerationCountQuery, userID, localDate, r.pendingWindow).Scan(&used); err != nil {
		return 0, fmt.Errorf("counting program generations: %w", err)
	}
	return used, nil
}
