package dao

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// SavedMealDAO owns saved meals and their items. Every write runs under the
// athlete's nutrition: advisory lock, the same lock food entry writes and day
// saves take, so logging a meal can insert entries and rewrite the day as
// their sum in one transaction without interleaving with either.
type SavedMealDAO interface {
	ListMeals(ctx context.Context, userID uuid.UUID) ([]model.SavedMeal, error)
	CreateMeal(ctx context.Context, userID uuid.UUID, req model.SaveMealRequest, operationKey, requestHash string) (*model.SavedMeal, model.SavedMealOutcome, error)
	UpdateMeal(ctx context.Context, userID, mealID uuid.UUID, req model.SaveMealRequest, operationKey, requestHash string) (*model.SavedMeal, model.SavedMealOutcome, error)
	DeleteMeal(ctx context.Context, userID, mealID uuid.UUID, expectedRevision int64, operationKey, requestHash string) (model.SavedMealOutcome, error)
	LogMeal(ctx context.Context, userID, mealID uuid.UUID, date string, items []model.MealItemInput, loggedAt time.Time, operationKey, requestHash string) (*model.MealLogged, model.SavedMealOutcome, error)
}

type savedMealDAO struct {
	pool *pgxpool.Pool
}

func NewSavedMealDAO(pool *pgxpool.Pool) SavedMealDAO {
	return &savedMealDAO{pool: pool}
}

const (
	savedMealCreateScope = "saved-meals/post"
	savedMealUpdateScope = "saved-meals/put"
	savedMealDeleteScope = "saved-meals/delete"
	savedMealLogScope    = "saved-meals/log"

	savedMealColumns = `m.id, m.name, m.revision, m.last_logged_at, m.created_at, m.updated_at`

	savedMealNameIndex = "ux_saved_meals_user_name"
)

// savedMealLedger stores the outcome and the full body so a verbatim retry
// replays the original status and response.
type savedMealLedger struct {
	Outcome model.SavedMealOutcome `json:"outcome"`
	Meal    *model.SavedMeal       `json:"meal,omitempty"`
	Logged  *model.MealLogged      `json:"logged,omitempty"`
}

// savedItemRow is one stored item before its food is resolved.
type savedItemRow struct {
	mealID uuid.UUID
	input  model.MealItemInput
}

// ListMeals reads the meals, their items, and the items' foods inside one
// read-only repeatable-read transaction, so a meal changed mid-read appears
// whole as it was or whole as it became, never with another version's items.
func (r *savedMealDAO) ListMeals(ctx context.Context, userID uuid.UUID) ([]model.SavedMeal, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("beginning saved meal list: %w", err)
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT `+savedMealColumns+` FROM saved_meals m
		WHERE m.user_id=$1
		ORDER BY m.last_logged_at DESC NULLS LAST, m.created_at DESC, m.id`, userID)
	if err != nil {
		return nil, fmt.Errorf("listing saved meals: %w", err)
	}
	meals := []model.SavedMeal{}
	for rows.Next() {
		var meal model.SavedMeal
		if err := scanSavedMeal(rows, &meal); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scanning saved meal: %w", err)
		}
		meals = append(meals, meal)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing saved meals: %w", err)
	}
	if len(meals) == 0 {
		return meals, nil
	}
	stored, err := readSavedItems(ctx, tx, userID, nil)
	if err != nil {
		return nil, err
	}
	foods, err := listFoodsByIDs(ctx, tx, userID, itemFoodIDs(stored))
	if err != nil {
		return nil, err
	}
	byMeal := make(map[uuid.UUID][]model.SavedMealItem, len(meals))
	for _, row := range stored {
		byMeal[row.mealID] = append(byMeal[row.mealID], storedMealItem(foods, row.input))
	}
	for i := range meals {
		setMealItems(&meals[i], byMeal[meals[i].ID])
	}
	return meals, nil
}

func (r *savedMealDAO) CreateMeal(ctx context.Context, userID uuid.UUID, req model.SaveMealRequest, operationKey, requestHash string) (*model.SavedMeal, model.SavedMealOutcome, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, model.SavedMealCreated, fmt.Errorf("beginning saved meal create: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, nutritionLockNamespace, userID); err != nil {
		return nil, model.SavedMealCreated, err
	}
	var replay savedMealLedger
	if found, err := findIdempotency(ctx, tx, userID, savedMealCreateScope, operationKey, requestHash, &replay); err != nil {
		return nil, model.SavedMealCreated, err
	} else if found {
		return replay.Meal, replay.Outcome, nil
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM saved_meals WHERE user_id=$1`, userID).Scan(&count); err != nil {
		return nil, model.SavedMealCreated, fmt.Errorf("counting saved meals: %w", err)
	}
	if count >= model.MaxSavedMeals {
		return nil, model.SavedMealCreated, &model.SavedMealLimitError{Limit: model.MaxSavedMeals}
	}
	if err := checkSavedMealName(ctx, tx, userID, uuid.Nil, req.Name); err != nil {
		return nil, model.SavedMealCreated, err
	}
	// An archived own food cannot start a new meal.
	items, err := resolveMealItems(ctx, tx, userID, req.Items, nil)
	if err != nil {
		return nil, model.SavedMealCreated, err
	}
	var meal model.SavedMeal
	err = scanSavedMeal(tx.QueryRow(ctx, `
		INSERT INTO saved_meals AS m (id, user_id, name)
		VALUES ($1,$2,$3)
		RETURNING `+savedMealColumns, uuid.New(), userID, req.Name), &meal)
	if err != nil {
		return nil, model.SavedMealCreated, savedMealWriteError("inserting saved meal", err)
	}
	if err := insertSavedItems(ctx, tx, userID, meal.ID, items); err != nil {
		return nil, model.SavedMealCreated, err
	}
	setMealItems(&meal, items)
	if err := insertIdempotency(ctx, tx, userID, savedMealRecord(savedMealCreateScope, operationKey, requestHash, 201, &meal), savedMealLedger{Outcome: model.SavedMealCreated, Meal: &meal}); err != nil {
		return nil, model.SavedMealCreated, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, model.SavedMealCreated, fmt.Errorf("committing saved meal create: %w", err)
	}
	return &meal, model.SavedMealCreated, nil
}

func (r *savedMealDAO) UpdateMeal(ctx context.Context, userID, mealID uuid.UUID, req model.SaveMealRequest, operationKey, requestHash string) (*model.SavedMeal, model.SavedMealOutcome, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, model.SavedMealReplaced, fmt.Errorf("beginning saved meal update: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, nutritionLockNamespace, userID); err != nil {
		return nil, model.SavedMealReplaced, err
	}
	var replay savedMealLedger
	if found, err := findIdempotency(ctx, tx, userID, savedMealUpdateScope, operationKey, requestHash, &replay); err != nil {
		return nil, model.SavedMealReplaced, err
	} else if found {
		return replay.Meal, replay.Outcome, nil
	}
	existing, stored, err := lockSavedMeal(ctx, tx, userID, mealID)
	if err != nil {
		return nil, model.SavedMealReplaced, err
	}
	if req.ExpectedRevision == nil || *req.ExpectedRevision != existing.Revision {
		return nil, model.SavedMealReplaced, revisionConflict(req.ExpectedRevision, existing.Revision, existing)
	}
	if err := checkSavedMealName(ctx, tx, userID, mealID, req.Name); err != nil {
		return nil, model.SavedMealReplaced, err
	}
	// A food the meal already holds stays usable after the athlete archives
	// it; an archived food cannot be added.
	items, err := resolveMealItems(ctx, tx, userID, req.Items, heldFoodIDs(stored))
	if err != nil {
		return nil, model.SavedMealReplaced, err
	}
	var meal model.SavedMeal
	err = scanSavedMeal(tx.QueryRow(ctx, `
		UPDATE saved_meals AS m
		SET name=$3, revision=revision+1, updated_at=now()
		WHERE m.user_id=$1 AND m.id=$2
		RETURNING `+savedMealColumns, userID, mealID, req.Name), &meal)
	if err != nil {
		return nil, model.SavedMealReplaced, savedMealWriteError("updating saved meal", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM saved_meal_items
		WHERE meal_id IN (SELECT id FROM saved_meals WHERE user_id=$1 AND id=$2)`, userID, mealID); err != nil {
		return nil, model.SavedMealReplaced, fmt.Errorf("replacing saved meal items: %w", err)
	}
	if err := insertSavedItems(ctx, tx, userID, mealID, items); err != nil {
		return nil, model.SavedMealReplaced, err
	}
	setMealItems(&meal, items)
	if err := insertIdempotency(ctx, tx, userID, savedMealRecord(savedMealUpdateScope, operationKey, requestHash, 200, &meal), savedMealLedger{Outcome: model.SavedMealReplaced, Meal: &meal}); err != nil {
		return nil, model.SavedMealReplaced, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, model.SavedMealReplaced, fmt.Errorf("committing saved meal update: %w", err)
	}
	return &meal, model.SavedMealReplaced, nil
}

func (r *savedMealDAO) DeleteMeal(ctx context.Context, userID, mealID uuid.UUID, expectedRevision int64, operationKey, requestHash string) (model.SavedMealOutcome, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return model.SavedMealRemoved, fmt.Errorf("beginning saved meal delete: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, nutritionLockNamespace, userID); err != nil {
		return model.SavedMealRemoved, err
	}
	var replay savedMealLedger
	if found, err := findIdempotency(ctx, tx, userID, savedMealDeleteScope, operationKey, requestHash, &replay); err != nil {
		return model.SavedMealRemoved, err
	} else if found {
		return replay.Outcome, nil
	}
	existing, _, err := lockSavedMeal(ctx, tx, userID, mealID)
	if err != nil {
		return model.SavedMealRemoved, err
	}
	if expectedRevision != existing.Revision {
		return model.SavedMealRemoved, revisionConflict(&expectedRevision, existing.Revision, existing)
	}
	// Items cascade. Food entries never reference a meal, so every day the
	// meal was logged to is untouched.
	if _, err := tx.Exec(ctx, `DELETE FROM saved_meals WHERE user_id=$1 AND id=$2`, userID, mealID); err != nil {
		return model.SavedMealRemoved, fmt.Errorf("deleting saved meal: %w", err)
	}
	if err := insertIdempotency(ctx, tx, userID, savedMealRecord(savedMealDeleteScope, operationKey, requestHash, 204, existing), savedMealLedger{Outcome: model.SavedMealRemoved}); err != nil {
		return model.SavedMealRemoved, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.SavedMealRemoved, fmt.Errorf("committing saved meal delete: %w", err)
	}
	return model.SavedMealRemoved, nil
}

// LogMeal inserts one ordinary food entry per item and rewrites the day as
// the sum of its entries, all in one transaction: every entry lands or none
// does. A nil items logs the meal as saved; otherwise items replaces the
// saved list for this log only.
func (r *savedMealDAO) LogMeal(ctx context.Context, userID, mealID uuid.UUID, date string, items []model.MealItemInput, loggedAt time.Time, operationKey, requestHash string) (*model.MealLogged, model.SavedMealOutcome, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, model.SavedMealLogged, fmt.Errorf("beginning saved meal log: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, nutritionLockNamespace, userID); err != nil {
		return nil, model.SavedMealLogged, err
	}
	var replay savedMealLedger
	if found, err := findIdempotency(ctx, tx, userID, savedMealLogScope, operationKey, requestHash, &replay); err != nil {
		return nil, model.SavedMealLogged, err
	} else if found {
		return replay.Logged, replay.Outcome, nil
	}
	meal, stored, err := lockSavedMeal(ctx, tx, userID, mealID)
	if err != nil {
		return nil, model.SavedMealLogged, err
	}
	if items == nil {
		items = make([]model.MealItemInput, 0, len(stored))
		for _, row := range stored {
			items = append(items, row.input)
		}
	}
	// An archived own food still logs from a meal that already holds it; an
	// override list cannot introduce one.
	resolved, err := resolveMealItems(ctx, tx, userID, items, heldFoodIDs(stored))
	if err != nil {
		return nil, model.SavedMealLogged, err
	}
	for i, item := range resolved {
		if !item.Available {
			return nil, model.SavedMealLogged, mealItemError(i, "food facts are not available")
		}
	}
	existing, err := lockNutritionDay(ctx, tx, userID, date)
	if err != nil {
		return nil, model.SavedMealLogged, err
	}
	converted, err := convertTypedTotals(ctx, tx, userID, date, existing)
	if err != nil {
		return nil, model.SavedMealLogged, err
	}
	entries := make([]model.FoodEntry, 0, len(resolved))
	for i, item := range resolved {
		amount, serving := item.Amount, item.Serving
		entry := &model.FoodEntry{
			Date:       date,
			Kind:       model.FoodEntryKindFood,
			Serving:    &serving,
			Amount:     &amount,
			EnergyKcal: item.EnergyKcal,
			ProteinG:   item.ProteinG,
			CarbsG:     item.CarbsG,
			FatG:       item.FatG,
			// Entries inserted in one transaction share created_at, so a
			// microsecond step keeps the meal's order in the day list.
			LoggedAt: loggedAt.Add(time.Duration(i) * time.Microsecond),
		}
		if item.Food.Source == model.FoodSourceOwn {
			entry.UserFoodID = &item.Food.ID
		} else {
			entry.CatalogFoodID = &item.Food.ID
		}
		saved, err := insertFoodEntry(ctx, tx, userID, entry)
		if err != nil {
			return nil, model.SavedMealLogged, err
		}
		entries = append(entries, *saved)
	}
	day, err := recomputeNutritionDay(ctx, tx, userID, date, existing)
	if err != nil {
		return nil, model.SavedMealLogged, err
	}
	// The revision is left alone: logging changes nothing the athlete
	// authored, so it must not make a pending edit stale.
	if _, err := tx.Exec(ctx, `UPDATE saved_meals SET last_logged_at=now() WHERE user_id=$1 AND id=$2`, userID, mealID); err != nil {
		return nil, model.SavedMealLogged, fmt.Errorf("stamping saved meal log: %w", err)
	}
	response := &model.MealLogged{Entries: entries, Day: day, Converted: converted}
	if err := insertIdempotency(ctx, tx, userID, savedMealRecord(savedMealLogScope, operationKey, requestHash, 201, meal), savedMealLedger{Outcome: model.SavedMealLogged, Logged: response}); err != nil {
		return nil, model.SavedMealLogged, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, model.SavedMealLogged, fmt.Errorf("committing saved meal log: %w", err)
	}
	return response, model.SavedMealLogged, nil
}

// convertTypedTotals mirrors CreateEntry: typed totals already on the day
// become one quick-add entry so nothing the athlete saved disappears when
// foods are logged to it. It returns an empty list when there is nothing to
// convert.
func convertTypedTotals(ctx context.Context, tx pgx.Tx, userID uuid.UUID, date string, existing *model.NutritionDay) ([]model.FoodEntry, error) {
	converted := []model.FoodEntry{}
	if existing == nil || existing.Source == model.NutritionSourceFoods {
		return converted, nil
	}
	quick := &model.FoodEntry{
		Date:       date,
		Kind:       model.FoodEntryKindQuickAdd,
		EnergyKcal: existing.EnergyKcal,
		ProteinG:   existing.ProteinG,
		CarbsG:     existing.CarbsG,
		FatG:       existing.FatG,
		LoggedAt:   existing.LoggedAt,
	}
	saved, err := insertFoodEntry(ctx, tx, userID, quick)
	if err != nil {
		return nil, err
	}
	return append(converted, *saved), nil
}

// lockSavedMeal locks the athlete's meal row and returns it with its items
// resolved to current food facts, plus the stored rows. Unknown and foreign
// ids share one not-found.
func lockSavedMeal(ctx context.Context, tx pgx.Tx, userID, mealID uuid.UUID) (*model.SavedMeal, []savedItemRow, error) {
	var meal model.SavedMeal
	err := scanSavedMeal(tx.QueryRow(ctx, `SELECT `+savedMealColumns+` FROM saved_meals m
		WHERE m.user_id=$1 AND m.id=$2
		FOR UPDATE`, userID, mealID), &meal)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, &model.NotFoundError{Message: "saved meal not found"}
	}
	if err != nil {
		return nil, nil, fmt.Errorf("locking saved meal: %w", err)
	}
	stored, err := readSavedItems(ctx, tx, userID, &mealID)
	if err != nil {
		return nil, nil, err
	}
	foods, err := listFoodsByIDs(ctx, tx, userID, itemFoodIDs(stored))
	if err != nil {
		return nil, nil, err
	}
	items := make([]model.SavedMealItem, 0, len(stored))
	for _, row := range stored {
		items = append(items, storedMealItem(foods, row.input))
	}
	setMealItems(&meal, items)
	return &meal, stored, nil
}

// readSavedItems reads the stored items of one meal, or of every meal the
// athlete owns when mealID is nil, in meal and position order.
func readSavedItems(ctx context.Context, q querier, userID uuid.UUID, mealID *uuid.UUID) ([]savedItemRow, error) {
	rows, err := q.Query(ctx, `
		SELECT i.meal_id, COALESCE(i.catalog_food_id, i.user_food_id), i.serving_label, i.serving_g, i.amount
		FROM saved_meal_items i
		JOIN saved_meals m ON m.id = i.meal_id
		WHERE m.user_id=$1 AND ($2::uuid IS NULL OR m.id=$2)
		ORDER BY i.meal_id, i.position`, userID, mealID)
	if err != nil {
		return nil, fmt.Errorf("reading saved meal items: %w", err)
	}
	defer rows.Close()
	var out []savedItemRow
	for rows.Next() {
		var row savedItemRow
		if err := rows.Scan(&row.mealID, &row.input.Food.ID, &row.input.Serving.Label, &row.input.Serving.Grams, &row.input.Amount); err != nil {
			return nil, fmt.Errorf("scanning saved meal item: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading saved meal items: %w", err)
	}
	return out, nil
}

// resolveMealItems resolves each input to a food the athlete can use and one
// of its servings. A food that is unknown, another athlete's, or archived and
// not in held gets one shared answer naming the item, so nothing is revealed
// about foods the athlete cannot use.
func resolveMealItems(ctx context.Context, q querier, userID uuid.UUID, inputs []model.MealItemInput, held map[uuid.UUID]bool) ([]model.SavedMealItem, error) {
	ids := make([]uuid.UUID, 0, len(inputs))
	for _, input := range inputs {
		ids = append(ids, input.Food.ID)
	}
	foods, err := listFoodsByIDs(ctx, q, userID, ids)
	if err != nil {
		return nil, err
	}
	items := make([]model.SavedMealItem, 0, len(inputs))
	for i, input := range inputs {
		food, ok := foods[input.Food.ID]
		if !ok || (food.Archived && !held[food.ID]) {
			return nil, mealItemError(i, "food not available")
		}
		serving, err := model.ResolveServing(&food, input.Serving)
		if err != nil {
			return nil, mealItemError(i, "serving must be one of the food's servings or a grams-only serving")
		}
		items = append(items, model.NewSavedMealItem(food, serving, input.Amount))
	}
	return items, nil
}

// storedMealItem builds an item from a stored row. The serving is kept as it
// was saved; a food that no longer resolves yields an unavailable item.
func storedMealItem(foods map[uuid.UUID]model.Food, input model.MealItemInput) model.SavedMealItem {
	food, ok := foods[input.Food.ID]
	if !ok {
		return model.SavedMealItem{Food: model.Food{ID: input.Food.ID, Servings: []model.Serving{}}, Serving: input.Serving, Amount: input.Amount}
	}
	return model.NewSavedMealItem(food, input.Serving, input.Amount)
}

func insertSavedItems(ctx context.Context, tx pgx.Tx, userID, mealID uuid.UUID, items []model.SavedMealItem) error {
	for position, item := range items {
		var catalogID, userFoodID *uuid.UUID
		if item.Food.Source == model.FoodSourceOwn {
			userFoodID = &item.Food.ID
		} else {
			catalogID = &item.Food.ID
		}
		// The owner predicate keeps an item from ever attaching to a meal
		// the athlete does not own.
		tag, err := tx.Exec(ctx, `
			INSERT INTO saved_meal_items (id, meal_id, position, catalog_food_id, user_food_id, serving_label, serving_g, amount)
			SELECT $1, m.id, $4, $5, $6, $7, $8, $9
			FROM saved_meals m
			WHERE m.user_id=$2 AND m.id=$3`,
			uuid.New(), userID, mealID, position, catalogID, userFoodID, item.Serving.Label, item.Serving.Grams, item.Amount)
		if err != nil {
			return fmt.Errorf("inserting saved meal item: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return &model.NotFoundError{Message: "saved meal not found"}
		}
	}
	return nil
}

// checkSavedMealName refuses a name another of the athlete's meals already
// uses, ignoring case. The nutrition lock makes the check exact; the unique
// index is the backstop.
func checkSavedMealName(ctx context.Context, tx pgx.Tx, userID, exceptID uuid.UUID, name string) error {
	var taken bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM saved_meals
			WHERE user_id=$1 AND lower(name)=lower($2) AND id<>$3)`, userID, name, exceptID).Scan(&taken); err != nil {
		return fmt.Errorf("checking saved meal name: %w", err)
	}
	if taken {
		return savedMealNameTaken()
	}
	return nil
}

func savedMealNameTaken() error {
	return &model.ValidationError{Message: "another saved meal already has this name", Field: "name"}
}

// savedMealWriteError maps the name index violation to the same answer as
// the pre-check and wraps everything else.
func savedMealWriteError(action string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == savedMealNameIndex {
		return savedMealNameTaken()
	}
	return fmt.Errorf("%s: %w", action, err)
}

func mealItemError(index int, message string) error {
	return &model.ValidationError{Message: message, Field: "items", Index: &index}
}

func setMealItems(meal *model.SavedMeal, items []model.SavedMealItem) {
	if items == nil {
		items = []model.SavedMealItem{}
	}
	meal.Items = items
	meal.Totals = model.SumMealItems(items)
}

func itemFoodIDs(rows []savedItemRow) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.input.Food.ID)
	}
	return ids
}

func heldFoodIDs(rows []savedItemRow) map[uuid.UUID]bool {
	held := make(map[uuid.UUID]bool, len(rows))
	for _, row := range rows {
		held[row.input.Food.ID] = true
	}
	return held
}

func savedMealRecord(scope, operationKey, requestHash string, status int, meal *model.SavedMeal) model.IdempotencyRecord {
	record := model.IdempotencyRecord{Scope: scope, OperationKey: operationKey, RequestHash: requestHash, ResponseStatus: status, ResourceType: "saved_meal"}
	if meal != nil {
		record.ResourceID = &meal.ID
		record.ResourceRevision = &meal.Revision
	}
	return record
}

func scanSavedMeal(row rowScanner, meal *model.SavedMeal) error {
	return row.Scan(&meal.ID, &meal.Name, &meal.Revision, &meal.LastLoggedAt, &meal.CreatedAt, &meal.UpdatedAt)
}
