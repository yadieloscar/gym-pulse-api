package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// FoodDAO owns the shared catalog, private foods, barcode misses, and recents.
// Catalog rows carry no owner; every private-food and recents statement is
// scoped by user_id.
type FoodDAO interface {
	UpsertCatalogFoods(ctx context.Context, foods []model.Food) ([]model.Food, error)
	SearchCatalog(ctx context.Context, query string, limit int) ([]model.Food, error)
	// FindCatalogByGTIN and FindUserFoodByGTIN report found=false for a miss.
	FindCatalogByGTIN(ctx context.Context, gtin string) (*model.Food, bool, error)
	SearchUserFoods(ctx context.Context, userID uuid.UUID, query string, limit int) ([]model.Food, error)
	FindUserFoodByGTIN(ctx context.Context, userID uuid.UUID, gtin string) (*model.Food, bool, error)
	// GetFood resolves a catalog or own food by id for the athlete, archived
	// own foods included; anything else is the shared not-found error.
	GetFood(ctx context.Context, userID, id uuid.UUID) (*model.Food, error)
	ListFoodsByIDs(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]model.Food, error)
	RecordBarcodeMiss(ctx context.Context, gtin string, at time.Time) error
	IsRecentMiss(ctx context.Context, gtin string, since time.Time) (bool, error)
	ListRecentFoods(ctx context.Context, userID uuid.UUID, since time.Time, limit int) ([]model.RecentFood, error)
	CreateUserFood(ctx context.Context, userID uuid.UUID, req model.SaveUserFoodRequest, gtin *string, operationKey, requestHash string) (*model.Food, model.UserFoodOutcome, error)
	UpdateUserFood(ctx context.Context, userID, id uuid.UUID, req model.SaveUserFoodRequest, gtin *string, operationKey, requestHash string) (*model.Food, model.UserFoodOutcome, error)
	ArchiveUserFood(ctx context.Context, userID, id uuid.UUID, expectedRevision int64, operationKey, requestHash string) (model.UserFoodOutcome, error)
}

type foodDAO struct {
	pool *pgxpool.Pool
}

func NewFoodDAO(pool *pgxpool.Pool) FoodDAO {
	return &foodDAO{pool: pool}
}

const (
	userFoodCreateScope  = "user-foods/post"
	userFoodUpdateScope  = "user-foods/put"
	userFoodArchiveScope = "user-foods/delete"

	catalogColumns  = `id, source, source_id, data_type, gtin, name, brand, energy_kcal_100g, protein_g_100g, carbs_g_100g, fat_g_100g, servings, fetched_at`
	userFoodColumns = `id, name, brand, gtin, serving_label, serving_g, energy_kcal, protein_g, carbs_g, fat_g, archived_at, revision`
)

// userFoodLedger stores the outcome so a verbatim retry replays the status.
type userFoodLedger struct {
	Outcome model.UserFoodOutcome `json:"outcome"`
	Food    *model.Food           `json:"food"`
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func (r *foodDAO) UpsertCatalogFoods(ctx context.Context, foods []model.Food) ([]model.Food, error) {
	saved := make([]model.Food, 0, len(foods))
	for _, food := range foods {
		servings, err := json.Marshal(food.Servings)
		if err != nil {
			return nil, fmt.Errorf("encoding servings: %w", err)
		}
		if food.ID == uuid.Nil {
			food.ID = uuid.New()
		}
		var dataType, gtin *string
		if food.DataType != "" {
			dataType = &food.DataType
		}
		if food.GTIN != "" {
			gtin = &food.GTIN
		}
		var row model.Food
		err = scanCatalogFood(r.pool.QueryRow(ctx, `
			INSERT INTO food_catalog (id, source, source_id, data_type, gtin, name, brand,
				energy_kcal_100g, protein_g_100g, carbs_g_100g, fat_g_100g, servings, fetched_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			ON CONFLICT (source, source_id) DO UPDATE SET
				data_type=EXCLUDED.data_type, gtin=EXCLUDED.gtin, name=EXCLUDED.name, brand=EXCLUDED.brand,
				energy_kcal_100g=EXCLUDED.energy_kcal_100g, protein_g_100g=EXCLUDED.protein_g_100g,
				carbs_g_100g=EXCLUDED.carbs_g_100g, fat_g_100g=EXCLUDED.fat_g_100g,
				servings=EXCLUDED.servings, fetched_at=EXCLUDED.fetched_at, updated_at=now()
			RETURNING `+catalogColumns,
			food.ID, food.Source, food.SourceID, dataType, gtin, food.Name, food.Brand,
			food.Per100g.EnergyKcal, food.Per100g.ProteinG, food.Per100g.CarbsG, food.Per100g.FatG,
			string(servings), food.FetchedAt), &row)
		if err != nil {
			return nil, fmt.Errorf("upserting catalog food: %w", err)
		}
		saved = append(saved, row)
	}
	return saved, nil
}

func (r *foodDAO) SearchCatalog(ctx context.Context, query string, limit int) ([]model.Food, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+catalogColumns+`
		FROM food_catalog
		WHERE search_vector @@ websearch_to_tsquery('simple', $1)
		ORDER BY ts_rank(search_vector, websearch_to_tsquery('simple', $1)) DESC, name ASC
		LIMIT $2`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("searching food catalog: %w", err)
	}
	defer rows.Close()
	return collectCatalogFoods(rows)
}

func (r *foodDAO) FindCatalogByGTIN(ctx context.Context, gtin string) (*model.Food, bool, error) {
	var food model.Food
	err := scanCatalogFood(r.pool.QueryRow(ctx, `
		SELECT `+catalogColumns+`
		FROM food_catalog
		WHERE gtin=$1
		ORDER BY (source = 'usda') DESC, fetched_at DESC
		LIMIT 1`, gtin), &food)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("finding catalog food by barcode: %w", err)
	}
	return &food, true, nil
}

func (r *foodDAO) SearchUserFoods(ctx context.Context, userID uuid.UUID, query string, limit int) ([]model.Food, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+userFoodColumns+`
		FROM user_foods
		WHERE user_id=$1 AND archived_at IS NULL
		  AND (name ILIKE $2 || '%' OR name ILIKE '% ' || $2 || '%' OR brand ILIKE $2 || '%')
		ORDER BY name ASC
		LIMIT $3`, userID, query, limit)
	if err != nil {
		return nil, fmt.Errorf("searching user foods: %w", err)
	}
	defer rows.Close()
	foods := []model.Food{}
	for rows.Next() {
		var food model.Food
		if err := scanUserFood(rows, &food); err != nil {
			return nil, fmt.Errorf("scanning user food: %w", err)
		}
		foods = append(foods, food)
	}
	return foods, rows.Err()
}

func (r *foodDAO) FindUserFoodByGTIN(ctx context.Context, userID uuid.UUID, gtin string) (*model.Food, bool, error) {
	var food model.Food
	err := scanUserFood(r.pool.QueryRow(ctx, `
		SELECT `+userFoodColumns+`
		FROM user_foods
		WHERE user_id=$1 AND gtin=$2 AND archived_at IS NULL
		ORDER BY updated_at DESC
		LIMIT 1`, userID, gtin), &food)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("finding user food by barcode: %w", err)
	}
	return &food, true, nil
}

func (r *foodDAO) GetFood(ctx context.Context, userID, id uuid.UUID) (*model.Food, error) {
	return getFood(ctx, r.pool, userID, id)
}

func getFood(ctx context.Context, q querier, userID, id uuid.UUID) (*model.Food, error) {
	var food model.Food
	err := scanCatalogFood(q.QueryRow(ctx, `SELECT `+catalogColumns+` FROM food_catalog WHERE id=$1`, id), &food)
	if err == nil {
		return &food, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("reading catalog food: %w", err)
	}
	err = scanUserFood(q.QueryRow(ctx, `SELECT `+userFoodColumns+` FROM user_foods WHERE user_id=$1 AND id=$2`, userID, id), &food)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "food not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("reading user food: %w", err)
	}
	return &food, nil
}

func (r *foodDAO) ListFoodsByIDs(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]model.Food, error) {
	out := make(map[uuid.UUID]model.Food, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT `+catalogColumns+` FROM food_catalog WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("listing catalog foods: %w", err)
	}
	catalog, err := collectCatalogFoods(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	for _, food := range catalog {
		out[food.ID] = food
	}
	rows, err = r.pool.Query(ctx, `SELECT `+userFoodColumns+` FROM user_foods WHERE user_id=$1 AND id = ANY($2)`, userID, ids)
	if err != nil {
		return nil, fmt.Errorf("listing user foods: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var food model.Food
		if err := scanUserFood(rows, &food); err != nil {
			return nil, fmt.Errorf("scanning user food: %w", err)
		}
		out[food.ID] = food
	}
	return out, rows.Err()
}

func (r *foodDAO) RecordBarcodeMiss(ctx context.Context, gtin string, at time.Time) error {
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO food_barcode_misses (gtin, checked_at) VALUES ($1, $2)
		ON CONFLICT (gtin) DO UPDATE SET checked_at=EXCLUDED.checked_at`, gtin, at); err != nil {
		return fmt.Errorf("recording barcode miss: %w", err)
	}
	return nil
}

func (r *foodDAO) IsRecentMiss(ctx context.Context, gtin string, since time.Time) (bool, error) {
	var found bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM food_barcode_misses WHERE gtin=$1 AND checked_at >= $2)`, gtin, since).Scan(&found); err != nil {
		return false, fmt.Errorf("checking barcode miss: %w", err)
	}
	return found, nil
}

func (r *foodDAO) ListRecentFoods(ctx context.Context, userID uuid.UUID, since time.Time, limit int) ([]model.RecentFood, error) {
	rows, err := r.pool.Query(ctx, `
		WITH recent AS (
			SELECT COALESCE(catalog_food_id, user_food_id) AS food_id,
			       count(*) AS times_logged, max(logged_at) AS last_logged_at
			FROM food_entries
			WHERE user_id=$1 AND kind='food' AND logged_at >= $2
			GROUP BY 1
			ORDER BY times_logged DESC, last_logged_at DESC
			LIMIT $3
		)
		SELECT r.food_id, r.times_logged, r.last_logged_at, e.serving_label, e.serving_g, e.amount
		FROM recent r
		JOIN LATERAL (
			SELECT serving_label, serving_g, amount
			FROM food_entries
			WHERE user_id=$1 AND kind='food' AND COALESCE(catalog_food_id, user_food_id)=r.food_id
			ORDER BY logged_at DESC
			LIMIT 1
		) e ON true
		ORDER BY r.times_logged DESC, r.last_logged_at DESC`, userID, since, limit)
	if err != nil {
		return nil, fmt.Errorf("listing recent foods: %w", err)
	}
	defer rows.Close()
	recents := []model.RecentFood{}
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		var recent model.RecentFood
		if err := rows.Scan(&id, &recent.TimesLogged, &recent.LastLoggedAt, &recent.LastServing.Label, &recent.LastServing.Grams, &recent.LastAmount); err != nil {
			return nil, fmt.Errorf("scanning recent food: %w", err)
		}
		recent.Food.ID = id
		recents = append(recents, recent)
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading recent foods: %w", err)
	}
	foods, err := r.ListFoodsByIDs(ctx, userID, ids)
	if err != nil {
		return nil, err
	}
	out := make([]model.RecentFood, 0, len(recents))
	for _, recent := range recents {
		food, ok := foods[recent.Food.ID]
		if !ok {
			continue
		}
		recent.Food = food
		out = append(out, recent)
	}
	return out, nil
}

func (r *foodDAO) CreateUserFood(ctx context.Context, userID uuid.UUID, req model.SaveUserFoodRequest, gtin *string, operationKey, requestHash string) (*model.Food, model.UserFoodOutcome, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, model.UserFoodCreated, fmt.Errorf("beginning user food create: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, nutritionLockNamespace, userID); err != nil {
		return nil, model.UserFoodCreated, err
	}
	var replay userFoodLedger
	if found, err := findIdempotency(ctx, tx, userID, userFoodCreateScope, operationKey, requestHash, &replay); err != nil {
		return nil, model.UserFoodCreated, err
	} else if found {
		return replay.Food, replay.Outcome, nil
	}
	var food model.Food
	err = scanUserFood(tx.QueryRow(ctx, `
		INSERT INTO user_foods (id, user_id, name, brand, gtin, serving_label, serving_g, energy_kcal, protein_g, carbs_g, fat_g)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING `+userFoodColumns, uuid.New(), userID, req.Name, req.Brand, gtin, req.Serving.Label, req.Serving.Grams,
		req.EnergyKcal, req.ProteinG, req.CarbsG, req.FatG), &food)
	if err != nil {
		return nil, model.UserFoodCreated, fmt.Errorf("inserting user food: %w", err)
	}
	if err := insertIdempotency(ctx, tx, userID, userFoodRecord(userFoodCreateScope, operationKey, requestHash, 201, &food), userFoodLedger{Outcome: model.UserFoodCreated, Food: &food}); err != nil {
		return nil, model.UserFoodCreated, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, model.UserFoodCreated, fmt.Errorf("committing user food create: %w", err)
	}
	return &food, model.UserFoodCreated, nil
}

func (r *foodDAO) UpdateUserFood(ctx context.Context, userID, id uuid.UUID, req model.SaveUserFoodRequest, gtin *string, operationKey, requestHash string) (*model.Food, model.UserFoodOutcome, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, model.UserFoodReplaced, fmt.Errorf("beginning user food update: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, nutritionLockNamespace, userID); err != nil {
		return nil, model.UserFoodReplaced, err
	}
	var replay userFoodLedger
	if found, err := findIdempotency(ctx, tx, userID, userFoodUpdateScope, operationKey, requestHash, &replay); err != nil {
		return nil, model.UserFoodReplaced, err
	} else if found {
		return replay.Food, replay.Outcome, nil
	}
	existing, err := lockUserFood(ctx, tx, userID, id)
	if err != nil {
		return nil, model.UserFoodReplaced, err
	}
	if req.ExpectedRevision == nil || *req.ExpectedRevision != *existing.Revision {
		return nil, model.UserFoodReplaced, revisionConflict(req.ExpectedRevision, *existing.Revision, existing)
	}
	var food model.Food
	err = scanUserFood(tx.QueryRow(ctx, `
		UPDATE user_foods
		SET name=$3, brand=$4, gtin=$5, serving_label=$6, serving_g=$7, energy_kcal=$8, protein_g=$9, carbs_g=$10, fat_g=$11,
		    revision=revision+1, updated_at=now()
		WHERE user_id=$1 AND id=$2
		RETURNING `+userFoodColumns, userID, id, req.Name, req.Brand, gtin, req.Serving.Label, req.Serving.Grams,
		req.EnergyKcal, req.ProteinG, req.CarbsG, req.FatG), &food)
	if err != nil {
		return nil, model.UserFoodReplaced, fmt.Errorf("updating user food: %w", err)
	}
	if err := insertIdempotency(ctx, tx, userID, userFoodRecord(userFoodUpdateScope, operationKey, requestHash, 200, &food), userFoodLedger{Outcome: model.UserFoodReplaced, Food: &food}); err != nil {
		return nil, model.UserFoodReplaced, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, model.UserFoodReplaced, fmt.Errorf("committing user food update: %w", err)
	}
	return &food, model.UserFoodReplaced, nil
}

func (r *foodDAO) ArchiveUserFood(ctx context.Context, userID, id uuid.UUID, expectedRevision int64, operationKey, requestHash string) (model.UserFoodOutcome, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return model.UserFoodArchived, fmt.Errorf("beginning user food archive: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, nutritionLockNamespace, userID); err != nil {
		return model.UserFoodArchived, err
	}
	var replay userFoodLedger
	if found, err := findIdempotency(ctx, tx, userID, userFoodArchiveScope, operationKey, requestHash, &replay); err != nil {
		return model.UserFoodArchived, err
	} else if found {
		return replay.Outcome, nil
	}
	existing, err := lockUserFood(ctx, tx, userID, id)
	if err != nil {
		return model.UserFoodArchived, err
	}
	if expectedRevision != *existing.Revision {
		return model.UserFoodArchived, revisionConflict(&expectedRevision, *existing.Revision, existing)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE user_foods SET archived_at=COALESCE(archived_at, now()), revision=revision+1, updated_at=now()
		WHERE user_id=$1 AND id=$2`, userID, id); err != nil {
		return model.UserFoodArchived, fmt.Errorf("archiving user food: %w", err)
	}
	if err := insertIdempotency(ctx, tx, userID, userFoodRecord(userFoodArchiveScope, operationKey, requestHash, 204, existing), userFoodLedger{Outcome: model.UserFoodArchived}); err != nil {
		return model.UserFoodArchived, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.UserFoodArchived, fmt.Errorf("committing user food archive: %w", err)
	}
	return model.UserFoodArchived, nil
}

func lockUserFood(ctx context.Context, tx pgx.Tx, userID, id uuid.UUID) (*model.Food, error) {
	var food model.Food
	err := scanUserFood(tx.QueryRow(ctx, `SELECT `+userFoodColumns+` FROM user_foods WHERE user_id=$1 AND id=$2 FOR UPDATE`, userID, id), &food)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "food not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("locking user food: %w", err)
	}
	return &food, nil
}

func userFoodRecord(scope, operationKey, requestHash string, status int, food *model.Food) model.IdempotencyRecord {
	record := model.IdempotencyRecord{Scope: scope, OperationKey: operationKey, RequestHash: requestHash, ResponseStatus: status, ResourceType: "user_food"}
	if food != nil {
		record.ResourceID = &food.ID
		record.ResourceRevision = food.Revision
	}
	return record
}

// revisionConflict mirrors checkNutritionRevision for resources whose
// revision is never zero.
func revisionConflict(expected *int64, actual int64, authoritative any) error {
	if expected == nil {
		return &model.ValidationError{Message: "expected_revision is required", Field: "expected_revision"}
	}
	return &model.ConflictError{Message: "resource revision conflict", Expected: *expected, Actual: actual, Authoritative: authoritative}
}

func collectCatalogFoods(rows pgx.Rows) ([]model.Food, error) {
	foods := []model.Food{}
	for rows.Next() {
		var food model.Food
		if err := scanCatalogFood(rows, &food); err != nil {
			return nil, fmt.Errorf("scanning catalog food: %w", err)
		}
		foods = append(foods, food)
	}
	return foods, rows.Err()
}

func scanCatalogFood(row rowScanner, food *model.Food) error {
	var dataType, gtin *string
	var servings []byte
	if err := row.Scan(&food.ID, &food.Source, &food.SourceID, &dataType, &gtin, &food.Name, &food.Brand,
		&food.Per100g.EnergyKcal, &food.Per100g.ProteinG, &food.Per100g.CarbsG, &food.Per100g.FatG, &servings, &food.FetchedAt); err != nil {
		return err
	}
	if dataType != nil {
		food.DataType = *dataType
	}
	if gtin != nil {
		food.GTIN = *gtin
		display := model.DisplayGTIN(*gtin)
		food.Barcode = &display
	}
	if err := json.Unmarshal(servings, &food.Servings); err != nil {
		return fmt.Errorf("decoding servings: %w", err)
	}
	food.Servings = model.EnsureHundredGramServing(food.Servings)
	if food.Source == model.FoodSourceOpenFoodFacts {
		food.Attribution = model.OpenFoodFactsAttribution(food.SourceID)
	}
	return nil
}

func scanUserFood(row rowScanner, food *model.Food) error {
	var gtin *string
	var archivedAt *time.Time
	var serving model.Serving
	var energy int
	var protein, carbs, fat float64
	var revision int64
	if err := row.Scan(&food.ID, &food.Name, &food.Brand, &gtin, &serving.Label, &serving.Grams, &energy, &protein, &carbs, &fat, &archivedAt, &revision); err != nil {
		return err
	}
	food.Source = model.FoodSourceOwn
	food.Servings = model.EnsureHundredGramServing([]model.Serving{serving})
	food.Per100g = model.UserFoodPer100g(serving.Grams, energy, protein, carbs, fat)
	food.Revision = &revision
	food.Archived = archivedAt != nil
	if gtin != nil {
		food.GTIN = *gtin
		display := model.DisplayGTIN(*gtin)
		food.Barcode = &display
	}
	return nil
}
