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

func TestTemplateHandler_List(t *testing.T) {
	uid := uuid.New()

	t.Run("success forwards query params", func(t *testing.T) {
		gotType, gotSub := "", ""
		svc := &MockTemplateService{
			ListFunc: func(ctx context.Context, u uuid.UUID, tf, sf string) ([]model.TemplateSummary, error) {
				gotType, gotSub = tf, sf
				return []model.TemplateSummary{{Name: "T"}}, nil
			},
		}
		rec := httptest.NewRecorder()
		NewTemplateHandler(svc).List(rec, newReq(t, "GET", "/?type=push&subtype=hypertrophy", nil, uid))
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d", rec.Code)
		}
		if gotType != "push" || gotSub != "hypertrophy" {
			t.Errorf("filters not forwarded: %s/%s", gotType, gotSub)
		}
	})

	t.Run("validation error -> 422", func(t *testing.T) {
		svc := &MockTemplateService{
			ListFunc: func(ctx context.Context, u uuid.UUID, tf, sf string) ([]model.TemplateSummary, error) {
				return nil, &model.ValidationError{Message: "bad", Field: "type"}
			},
		}
		rec := httptest.NewRecorder()
		NewTemplateHandler(svc).List(rec, newReq(t, "GET", "/?type=bogus", nil, uid))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("expected 422, got %d", rec.Code)
		}
	})
}

func TestTemplateHandler_GetByID(t *testing.T) {
	uid := uuid.New()
	tid := uuid.New()

	t.Run("success", func(t *testing.T) {
		svc := &MockTemplateService{
			GetByIDFunc: func(ctx context.Context, u, id uuid.UUID) (*model.WorkoutTemplate, error) {
				return &model.WorkoutTemplate{ID: id, Name: "T"}, nil
			},
		}
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "GET", "/", nil, uid), "id", tid.String())
		NewTemplateHandler(svc).GetByID(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d", rec.Code)
		}
	})

	t.Run("invalid uuid -> 400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "GET", "/", nil, uid), "id", "not-a-uuid")
		NewTemplateHandler(&MockTemplateService{}).GetByID(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("not found -> 404", func(t *testing.T) {
		svc := &MockTemplateService{
			GetByIDFunc: func(ctx context.Context, u, id uuid.UUID) (*model.WorkoutTemplate, error) {
				return nil, &model.NotFoundError{Message: "nope"}
			},
		}
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "GET", "/", nil, uid), "id", tid.String())
		NewTemplateHandler(svc).GetByID(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rec.Code)
		}
	})
}

func TestTemplateHandler_Create(t *testing.T) {
	uid := uuid.New()

	body := model.CreateTemplateRequest{Name: "X", TypeID: "push", SubtypeID: "hypertrophy"}

	t.Run("success -> 201", func(t *testing.T) {
		svc := &MockTemplateService{
			CreateFunc: func(ctx context.Context, u uuid.UUID, r model.CreateTemplateRequest) (*model.WorkoutTemplate, error) {
				return &model.WorkoutTemplate{ID: uuid.New(), Name: r.Name}, nil
			},
		}
		rec := httptest.NewRecorder()
		NewTemplateHandler(svc).Create(rec, newReq(t, "POST", "/", body, uid))
		if rec.Code != http.StatusCreated {
			t.Errorf("expected 201, got %d", rec.Code)
		}
	})

	t.Run("bad json -> 400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		NewTemplateHandler(&MockTemplateService{}).Create(rec, newReq(t, "POST", "/", "garbage", uid))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("svc error -> 500", func(t *testing.T) {
		svc := &MockTemplateService{
			CreateFunc: func(ctx context.Context, u uuid.UUID, r model.CreateTemplateRequest) (*model.WorkoutTemplate, error) {
				return nil, errors.New("db")
			},
		}
		rec := httptest.NewRecorder()
		NewTemplateHandler(svc).Create(rec, newReq(t, "POST", "/", body, uid))
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", rec.Code)
		}
	})
}

func TestTemplateHandler_Update(t *testing.T) {
	uid := uuid.New()
	tid := uuid.New()
	body := model.CreateTemplateRequest{Name: "X", TypeID: "push", SubtypeID: "hypertrophy"}

	t.Run("success", func(t *testing.T) {
		svc := &MockTemplateService{
			UpdateFunc: func(ctx context.Context, u, id uuid.UUID, r model.CreateTemplateRequest) (*model.WorkoutTemplate, error) {
				return &model.WorkoutTemplate{ID: id, Name: r.Name}, nil
			},
		}
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "PUT", "/", body, uid), "id", tid.String())
		NewTemplateHandler(svc).Update(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("invalid id -> 400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "PUT", "/", body, uid), "id", "bad")
		NewTemplateHandler(&MockTemplateService{}).Update(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("bad json -> 400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "PUT", "/", "junk", uid), "id", tid.String())
		NewTemplateHandler(&MockTemplateService{}).Update(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("not found -> 404", func(t *testing.T) {
		svc := &MockTemplateService{
			UpdateFunc: func(ctx context.Context, u, id uuid.UUID, r model.CreateTemplateRequest) (*model.WorkoutTemplate, error) {
				return nil, &model.NotFoundError{Message: "nope"}
			},
		}
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "PUT", "/", body, uid), "id", tid.String())
		NewTemplateHandler(svc).Update(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rec.Code)
		}
	})
}

func TestTemplateHandler_Delete(t *testing.T) {
	uid := uuid.New()
	tid := uuid.New()

	t.Run("success -> 204", func(t *testing.T) {
		called := false
		svc := &MockTemplateService{
			DeleteFunc: func(ctx context.Context, u, id uuid.UUID) error {
				called = true
				return nil
			},
		}
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "DELETE", "/", nil, uid), "id", tid.String())
		NewTemplateHandler(svc).Delete(rec, req)
		if rec.Code != http.StatusNoContent || !called {
			t.Errorf("status=%d called=%v", rec.Code, called)
		}
	})

	t.Run("invalid id -> 400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "DELETE", "/", nil, uid), "id", "bad")
		NewTemplateHandler(&MockTemplateService{}).Delete(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("svc error -> 500", func(t *testing.T) {
		svc := &MockTemplateService{
			DeleteFunc: func(ctx context.Context, u, id uuid.UUID) error {
				return errors.New("db")
			},
		}
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "DELETE", "/", nil, uid), "id", tid.String())
		NewTemplateHandler(svc).Delete(rec, req)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", rec.Code)
		}
	})
}
