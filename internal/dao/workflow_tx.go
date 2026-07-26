package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

const (
	programLockNamespace  = "program:"
	scheduleLockNamespace = "schedule:"
)

func lockUserDomain(ctx context.Context, tx pgx.Tx, namespace string, userID uuid.UUID) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, namespace+userID.String()); err != nil {
		return fmt.Errorf("locking user workflow: %w", err)
	}
	return nil
}

func findIdempotency(ctx context.Context, tx pgx.Tx, userID uuid.UUID, scope, operationKey, requestHash string, out any) (bool, error) {
	var storedHash string
	var body []byte
	var expired bool
	err := tx.QueryRow(ctx, `
		SELECT request_hash, response_body, expires_at IS NOT NULL AND expires_at <= now()
		FROM idempotency_records
		WHERE user_id=$1 AND scope=$2 AND operation_key=$3
		FOR UPDATE`, userID, scope, operationKey).Scan(&storedHash, &body, &expired)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking idempotency record: %w", err)
	}
	if expired {
		if _, err := tx.Exec(ctx, `DELETE FROM idempotency_records WHERE user_id=$1 AND scope=$2 AND operation_key=$3`, userID, scope, operationKey); err != nil {
			return false, fmt.Errorf("deleting expired idempotency record: %w", err)
		}
		return false, nil
	}
	if storedHash != requestHash {
		return false, &model.ConflictError{Message: "idempotency key was already used with a different payload"}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return false, fmt.Errorf("decoding idempotency response: %w", err)
	}
	return true, nil
}

func insertIdempotency(ctx context.Context, tx pgx.Tx, userID uuid.UUID, record model.IdempotencyRecord, response any) error {
	body, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("encoding idempotency response: %w", err)
	}
	var document any
	if err := json.Unmarshal(body, &document); err != nil {
		return fmt.Errorf("decoding idempotency response document: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO idempotency_records (
			user_id, scope, operation_key, request_hash, response_status,
			response_body, resource_type, resource_id, resource_revision, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, userID, record.Scope,
		record.OperationKey, record.RequestHash, record.ResponseStatus, document,
		record.ResourceType, record.ResourceID, record.ResourceRevision, record.ExpiresAt)
	if err != nil {
		return fmt.Errorf("creating idempotency record: %w", err)
	}
	return nil
}
