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

func TestProfileHandler_Get(t *testing.T) {
	uid := uuid.New()

	t.Run("success", func(t *testing.T) {
		svc := &MockProfileService{
			GetFunc: func(ctx context.Context, u uuid.UUID) (*model.UserProfile, error) {
				return &model.UserProfile{ID: u}, nil
			},
		}
		rec := httptest.NewRecorder()
		NewProfileHandler(svc).Get(rec, newReq(t, "GET", "/", nil, uid))
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("error -> 500", func(t *testing.T) {
		svc := &MockProfileService{
			GetFunc: func(ctx context.Context, u uuid.UUID) (*model.UserProfile, error) {
				return nil, errors.New("db")
			},
		}
		rec := httptest.NewRecorder()
		NewProfileHandler(svc).Get(rec, newReq(t, "GET", "/", nil, uid))
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", rec.Code)
		}
	})
}

func TestProfileHandler_Update(t *testing.T) {
	uid := uuid.New()
	name := "Jane"
	body := model.UpdateProfileRequest{DisplayName: &name}

	t.Run("success", func(t *testing.T) {
		svc := &MockProfileService{
			UpdateFunc: func(ctx context.Context, u uuid.UUID, r model.UpdateProfileRequest) (*model.UserProfile, error) {
				return &model.UserProfile{ID: u, DisplayName: r.DisplayName}, nil
			},
		}
		rec := httptest.NewRecorder()
		NewProfileHandler(svc).Update(rec, newReq(t, "PUT", "/", body, uid))
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("bad json -> 400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		NewProfileHandler(&MockProfileService{}).Update(rec, newReq(t, "PUT", "/", "junk", uid))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("validation -> 422", func(t *testing.T) {
		svc := &MockProfileService{
			UpdateFunc: func(ctx context.Context, u uuid.UUID, r model.UpdateProfileRequest) (*model.UserProfile, error) {
				return nil, &model.ValidationError{Message: "bad", Field: "body"}
			},
		}
		rec := httptest.NewRecorder()
		NewProfileHandler(svc).Update(rec, newReq(t, "PUT", "/", body, uid))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("expected 422, got %d", rec.Code)
		}
	})
}
