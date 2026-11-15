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

type savedMealHandlerService struct {
	err        error
	id         uuid.UUID
	date       string
	deletedRev int64
	deletedKey string
	logged     *model.LogMealRequest
}

func (s *savedMealHandlerService) ListMeals(context.Context, uuid.UUID) (*model.SavedMeals, error) {
	return &model.SavedMeals{Meals: []model.SavedMeal{}}, s.err
}
func (s *savedMealHandlerService) CreateMeal(_ context.Context, _ uuid.UUID, req model.SaveMealRequest) (*model.SavedMeal, model.SavedMealOutcome, error) {
	if s.err != nil {
		return nil, model.SavedMealCreated, s.err
	}
	return &model.SavedMeal{Name: req.Name, Items: []model.SavedMealItem{}, Revision: 1}, model.SavedMealCreated, nil
}
func (s *savedMealHandlerService) UpdateMeal(_ context.Context, _, id uuid.UUID, req model.SaveMealRequest) (*model.SavedMeal, model.SavedMealOutcome, error) {
	s.id = id
	if s.err != nil {
		return nil, model.SavedMealReplaced, s.err
	}
	return &model.SavedMeal{Name: req.Name, Items: []model.SavedMealItem{}, Revision: 2}, model.SavedMealReplaced, nil
}
func (s *savedMealHandlerService) DeleteMeal(_ context.Context, _, id uuid.UUID, rev int64, key string) (model.SavedMealOutcome, error) {
	s.id, s.deletedRev, s.deletedKey = id, rev, key
	return model.SavedMealRemoved, s.err
}
func (s *savedMealHandlerService) LogMeal(_ context.Context, _ uuid.UUID, date string, req model.LogMealRequest) (*model.MealLogged, model.SavedMealOutcome, error) {
	s.date, s.logged = date, &req
	if s.err != nil {
		return nil, model.SavedMealLogged, s.err
	}
	return &model.MealLogged{Entries: []model.FoodEntry{}, Converted: []model.FoodEntry{}}, model.SavedMealLogged, nil
}

func mealBody(key string) map[string]any {
	return map[string]any{
		"name":          "Usual breakfast",
		"items":         []any{map[string]any{"food": map[string]any{"id": uuid.NewString()}, "serving": map[string]any{"label": "1 cup", "grams": 81}, "amount": 1}},
		"operation_key": key,
	}
}

func TestSavedMealHandlerStatuses(t *testing.T) {
	svc := &savedMealHandlerService{}
	h := NewSavedMealHandler(svc)

	rec := httptest.NewRecorder()
	h.List(rec, guidedHandlerRequest(t, http.MethodGet, "/", nil, uuid.New(), "", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "{\"meals\":[]}\n" {
		t.Fatalf("list → 200 with an empty array: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.Create(rec, guidedHandlerRequest(t, http.MethodPost, "/", mealBody("meal-1"), uuid.New(), "meal-1", nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create → 201: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.Create(rec, guidedHandlerRequest(t, http.MethodPost, "/", mealBody("meal-1"), uuid.New(), "other", nil))
	if rec.Code != http.StatusUnprocessableEntity || !containsField(rec, "Idempotency-Key") {
		t.Fatalf("header/body mismatch → 422: %d", rec.Code)
	}

	id := uuid.New()
	body := mealBody("meal-2")
	body["expected_revision"] = 1
	rec = httptest.NewRecorder()
	h.Update(rec, guidedHandlerRequest(t, http.MethodPut, "/", body, uuid.New(), "meal-2", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusOK || svc.id != id {
		t.Fatalf("update → 200 for the path id: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.Update(rec, guidedHandlerRequest(t, http.MethodPut, "/", body, uuid.New(), "meal-2", map[string]string{"id": "not-a-uuid"}))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("malformed id → shared 404: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.Delete(rec, guidedHandlerRequest(t, http.MethodDelete, "/?expected_revision=4", nil, uuid.New(), "del-1", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 || svc.deletedRev != 4 || svc.deletedKey != "del-1" {
		t.Fatalf("delete → 204 with the query revision and header key: %d %+v", rec.Code, svc)
	}
	rec = httptest.NewRecorder()
	h.Delete(rec, guidedHandlerRequest(t, http.MethodDelete, "/", nil, uuid.New(), "del-1", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusUnprocessableEntity || !containsField(rec, "expected_revision") {
		t.Fatalf("missing expected_revision → 422: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.Delete(rec, guidedHandlerRequest(t, http.MethodDelete, "/?expected_revision=4", nil, uuid.New(), "", map[string]string{"id": id.String()}))
	if rec.Code != http.StatusUnprocessableEntity || !containsField(rec, "Idempotency-Key") {
		t.Fatalf("missing header → 422: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.Log(rec, guidedHandlerRequest(t, http.MethodPost, "/", map[string]any{"meal": map[string]any{"id": id.String()}, "operation_key": "log-1"}, uuid.New(), "log-1", map[string]string{"date": "2026-11-15"}))
	if rec.Code != http.StatusCreated || svc.date != "2026-11-15" || svc.logged.Meal.ID != id || svc.logged.Items != nil {
		t.Fatalf("log → 201 for the path date; absent items stay nil: %d %s", rec.Code, rec.Body.String())
	}
	var logged map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &logged); err != nil {
		t.Fatal(err)
	}
	if _, ok := logged["entries"].([]any); !ok {
		t.Fatalf("entries is an array: %s", rec.Body.String())
	}
	if _, ok := logged["converted"].([]any); !ok {
		t.Fatalf("converted is an array, never null: %s", rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.Log(rec, guidedHandlerRequest(t, http.MethodPost, "/", map[string]any{"meal": map[string]any{"id": id.String()}, "items": []any{}, "operation_key": "log-2"}, uuid.New(), "log-2", map[string]string{"date": "2026-11-15"}))
	if rec.Code != http.StatusCreated || svc.logged.Items == nil {
		t.Fatalf("a present empty items list reaches the service as non-nil so it can be refused: %d", rec.Code)
	}
}

func TestSavedMealHandlerErrorMapping(t *testing.T) {
	index := 2
	id := uuid.New()
	for _, test := range []struct {
		label  string
		err    error
		status int
		code   string
		check  func(t *testing.T, body map[string]any)
	}{
		{"item fault names its index", &model.ValidationError{Message: "food not available", Field: "items", Index: &index}, http.StatusUnprocessableEntity, "VALIDATION_ERROR", func(t *testing.T, body map[string]any) {
			details, _ := body["details"].(map[string]any)
			if details["field"] != "items" || details["index"] != float64(2) {
				t.Fatalf("details = %v", details)
			}
		}},
		{"name fault carries no index", &model.ValidationError{Message: "another saved meal already has this name", Field: "name"}, http.StatusUnprocessableEntity, "VALIDATION_ERROR", func(t *testing.T, body map[string]any) {
			details, _ := body["details"].(map[string]any)
			if _, present := details["index"]; details["field"] != "name" || present {
				t.Fatalf("details = %v", details)
			}
		}},
		{"limit", &model.SavedMealLimitError{Limit: model.MaxSavedMeals}, http.StatusConflict, "SAVED_MEAL_LIMIT_REACHED", func(t *testing.T, body map[string]any) {
			details, _ := body["details"].(map[string]any)
			if details["limit"] != float64(50) {
				t.Fatalf("details = %v", details)
			}
		}},
		{"stale revision carries the meal", &model.ConflictError{Message: "resource revision conflict", Expected: 1, Actual: 2, Authoritative: &model.SavedMeal{ID: id, Name: "Usual breakfast", Items: []model.SavedMealItem{}, Revision: 2}}, http.StatusConflict, "REVISION_CONFLICT", func(t *testing.T, body map[string]any) {
			resource, _ := body["resource"].(map[string]any)
			if resource["id"] != id.String() || resource["revision"] != float64(2) {
				t.Fatalf("resource = %v", resource)
			}
		}},
		{"payload mismatch", &model.ConflictError{Message: "idempotency key was already used with a different payload"}, http.StatusConflict, "IDEMPOTENCY_CONFLICT", nil},
		{"unknown or foreign meal", &model.NotFoundError{Message: "saved meal not found"}, http.StatusNotFound, "NOT_FOUND", nil},
	} {
		h := NewSavedMealHandler(&savedMealHandlerService{err: test.err})
		body := mealBody("k")
		body["expected_revision"] = 1
		rec := httptest.NewRecorder()
		h.Update(rec, guidedHandlerRequest(t, http.MethodPut, "/", body, uuid.New(), "k", map[string]string{"id": id.String()}))
		var decoded map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("%s: %v", test.label, err)
		}
		if rec.Code != test.status || decoded["code"] != test.code {
			t.Fatalf("%s: %d %s", test.label, rec.Code, rec.Body.String())
		}
		if test.check != nil {
			test.check(t, decoded)
		}
	}
}
