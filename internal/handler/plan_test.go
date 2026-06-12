package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func TestPlanHandler_Get(t *testing.T) {
	uid := uuid.New()

	t.Run("success forwards window and returns plan", func(t *testing.T) {
		var gotFrom, gotTo string
		svc := &MockPlanService{GetFunc: func(ctx context.Context, u uuid.UUID, from, to string) (*model.PlanResponse, error) {
			gotFrom, gotTo = from, to
			return &model.PlanResponse{
				Weekly:    []model.WeeklyPlanDay{{Weekday: 1, Rest: true}},
				Overrides: []model.PlanOverride{},
			}, nil
		}}
		rec := httptest.NewRecorder()
		NewPlanHandler(svc).Get(rec, newReq(t, "GET", "/?from=2026-06-01&to=2026-06-30", nil, uid))
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if gotFrom != "2026-06-01" || gotTo != "2026-06-30" {
			t.Errorf("window forwarded %q..%q", gotFrom, gotTo)
		}
		var got model.PlanResponse
		decodeBody(t, rec, &got)
		if len(got.Weekly) != 1 {
			t.Errorf("weekly len=%d", len(got.Weekly))
		}
	})

	t.Run("validation error -> 422", func(t *testing.T) {
		svc := &MockPlanService{GetFunc: func(ctx context.Context, u uuid.UUID, from, to string) (*model.PlanResponse, error) {
			return nil, &model.ValidationError{Message: "invalid from date", Field: "from"}
		}}
		rec := httptest.NewRecorder()
		NewPlanHandler(svc).Get(rec, newReq(t, "GET", "/?from=bogus", nil, uid))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("status=%d want 422", rec.Code)
		}
	})
}

func TestPlanHandler_PutWeekly(t *testing.T) {
	uid := uuid.New()

	t.Run("success", func(t *testing.T) {
		svc := &MockPlanService{PutWeeklyFunc: func(ctx context.Context, u uuid.UUID, req model.PutWeeklyPlanRequest) ([]model.WeeklyPlanDay, error) {
			return req.Days, nil
		}}
		body := map[string]any{"days": []map[string]any{{"weekday": 1, "rest": true}}}
		rec := httptest.NewRecorder()
		NewPlanHandler(svc).PutWeekly(rec, newReq(t, "PUT", "/", body, uid))
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("malformed body -> 400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := newReq(t, "PUT", "/", nil, uid)
		req.Body = http.NoBody
		NewPlanHandler(&MockPlanService{}).PutWeekly(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status=%d want 400", rec.Code)
		}
	})

	t.Run("validation error -> 422", func(t *testing.T) {
		svc := &MockPlanService{PutWeeklyFunc: func(ctx context.Context, u uuid.UUID, req model.PutWeeklyPlanRequest) ([]model.WeeklyPlanDay, error) {
			return nil, &model.ValidationError{Message: "duplicate weekday in plan", Field: "days"}
		}}
		body := map[string]any{"days": []map[string]any{{"weekday": 3}, {"weekday": 3}}}
		rec := httptest.NewRecorder()
		NewPlanHandler(svc).PutWeekly(rec, newReq(t, "PUT", "/", body, uid))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("status=%d want 422", rec.Code)
		}
	})
}

func TestPlanHandler_Overrides(t *testing.T) {
	uid := uuid.New()

	t.Run("put success -> 204", func(t *testing.T) {
		svc := &MockPlanService{}
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "PUT", "/", map[string]any{"rest": true}, uid), "date", "2026-06-15")
		NewPlanHandler(svc).PutOverride(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("put foreign template -> 404", func(t *testing.T) {
		svc := &MockPlanService{PutOverrideFunc: func(ctx context.Context, u uuid.UUID, date string, req model.PutPlanOverrideRequest) error {
			return &model.NotFoundError{Message: "template not found"}
		}}
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "PUT", "/", map[string]any{"template_id": uuid.New()}, uid), "date", "2026-06-15")
		NewPlanHandler(svc).PutOverride(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("status=%d want 404", rec.Code)
		}
	})

	t.Run("delete success -> 204, missing -> 404", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "DELETE", "/", nil, uid), "date", "2026-06-15")
		NewPlanHandler(&MockPlanService{}).DeleteOverride(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status=%d", rec.Code)
		}

		svc := &MockPlanService{DeleteOverrideFunc: func(ctx context.Context, u uuid.UUID, date string) error {
			return &model.NotFoundError{Message: "plan override not found"}
		}}
		rec = httptest.NewRecorder()
		req = withURLParam(newReq(t, "DELETE", "/", nil, uid), "date", "2026-06-15")
		NewPlanHandler(svc).DeleteOverride(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("status=%d want 404", rec.Code)
		}
	})
}
