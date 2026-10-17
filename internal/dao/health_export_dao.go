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

const healthExportLockNamespace = "health-export:"

const healthExportColumns = `id, workout_kind, workout_id, source, external_id, exported_at, created_at`

type HealthExportDAO interface {
	// Record stores the export atomically with the idempotency ledger and
	// reports whether it recorded, replayed, or found the same identity.
	Record(ctx context.Context, userID uuid.UUID, export *model.HealthExport, operationKey, requestHash string) (*model.HealthExport, model.HealthExportOutcome, error)
	// List returns owned exports whose workout date falls in the inclusive range.
	List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.HealthExport, error)
}

type healthExportDAO struct{ pool *pgxpool.Pool }

func NewHealthExportDAO(pool *pgxpool.Pool) HealthExportDAO {
	return &healthExportDAO{pool: pool}
}

// workoutDateQuery resolves the date of an owned, finished workout of the given
// kind. A missed planned workout and a rest log are not finished.
const workoutDateQuery = `
		SELECT to_char(w.date, 'YYYY-MM-DD') FROM (
			SELECT date FROM scheduled_workouts
			WHERE $1 = 'scheduled_workout' AND id=$2 AND user_id=$3
			  AND finalized_at IS NOT NULL AND status IN ('completed', 'incomplete')
			UNION ALL
			SELECT date FROM day_logs
			WHERE $1 = 'day_log' AND id=$2 AND user_id=$3 AND type_id <> 'rest'
		) w LIMIT 1`

func (r *healthExportDAO) Record(ctx context.Context, userID uuid.UUID, export *model.HealthExport, operationKey, requestHash string) (*model.HealthExport, model.HealthExportOutcome, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, model.HealthExportRecorded, fmt.Errorf("beginning health export record: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := lockUserDomain(ctx, tx, healthExportLockNamespace, userID); err != nil {
		return nil, model.HealthExportRecorded, err
	}
	var replay model.HealthExport
	if found, err := findIdempotency(ctx, tx, userID, "health-exports/record", operationKey, requestHash, &replay); err != nil {
		return nil, model.HealthExportRecorded, err
	} else if found {
		return &replay, model.HealthExportReplayed, nil
	}

	err = tx.QueryRow(ctx, workoutDateQuery, string(export.WorkoutKind), export.WorkoutID, userID).Scan(&export.Date)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, model.HealthExportRecorded, &model.NotFoundError{Message: "finished workout not found"}
	}
	if err != nil {
		return nil, model.HealthExportRecorded, fmt.Errorf("resolving exported workout: %w", err)
	}

	existing := &model.HealthExport{Date: export.Date}
	err = scanHealthExport(tx.QueryRow(ctx, `
		SELECT `+healthExportColumns+`
		FROM health_exports
		WHERE user_id=$1 AND workout_kind=$2 AND workout_id=$3 AND source=$4`,
		userID, export.WorkoutKind, export.WorkoutID, export.Source), existing)
	if err == nil {
		if existing.ExternalID == export.ExternalID {
			return existing, model.HealthExportExisting, nil
		}
		return nil, model.HealthExportRecorded, &model.ConflictError{Message: "health export identity already recorded", Authoritative: existing}
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, model.HealthExportRecorded, fmt.Errorf("querying health export: %w", err)
	}

	if export.ID == uuid.Nil {
		export.ID = uuid.New()
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO health_exports (id, user_id, workout_kind, workout_id, source, external_id, exported_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING created_at`, export.ID, userID, export.WorkoutKind, export.WorkoutID,
		export.Source, export.ExternalID, export.ExportedAt,
	).Scan(&export.CreatedAt)
	if err != nil {
		return nil, model.HealthExportRecorded, fmt.Errorf("inserting health export: %w", err)
	}

	record := model.IdempotencyRecord{
		Scope:          "health-exports/record",
		OperationKey:   operationKey,
		RequestHash:    requestHash,
		ResponseStatus: 201,
		ResourceType:   "health_export",
		ResourceID:     &export.ID,
	}
	if err := insertIdempotency(ctx, tx, userID, record, export); err != nil {
		return nil, model.HealthExportRecorded, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, model.HealthExportRecorded, fmt.Errorf("committing health export record: %w", err)
	}
	return export, model.HealthExportRecorded, nil
}

func (r *healthExportDAO) List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.HealthExport, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT e.id, e.workout_kind, e.workout_id, e.source, e.external_id, e.exported_at, e.created_at,
		       to_char(w.date, 'YYYY-MM-DD')
		FROM health_exports e
		JOIN LATERAL (
			SELECT date FROM scheduled_workouts WHERE e.workout_kind = 'scheduled_workout' AND id = e.workout_id AND user_id = e.user_id
			UNION ALL
			SELECT date FROM day_logs WHERE e.workout_kind = 'day_log' AND id = e.workout_id AND user_id = e.user_id
		) w ON true
		WHERE e.user_id=$1 AND w.date BETWEEN $2 AND $3
		ORDER BY w.date DESC, e.exported_at DESC, e.id DESC`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying health exports: %w", err)
	}
	defer rows.Close()

	exports := []model.HealthExport{}
	for rows.Next() {
		var export model.HealthExport
		if err := rows.Scan(&export.ID, &export.WorkoutKind, &export.WorkoutID, &export.Source,
			&export.ExternalID, &export.ExportedAt, &export.CreatedAt, &export.Date); err != nil {
			return nil, fmt.Errorf("scanning health export: %w", err)
		}
		exports = append(exports, export)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating health exports: %w", err)
	}
	return exports, nil
}

func scanHealthExport(row rowScanner, export *model.HealthExport) error {
	return row.Scan(&export.ID, &export.WorkoutKind, &export.WorkoutID, &export.Source,
		&export.ExternalID, &export.ExportedAt, &export.CreatedAt)
}
