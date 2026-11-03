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

const mobilitySessionColumns = `id, to_char(date, 'YYYY-MM-DD'), routine_id, routine_name, context,
		       duration_seconds, moves_completed, moves_total, content_version, operation_key, created_at`

// MobilitySessionDAO persists finished stretching routines. It reads and
// writes only mobility_sessions.
type MobilitySessionDAO interface {
	List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.MobilitySession, error)
	Get(ctx context.Context, userID, sessionID uuid.UUID) (*model.MobilitySession, error)
	// Create inserts the entry unless the athlete already used its operation
	// key. An existing key with the same request hash replays the stored
	// entry; a different hash is an idempotency conflict.
	Create(ctx context.Context, userID uuid.UUID, session *model.MobilitySession, requestHash string) (*model.MobilitySession, model.MobilitySessionCreateOutcome, error)
	// Delete removes an owned entry; a missing or foreign id is not found.
	Delete(ctx context.Context, userID, sessionID uuid.UUID) error
}

type mobilitySessionDAO struct{ pool *pgxpool.Pool }

// NewMobilitySessionDAO returns a MobilitySessionDAO backed by pool.
func NewMobilitySessionDAO(pool *pgxpool.Pool) MobilitySessionDAO {
	return &mobilitySessionDAO{pool: pool}
}

func (r *mobilitySessionDAO) List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.MobilitySession, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+mobilitySessionColumns+`
		FROM mobility_sessions
		WHERE user_id=$1 AND date BETWEEN $2 AND $3
		ORDER BY date DESC, created_at DESC, id DESC`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying mobility sessions: %w", err)
	}
	defer rows.Close()

	sessions := []model.MobilitySession{}
	for rows.Next() {
		var session model.MobilitySession
		if err := scanMobilitySession(rows, &session); err != nil {
			return nil, fmt.Errorf("scanning mobility session: %w", err)
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating mobility sessions: %w", err)
	}
	return sessions, nil
}

func (r *mobilitySessionDAO) Get(ctx context.Context, userID, sessionID uuid.UUID) (*model.MobilitySession, error) {
	session := &model.MobilitySession{}
	err := scanMobilitySession(r.pool.QueryRow(ctx, `
		SELECT `+mobilitySessionColumns+`
		FROM mobility_sessions WHERE id=$1 AND user_id=$2`, sessionID, userID), session)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "mobility session not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("querying mobility session: %w", err)
	}
	return session, nil
}

func (r *mobilitySessionDAO) Create(ctx context.Context, userID uuid.UUID, session *model.MobilitySession, requestHash string) (*model.MobilitySession, model.MobilitySessionCreateOutcome, error) {
	if session.ID == uuid.Nil {
		session.ID = uuid.New()
	}
	// The unique (user_id, operation_key) index serializes concurrent retries:
	// exactly one insert wins and every other attempt reads the winner.
	created := &model.MobilitySession{}
	err := scanMobilitySession(r.pool.QueryRow(ctx, `
		INSERT INTO mobility_sessions (
			id, user_id, date, routine_id, routine_name, context, duration_seconds,
			moves_completed, moves_total, content_version, operation_key, request_hash)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (user_id, operation_key) DO NOTHING
		RETURNING `+mobilitySessionColumns, session.ID, userID, session.Date, session.RoutineID,
		session.RoutineName, session.Context, session.DurationSeconds, session.MovesCompleted,
		session.MovesTotal, session.ContentVersion, session.OperationKey, requestHash), created)
	if err == nil {
		return created, model.MobilitySessionCreated, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, model.MobilitySessionCreated, fmt.Errorf("inserting mobility session: %w", err)
	}

	existing := &model.MobilitySession{}
	var storedHash string
	err = r.pool.QueryRow(ctx, `
		SELECT `+mobilitySessionColumns+`, request_hash
		FROM mobility_sessions WHERE user_id=$1 AND operation_key=$2`, userID, session.OperationKey).Scan(
		&existing.ID, &existing.Date, &existing.RoutineID, &existing.RoutineName, &existing.Context,
		&existing.DurationSeconds, &existing.MovesCompleted, &existing.MovesTotal, &existing.ContentVersion,
		&existing.OperationKey, &existing.CreatedAt, &storedHash)
	if errors.Is(err, pgx.ErrNoRows) {
		// The conflicting row was deleted between the insert and this read.
		return nil, model.MobilitySessionCreated, &model.ConflictError{Message: "idempotency key is in use by a concurrent request"}
	}
	if err != nil {
		return nil, model.MobilitySessionCreated, fmt.Errorf("querying mobility session replay: %w", err)
	}
	if storedHash != requestHash {
		return nil, model.MobilitySessionCreated, &model.ConflictError{Message: "idempotency key was already used with a different payload"}
	}
	return existing, model.MobilitySessionReplayed, nil
}

func (r *mobilitySessionDAO) Delete(ctx context.Context, userID, sessionID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM mobility_sessions WHERE id=$1 AND user_id=$2`, sessionID, userID)
	if err != nil {
		return fmt.Errorf("deleting mobility session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &model.NotFoundError{Message: "mobility session not found"}
	}
	return nil
}

func scanMobilitySession(row rowScanner, session *model.MobilitySession) error {
	return row.Scan(&session.ID, &session.Date, &session.RoutineID, &session.RoutineName, &session.Context,
		&session.DurationSeconds, &session.MovesCompleted, &session.MovesTotal, &session.ContentVersion,
		&session.OperationKey, &session.CreatedAt)
}
