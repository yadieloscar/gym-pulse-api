package service

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// FoodService validates food lookups and entry writes, merges the outside
// sources with the catalog, and delegates every write to the DAOs.
type FoodService interface {
	Search(ctx context.Context, userID uuid.UUID, query string, limit int) (*model.FoodSearchResult, error)
	LookupBarcode(ctx context.Context, userID uuid.UUID, code string) (*model.Food, error)
	ListRecent(ctx context.Context, userID uuid.UUID) (*model.RecentFoods, error)
	CreateUserFood(ctx context.Context, userID uuid.UUID, req model.SaveUserFoodRequest) (*model.Food, model.UserFoodOutcome, error)
	UpdateUserFood(ctx context.Context, userID, id uuid.UUID, req model.SaveUserFoodRequest) (*model.Food, model.UserFoodOutcome, error)
	ArchiveUserFood(ctx context.Context, userID, id uuid.UUID, expectedRevision int64, operationKey string) (model.UserFoodOutcome, error)
	ListDayEntries(ctx context.Context, userID uuid.UUID, date string) (*model.DayEntries, error)
	CreateEntry(ctx context.Context, userID uuid.UUID, date string, req model.CreateFoodEntryRequest) (*model.FoodEntryCreated, model.FoodEntryOutcome, error)
	UpdateEntry(ctx context.Context, userID, id uuid.UUID, req model.UpdateFoodEntryRequest) (*model.FoodEntryUpdated, model.FoodEntryOutcome, error)
	DeleteEntry(ctx context.Context, userID, id uuid.UUID, expectedRevision int64, operationKey string) (*model.FoodEntryDeleted, model.FoodEntryOutcome, error)
}

type foodService struct {
	foods    dao.FoodDAO
	entries  dao.FoodEntryDAO
	profiles dao.TrainingProfileDAO
	usda     FoodSource
	off      FoodSource
	now      func() time.Time
}

// NewFoodService wires the DAOs and the two outside sources (USDA first, Open
// Food Facts as the barcode fallback).
func NewFoodService(foods dao.FoodDAO, entries dao.FoodEntryDAO, profiles dao.TrainingProfileDAO, usda, off FoodSource) FoodService {
	return &foodService{foods: foods, entries: entries, profiles: profiles, usda: usda, off: off, now: time.Now}
}

const (
	foodSearchMaxResults = 25
	foodSearchMinQuery   = 2
	foodSearchMaxQuery   = 100
	catalogRefreshAge    = 90 * 24 * time.Hour
	barcodeMissTTL       = 7 * 24 * time.Hour
	recentFoodsWindow    = 30 * 24 * time.Hour
	recentFoodsLimit     = 20
	maxEntryAmount       = 50
	minEntryAmount       = 0.1
	maxServingGrams      = 5000
)

// localTodayFromProfile returns the athlete's current calendar date in their
// saved training-profile timezone as YYYY-MM-DD.
func localTodayFromProfile(ctx context.Context, profiles dao.TrainingProfileDAO, userID uuid.UUID, now time.Time) (string, error) {
	profile, err := profiles.Get(ctx, userID)
	if err != nil {
		return "", err
	}
	location, err := time.LoadLocation(profile.Timezone)
	if err != nil {
		return "", &model.ValidationError{Message: "invalid saved timezone", Field: "timezone"}
	}
	return now.In(location).Format("2006-01-02"), nil
}

func (s *foodService) Search(ctx context.Context, userID uuid.UUID, query string, limit int) (*model.FoodSearchResult, error) {
	query = strings.TrimSpace(query)
	if n := len([]rune(query)); n < foodSearchMinQuery || n > foodSearchMaxQuery {
		return nil, &model.ValidationError{Message: "q must be between 2 and 100 characters", Field: "q"}
	}
	if limit <= 0 || limit > foodSearchMaxResults {
		return nil, &model.ValidationError{Message: "limit must be between 1 and 25", Field: "limit"}
	}
	result := &model.FoodSearchResult{Foods: []model.Food{}}
	own, err := s.foods.SearchUserFoods(ctx, userID, query, limit)
	if err != nil {
		return nil, err
	}
	var fetched []model.Food
	sourceCtx, cancel := context.WithTimeout(ctx, foodSourceTimeout)
	outside, err := s.usda.Search(sourceCtx, query, limit)
	cancel()
	switch {
	case err == nil:
		if len(outside) > 0 {
			if fetched, err = s.foods.UpsertCatalogFoods(ctx, outside); err != nil {
				return nil, err
			}
		}
	case errors.Is(err, ErrSourceUnavailable), errors.Is(err, context.DeadlineExceeded):
		result.Incomplete = true
	default:
		slog.Warn("food search source failed", "error", err)
		result.Incomplete = true
	}
	local, err := s.foods.SearchCatalog(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	add := func(foods []model.Food) {
		for _, food := range foods {
			key := string(food.Source) + ":" + food.SourceID
			if food.Source == model.FoodSourceOwn {
				key = "own:" + food.ID.String()
			}
			if seen[key] || len(result.Foods) >= limit {
				continue
			}
			seen[key] = true
			result.Foods = append(result.Foods, food)
		}
	}
	add(own)
	add(fetched)
	add(local)
	return result, nil
}

func (s *foodService) LookupBarcode(ctx context.Context, userID uuid.UUID, code string) (*model.Food, error) {
	gtin, err := model.NormalizeGTIN(code)
	if err != nil {
		return nil, err
	}
	if own, found, err := s.foods.FindUserFoodByGTIN(ctx, userID, gtin); err != nil {
		return nil, err
	} else if found {
		return own, nil
	}
	cached, found, err := s.foods.FindCatalogByGTIN(ctx, gtin)
	if err != nil {
		return nil, err
	}
	if found {
		if s.now().Sub(cached.FetchedAt) > catalogRefreshAge {
			if refreshed, _, _ := s.lookupSources(ctx, gtin); refreshed != nil {
				return s.storeFood(ctx, refreshed)
			}
		}
		return cached, nil
	}
	if missed, err := s.foods.IsRecentMiss(ctx, gtin, s.now().Add(-barcodeMissTTL)); err != nil {
		return nil, err
	} else if missed {
		return nil, &model.NotFoundError{Message: "food not found"}
	}
	sourced, answered, err := s.lookupSources(ctx, gtin)
	if err != nil {
		return nil, err
	}
	if sourced != nil {
		return s.storeFood(ctx, sourced)
	}
	if answered {
		if err := s.foods.RecordBarcodeMiss(ctx, gtin, s.now()); err != nil {
			return nil, err
		}
	}
	return nil, &model.NotFoundError{Message: "food not found"}
}

// lookupSources asks USDA then Open Food Facts. answered is true only when
// both sources were reachable and said no, so a miss is worth remembering. A
// spent fallback budget is returned as FoodLookupBudgetError.
func (s *foodService) lookupSources(ctx context.Context, gtin string) (found *model.Food, answered bool, err error) {
	answered = true
	sourceCtx, cancel := context.WithTimeout(ctx, foodSourceTimeout)
	defer cancel()
	food, hit, usdaErr := s.usda.LookupBarcode(sourceCtx, gtin)
	if usdaErr == nil && hit {
		return food, true, nil
	}
	if usdaErr != nil {
		answered = false
	}
	food, hit, offErr := s.off.LookupBarcode(sourceCtx, gtin)
	var budget *BudgetSpentError
	switch {
	case offErr == nil && hit:
		return food, true, nil
	case errors.As(offErr, &budget):
		return nil, false, &model.FoodLookupBudgetError{RetryAfterSeconds: int(math.Ceil(budget.RetryAfter.Seconds()))}
	case offErr != nil:
		answered = false
	}
	return nil, answered, nil
}

func (s *foodService) storeFood(ctx context.Context, food *model.Food) (*model.Food, error) {
	saved, err := s.foods.UpsertCatalogFoods(ctx, []model.Food{*food})
	if err != nil {
		return nil, err
	}
	return &saved[0], nil
}

func (s *foodService) ListRecent(ctx context.Context, userID uuid.UUID) (*model.RecentFoods, error) {
	foods, err := s.foods.ListRecentFoods(ctx, userID, s.now().Add(-recentFoodsWindow), recentFoodsLimit)
	if err != nil {
		return nil, err
	}
	if foods == nil {
		foods = []model.RecentFood{}
	}
	return &model.RecentFoods{Foods: foods}, nil
}

func (s *foodService) CreateUserFood(ctx context.Context, userID uuid.UUID, req model.SaveUserFoodRequest) (*model.Food, model.UserFoodOutcome, error) {
	gtin, err := normalizeUserFoodRequest(&req)
	if err != nil {
		return nil, model.UserFoodCreated, err
	}
	hash, err := hashPayload(req)
	if err != nil {
		return nil, model.UserFoodCreated, err
	}
	return s.foods.CreateUserFood(ctx, userID, req, gtin, req.OperationKey, hash)
}

func (s *foodService) UpdateUserFood(ctx context.Context, userID, id uuid.UUID, req model.SaveUserFoodRequest) (*model.Food, model.UserFoodOutcome, error) {
	gtin, err := normalizeUserFoodRequest(&req)
	if err != nil {
		return nil, model.UserFoodReplaced, err
	}
	if req.ExpectedRevision == nil || *req.ExpectedRevision < 1 {
		return nil, model.UserFoodReplaced, &model.ValidationError{Message: "expected_revision must be 1 or greater", Field: "expected_revision"}
	}
	hash, err := hashPayload(struct {
		ID      uuid.UUID
		Request model.SaveUserFoodRequest
	}{id, req})
	if err != nil {
		return nil, model.UserFoodReplaced, err
	}
	return s.foods.UpdateUserFood(ctx, userID, id, req, gtin, req.OperationKey, hash)
}

func (s *foodService) ArchiveUserFood(ctx context.Context, userID, id uuid.UUID, expectedRevision int64, operationKey string) (model.UserFoodOutcome, error) {
	operationKey = strings.TrimSpace(operationKey)
	if err := validateNutritionOperationKey(operationKey); err != nil {
		return model.UserFoodArchived, err
	}
	if expectedRevision < 1 {
		return model.UserFoodArchived, &model.ValidationError{Message: "expected_revision must be 1 or greater", Field: "expected_revision"}
	}
	hash, err := hashPayload(struct {
		ID               uuid.UUID
		ExpectedRevision int64
	}{id, expectedRevision})
	if err != nil {
		return model.UserFoodArchived, err
	}
	return s.foods.ArchiveUserFood(ctx, userID, id, expectedRevision, operationKey, hash)
}

func (s *foodService) ListDayEntries(ctx context.Context, userID uuid.UUID, date string) (*model.DayEntries, error) {
	if _, err := model.ParseDate(date); err != nil {
		return nil, &model.ValidationError{Message: "date must be YYYY-MM-DD", Field: "date"}
	}
	return s.entries.ListDayEntries(ctx, userID, date)
}

func (s *foodService) CreateEntry(ctx context.Context, userID uuid.UUID, date string, req model.CreateFoodEntryRequest) (*model.FoodEntryCreated, model.FoodEntryOutcome, error) {
	today, err := localTodayFromProfile(ctx, s.profiles, userID, s.now())
	if err != nil {
		return nil, model.FoodEntryCreatedOutcome, err
	}
	req.OperationKey = strings.TrimSpace(req.OperationKey)
	if err := validateNutritionDate(date, today, "date"); err != nil {
		return nil, model.FoodEntryCreatedOutcome, err
	}
	if err := validateNutritionOperationKey(req.OperationKey); err != nil {
		return nil, model.FoodEntryCreatedOutcome, err
	}
	if (req.Food == nil) == (req.QuickAdd == nil) {
		return nil, model.FoodEntryCreatedOutcome, &model.ValidationError{Message: "send exactly one of food or quick_add", Field: "food"}
	}
	hash, err := hashPayload(struct {
		Date    string
		Request model.CreateFoodEntryRequest
	}{date, req})
	if err != nil {
		return nil, model.FoodEntryCreatedOutcome, err
	}
	entry := &model.FoodEntry{Date: date}
	if req.LoggedAt != nil {
		entry.LoggedAt = *req.LoggedAt
	} else {
		entry.LoggedAt = s.now()
	}
	if req.Food != nil {
		if req.Serving == nil {
			return nil, model.FoodEntryCreatedOutcome, &model.ValidationError{Message: "serving is required with food", Field: "serving"}
		}
		if err := validateEntryAmount(req.Amount); err != nil {
			return nil, model.FoodEntryCreatedOutcome, err
		}
		food, err := s.foods.GetFood(ctx, userID, req.Food.ID)
		if err != nil {
			return nil, model.FoodEntryCreatedOutcome, err
		}
		if food.Archived {
			return nil, model.FoodEntryCreatedOutcome, &model.NotFoundError{Message: "food not found"}
		}
		serving, err := model.ResolveServing(food, *req.Serving)
		if err != nil {
			return nil, model.FoodEntryCreatedOutcome, err
		}
		energy, protein, carbs, fat := model.EntryValues(food.Per100g, serving.Grams, *req.Amount)
		entry.Kind = model.FoodEntryKindFood
		entry.Serving, entry.Amount = &serving, req.Amount
		entry.EnergyKcal, entry.ProteinG, entry.CarbsG, entry.FatG = &energy, &protein, &carbs, &fat
		if food.Source == model.FoodSourceOwn {
			entry.UserFoodID = &food.ID
		} else {
			entry.CatalogFoodID = &food.ID
		}
	} else {
		if err := validateQuickAdd(req.QuickAdd); err != nil {
			return nil, model.FoodEntryCreatedOutcome, err
		}
		entry.Kind = model.FoodEntryKindQuickAdd
		entry.EnergyKcal = req.QuickAdd.EnergyKcal
		entry.ProteinG, entry.CarbsG, entry.FatG = roundedGrams(req.QuickAdd.ProteinG), roundedGrams(req.QuickAdd.CarbsG), roundedGrams(req.QuickAdd.FatG)
	}
	return s.entries.CreateEntry(ctx, userID, entry, req.OperationKey, hash)
}

func (s *foodService) UpdateEntry(ctx context.Context, userID, id uuid.UUID, req model.UpdateFoodEntryRequest) (*model.FoodEntryUpdated, model.FoodEntryOutcome, error) {
	req.OperationKey = strings.TrimSpace(req.OperationKey)
	if err := validateNutritionOperationKey(req.OperationKey); err != nil {
		return nil, model.FoodEntryReplacedOutcome, err
	}
	if req.ExpectedRevision < 1 {
		return nil, model.FoodEntryReplacedOutcome, &model.ValidationError{Message: "expected_revision must be 1 or greater", Field: "expected_revision"}
	}
	hasServing := req.Serving != nil || req.Amount != nil
	if hasServing == (req.QuickAdd != nil) {
		return nil, model.FoodEntryReplacedOutcome, &model.ValidationError{Message: "send serving and amount, or quick_add", Field: "serving"}
	}
	if hasServing {
		if req.Serving == nil {
			return nil, model.FoodEntryReplacedOutcome, &model.ValidationError{Message: "serving is required with amount", Field: "serving"}
		}
		if err := validateEntryAmount(req.Amount); err != nil {
			return nil, model.FoodEntryReplacedOutcome, err
		}
	} else {
		if err := validateQuickAdd(req.QuickAdd); err != nil {
			return nil, model.FoodEntryReplacedOutcome, err
		}
		req.QuickAdd.ProteinG, req.QuickAdd.CarbsG, req.QuickAdd.FatG = roundedGrams(req.QuickAdd.ProteinG), roundedGrams(req.QuickAdd.CarbsG), roundedGrams(req.QuickAdd.FatG)
	}
	hash, err := hashPayload(struct {
		ID      uuid.UUID
		Request model.UpdateFoodEntryRequest
	}{id, req})
	if err != nil {
		return nil, model.FoodEntryReplacedOutcome, err
	}
	return s.entries.UpdateEntry(ctx, userID, id, req, req.OperationKey, hash)
}

func (s *foodService) DeleteEntry(ctx context.Context, userID, id uuid.UUID, expectedRevision int64, operationKey string) (*model.FoodEntryDeleted, model.FoodEntryOutcome, error) {
	operationKey = strings.TrimSpace(operationKey)
	if err := validateNutritionOperationKey(operationKey); err != nil {
		return nil, model.FoodEntryRemovedOutcome, err
	}
	if expectedRevision < 1 {
		return nil, model.FoodEntryRemovedOutcome, &model.ValidationError{Message: "expected_revision must be 1 or greater", Field: "expected_revision"}
	}
	hash, err := hashPayload(struct {
		ID               uuid.UUID
		ExpectedRevision int64
	}{id, expectedRevision})
	if err != nil {
		return nil, model.FoodEntryRemovedOutcome, err
	}
	return s.entries.DeleteEntry(ctx, userID, id, expectedRevision, operationKey, hash)
}

// normalizeUserFoodRequest trims and validates a label as typed, returning the
// normalised GTIN when a barcode was given.
func normalizeUserFoodRequest(req *model.SaveUserFoodRequest) (*string, error) {
	req.OperationKey = strings.TrimSpace(req.OperationKey)
	if err := validateNutritionOperationKey(req.OperationKey); err != nil {
		return nil, err
	}
	req.Name = strings.TrimSpace(req.Name)
	if n := len([]rune(req.Name)); n < 1 || n > 200 {
		return nil, &model.ValidationError{Message: "name must be between 1 and 200 characters", Field: "name"}
	}
	if req.Brand != nil {
		brand := strings.TrimSpace(*req.Brand)
		switch {
		case brand == "":
			req.Brand = nil
		case len([]rune(brand)) > 120:
			return nil, &model.ValidationError{Message: "brand must be at most 120 characters", Field: "brand"}
		default:
			req.Brand = &brand
		}
	}
	var gtin *string
	if req.Barcode != nil {
		code := strings.TrimSpace(*req.Barcode)
		switch normalized, err := model.NormalizeGTIN(code); {
		case code == "":
			req.Barcode = nil
		case err != nil:
			var validation *model.ValidationError
			if errors.As(err, &validation) {
				return nil, &model.ValidationError{Message: validation.Message, Field: "barcode"}
			}
			return nil, err
		default:
			gtin = &normalized
			req.Barcode = &code
		}
	}
	req.Serving.Label = strings.TrimSpace(req.Serving.Label)
	if n := len([]rune(req.Serving.Label)); n < 1 || n > 60 {
		return nil, &model.ValidationError{Message: "serving label must be between 1 and 60 characters", Field: "serving"}
	}
	if g := req.Serving.Grams; math.IsNaN(g) || math.IsInf(g, 0) || g <= 0 || g > maxServingGrams || math.Abs(g*10-math.Round(g*10)) > 1e-6 {
		return nil, &model.ValidationError{Message: "serving grams must be between 0 and 5,000 with at most one decimal", Field: "serving"}
	}
	energy := req.EnergyKcal
	if err := validateEnergy(&energy, 0); err != nil {
		return nil, err
	}
	for _, macro := range []struct {
		field string
		value *float64
	}{{"protein_g", &req.ProteinG}, {"carbs_g", &req.CarbsG}, {"fat_g", &req.FatG}} {
		if err := validateGrams(macro.value, macro.field, false); err != nil {
			return nil, err
		}
	}
	return gtin, nil
}

func validateEntryAmount(amount *float64) error {
	if amount == nil {
		return &model.ValidationError{Message: "amount is required with food", Field: "amount"}
	}
	a := *amount
	if math.IsNaN(a) || math.IsInf(a, 0) || a < minEntryAmount || a > maxEntryAmount {
		return &model.ValidationError{Message: "amount must be between 0.1 and 50", Field: "amount"}
	}
	if math.Abs(a*100-math.Round(a*100)) > 1e-6 {
		return &model.ValidationError{Message: "amount allows at most two decimals", Field: "amount"}
	}
	return nil
}

func validateQuickAdd(quick *model.QuickAdd) error {
	if quick == nil || !quick.HasValue() {
		return &model.ValidationError{Message: "quick_add needs at least one value", Field: "quick_add"}
	}
	if err := validateEnergy(quick.EnergyKcal, 0); err != nil {
		return err
	}
	for _, macro := range []struct {
		field string
		value *float64
	}{{"protein_g", quick.ProteinG}, {"carbs_g", quick.CarbsG}, {"fat_g", quick.FatG}} {
		if err := validateGrams(macro.value, macro.field, false); err != nil {
			return err
		}
	}
	return nil
}
