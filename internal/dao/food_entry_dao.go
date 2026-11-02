package dao

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// FoodEntryDAO owns food entries and, in the same transaction, the
// nutrition_days row that is their sum. Every write runs under the athlete's
// nutrition: advisory lock so entry writes and day saves never interleave.
type FoodEntryDAO interface {
	ListDayEntries(ctx context.Context, userID uuid.UUID, date string) (*model.DayEntries, error)
	CreateEntry(ctx context.Context, userID uuid.UUID, entry *model.FoodEntry, operationKey, requestHash string) (*model.FoodEntryCreated, model.FoodEntryOutcome, error)
	UpdateEntry(ctx context.Context, userID, entryID uuid.UUID, change model.UpdateFoodEntryRequest, operationKey, requestHash string) (*model.FoodEntryUpdated, model.FoodEntryOutcome, error)
	DeleteEntry(ctx context.Context, userID, entryID uuid.UUID, expectedRevision int64, operationKey, requestHash string) (*model.FoodEntryDeleted, model.FoodEntryOutcome, error)
}

type foodEntryDAO struct {
	pool *pgxpool.Pool
}

func NewFoodEntryDAO(pool *pgxpool.Pool) FoodEntryDAO {
	return &foodEntryDAO{pool: pool}
}

const (
	foodEntryCreateScope = "food-entries/post"
	foodEntryUpdateScope = "food-entries/put"
	foodEntryDeleteScope = "food-entries/delete"

	foodEntryColumns = `e.id, to_char(e.date, 'YYYY-MM-DD'), e.kind, e.catalog_food_id, e.user_food_id, e.serving_label, e.serving_g, e.amount,
		e.energy_kcal, e.protein_g, e.carbs_g, e.fat_g, e.revision, e.logged_at, e.created_at, e.updated_at,
		c.source, c.name, c.brand, u.name, u.brand`
	foodEntryFrom = `FROM food_entries e
		LEFT JOIN food_catalog c ON c.id = e.catalog_food_id
		LEFT JOIN user_foods u ON u.id = e.user_food_id`
)

// foodEntryLedger stores the outcome and the full envelope so a verbatim
// retry replays the original status and body.
type foodEntryLedger struct {
	Outcome model.FoodEntryOutcome  `json:"outcome"`
	Created *model.FoodEntryCreated `json:"created,omitempty"`
	Updated *model.FoodEntryUpdated `json:"updated,omitempty"`
	Deleted *model.FoodEntryDeleted `json:"deleted,omitempty"`
}

func (r *foodEntryDAO) ListDayEntries(ctx context.Context, userID uuid.UUID, date string) (*model.DayEntries, error) {
	result := &model.DayEntries{Entries: []model.FoodEntry{}}
	day := &model.NutritionDay{}
	err := scanNutritionDay(r.pool.QueryRow(ctx, `SELECT `+nutritionDayColumns+` FROM nutrition_days WHERE user_id=$1 AND date=$2`, userID, date), day)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return nil, fmt.Errorf("reading nutrition day: %w", err)
	default:
		result.Day = day
	}
	rows, err := r.pool.Query(ctx, `SELECT `+foodEntryColumns+` `+foodEntryFrom+`
		WHERE e.user_id=$1 AND e.date=$2
		ORDER BY e.logged_at ASC, e.created_at ASC`, userID, date)
	if err != nil {
		return nil, fmt.Errorf("listing food entries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var entry model.FoodEntry
		if err := scanFoodEntry(rows, &entry); err != nil {
			return nil, fmt.Errorf("scanning food entry: %w", err)
		}
		result.Entries = append(result.Entries, entry)
	}
	return result, rows.Err()
}

func (r *foodEntryDAO) CreateEntry(ctx context.Context, userID uuid.UUID, entry *model.FoodEntry, operationKey, requestHash string) (*model.FoodEntryCreated, model.FoodEntryOutcome, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, model.FoodEntryCreatedOutcome, fmt.Errorf("beginning food entry create: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, nutritionLockNamespace, userID); err != nil {
		return nil, model.FoodEntryCreatedOutcome, err
	}
	var replay foodEntryLedger
	if found, err := findIdempotency(ctx, tx, userID, foodEntryCreateScope, operationKey, requestHash, &replay); err != nil {
		return nil, model.FoodEntryCreatedOutcome, err
	} else if found {
		return replay.Created, replay.Outcome, nil
	}
	if entry.Kind == model.FoodEntryKindFood {
		// The food must still resolve for this athlete; an archived own food
		// cannot start a new entry.
		food, err := getFood(ctx, tx, userID, foodRefID(entry))
		if err != nil {
			return nil, model.FoodEntryCreatedOutcome, err
		}
		if food.Archived {
			return nil, model.FoodEntryCreatedOutcome, &model.NotFoundError{Message: "food not found"}
		}
	}
	existing, err := lockNutritionDay(ctx, tx, userID, entry.Date)
	if err != nil {
		return nil, model.FoodEntryCreatedOutcome, err
	}
	converted := []model.FoodEntry{}
	if existing != nil && existing.Source != model.NutritionSourceFoods {
		// Typed totals become one quick-add entry so nothing the athlete saved
		// disappears when they start logging foods.
		quick := &model.FoodEntry{
			Date:       entry.Date,
			Kind:       model.FoodEntryKindQuickAdd,
			EnergyKcal: existing.EnergyKcal,
			ProteinG:   existing.ProteinG,
			CarbsG:     existing.CarbsG,
			FatG:       existing.FatG,
			LoggedAt:   existing.LoggedAt,
		}
		saved, err := insertFoodEntry(ctx, tx, userID, quick)
		if err != nil {
			return nil, model.FoodEntryCreatedOutcome, err
		}
		converted = append(converted, *saved)
	}
	saved, err := insertFoodEntry(ctx, tx, userID, entry)
	if err != nil {
		return nil, model.FoodEntryCreatedOutcome, err
	}
	day, err := recomputeNutritionDay(ctx, tx, userID, entry.Date, existing)
	if err != nil {
		return nil, model.FoodEntryCreatedOutcome, err
	}
	response := &model.FoodEntryCreated{Entry: *saved, Day: day, Converted: converted}
	if err := insertIdempotency(ctx, tx, userID, foodEntryRecord(foodEntryCreateScope, operationKey, requestHash, 201, saved), foodEntryLedger{Outcome: model.FoodEntryCreatedOutcome, Created: response}); err != nil {
		return nil, model.FoodEntryCreatedOutcome, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, model.FoodEntryCreatedOutcome, fmt.Errorf("committing food entry create: %w", err)
	}
	return response, model.FoodEntryCreatedOutcome, nil
}

func (r *foodEntryDAO) UpdateEntry(ctx context.Context, userID, entryID uuid.UUID, change model.UpdateFoodEntryRequest, operationKey, requestHash string) (*model.FoodEntryUpdated, model.FoodEntryOutcome, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, model.FoodEntryReplacedOutcome, fmt.Errorf("beginning food entry update: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, nutritionLockNamespace, userID); err != nil {
		return nil, model.FoodEntryReplacedOutcome, err
	}
	var replay foodEntryLedger
	if found, err := findIdempotency(ctx, tx, userID, foodEntryUpdateScope, operationKey, requestHash, &replay); err != nil {
		return nil, model.FoodEntryReplacedOutcome, err
	} else if found {
		return replay.Updated, replay.Outcome, nil
	}
	existing, err := lockFoodEntry(ctx, tx, userID, entryID)
	if err != nil {
		return nil, model.FoodEntryReplacedOutcome, err
	}
	if change.ExpectedRevision != existing.Revision {
		return nil, model.FoodEntryReplacedOutcome, &model.ConflictError{Message: "resource revision conflict", Expected: change.ExpectedRevision, Actual: existing.Revision, Authoritative: existing}
	}
	if err := applyEntryChange(ctx, tx, userID, existing, change); err != nil {
		return nil, model.FoodEntryReplacedOutcome, err
	}
	day, err := lockNutritionDay(ctx, tx, userID, existing.Date)
	if err != nil {
		return nil, model.FoodEntryReplacedOutcome, err
	}
	saved, err := updateFoodEntry(ctx, tx, userID, existing)
	if err != nil {
		return nil, model.FoodEntryReplacedOutcome, err
	}
	day, err = recomputeNutritionDay(ctx, tx, userID, existing.Date, day)
	if err != nil {
		return nil, model.FoodEntryReplacedOutcome, err
	}
	response := &model.FoodEntryUpdated{Entry: *saved, Day: day}
	if err := insertIdempotency(ctx, tx, userID, foodEntryRecord(foodEntryUpdateScope, operationKey, requestHash, 200, saved), foodEntryLedger{Outcome: model.FoodEntryReplacedOutcome, Updated: response}); err != nil {
		return nil, model.FoodEntryReplacedOutcome, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, model.FoodEntryReplacedOutcome, fmt.Errorf("committing food entry update: %w", err)
	}
	return response, model.FoodEntryReplacedOutcome, nil
}

func (r *foodEntryDAO) DeleteEntry(ctx context.Context, userID, entryID uuid.UUID, expectedRevision int64, operationKey, requestHash string) (*model.FoodEntryDeleted, model.FoodEntryOutcome, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, model.FoodEntryRemovedOutcome, fmt.Errorf("beginning food entry delete: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, nutritionLockNamespace, userID); err != nil {
		return nil, model.FoodEntryRemovedOutcome, err
	}
	var replay foodEntryLedger
	if found, err := findIdempotency(ctx, tx, userID, foodEntryDeleteScope, operationKey, requestHash, &replay); err != nil {
		return nil, model.FoodEntryRemovedOutcome, err
	} else if found {
		return replay.Deleted, replay.Outcome, nil
	}
	existing, err := lockFoodEntry(ctx, tx, userID, entryID)
	if err != nil {
		return nil, model.FoodEntryRemovedOutcome, err
	}
	if expectedRevision != existing.Revision {
		return nil, model.FoodEntryRemovedOutcome, &model.ConflictError{Message: "resource revision conflict", Expected: expectedRevision, Actual: existing.Revision, Authoritative: existing}
	}
	day, err := lockNutritionDay(ctx, tx, userID, existing.Date)
	if err != nil {
		return nil, model.FoodEntryRemovedOutcome, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM food_entries WHERE user_id=$1 AND id=$2`, userID, entryID); err != nil {
		return nil, model.FoodEntryRemovedOutcome, fmt.Errorf("deleting food entry: %w", err)
	}
	day, err = recomputeNutritionDay(ctx, tx, userID, existing.Date, day)
	if err != nil {
		return nil, model.FoodEntryRemovedOutcome, err
	}
	response := &model.FoodEntryDeleted{Day: day}
	record := foodEntryRecord(foodEntryDeleteScope, operationKey, requestHash, 200, existing)
	if err := insertIdempotency(ctx, tx, userID, record, foodEntryLedger{Outcome: model.FoodEntryRemovedOutcome, Deleted: response}); err != nil {
		return nil, model.FoodEntryRemovedOutcome, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, model.FoodEntryRemovedOutcome, fmt.Errorf("committing food entry delete: %w", err)
	}
	return response, model.FoodEntryRemovedOutcome, nil
}

// applyEntryChange recomputes the snapshot from the entry's food for a food
// entry, or replaces the quick-add values. The food's current facts are used
// on purpose: a change is a new logging decision, while untouched entries keep
// theirs.
func applyEntryChange(ctx context.Context, tx pgx.Tx, userID uuid.UUID, entry *model.FoodEntry, change model.UpdateFoodEntryRequest) error {
	switch entry.Kind {
	case model.FoodEntryKindFood:
		if change.QuickAdd != nil || change.Serving == nil || change.Amount == nil {
			return &model.ValidationError{Message: "a food entry takes serving and amount", Field: "serving"}
		}
		food, err := getFood(ctx, tx, userID, foodRefID(entry))
		if err != nil {
			return err
		}
		serving, err := model.ResolveServing(food, *change.Serving)
		if err != nil {
			return err
		}
		energy, protein, carbs, fat := model.EntryValues(food.Per100g, serving.Grams, *change.Amount)
		entry.Serving, entry.Amount = &serving, change.Amount
		entry.EnergyKcal, entry.ProteinG, entry.CarbsG, entry.FatG = &energy, &protein, &carbs, &fat
	default:
		if change.QuickAdd == nil || change.Serving != nil || change.Amount != nil {
			return &model.ValidationError{Message: "a quick-add entry takes quick_add values", Field: "quick_add"}
		}
		entry.EnergyKcal, entry.ProteinG, entry.CarbsG, entry.FatG = change.QuickAdd.EnergyKcal, change.QuickAdd.ProteinG, change.QuickAdd.CarbsG, change.QuickAdd.FatG
	}
	return nil
}

func foodRefID(entry *model.FoodEntry) uuid.UUID {
	if entry.CatalogFoodID != nil {
		return *entry.CatalogFoodID
	}
	if entry.UserFoodID != nil {
		return *entry.UserFoodID
	}
	return uuid.Nil
}

// lockNutritionDay returns the locked day row, or nil when the date has none.
func lockNutritionDay(ctx context.Context, tx pgx.Tx, userID uuid.UUID, date string) (*model.NutritionDay, error) {
	day := &model.NutritionDay{}
	err := scanNutritionDay(tx.QueryRow(ctx, `SELECT `+nutritionDayColumns+` FROM nutrition_days WHERE user_id=$1 AND date=$2 FOR UPDATE`, userID, date), day)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		day = nil
	case err != nil:
		return nil, fmt.Errorf("locking nutrition day: %w", err)
	}
	return day, nil
}

func lockFoodEntry(ctx context.Context, tx pgx.Tx, userID, id uuid.UUID) (*model.FoodEntry, error) {
	var entry model.FoodEntry
	err := scanFoodEntry(tx.QueryRow(ctx, `SELECT `+foodEntryColumns+` `+foodEntryFrom+`
		WHERE e.user_id=$1 AND e.id=$2
		FOR UPDATE OF e`, userID, id), &entry)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "food entry not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("locking food entry: %w", err)
	}
	return &entry, nil
}

func insertFoodEntry(ctx context.Context, tx pgx.Tx, userID uuid.UUID, entry *model.FoodEntry) (*model.FoodEntry, error) {
	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	if entry.LoggedAt.IsZero() {
		entry.LoggedAt = time.Now()
	}
	var servingLabel *string
	var servingG *float64
	if entry.Serving != nil {
		servingLabel, servingG = &entry.Serving.Label, &entry.Serving.Grams
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO food_entries (id, user_id, date, kind, catalog_food_id, user_food_id, serving_label, serving_g, amount,
			energy_kcal, protein_g, carbs_g, fat_g, logged_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		entry.ID, userID, entry.Date, entry.Kind, entry.CatalogFoodID, entry.UserFoodID, servingLabel, servingG, entry.Amount,
		entry.EnergyKcal, entry.ProteinG, entry.CarbsG, entry.FatG, entry.LoggedAt); err != nil {
		return nil, fmt.Errorf("inserting food entry: %w", err)
	}
	return readFoodEntry(ctx, tx, userID, entry.ID)
}

func updateFoodEntry(ctx context.Context, tx pgx.Tx, userID uuid.UUID, entry *model.FoodEntry) (*model.FoodEntry, error) {
	var servingLabel *string
	var servingG *float64
	if entry.Serving != nil {
		servingLabel, servingG = &entry.Serving.Label, &entry.Serving.Grams
	}
	if _, err := tx.Exec(ctx, `
		UPDATE food_entries
		SET serving_label=$3, serving_g=$4, amount=$5, energy_kcal=$6, protein_g=$7, carbs_g=$8, fat_g=$9,
		    revision=revision+1, updated_at=now()
		WHERE user_id=$1 AND id=$2`,
		userID, entry.ID, servingLabel, servingG, entry.Amount, entry.EnergyKcal, entry.ProteinG, entry.CarbsG, entry.FatG); err != nil {
		return nil, fmt.Errorf("updating food entry: %w", err)
	}
	return readFoodEntry(ctx, tx, userID, entry.ID)
}

func readFoodEntry(ctx context.Context, tx pgx.Tx, userID, id uuid.UUID) (*model.FoodEntry, error) {
	var entry model.FoodEntry
	if err := scanFoodEntry(tx.QueryRow(ctx, `SELECT `+foodEntryColumns+` `+foodEntryFrom+` WHERE e.user_id=$1 AND e.id=$2`, userID, id), &entry); err != nil {
		return nil, fmt.Errorf("reading food entry: %w", err)
	}
	return &entry, nil
}

// recomputeNutritionDay writes the day as the sum of its entries: deletes the
// row when no entries remain, otherwise inserts or updates it as source foods
// with the revision advanced.
func recomputeNutritionDay(ctx context.Context, tx pgx.Tx, userID uuid.UUID, date string, existing *model.NutritionDay) (*model.NutritionDay, error) {
	var count int64
	var energy *int64
	var protein, carbs, fat *float64
	if err := tx.QueryRow(ctx, `
		SELECT count(*), sum(energy_kcal), sum(protein_g), sum(carbs_g), sum(fat_g)
		FROM food_entries
		WHERE user_id=$1 AND date=$2`, userID, date).Scan(&count, &energy, &protein, &carbs, &fat); err != nil {
		return nil, fmt.Errorf("summing food entries: %w", err)
	}
	var removed *model.NutritionDay
	if count == 0 {
		if existing != nil {
			if _, err := tx.Exec(ctx, `DELETE FROM nutrition_days WHERE user_id=$1 AND date=$2`, userID, date); err != nil {
				return nil, fmt.Errorf("deleting emptied nutrition day: %w", err)
			}
		}
		return removed, nil
	}
	day := &model.NutritionDay{Date: date, ProteinG: protein, CarbsG: carbs, FatG: fat, Source: model.NutritionSourceFoods}
	if energy != nil {
		v := int(*energy)
		day.EnergyKcal = &v
	}
	if existing == nil {
		return insertNutritionDay(ctx, tx, userID, day)
	}
	return updateNutritionDay(ctx, tx, userID, day)
}

func foodEntryRecord(scope, operationKey, requestHash string, status int, entry *model.FoodEntry) model.IdempotencyRecord {
	record := model.IdempotencyRecord{Scope: scope, OperationKey: operationKey, RequestHash: requestHash, ResponseStatus: status, ResourceType: "food_entry"}
	if entry != nil {
		record.ResourceID = &entry.ID
		record.ResourceRevision = &entry.Revision
	}
	return record
}

func scanFoodEntry(row rowScanner, entry *model.FoodEntry) error {
	var servingLabel *string
	var servingG *float64
	var catalogSource, catalogName, catalogBrand, userName, userBrand *string
	if err := row.Scan(&entry.ID, &entry.Date, &entry.Kind, &entry.CatalogFoodID, &entry.UserFoodID, &servingLabel, &servingG, &entry.Amount,
		&entry.EnergyKcal, &entry.ProteinG, &entry.CarbsG, &entry.FatG, &entry.Revision, &entry.LoggedAt, &entry.CreatedAt, &entry.UpdatedAt,
		&catalogSource, &catalogName, &catalogBrand, &userName, &userBrand); err != nil {
		return err
	}
	if servingLabel != nil && servingG != nil {
		entry.Serving = &model.Serving{Label: *servingLabel, Grams: *servingG}
	}
	switch {
	case entry.CatalogFoodID != nil && catalogName != nil:
		entry.Food = &model.FoodSummary{ID: *entry.CatalogFoodID, Source: model.FoodSource(*catalogSource), Name: *catalogName, Brand: catalogBrand}
	case entry.UserFoodID != nil && userName != nil:
		entry.Food = &model.FoodSummary{ID: *entry.UserFoodID, Source: model.FoodSourceOwn, Name: *userName, Brand: userBrand}
	}
	return nil
}
