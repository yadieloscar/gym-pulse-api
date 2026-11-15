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

const (
	kneeKitLockNamespace = "knee-kit:"
	kneeKitDayScope      = "knee-kit-days/put"
)

const kneeKitDayColumns = `id, to_char(date, 'YYYY-MM-DD'), completed_items, morning_pain, activity_pain, sharp_pain, limping, revision, created_at, updated_at`

// KneeKitDAO owns the knee_kit_days table. No other DAO reads it.
type KneeKitDAO interface {
	// SaveDay creates or fully replaces the athlete's record for day.Date
	// atomically with the idempotency ledger. expectedRevision is 0 when no
	// record should exist, otherwise the stored revision.
	SaveDay(ctx context.Context, userID uuid.UUID, day *model.KneeKitDay, expectedRevision int64, operationKey, requestHash string) (*model.KneeKitDay, model.KneeKitDayOutcome, error)
	// ListDays returns the owned days in the inclusive range, oldest first.
	ListDays(ctx context.Context, userID uuid.UUID, from, to string) ([]model.KneeKitDay, error)
}

type kneeKitDAO struct{ pool *pgxpool.Pool }

// NewKneeKitDAO creates a KneeKitDAO backed by the given connection pool.
func NewKneeKitDAO(pool *pgxpool.Pool) KneeKitDAO {
	return &kneeKitDAO{pool: pool}
}

// kneeKitDayLedger is the idempotency ledger document for a day save. It
// carries the outcome so a verbatim retry replays the original status.
type kneeKitDayLedger struct {
	Outcome model.KneeKitDayOutcome `json:"outcome"`
	Day     *model.KneeKitDay       `json:"day"`
}

func (r *kneeKitDAO) SaveDay(ctx context.Context, userID uuid.UUID, day *model.KneeKitDay, expectedRevision int64, operationKey, requestHash string) (*model.KneeKitDay, model.KneeKitDayOutcome, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, model.KneeKitDayCreated, fmt.Errorf("beginning knee kit day save: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := lockUserDomain(ctx, tx, kneeKitLockNamespace, userID); err != nil {
		return nil, model.KneeKitDayCreated, err
	}
	var replay kneeKitDayLedger
	if found, err := findIdempotency(ctx, tx, userID, kneeKitDayScope, operationKey, requestHash, &replay); err != nil {
		return nil, model.KneeKitDayCreated, err
	} else if found {
		return replay.Day, replay.Outcome, nil
	}

	existing := &model.KneeKitDay{}
	err = scanKneeKitDay(tx.QueryRow(ctx, `
		SELECT `+kneeKitDayColumns+`
		FROM knee_kit_days
		WHERE user_id=$1 AND date=$2
		FOR UPDATE`, userID, day.Date), existing)
	if errors.Is(err, pgx.ErrNoRows) {
		existing = nil
	} else if err != nil {
		return nil, model.KneeKitDayCreated, fmt.Errorf("querying knee kit day: %w", err)
	}
	if err := checkKneeKitRevision(expectedRevision, existing); err != nil {
		return nil, model.KneeKitDayCreated, err
	}

	result := &model.KneeKitDay{}
	outcome := model.KneeKitDayReplaced
	status := 200
	if existing == nil {
		outcome, status = model.KneeKitDayCreated, 201
		err = scanKneeKitDay(tx.QueryRow(ctx, `
			INSERT INTO knee_kit_days (id, user_id, date, completed_items, morning_pain, activity_pain, sharp_pain, limping)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			RETURNING `+kneeKitDayColumns, uuid.New(), userID, day.Date, day.CompletedItems, day.MorningPain, day.ActivityPain, day.SharpPain, day.Limping), result)
	} else {
		err = scanKneeKitDay(tx.QueryRow(ctx, `
			UPDATE knee_kit_days
			SET completed_items=$3, morning_pain=$4, activity_pain=$5, sharp_pain=$6, limping=$7,
			    revision=revision+1, updated_at=now()
			WHERE user_id=$1 AND date=$2
			RETURNING `+kneeKitDayColumns, userID, day.Date, day.CompletedItems, day.MorningPain, day.ActivityPain, day.SharpPain, day.Limping), result)
	}
	if err != nil {
		return nil, model.KneeKitDayCreated, fmt.Errorf("writing knee kit day: %w", err)
	}

	record := model.IdempotencyRecord{
		Scope:            kneeKitDayScope,
		OperationKey:     operationKey,
		RequestHash:      requestHash,
		ResponseStatus:   status,
		ResourceType:     "knee_kit_day",
		ResourceID:       &result.ID,
		ResourceRevision: &result.Revision,
	}
	if err := insertIdempotency(ctx, tx, userID, record, kneeKitDayLedger{Outcome: outcome, Day: result}); err != nil {
		return nil, model.KneeKitDayCreated, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, model.KneeKitDayCreated, fmt.Errorf("committing knee kit day save: %w", err)
	}
	return result, outcome, nil
}

// checkKneeKitRevision enforces optimistic concurrency. A missing row has
// revision 0; the conflict carries the stored row when there is one, and a
// true nil interface otherwise so the conflict body omits "resource".
func checkKneeKitRevision(expected int64, existing *model.KneeKitDay) error {
	var actual int64
	var authoritative any
	if existing != nil {
		actual, authoritative = existing.Revision, existing
	}
	if expected != actual {
		return &model.ConflictError{Message: "resource revision conflict", Expected: expected, Actual: actual, Authoritative: authoritative}
	}
	return nil
}

func (r *kneeKitDAO) ListDays(ctx context.Context, userID uuid.UUID, from, to string) ([]model.KneeKitDay, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+kneeKitDayColumns+`
		FROM knee_kit_days
		WHERE user_id=$1 AND date BETWEEN $2 AND $3
		ORDER BY date ASC`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying knee kit days: %w", err)
	}
	defer rows.Close()
	days := []model.KneeKitDay{}
	for rows.Next() {
		var day model.KneeKitDay
		if err := scanKneeKitDay(rows, &day); err != nil {
			return nil, fmt.Errorf("scanning knee kit day: %w", err)
		}
		days = append(days, day)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating knee kit days: %w", err)
	}
	return days, nil
}

func scanKneeKitDay(row rowScanner, day *model.KneeKitDay) error {
	if err := row.Scan(&day.ID, &day.Date, &day.CompletedItems, &day.MorningPain, &day.ActivityPain,
		&day.SharpPain, &day.Limping, &day.Revision, &day.CreatedAt, &day.UpdatedAt); err != nil {
		return err
	}
	// The contract promises an array, never null.
	if day.CompletedItems == nil {
		day.CompletedItems = []string{}
	}
	return nil
}
