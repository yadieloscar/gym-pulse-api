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

func TestSettingsHandler_Get(t *testing.T) {
	uid := uuid.New()

	t.Run("success", func(t *testing.T) {
		svc := &MockSettingsService{
			GetFunc: func(ctx context.Context, u uuid.UUID) (*model.UserSettings, error) {
				return &model.UserSettings{WeightUnit: "lb", WeeklyGoal: 5}, nil
			},
		}
		h := NewSettingsHandler(svc)
		rec := httptest.NewRecorder()
		h.Get(rec, newReq(t, "GET", "/", nil, uid))
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var got model.UserSettings
		decodeBody(t, rec, &got)
		if got.WeightUnit != "lb" || got.WeeklyGoal != 5 {
			t.Errorf("unexpected: %+v", got)
		}
	})

	t.Run("service error -> 500", func(t *testing.T) {
		svc := &MockSettingsService{
			GetFunc: func(ctx context.Context, u uuid.UUID) (*model.UserSettings, error) {
				return nil, errors.New("boom")
			},
		}
		rec := httptest.NewRecorder()
		NewSettingsHandler(svc).Get(rec, newReq(t, "GET", "/", nil, uid))
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", rec.Code)
		}
	})
}

func TestSettingsHandler_Update(t *testing.T) {
	uid := uuid.New()

	t.Run("success", func(t *testing.T) {
		svc := &MockSettingsService{
			UpdateFunc: func(ctx context.Context, u uuid.UUID, r model.UserSettings) (*model.UserSettings, error) {
				return &r, nil
			},
		}
		rec := httptest.NewRecorder()
		body := model.UserSettings{WeightUnit: "kg", WeeklyGoal: 1}
		NewSettingsHandler(svc).Update(rec, newReq(t, "PUT", "/", body, uid))
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var got model.UserSettings
		decodeBody(t, rec, &got)
		if got.WeeklyGoal != 1 {
			t.Errorf("expected weekly_goal=1, got %d", got.WeeklyGoal)
		}
	})

	t.Run("bad json -> 400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		NewSettingsHandler(&MockSettingsService{}).Update(rec, newReq(t, "PUT", "/", "{not json", uid))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("validation -> 422 VALIDATION_ERROR field=body", func(t *testing.T) {
		svc := &MockSettingsService{
			UpdateFunc: func(ctx context.Context, u uuid.UUID, r model.UserSettings) (*model.UserSettings, error) {
				return nil, &model.ValidationError{Message: "invalid settings", Field: "body"}
			},
		}
		rec := httptest.NewRecorder()
		NewSettingsHandler(svc).Update(rec, newReq(t, "PUT", "/", map[string]any{}, uid))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d", rec.Code)
		}
		var got struct {
			Error   string            `json:"error"`
			Code    string            `json:"code"`
			Details map[string]string `json:"details"`
		}
		decodeBody(t, rec, &got)
		if got.Code != "VALIDATION_ERROR" {
			t.Errorf("expected code VALIDATION_ERROR, got %q", got.Code)
		}
		if got.Error != "invalid settings" {
			t.Errorf("expected error 'invalid settings', got %q", got.Error)
		}
		if got.Details["field"] != "body" {
			t.Errorf("expected field=body, got %+v", got.Details)
		}
	})

	// Use real service-error mapping via not-found
	t.Run("not found -> 404", func(t *testing.T) {
		svc := &MockSettingsService{
			UpdateFunc: func(ctx context.Context, u uuid.UUID, r model.UserSettings) (*model.UserSettings, error) {
				return nil, &model.NotFoundError{Message: "missing"}
			},
		}
		rec := httptest.NewRecorder()
		NewSettingsHandler(svc).Update(rec, newReq(t, "PUT", "/", model.UserSettings{WeightUnit: "lb", WeeklyGoal: 3}, uid))
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("conflict -> 409", func(t *testing.T) {
		svc := &MockSettingsService{
			UpdateFunc: func(ctx context.Context, u uuid.UUID, r model.UserSettings) (*model.UserSettings, error) {
				return nil, &model.ConflictError{Message: "dupe"}
			},
		}
		rec := httptest.NewRecorder()
		NewSettingsHandler(svc).Update(rec, newReq(t, "PUT", "/", model.UserSettings{WeightUnit: "lb", WeeklyGoal: 3}, uid))
		if rec.Code != http.StatusConflict {
			t.Errorf("expected 409, got %d", rec.Code)
		}
	})
}
