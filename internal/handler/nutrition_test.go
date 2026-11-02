package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type nutritionHandlerService struct {
	day           *model.NutritionDay
	dayOutcome    model.NutritionDayOutcome
	target        *model.NutritionTarget
	targetOutcome model.NutritionTargetOutcome
	listed        *model.NutritionRange
	err           error
	date          string
	savedDay      model.SaveNutritionDayRequest
	savedTarget   model.SaveNutritionTargetRequest
	from, to      string
}

func (s *nutritionHandlerService) SaveDay(_ context.Context, _ uuid.UUID, date string, req model.SaveNutritionDayRequest) (*model.NutritionDay, model.NutritionDayOutcome, error) {
	s.date, s.savedDay = date, req
	if s.err != nil {
		return nil, model.NutritionDayCreated, s.err
	}
	return s.day, s.dayOutcome, nil
}

func (s *nutritionHandlerService) ListDays(_ context.Context, _ uuid.UUID, from, to string) (*model.NutritionRange, error) {
	s.from, s.to = from, to
	return s.listed, s.err
}

func (s *nutritionHandlerService) GetTarget(context.Context, uuid.UUID) (*model.NutritionTarget, error) {
	return s.target, s.err
}

func (s *nutritionHandlerService) SaveTarget(_ context.Context, _ uuid.UUID, req model.SaveNutritionTargetRequest) (*model.NutritionTarget, model.NutritionTargetOutcome, error) {
	s.savedTarget = req
	if s.err != nil {
		return nil, model.NutritionTargetCreated, s.err
	}
	return s.target, s.targetOutcome, nil
}

func nutritionDayRequest(t *testing.T, body any, key string) *http.Request {
	t.Helper()
	return guidedHandlerRequest(t, http.MethodPut, "/", body, uuid.New(), key, map[string]string{"date": "2026-10-25"})
}

func TestNutritionHandlerSaveDayStatusFollowsOutcome(t *testing.T) {
	protein := 162.5
	zero := int64(0)
	day := &model.NutritionDay{ID: uuid.New(), Date: "2026-10-25", ProteinG: &protein, Source: model.NutritionSourceManual, Revision: 1}
	req := model.SaveNutritionDayRequest{ProteinG: &protein, ExpectedRevision: &zero, OperationKey: "day-1"}
	for _, test := range []struct {
		name    string
		outcome model.NutritionDayOutcome
		status  int
		body    bool
	}{
		{"created", model.NutritionDayCreated, http.StatusCreated, true},
		{"replaced", model.NutritionDayReplaced, http.StatusOK, true},
		{"ignored", model.NutritionDayIgnored, http.StatusOK, true},
		{"removed", model.NutritionDayRemoved, http.StatusNoContent, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var served *model.NutritionDay
			if test.body {
				served = day
			}
			svc := &nutritionHandlerService{day: served, dayOutcome: test.outcome}
			rec := httptest.NewRecorder()
			NewNutritionHandler(svc).SaveDay(rec, nutritionDayRequest(t, req, "day-1"))
			if rec.Code != test.status {
				t.Fatalf("status=%d want %d body=%s", rec.Code, test.status, rec.Body.String())
			}
			if svc.date != "2026-10-25" || svc.savedDay.ProteinG == nil || *svc.savedDay.ProteinG != 162.5 {
				t.Fatalf("date or body not routed: date=%s req=%+v", svc.date, svc.savedDay)
			}
			if !test.body {
				if rec.Body.Len() != 0 {
					t.Fatalf("204 must have no body, got %s", rec.Body.String())
				}
				return
			}
			var got model.NutritionDay
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.ID != day.ID || *got.ProteinG != 162.5 {
				t.Fatalf("body=%s err=%v", rec.Body.String(), err)
			}
			if strings.Contains(rec.Body.String(), "energy_kcal") {
				t.Fatal("absent values must be omitted")
			}
		})
	}
}

func TestNutritionHandlerSaveDayErrors(t *testing.T) {
	protein := 100.0
	two := int64(2)
	req := model.SaveNutritionDayRequest{ProteinG: &protein, ExpectedRevision: &two, OperationKey: "day-2"}
	stored := &model.NutritionDay{ID: uuid.New(), Date: "2026-10-25", Revision: 3, Source: model.NutritionSourceManual}

	stale := httptest.NewRecorder()
	NewNutritionHandler(&nutritionHandlerService{err: &model.ConflictError{Message: "resource revision conflict", Expected: 2, Actual: 3, Authoritative: stored}}).SaveDay(stale, nutritionDayRequest(t, req, "day-2"))
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), `"code":"REVISION_CONFLICT"`) || !strings.Contains(stale.Body.String(), `"actual_revision":3`) || !strings.Contains(stale.Body.String(), `"revision":3`) {
		t.Fatalf("stale status=%d body=%s", stale.Code, stale.Body.String())
	}

	gone := httptest.NewRecorder()
	NewNutritionHandler(&nutritionHandlerService{err: &model.ConflictError{Message: "resource revision conflict", Expected: 2, Actual: 0}}).SaveDay(gone, nutritionDayRequest(t, req, "day-2"))
	if gone.Code != http.StatusConflict || !strings.Contains(gone.Body.String(), `"code":"REVISION_CONFLICT"`) || !strings.Contains(gone.Body.String(), `"actual_revision":0`) {
		t.Fatalf("missing-row revision conflict status=%d body=%s", gone.Code, gone.Body.String())
	}

	if strings.Contains(gone.Body.String(), `"resource"`) {
		t.Fatalf("a missing-row conflict must omit resource, got %s", gone.Body.String())
	}

	// Other domains' revision conflicts with Actual 0 keep their generic code.
	other := httptest.NewRecorder()
	NewNutritionHandler(&nutritionHandlerService{err: &model.ConflictError{Message: "training profile revision conflict", Expected: 2}}).SaveDay(other, nutritionDayRequest(t, req, "day-2"))
	if other.Code != http.StatusConflict || !strings.Contains(other.Body.String(), `"code":"CONFLICT"`) {
		t.Fatalf("unrelated conflict status=%d body=%s", other.Code, other.Body.String())
	}

	reused := httptest.NewRecorder()
	NewNutritionHandler(&nutritionHandlerService{err: &model.ConflictError{Message: "idempotency key was already used with a different payload"}}).SaveDay(reused, nutritionDayRequest(t, req, "day-2"))
	if reused.Code != http.StatusConflict || !strings.Contains(reused.Body.String(), `"code":"IDEMPOTENCY_CONFLICT"`) {
		t.Fatalf("idempotency status=%d body=%s", reused.Code, reused.Body.String())
	}

	invalid := httptest.NewRecorder()
	NewNutritionHandler(&nutritionHandlerService{err: &model.ValidationError{Message: "cannot log future dates", Field: "date"}}).SaveDay(invalid, nutritionDayRequest(t, req, "day-2"))
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), `"field":"date"`) {
		t.Fatalf("invalid status=%d body=%s", invalid.Code, invalid.Body.String())
	}

	noProfile := httptest.NewRecorder()
	NewNutritionHandler(&nutritionHandlerService{err: &model.NotFoundError{Message: "training profile not found"}}).SaveDay(noProfile, nutritionDayRequest(t, req, "day-2"))
	if noProfile.Code != http.StatusNotFound || !strings.Contains(noProfile.Body.String(), "NOT_FOUND") {
		t.Fatalf("no profile status=%d body=%s", noProfile.Code, noProfile.Body.String())
	}

	badKey := httptest.NewRecorder()
	NewNutritionHandler(&nutritionHandlerService{}).SaveDay(badKey, nutritionDayRequest(t, req, "header-key"))
	if badKey.Code != http.StatusUnprocessableEntity || !strings.Contains(badKey.Body.String(), "Idempotency-Key") {
		t.Fatalf("bad key status=%d body=%s", badKey.Code, badKey.Body.String())
	}

	badJSON := httptest.NewRecorder()
	NewNutritionHandler(&nutritionHandlerService{}).SaveDay(badJSON, nutritionDayRequest(t, `{"protein_g":`, "day-2"))
	if badJSON.Code != http.StatusBadRequest {
		t.Fatalf("bad json status=%d body=%s", badJSON.Code, badJSON.Body.String())
	}
}

func TestNutritionHandlerListDays(t *testing.T) {
	empty := httptest.NewRecorder()
	svc := &nutritionHandlerService{listed: &model.NutritionRange{Days: []model.NutritionDay{}, Targets: []model.NutritionTarget{}}}
	NewNutritionHandler(svc).ListDays(empty, guidedHandlerRequest(t, http.MethodGet, "/?from=2026-10-01&to=2026-10-25", nil, uuid.New(), "", nil))
	if empty.Code != http.StatusOK || strings.TrimSpace(empty.Body.String()) != `{"days":[],"targets":[]}` || svc.from != "2026-10-01" || svc.to != "2026-10-25" {
		t.Fatalf("empty status=%d body=%s from=%s to=%s", empty.Code, empty.Body.String(), svc.from, svc.to)
	}

	protein := 150.0
	listed := httptest.NewRecorder()
	NewNutritionHandler(&nutritionHandlerService{listed: &model.NutritionRange{
		Days:    []model.NutritionDay{{ID: uuid.New(), Date: "2026-10-24", ProteinG: &protein, Source: model.NutritionSourceAppleHealth, Revision: 1}},
		Targets: []model.NutritionTarget{{ID: uuid.New(), EffectiveFrom: "2026-10-01", ProteinG: 150, Revision: 1}},
	}}).ListDays(listed, guidedHandlerRequest(t, http.MethodGet, "/?from=2026-10-20&to=2026-10-25", nil, uuid.New(), "", nil))
	var body model.NutritionRange
	if err := json.Unmarshal(listed.Body.Bytes(), &body); err != nil || len(body.Days) != 1 || body.Days[0].Source != model.NutritionSourceAppleHealth || len(body.Targets) != 1 || body.Targets[0].EffectiveFrom != "2026-10-01" {
		t.Fatalf("list body=%s err=%v", listed.Body.String(), err)
	}
	if !strings.Contains(listed.Body.String(), `"energy_kcal":null`) {
		t.Fatal("target optional values must be present as null")
	}

	invalid := httptest.NewRecorder()
	NewNutritionHandler(&nutritionHandlerService{err: &model.ValidationError{Message: "date range must not exceed 366 days", Field: "to"}}).ListDays(invalid, guidedHandlerRequest(t, http.MethodGet, "/?from=2025-01-01&to=2026-10-25", nil, uuid.New(), "", nil))
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), `"field":"to"`) {
		t.Fatalf("invalid status=%d body=%s", invalid.Code, invalid.Body.String())
	}
}

func TestNutritionHandlerTarget(t *testing.T) {
	target := &model.NutritionTarget{ID: uuid.New(), EffectiveFrom: "2026-10-25", ProteinG: 150, Revision: 1}

	got := httptest.NewRecorder()
	NewNutritionHandler(&nutritionHandlerService{target: target}).GetTarget(got, guidedHandlerRequest(t, http.MethodGet, "/", nil, uuid.New(), "", nil))
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"protein_g":150`) {
		t.Fatalf("get status=%d body=%s", got.Code, got.Body.String())
	}

	none := httptest.NewRecorder()
	NewNutritionHandler(&nutritionHandlerService{err: &model.NotFoundError{Message: "nutrition target not found"}}).GetTarget(none, guidedHandlerRequest(t, http.MethodGet, "/", nil, uuid.New(), "", nil))
	if none.Code != http.StatusNotFound || !strings.Contains(none.Body.String(), `"code":"NOT_FOUND"`) {
		t.Fatalf("none status=%d body=%s", none.Code, none.Body.String())
	}

	req := model.SaveNutritionTargetRequest{ProteinG: 150, ExpectedRevision: 0, OperationKey: "target-1"}
	for _, test := range []struct {
		outcome model.NutritionTargetOutcome
		status  int
	}{
		{model.NutritionTargetCreated, http.StatusCreated},
		{model.NutritionTargetReplaced, http.StatusOK},
		{model.NutritionTargetUnchanged, http.StatusOK},
	} {
		svc := &nutritionHandlerService{target: target, targetOutcome: test.outcome}
		rec := httptest.NewRecorder()
		NewNutritionHandler(svc).SaveTarget(rec, guidedHandlerRequest(t, http.MethodPut, "/", req, uuid.New(), "target-1", nil))
		if rec.Code != test.status || svc.savedTarget.ProteinG != 150 {
			t.Fatalf("outcome %d status=%d body=%s", test.outcome, rec.Code, rec.Body.String())
		}
	}

	stale := httptest.NewRecorder()
	NewNutritionHandler(&nutritionHandlerService{err: &model.ConflictError{Message: "resource revision conflict", Expected: 0, Actual: 1, Authoritative: target}}).SaveTarget(stale, guidedHandlerRequest(t, http.MethodPut, "/", req, uuid.New(), "target-1", nil))
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), `"code":"REVISION_CONFLICT"`) || !strings.Contains(stale.Body.String(), `"effective_from":"2026-10-25"`) {
		t.Fatalf("stale status=%d body=%s", stale.Code, stale.Body.String())
	}

	badKey := httptest.NewRecorder()
	NewNutritionHandler(&nutritionHandlerService{}).SaveTarget(badKey, guidedHandlerRequest(t, http.MethodPut, "/", req, uuid.New(), "other", nil))
	if badKey.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad key status=%d body=%s", badKey.Code, badKey.Body.String())
	}
}
