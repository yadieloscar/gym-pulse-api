package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func TestBodyWeightHandler_Create(t *testing.T) {
	uid := uuid.New()
	body := model.CreateBodyWeightRequest{Date: "2024-01-01", Weight: 180, Unit: "lb"}

	t.Run("success -> 201", func(t *testing.T) {
		svc := &MockBodyWeightService{
			LogWeightFunc: func(ctx context.Context, u uuid.UUID, r model.CreateBodyWeightRequest) (*model.BodyWeight, error) {
				return &model.BodyWeight{ID: uuid.New(), Weight: r.Weight, Unit: r.Unit, Date: r.Date}, nil
			},
		}
		rec := httptest.NewRecorder()
		NewBodyWeightHandler(svc).Create(rec, newReq(t, "POST", "/", body, uid))
		if rec.Code != http.StatusCreated {
			t.Errorf("expected 201, got %d", rec.Code)
		}
	})

	t.Run("bad json -> 400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		NewBodyWeightHandler(&MockBodyWeightService{}).Create(rec, newReq(t, "POST", "/", "junk", uid))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("validation -> 422", func(t *testing.T) {
		svc := &MockBodyWeightService{
			LogWeightFunc: func(ctx context.Context, u uuid.UUID, r model.CreateBodyWeightRequest) (*model.BodyWeight, error) {
				return nil, &model.ValidationError{Message: "bad", Field: "body"}
			},
		}
		rec := httptest.NewRecorder()
		NewBodyWeightHandler(svc).Create(rec, newReq(t, "POST", "/", body, uid))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("expected 422, got %d", rec.Code)
		}
	})
}

func TestBodyWeightHandler_List(t *testing.T) {
	uid := uuid.New()

	t.Run("success", func(t *testing.T) {
		svc := &MockBodyWeightService{
			ListWeightsFunc: func(ctx context.Context, u uuid.UUID) ([]model.BodyWeight, error) {
				return []model.BodyWeight{{Weight: 180}}, nil
			},
		}
		rec := httptest.NewRecorder()
		NewBodyWeightHandler(svc).List(rec, newReq(t, "GET", "/", nil, uid))
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("error -> 500", func(t *testing.T) {
		svc := &MockBodyWeightService{
			ListWeightsFunc: func(ctx context.Context, u uuid.UUID) ([]model.BodyWeight, error) {
				return nil, errors.New("db")
			},
		}
		rec := httptest.NewRecorder()
		NewBodyWeightHandler(svc).List(rec, newReq(t, "GET", "/", nil, uid))
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", rec.Code)
		}
	})
}

func TestBodyWeightHandler_Delete(t *testing.T) {
	uid := uuid.New()
	eid := uuid.New()

	t.Run("success -> 204", func(t *testing.T) {
		svc := &MockBodyWeightService{
			DeleteWeightFunc: func(ctx context.Context, u, e uuid.UUID) error {
				return nil
			},
		}
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "DELETE", "/", nil, uid), "id", eid.String())
		NewBodyWeightHandler(svc).Delete(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Errorf("expected 204, got %d", rec.Code)
		}
	})

	t.Run("invalid id -> 400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "DELETE", "/", nil, uid), "id", "bad")
		NewBodyWeightHandler(&MockBodyWeightService{}).Delete(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("not found -> 404", func(t *testing.T) {
		svc := &MockBodyWeightService{
			DeleteWeightFunc: func(ctx context.Context, u, e uuid.UUID) error {
				return &model.NotFoundError{Message: "nope"}
			},
		}
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "DELETE", "/", nil, uid), "id", eid.String())
		NewBodyWeightHandler(svc).Delete(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rec.Code)
		}
	})
}

func TestHealthCheck(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health", nil)
	HealthCheck(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}
