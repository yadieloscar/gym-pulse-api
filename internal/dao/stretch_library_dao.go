package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

const (
	stretchLibraryLockNamespace = "stretch-library:"
	stretchLibraryScope         = "stretch-library/put"
)

// StretchLibraryDAO owns the stretch_libraries table: one document per
// athlete holding the stretch moves and routines they wrote.
type StretchLibraryDAO interface {
	// Get returns the athlete's library, or the empty library at revision 0
	// when no row exists.
	Get(ctx context.Context, userID uuid.UUID) (*model.StretchLibrary, error)
	// Save replaces the whole library atomically with the idempotency
	// ledger. expectedRevision is the stored revision, 0 when no library
	// exists; a mismatch is a ConflictError carrying the current library.
	Save(ctx context.Context, userID uuid.UUID, moves []model.StretchMove, routines []model.StretchRoutine, expectedRevision int64, operationKey, requestHash string) (*model.StretchLibrary, error)
}

type stretchLibraryDAO struct{ pool *pgxpool.Pool }

// NewStretchLibraryDAO returns a StretchLibraryDAO.
func NewStretchLibraryDAO(pool *pgxpool.Pool) StretchLibraryDAO {
	return &stretchLibraryDAO{pool: pool}
}

// stretchLibraryContent is the stored JSONB document.
type stretchLibraryContent struct {
	Moves    []model.StretchMove    `json:"moves"`
	Routines []model.StretchRoutine `json:"routines"`
}

func (r *stretchLibraryDAO) Get(ctx context.Context, userID uuid.UUID) (*model.StretchLibrary, error) {
	library, err := scanStretchLibrary(r.pool.QueryRow(ctx, `
		SELECT revision, content, updated_at
		FROM stretch_libraries
		WHERE user_id=$1`, userID))
	if err != nil {
		return nil, fmt.Errorf("querying stretch library: %w", err)
	}
	return library, nil
}

func (r *stretchLibraryDAO) Save(ctx context.Context, userID uuid.UUID, moves []model.StretchMove, routines []model.StretchRoutine, expectedRevision int64, operationKey, requestHash string) (*model.StretchLibrary, error) {
	content, err := json.Marshal(stretchLibraryContent{Moves: moves, Routines: routines})
	if err != nil {
		return nil, fmt.Errorf("encoding stretch library: %w", err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning stretch library save: %w", err)
	}
	defer tx.Rollback(ctx)

	// The lock orders concurrent first saves, which have no row to lock.
	if err := lockUserDomain(ctx, tx, stretchLibraryLockNamespace, userID); err != nil {
		return nil, err
	}
	var replay model.StretchLibrary
	if found, err := findIdempotency(ctx, tx, userID, stretchLibraryScope, operationKey, requestHash, &replay); err != nil {
		return nil, err
	} else if found {
		return &replay, nil
	}

	current, err := scanStretchLibrary(tx.QueryRow(ctx, `
		SELECT revision, content, updated_at
		FROM stretch_libraries
		WHERE user_id=$1
		FOR UPDATE`, userID))
	if err != nil {
		return nil, fmt.Errorf("locking stretch library: %w", err)
	}
	if expectedRevision != current.Revision {
		return nil, &model.ConflictError{Message: "resource revision conflict", Expected: expectedRevision, Actual: current.Revision, Authoritative: current}
	}

	saved, err := scanStretchLibrary(tx.QueryRow(ctx, `
		INSERT INTO stretch_libraries AS l (user_id, content)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE
		SET content=EXCLUDED.content, revision=l.revision+1, updated_at=now()
		RETURNING revision, content, updated_at`, userID, content))
	if err != nil {
		return nil, fmt.Errorf("writing stretch library: %w", err)
	}

	record := model.IdempotencyRecord{
		Scope:            stretchLibraryScope,
		OperationKey:     operationKey,
		RequestHash:      requestHash,
		ResponseStatus:   200,
		ResourceType:     "stretch_library",
		ResourceID:       &userID,
		ResourceRevision: &saved.Revision,
	}
	if err := insertIdempotency(ctx, tx, userID, record, saved); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing stretch library save: %w", err)
	}
	return saved, nil
}

// scanStretchLibrary reads one library row. No row is the empty library at
// revision 0, not an error.
func scanStretchLibrary(row rowScanner) (*model.StretchLibrary, error) {
	library := model.EmptyStretchLibrary()
	var content []byte
	err := row.Scan(&library.Revision, &content, &library.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.EmptyStretchLibrary(), nil
	}
	if err != nil {
		return nil, err
	}
	var stored stretchLibraryContent
	if err := json.Unmarshal(content, &stored); err != nil {
		return nil, fmt.Errorf("decoding stretch library: %w", err)
	}
	if stored.Moves != nil {
		library.Moves = stored.Moves
	}
	if stored.Routines != nil {
		library.Routines = stored.Routines
	}
	for i := range library.Routines {
		if library.Routines[i].Items == nil {
			library.Routines[i].Items = []model.StretchRoutineItem{}
		}
	}
	return library, nil
}
