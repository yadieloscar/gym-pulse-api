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
	nutritionLockNamespace = "nutrition:"
	nutritionDayScope      = "nutrition-days/put"
	nutritionTargetScope   = "nutrition-target/put"
)

const nutritionDayColumns = `id, to_char(date, 'YYYY-MM-DD'), energy_kcal, protein_g, carbs_g, fat_g, source, revision, logged_at, created_at, updated_at`

const nutritionTargetColumns = `id, to_char(effective_from, 'YYYY-MM-DD'), protein_g, energy_kcal, carbs_g, fat_g, revision, created_at, updated_at`

// NutritionDAO owns the nutrition_days and nutrition_targets tables.
type NutritionDAO interface {
	// SaveDay creates, replaces, or removes the athlete's totals for day.Date
	// atomically with the idempotency ledger. expectedRevision is required for
	// athlete saves and ignored for Health saves; remove deletes the day.
	SaveDay(ctx context.Context, userID uuid.UUID, day *model.NutritionDay, expectedRevision *int64, remove bool, operationKey, requestHash string) (*model.NutritionDay, model.NutritionDayOutcome, error)
	// ListDays returns the owned days in the inclusive range oldest first and
	// the targets that applied to them.
	ListDays(ctx context.Context, userID uuid.UUID, from, to string) (*model.NutritionRange, error)
	// GetCurrentTarget returns the target in effect on localToday.
	GetCurrentTarget(ctx context.Context, userID uuid.UUID, localToday string) (*model.NutritionTarget, error)
	// SaveTarget stores the target for target.EffectiveFrom atomically with the
	// idempotency ledger; expectedRevision is the current target's revision.
	SaveTarget(ctx context.Context, userID uuid.UUID, target *model.NutritionTarget, expectedRevision int64, localToday, operationKey, requestHash string) (*model.NutritionTarget, model.NutritionTargetOutcome, error)
}

type nutritionDAO struct{ pool *pgxpool.Pool }

func NewNutritionDAO(pool *pgxpool.Pool) NutritionDAO {
	return &nutritionDAO{pool: pool}
}

// nutritionDayLedger is the idempotency ledger document for a day save. It
// carries the outcome so a verbatim retry replays the original status.
type nutritionDayLedger struct {
	Outcome model.NutritionDayOutcome `json:"outcome"`
	Day     *model.NutritionDay       `json:"day"`
}

type nutritionTargetLedger struct {
	Outcome model.NutritionTargetOutcome `json:"outcome"`
	Target  *model.NutritionTarget       `json:"target"`
}

func (r *nutritionDAO) SaveDay(ctx context.Context, userID uuid.UUID, day *model.NutritionDay, expectedRevision *int64, remove bool, operationKey, requestHash string) (*model.NutritionDay, model.NutritionDayOutcome, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, model.NutritionDayCreated, fmt.Errorf("beginning nutrition day save: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := lockUserDomain(ctx, tx, nutritionLockNamespace, userID); err != nil {
		return nil, model.NutritionDayCreated, err
	}
	var replay nutritionDayLedger
	if found, err := findIdempotency(ctx, tx, userID, nutritionDayScope, operationKey, requestHash, &replay); err != nil {
		return nil, model.NutritionDayCreated, err
	} else if found {
		return replay.Day, replay.Outcome, nil
	}

	existing := &model.NutritionDay{}
	err = scanNutritionDay(tx.QueryRow(ctx, `
		SELECT `+nutritionDayColumns+`
		FROM nutrition_days
		WHERE user_id=$1 AND date=$2
		FOR UPDATE`, userID, day.Date), existing)
	if errors.Is(err, pgx.ErrNoRows) {
		existing = nil
	} else if err != nil {
		return nil, model.NutritionDayCreated, fmt.Errorf("querying nutrition day: %w", err)
	}

	// A day that is the sum of its food entries refuses typed saves and
	// removes; a Health save over it falls through to the ignored branch.
	if existing != nil && existing.Source == model.NutritionSourceFoods && day.Source != model.NutritionSourceAppleHealth {
		return nil, model.NutritionDayCreated, &model.DayHasFoodEntriesError{Day: existing}
	}

	var result *model.NutritionDay
	var outcome model.NutritionDayOutcome
	switch {
	case remove:
		if err := checkNutritionRevision(expectedRevision, authoritativeDay(existing), existingRevision(existing)); err != nil {
			return nil, model.NutritionDayCreated, err
		}
		if existing != nil {
			if _, err := tx.Exec(ctx, `DELETE FROM nutrition_days WHERE user_id=$1 AND date=$2`, userID, day.Date); err != nil {
				return nil, model.NutritionDayCreated, fmt.Errorf("deleting nutrition day: %w", err)
			}
		}
		outcome = model.NutritionDayRemoved
	case day.Source == model.NutritionSourceAppleHealth:
		switch {
		case existing == nil:
			result, err = insertNutritionDay(ctx, tx, userID, day)
			outcome = model.NutritionDayCreated
		case existing.Source == model.NutritionSourceAppleHealth:
			result, err = updateNutritionDay(ctx, tx, userID, day)
			outcome = model.NutritionDayReplaced
		default:
			// The athlete's entry wins: return it unchanged and write only the ledger.
			result, outcome = existing, model.NutritionDayIgnored
		}
		if err != nil {
			return nil, model.NutritionDayCreated, err
		}
	default:
		if err := checkNutritionRevision(expectedRevision, authoritativeDay(existing), existingRevision(existing)); err != nil {
			return nil, model.NutritionDayCreated, err
		}
		day.Source = model.NutritionSourceManual
		if existing == nil {
			result, err = insertNutritionDay(ctx, tx, userID, day)
			outcome = model.NutritionDayCreated
		} else {
			result, err = updateNutritionDay(ctx, tx, userID, day)
			outcome = model.NutritionDayReplaced
		}
		if err != nil {
			return nil, model.NutritionDayCreated, err
		}
	}

	record := model.IdempotencyRecord{
		Scope:          nutritionDayScope,
		OperationKey:   operationKey,
		RequestHash:    requestHash,
		ResponseStatus: nutritionDayStatus(outcome),
		ResourceType:   "nutrition_day",
	}
	if result != nil {
		record.ResourceID = &result.ID
		record.ResourceRevision = &result.Revision
	}
	if err := insertIdempotency(ctx, tx, userID, record, nutritionDayLedger{Outcome: outcome, Day: result}); err != nil {
		return nil, model.NutritionDayCreated, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, model.NutritionDayCreated, fmt.Errorf("committing nutrition day save: %w", err)
	}
	return result, outcome, nil
}

// authoritativeDay returns a true nil interface for a missing row so the
// conflict body omits "resource" instead of carrying null.
func authoritativeDay(day *model.NutritionDay) any {
	if day == nil {
		return nil
	}
	return day
}

func existingRevision(day *model.NutritionDay) int64 {
	if day == nil {
		return 0
	}
	return day.Revision
}

// checkNutritionRevision enforces optimistic concurrency for athlete saves. A
// missing row has revision 0; the conflict carries the stored row when there
// is one.
func checkNutritionRevision(expected *int64, authoritative any, actual int64) error {
	if expected == nil {
		return &model.ValidationError{Message: "expected_revision is required", Field: "expected_revision"}
	}
	if *expected != actual {
		return &model.ConflictError{Message: "resource revision conflict", Expected: *expected, Actual: actual, Authoritative: authoritative}
	}
	return nil
}

func insertNutritionDay(ctx context.Context, tx pgx.Tx, userID uuid.UUID, day *model.NutritionDay) (*model.NutritionDay, error) {
	if day.ID == uuid.Nil {
		day.ID = uuid.New()
	}
	saved := &model.NutritionDay{}
	err := scanNutritionDay(tx.QueryRow(ctx, `
		INSERT INTO nutrition_days (id, user_id, date, energy_kcal, protein_g, carbs_g, fat_g, source, logged_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,now())
		RETURNING `+nutritionDayColumns, day.ID, userID, day.Date, day.EnergyKcal, day.ProteinG, day.CarbsG, day.FatG, day.Source), saved)
	if err != nil {
		return nil, fmt.Errorf("inserting nutrition day: %w", err)
	}
	return saved, nil
}

func updateNutritionDay(ctx context.Context, tx pgx.Tx, userID uuid.UUID, day *model.NutritionDay) (*model.NutritionDay, error) {
	saved := &model.NutritionDay{}
	err := scanNutritionDay(tx.QueryRow(ctx, `
		UPDATE nutrition_days
		SET energy_kcal=$3, protein_g=$4, carbs_g=$5, fat_g=$6, source=$7,
		    revision=revision+1, logged_at=now(), updated_at=now()
		WHERE user_id=$1 AND date=$2
		RETURNING `+nutritionDayColumns, userID, day.Date, day.EnergyKcal, day.ProteinG, day.CarbsG, day.FatG, day.Source), saved)
	if err != nil {
		return nil, fmt.Errorf("updating nutrition day: %w", err)
	}
	return saved, nil
}

func nutritionDayStatus(outcome model.NutritionDayOutcome) int {
	switch outcome {
	case model.NutritionDayCreated:
		return 201
	case model.NutritionDayRemoved:
		return 204
	default:
		return 200
	}
}

func (r *nutritionDAO) ListDays(ctx context.Context, userID uuid.UUID, from, to string) (*model.NutritionRange, error) {
	result := &model.NutritionRange{Days: []model.NutritionDay{}, Targets: []model.NutritionTarget{}}

	rows, err := r.pool.Query(ctx, `
		SELECT `+nutritionDayColumns+`
		FROM nutrition_days
		WHERE user_id=$1 AND date BETWEEN $2 AND $3
		ORDER BY date ASC`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying nutrition days: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var day model.NutritionDay
		if err := scanNutritionDay(rows, &day); err != nil {
			return nil, fmt.Errorf("scanning nutrition day: %w", err)
		}
		result.Days = append(result.Days, day)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating nutrition days: %w", err)
	}

	// The target in effect on `from` plus every target starting inside the range.
	targetRows, err := r.pool.Query(ctx, `
		SELECT `+nutritionTargetColumns+`
		FROM nutrition_targets
		WHERE user_id=$1 AND (
			effective_from BETWEEN $2 AND $3
			OR effective_from = (
				SELECT max(effective_from) FROM nutrition_targets WHERE user_id=$1 AND effective_from <= $2
			)
		)
		ORDER BY effective_from ASC`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying nutrition targets: %w", err)
	}
	defer targetRows.Close()
	for targetRows.Next() {
		var target model.NutritionTarget
		if err := scanNutritionTarget(targetRows, &target); err != nil {
			return nil, fmt.Errorf("scanning nutrition target: %w", err)
		}
		result.Targets = append(result.Targets, target)
	}
	if err := targetRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating nutrition targets: %w", err)
	}
	return result, nil
}

const currentNutritionTargetQuery = `
		SELECT ` + nutritionTargetColumns + `
		FROM nutrition_targets
		WHERE user_id=$1 AND effective_from <= $2
		ORDER BY effective_from DESC
		LIMIT 1`

func (r *nutritionDAO) GetCurrentTarget(ctx context.Context, userID uuid.UUID, localToday string) (*model.NutritionTarget, error) {
	target := &model.NutritionTarget{}
	err := scanNutritionTarget(r.pool.QueryRow(ctx, currentNutritionTargetQuery, userID, localToday), target)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "nutrition target not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("querying current nutrition target: %w", err)
	}
	return target, nil
}

func (r *nutritionDAO) SaveTarget(ctx context.Context, userID uuid.UUID, target *model.NutritionTarget, expectedRevision int64, localToday, operationKey, requestHash string) (*model.NutritionTarget, model.NutritionTargetOutcome, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, model.NutritionTargetCreated, fmt.Errorf("beginning nutrition target save: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := lockUserDomain(ctx, tx, nutritionLockNamespace, userID); err != nil {
		return nil, model.NutritionTargetCreated, err
	}
	var replay nutritionTargetLedger
	if found, err := findIdempotency(ctx, tx, userID, nutritionTargetScope, operationKey, requestHash, &replay); err != nil {
		return nil, model.NutritionTargetCreated, err
	} else if found {
		return replay.Target, replay.Outcome, nil
	}

	current := &model.NutritionTarget{}
	err = scanNutritionTarget(tx.QueryRow(ctx, currentNutritionTargetQuery+` FOR UPDATE`, userID, localToday), current)
	if errors.Is(err, pgx.ErrNoRows) {
		current = nil
	} else if err != nil {
		return nil, model.NutritionTargetCreated, fmt.Errorf("querying current nutrition target: %w", err)
	}
	var actual int64
	if current != nil {
		actual = current.Revision
	}
	if expectedRevision != actual {
		var authoritative any
		if current != nil {
			authoritative = current
		}
		return nil, model.NutritionTargetCreated, &model.ConflictError{Message: "resource revision conflict", Expected: expectedRevision, Actual: actual, Authoritative: authoritative}
	}

	var result *model.NutritionTarget
	var outcome model.NutritionTargetOutcome
	if current != nil && current.EffectiveFrom == target.EffectiveFrom && sameNutritionTargetValues(current, target) {
		result, outcome = current, model.NutritionTargetUnchanged
	} else {
		var existingID uuid.UUID
		err = tx.QueryRow(ctx, `SELECT id FROM nutrition_targets WHERE user_id=$1 AND effective_from=$2 FOR UPDATE`, userID, target.EffectiveFrom).Scan(&existingID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if target.ID == uuid.Nil {
				target.ID = uuid.New()
			}
			result = &model.NutritionTarget{}
			err = scanNutritionTarget(tx.QueryRow(ctx, `
				INSERT INTO nutrition_targets (id, user_id, effective_from, protein_g, energy_kcal, carbs_g, fat_g)
				VALUES ($1,$2,$3,$4,$5,$6,$7)
				RETURNING `+nutritionTargetColumns, target.ID, userID, target.EffectiveFrom, target.ProteinG, target.EnergyKcal, target.CarbsG, target.FatG), result)
			if err != nil {
				return nil, model.NutritionTargetCreated, fmt.Errorf("inserting nutrition target: %w", err)
			}
			outcome = model.NutritionTargetCreated
		case err != nil:
			return nil, model.NutritionTargetCreated, fmt.Errorf("querying nutrition target by date: %w", err)
		default:
			result = &model.NutritionTarget{}
			err = scanNutritionTarget(tx.QueryRow(ctx, `
				UPDATE nutrition_targets
				SET protein_g=$3, energy_kcal=$4, carbs_g=$5, fat_g=$6, revision=revision+1, updated_at=now()
				WHERE user_id=$1 AND effective_from=$2
				RETURNING `+nutritionTargetColumns, userID, target.EffectiveFrom, target.ProteinG, target.EnergyKcal, target.CarbsG, target.FatG), result)
			if err != nil {
				return nil, model.NutritionTargetCreated, fmt.Errorf("updating nutrition target: %w", err)
			}
			outcome = model.NutritionTargetReplaced
		}
	}

	status := 200
	if outcome == model.NutritionTargetCreated {
		status = 201
	}
	record := model.IdempotencyRecord{
		Scope:            nutritionTargetScope,
		OperationKey:     operationKey,
		RequestHash:      requestHash,
		ResponseStatus:   status,
		ResourceType:     "nutrition_target",
		ResourceID:       &result.ID,
		ResourceRevision: &result.Revision,
	}
	if err := insertIdempotency(ctx, tx, userID, record, nutritionTargetLedger{Outcome: outcome, Target: result}); err != nil {
		return nil, model.NutritionTargetCreated, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, model.NutritionTargetCreated, fmt.Errorf("committing nutrition target save: %w", err)
	}
	return result, outcome, nil
}

func sameNutritionTargetValues(a, b *model.NutritionTarget) bool {
	return a.ProteinG == b.ProteinG && sameIntPtr(a.EnergyKcal, b.EnergyKcal) &&
		sameFloatPtr(a.CarbsG, b.CarbsG) && sameFloatPtr(a.FatG, b.FatG)
}

func sameIntPtr(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func sameFloatPtr(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func scanNutritionDay(row rowScanner, day *model.NutritionDay) error {
	return row.Scan(&day.ID, &day.Date, &day.EnergyKcal, &day.ProteinG, &day.CarbsG, &day.FatG,
		&day.Source, &day.Revision, &day.LoggedAt, &day.CreatedAt, &day.UpdatedAt)
}

func scanNutritionTarget(row rowScanner, target *model.NutritionTarget) error {
	return row.Scan(&target.ID, &target.EffectiveFrom, &target.ProteinG, &target.EnergyKcal, &target.CarbsG, &target.FatG,
		&target.Revision, &target.CreatedAt, &target.UpdatedAt)
}
