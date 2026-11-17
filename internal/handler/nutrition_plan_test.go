package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// nutritionPlanHandlerStore is an in-memory profile DAO so the handler tests
// run the real service and calculation.
type nutritionPlanHandlerStore struct {
	stored *model.NutritionProfile
	err    error
}

func (s *nutritionPlanHandlerStore) Get(context.Context, uuid.UUID) (*model.NutritionProfile, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.stored == nil {
		return nil, &model.NotFoundError{Message: "nutrition plan not found"}
	}
	return s.stored, nil
}

func (s *nutritionPlanHandlerStore) Save(_ context.Context, _ uuid.UUID, profile *model.NutritionProfile, expected int64, _, _ string) (*model.NutritionProfile, model.NutritionProfileOutcome, error) {
	if s.err != nil {
		return nil, model.NutritionProfileCreated, s.err
	}
	var actual int64
	if s.stored != nil {
		actual = s.stored.Revision
	}
	if expected != actual {
		var authoritative any
		if s.stored != nil {
			authoritative = s.stored
		}
		return nil, model.NutritionProfileCreated, &model.ConflictError{Message: "resource revision conflict", Expected: expected, Actual: actual, Authoritative: authoritative}
	}
	outcome := model.NutritionProfileCreated
	if s.stored != nil {
		outcome = model.NutritionProfileReplaced
	}
	saved := *profile
	saved.Revision = actual + 1
	s.stored = &saved
	return &saved, outcome, nil
}

func nutritionPlanHandler(store *nutritionPlanHandlerStore) *NutritionPlanHandler {
	return NewNutritionPlanHandler(service.NewNutritionPlanService(store))
}

func calculatorBody(extra map[string]any) map[string]any {
	body := map[string]any{
		"unit_system": "imperial", "weight": 180, "height": 70, "age_years": 30, "sex": "male",
		"strength_days": 4, "cardio_days": 1, "goal": "maintenance",
	}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

func TestNutritionPlanHandlerOptions(t *testing.T) {
	rec := httptest.NewRecorder()
	nutritionPlanHandler(&nutritionPlanHandlerStore{}).Options(rec, guidedHandlerRequest(t, http.MethodGet, "/", nil, uuid.New(), "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"goals":[`, `"activity_levels":[`, `"exclusions":["rice","dairy","nuts","gluten","eggs","red_meat"]`, `"id":"chicken_breast"`, `"fdc_id":171534`, `"meals":[{"slot":1,"name":"Breakfast"}`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("options body lacks %s", want)
		}
	}
}

func TestNutritionPlanHandlerCalculate(t *testing.T) {
	rec := httptest.NewRecorder()
	nutritionPlanHandler(&nutritionPlanHandlerStore{}).Calculate(rec, guidedHandlerRequest(t, http.MethodPost, "/", calculatorBody(nil), uuid.New(), "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var plan model.NutritionPlan
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.BMRKcal != 1783 || plan.TDEEKcal != 2763 || plan.Macros.Protein.G != 153 || len(plan.MealTemplates) != 1 {
		t.Errorf("plan = %+v", plan)
	}
	if strings.Contains(rec.Body.String(), `"revision"`) {
		t.Error("a calculation is not a saved plan and carries no revision")
	}

	invalid := httptest.NewRecorder()
	nutritionPlanHandler(&nutritionPlanHandlerStore{}).Calculate(invalid, guidedHandlerRequest(t, http.MethodPost, "/", calculatorBody(map[string]any{"age_years": 16}), uuid.New(), "", nil))
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), `"code":"VALIDATION_ERROR"`) || !strings.Contains(invalid.Body.String(), `"field":"age_years"`) {
		t.Errorf("invalid status=%d body=%s", invalid.Code, invalid.Body.String())
	}

	malformed := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{"))
	nutritionPlanHandler(&nutritionPlanHandlerStore{}).Calculate(malformed, req)
	if malformed.Code != http.StatusBadRequest {
		t.Errorf("malformed status=%d", malformed.Code)
	}
}

func TestNutritionPlanHandlerSaveAndRead(t *testing.T) {
	store := &nutritionPlanHandlerStore{}
	h := nutritionPlanHandler(store)
	user := uuid.New()
	save := func(extra map[string]any, key string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.SaveProfile(rec, guidedHandlerRequest(t, http.MethodPut, "/", calculatorBody(extra), user, key, nil))
		return rec
	}

	missing := httptest.NewRecorder()
	h.GetPlan(missing, guidedHandlerRequest(t, http.MethodGet, "/", nil, user, "", nil))
	if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), `"code":"NOT_FOUND"`) {
		t.Fatalf("missing plan status=%d body=%s", missing.Code, missing.Body.String())
	}

	created := save(map[string]any{"expected_revision": 0, "operation_key": "p-1"}, "p-1")
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"revision":1`) || !strings.Contains(created.Body.String(), `"target_kcal":2763`) {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}

	replaced := save(map[string]any{"expected_revision": 1, "operation_key": "p-2", "goal": "muscle_gain"}, "p-2")
	if replaced.Code != http.StatusOK || !strings.Contains(replaced.Body.String(), `"revision":2`) || !strings.Contains(replaced.Body.String(), `"target_kcal":3063`) {
		t.Fatalf("replace status=%d body=%s", replaced.Code, replaced.Body.String())
	}

	stale := save(map[string]any{"expected_revision": 1, "operation_key": "p-3"}, "p-3")
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), `"code":"REVISION_CONFLICT"`) || !strings.Contains(stale.Body.String(), `"actual_revision":2`) || !strings.Contains(stale.Body.String(), `"target_kcal":3063`) {
		t.Fatalf("stale status=%d body=%s", stale.Code, stale.Body.String())
	}

	mismatch := save(map[string]any{"expected_revision": 2, "operation_key": "p-4"}, "other")
	if mismatch.Code != http.StatusUnprocessableEntity || !strings.Contains(mismatch.Body.String(), `"field":"Idempotency-Key"`) {
		t.Fatalf("key mismatch status=%d body=%s", mismatch.Code, mismatch.Body.String())
	}

	invalid := save(map[string]any{"expected_revision": 2, "operation_key": "p-5", "exclusions": []string{"soy"}}, "p-5")
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), `"field":"exclusions"`) {
		t.Fatalf("invalid status=%d body=%s", invalid.Code, invalid.Body.String())
	}

	read := httptest.NewRecorder()
	h.GetPlan(read, guidedHandlerRequest(t, http.MethodGet, "/", nil, user, "", nil))
	if read.Code != http.StatusOK || !strings.Contains(read.Body.String(), `"revision":2`) || !strings.Contains(read.Body.String(), `"goal":"muscle_gain"`) {
		t.Fatalf("read status=%d body=%s", read.Code, read.Body.String())
	}
}

func TestNutritionPlanHandlerStoreFailureIsNotLeaked(t *testing.T) {
	h := nutritionPlanHandler(&nutritionPlanHandlerStore{err: errors.New("pq: connection refused at 10.0.0.5")})
	rec := httptest.NewRecorder()
	h.GetPlan(rec, guidedHandlerRequest(t, http.MethodGet, "/", nil, uuid.New(), "", nil))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
