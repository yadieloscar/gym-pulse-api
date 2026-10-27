package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type foodHandlerService struct {
	err        error
	query      string
	limit      int
	code       string
	deletedRev int64
	deletedKey string
	id         uuid.UUID
}

func (s *foodHandlerService) Search(_ context.Context, _ uuid.UUID, query string, limit int) (*model.FoodSearchResult, error) {
	s.query, s.limit = query, limit
	if s.err != nil {
		return nil, s.err
	}
	return &model.FoodSearchResult{Foods: []model.Food{}, Incomplete: true}, nil
}
func (s *foodHandlerService) LookupBarcode(_ context.Context, _ uuid.UUID, code string) (*model.Food, error) {
	s.code = code
	if s.err != nil {
		return nil, s.err
	}
	return &model.Food{Source: model.FoodSourceUSDA, Servings: []model.Serving{model.HundredGramServing}}, nil
}
func (s *foodHandlerService) ListRecent(context.Context, uuid.UUID) (*model.RecentFoods, error) {
	return &model.RecentFoods{Foods: []model.RecentFood{}}, s.err
}
func (s *foodHandlerService) CreateUserFood(context.Context, uuid.UUID, model.SaveUserFoodRequest) (*model.Food, model.UserFoodOutcome, error) {
	return &model.Food{Source: model.FoodSourceOwn}, model.UserFoodCreated, s.err
}
func (s *foodHandlerService) UpdateUserFood(_ context.Context, _, id uuid.UUID, _ model.SaveUserFoodRequest) (*model.Food, model.UserFoodOutcome, error) {
	s.id = id
	return &model.Food{Source: model.FoodSourceOwn}, model.UserFoodReplaced, s.err
}
func (s *foodHandlerService) ArchiveUserFood(_ context.Context, _, id uuid.UUID, rev int64, key string) (model.UserFoodOutcome, error) {
	s.id, s.deletedRev, s.deletedKey = id, rev, key
	return model.UserFoodArchived, s.err
}
func (s *foodHandlerService) ListDayEntries(context.Context, uuid.UUID, string) (*model.DayEntries, error) {
	return &model.DayEntries{Entries: []model.FoodEntry{}}, s.err
}
func (s *foodHandlerService) CreateEntry(context.Context, uuid.UUID, string, model.CreateFoodEntryRequest) (*model.FoodEntryCreated, model.FoodEntryOutcome, error) {
	if s.err != nil {
		return nil, model.FoodEntryCreatedOutcome, s.err
	}
	return &model.FoodEntryCreated{Converted: []model.FoodEntry{}}, model.FoodEntryCreatedOutcome, nil
}
func (s *foodHandlerService) UpdateEntry(context.Context, uuid.UUID, uuid.UUID, model.UpdateFoodEntryRequest) (*model.FoodEntryUpdated, model.FoodEntryOutcome, error) {
	return &model.FoodEntryUpdated{}, model.FoodEntryReplacedOutcome, s.err
}
func (s *foodHandlerService) DeleteEntry(_ context.Context, _, id uuid.UUID, rev int64, key string) (*model.FoodEntryDeleted, model.FoodEntryOutcome, error) {
	s.id, s.deletedRev, s.deletedKey = id, rev, key
	if s.err != nil {
		return nil, model.FoodEntryRemovedOutcome, s.err
	}
	return &model.FoodEntryDeleted{}, model.FoodEntryRemovedOutcome, nil
}

func TestFoodHandlerSearchParsesLimit(t *testing.T) {
	svc := &foodHandlerService{}
	h := NewFoodHandler(svc)
	rec := httptest.NewRecorder()
	h.Search(rec, guidedHandlerRequest(t, http.MethodGet, "/?q=chicken", nil, uuid.New(), "", nil))
	if rec.Code != http.StatusOK || svc.query != "chicken" || svc.limit != 25 {
		t.Fatalf("default limit 25: %d %q %d", rec.Code, svc.query, svc.limit)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if foods, ok := body["foods"].([]any); !ok || len(foods) != 0 || body["incomplete"] != true {
		t.Errorf("envelope: %s", rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.Search(rec, guidedHandlerRequest(t, http.MethodGet, "/?q=chicken&limit=5", nil, uuid.New(), "", nil))
	if svc.limit != 5 {
		t.Errorf("limit parsed: %d", svc.limit)
	}
	rec = httptest.NewRecorder()
	h.Search(rec, guidedHandlerRequest(t, http.MethodGet, "/?q=chicken&limit=five", nil, uuid.New(), "", nil))
	if rec.Code != http.StatusUnprocessableEntity || !containsField(rec, "limit") {
		t.Errorf("bad limit → 422 limit: %d %s", rec.Code, rec.Body.String())
	}
}

func containsField(rec *httptest.ResponseRecorder, field string) bool {
	var body struct {
		Code    string            `json:"code"`
		Details map[string]string `json:"details"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return body.Code == "VALIDATION_ERROR" && body.Details["field"] == field
}

func TestFoodHandlerErrorMapping(t *testing.T) {
	day := &model.NutritionDay{ID: uuid.New(), Source: model.NutritionSourceFoods, Revision: 4}
	rec := httptest.NewRecorder()
	handleServiceError(rec, &model.DayHasFoodEntriesError{Day: day})
	var body struct {
		Code     string              `json:"code"`
		Resource *model.NutritionDay `json:"resource"`
		Details  map[string]int      `json:"details"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusConflict || body.Code != "DAY_HAS_FOOD_ENTRIES" || body.Resource == nil || body.Resource.ID != day.ID {
		t.Fatalf("foods day → 409 DAY_HAS_FOOD_ENTRIES with the day: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	handleServiceError(rec, &model.FoodLookupBudgetError{RetryAfterSeconds: 41})
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusNotFound || body.Code != "NOT_FOUND" || body.Details["retry_after_seconds"] != 41 {
		t.Fatalf("budget → 404 with retry_after_seconds: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	handleServiceError(rec, &model.ConflictError{Message: "resource revision conflict", Expected: 1, Actual: 2, Authoritative: &model.FoodEntry{Revision: 2}})
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusConflict || body.Code != "REVISION_CONFLICT" {
		t.Fatalf("entry revision conflict keeps REVISION_CONFLICT: %s", rec.Body.String())
	}
	rec = httptest.NewRecorder()
	handleServiceError(rec, &model.ConflictError{Message: "training profile changed during transition"})
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Code != "CONFLICT" {
		t.Fatalf("other domains keep their code: %s", rec.Body.String())
	}
}

func TestFoodHandlerMutationsAndDeleteParams(t *testing.T) {
	svc := &foodHandlerService{}
	h := NewFoodHandler(svc)
	rec := httptest.NewRecorder()
	h.CreateUserFood(rec, guidedHandlerRequest(t, http.MethodPost, "/", map[string]any{"name": "Whey", "serving": map[string]any{"label": "1 scoop", "grams": 30}, "energy_kcal": 120, "protein_g": 24, "carbs_g": 3, "fat_g": 1.5, "operation_key": "uf-1"}, uuid.New(), "uf-1", nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create → 201: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.CreateUserFood(rec, guidedHandlerRequest(t, http.MethodPost, "/", map[string]any{"name": "Whey", "operation_key": "uf-1"}, uuid.New(), "other", nil))
	if rec.Code != http.StatusUnprocessableEntity || !containsField(rec, "Idempotency-Key") {
		t.Fatalf("header/body mismatch → 422: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.UpdateUserFood(rec, guidedHandlerRequest(t, http.MethodPut, "/", map[string]any{"operation_key": "x"}, uuid.New(), "x", map[string]string{"id": "not-a-uuid"}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("malformed id → shared 404: %d", rec.Code)
	}
	id := uuid.New()
	rec = httptest.NewRecorder()
	h.ArchiveUserFood(rec, guidedHandlerRequest(t, http.MethodDelete, "/?expected_revision=3", nil, uuid.New(), "del-1", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusNoContent || svc.id != id || svc.deletedRev != 3 || svc.deletedKey != "del-1" {
		t.Fatalf("archive → 204 with query revision and header key: %d %+v", rec.Code, svc)
	}
	rec = httptest.NewRecorder()
	h.ArchiveUserFood(rec, guidedHandlerRequest(t, http.MethodDelete, "/", nil, uuid.New(), "del-1", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusUnprocessableEntity || !containsField(rec, "expected_revision") {
		t.Fatalf("missing expected_revision → 422: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.DeleteEntry(rec, guidedHandlerRequest(t, http.MethodDelete, "/?expected_revision=2", nil, uuid.New(), "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusUnprocessableEntity || !containsField(rec, "Idempotency-Key") {
		t.Fatalf("missing header → 422: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.DeleteEntry(rec, guidedHandlerRequest(t, http.MethodDelete, "/?expected_revision=2", nil, uuid.New(), "del-2", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK || rec.Body.String() != "{\"day\":null}\n" {
		t.Fatalf("delete → 200 {day:null}: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.CreateEntry(rec, guidedHandlerRequest(t, http.MethodPost, "/", map[string]any{"quick_add": map[string]any{"protein_g": 30}, "operation_key": "e-1"}, uuid.New(), "e-1", map[string]string{"date": "2026-10-27"}))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create entry → 201: %d %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if _, ok := created["converted"].([]any); !ok || created["day"] != nil {
		t.Fatalf("envelope keeps converted and a null day: %s", rec.Body.String())
	}
	svc.err = &model.NotFoundError{Message: "food not found"}
	rec = httptest.NewRecorder()
	h.LookupBarcode(rec, guidedHandlerRequest(t, http.MethodGet, "/", nil, uuid.New(), "", map[string]string{"code": "096619123452"}))
	if rec.Code != http.StatusNotFound || svc.code != "096619123452" {
		t.Fatalf("barcode passes through and maps 404: %d", rec.Code)
	}
}
