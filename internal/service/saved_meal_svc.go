package service

import (
	"context"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// SavedMealService saves an athlete's own meals (named lists of foods with a
// serving and amount) and logs one to a day as ordinary food entries. It
// validates what the request alone can show; the DAO transaction resolves
// foods under the athlete's nutrition lock. See docs/CONTRACTS.md "Saved
// meals".
type SavedMealService interface {
	ListMeals(ctx context.Context, userID uuid.UUID) (*model.SavedMeals, error)
	CreateMeal(ctx context.Context, userID uuid.UUID, req model.SaveMealRequest) (*model.SavedMeal, model.SavedMealOutcome, error)
	UpdateMeal(ctx context.Context, userID, id uuid.UUID, req model.SaveMealRequest) (*model.SavedMeal, model.SavedMealOutcome, error)
	DeleteMeal(ctx context.Context, userID, id uuid.UUID, expectedRevision int64, operationKey string) (model.SavedMealOutcome, error)
	LogMeal(ctx context.Context, userID uuid.UUID, date string, req model.LogMealRequest) (*model.MealLogged, model.SavedMealOutcome, error)
}

type savedMealService struct {
	meals    dao.SavedMealDAO
	profiles dao.TrainingProfileDAO
	now      func() time.Time
}

// minServingGrams is the smallest serving weight that survives storage to one
// decimal as more than zero.
const minServingGrams = 0.1

func NewSavedMealService(meals dao.SavedMealDAO, profiles dao.TrainingProfileDAO) SavedMealService {
	return &savedMealService{meals: meals, profiles: profiles, now: time.Now}
}

func (s *savedMealService) ListMeals(ctx context.Context, userID uuid.UUID) (*model.SavedMeals, error) {
	meals, err := s.meals.ListMeals(ctx, userID)
	if err != nil {
		return nil, err
	}
	if meals == nil {
		meals = []model.SavedMeal{}
	}
	return &model.SavedMeals{Meals: meals}, nil
}

func (s *savedMealService) CreateMeal(ctx context.Context, userID uuid.UUID, req model.SaveMealRequest) (*model.SavedMeal, model.SavedMealOutcome, error) {
	if req.ExpectedRevision != nil {
		return nil, model.SavedMealCreated, &model.ValidationError{Message: "expected_revision is not allowed when creating a saved meal", Field: "expected_revision"}
	}
	if err := normalizeSaveMealRequest(&req); err != nil {
		return nil, model.SavedMealCreated, err
	}
	hash, err := hashPayload(req)
	if err != nil {
		return nil, model.SavedMealCreated, err
	}
	return s.meals.CreateMeal(ctx, userID, req, req.OperationKey, hash)
}

func (s *savedMealService) UpdateMeal(ctx context.Context, userID, id uuid.UUID, req model.SaveMealRequest) (*model.SavedMeal, model.SavedMealOutcome, error) {
	if err := normalizeSaveMealRequest(&req); err != nil {
		return nil, model.SavedMealReplaced, err
	}
	if req.ExpectedRevision == nil || *req.ExpectedRevision < 1 {
		return nil, model.SavedMealReplaced, &model.ValidationError{Message: "expected_revision must be 1 or greater", Field: "expected_revision"}
	}
	hash, err := hashPayload(struct {
		ID      uuid.UUID
		Request model.SaveMealRequest
	}{id, req})
	if err != nil {
		return nil, model.SavedMealReplaced, err
	}
	return s.meals.UpdateMeal(ctx, userID, id, req, req.OperationKey, hash)
}

func (s *savedMealService) DeleteMeal(ctx context.Context, userID, id uuid.UUID, expectedRevision int64, operationKey string) (model.SavedMealOutcome, error) {
	operationKey = strings.TrimSpace(operationKey)
	if err := validateNutritionOperationKey(operationKey); err != nil {
		return model.SavedMealRemoved, err
	}
	if expectedRevision < 1 {
		return model.SavedMealRemoved, &model.ValidationError{Message: "expected_revision must be 1 or greater", Field: "expected_revision"}
	}
	hash, err := hashPayload(struct {
		ID               uuid.UUID
		ExpectedRevision int64
	}{id, expectedRevision})
	if err != nil {
		return model.SavedMealRemoved, err
	}
	return s.meals.DeleteMeal(ctx, userID, id, expectedRevision, operationKey, hash)
}

func (s *savedMealService) LogMeal(ctx context.Context, userID uuid.UUID, date string, req model.LogMealRequest) (*model.MealLogged, model.SavedMealOutcome, error) {
	today, err := localTodayFromProfile(ctx, s.profiles, userID, s.now())
	if err != nil {
		return nil, model.SavedMealLogged, err
	}
	req.OperationKey = strings.TrimSpace(req.OperationKey)
	if err := validateNutritionDate(date, today, "date"); err != nil {
		return nil, model.SavedMealLogged, err
	}
	if err := validateNutritionOperationKey(req.OperationKey); err != nil {
		return nil, model.SavedMealLogged, err
	}
	if req.Meal == nil || req.Meal.ID == uuid.Nil {
		return nil, model.SavedMealLogged, &model.ValidationError{Message: "meal is required", Field: "meal"}
	}
	if err := validateMealSection(req.Section); err != nil {
		return nil, model.SavedMealLogged, err
	}
	// A present items list replaces the saved items for this log; an absent
	// one logs the meal as saved. An empty list is neither.
	if req.Items != nil {
		if err := normalizeMealItems(req.Items); err != nil {
			return nil, model.SavedMealLogged, err
		}
	}
	hash, err := hashPayload(struct {
		Date    string
		Request model.LogMealRequest
	}{date, req})
	if err != nil {
		return nil, model.SavedMealLogged, err
	}
	loggedAt := s.now()
	if req.LoggedAt != nil {
		loggedAt = *req.LoggedAt
	}
	return s.meals.LogMeal(ctx, userID, req.Meal.ID, date, req.Items, req.Section, loggedAt, req.OperationKey, hash)
}

// normalizeSaveMealRequest trims and validates what the request alone can
// show: the operation key, the name, and the shape of the items.
func normalizeSaveMealRequest(req *model.SaveMealRequest) error {
	req.OperationKey = strings.TrimSpace(req.OperationKey)
	if err := validateNutritionOperationKey(req.OperationKey); err != nil {
		return err
	}
	req.Name = strings.TrimSpace(req.Name)
	if length := utf8.RuneCountInString(req.Name); length < 1 || length > model.MaxSavedMealNameLength {
		return &model.ValidationError{Message: "name must be between 1 and 60 characters", Field: "name"}
	}
	if err := validateMealSection(req.Section); err != nil {
		return err
	}
	return normalizeMealItems(req.Items)
}

// normalizeMealItems validates 1 to 20 items in place. A fault in one item
// names its zero-based index.
func normalizeMealItems(items []model.MealItemInput) error {
	if len(items) < 1 || len(items) > model.MaxSavedMealItems {
		return &model.ValidationError{Message: "items must hold between 1 and 20 foods", Field: "items"}
	}
	type foodServing struct {
		food  uuid.UUID
		label string
		grams float64
	}
	seen := make(map[foodServing]bool, len(items))
	for i := range items {
		item := &items[i]
		itemError := func(message string) error {
			index := i
			return &model.ValidationError{Message: message, Field: "items", Index: &index}
		}
		if item.Food.ID == uuid.Nil {
			return itemError("food is required")
		}
		item.Serving.Label = strings.TrimSpace(item.Serving.Label)
		if length := utf8.RuneCountInString(item.Serving.Label); length < 1 || length > 60 {
			return itemError("serving label must be between 1 and 60 characters")
		}
		// Weights are stored to one decimal, so the bounds and the duplicate
		// check below use the weight as it will be stored.
		storedGrams := math.Round(item.Serving.Grams*10) / 10
		if math.IsNaN(item.Serving.Grams) || storedGrams < minServingGrams || item.Serving.Grams > maxServingGrams {
			return itemError("serving grams must be between 0.1 and 5000")
		}
		if err := validateEntryAmount(&item.Amount); err != nil {
			return itemError(err.Error())
		}
		key := foodServing{food: item.Food.ID, label: item.Serving.Label, grams: storedGrams}
		if seen[key] {
			return itemError("the same food and serving is already in this meal")
		}
		seen[key] = true
	}
	return nil
}
