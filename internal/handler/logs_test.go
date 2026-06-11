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

func TestLogHandler_ListByWeek(t *testing.T) {
	uid := uuid.New()

	t.Run("missing week -> 400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		NewLogHandler(&MockLogService{}).ListByWeek(rec, newReq(t, "GET", "/", nil, uid))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		svc := &MockLogService{
			ListByWeekFunc: func(ctx context.Context, u uuid.UUID, w string) ([]model.DayLogSummary, error) {
				return []model.DayLogSummary{{Date: "2024-01-01"}}, nil
			},
		}
		rec := httptest.NewRecorder()
		NewLogHandler(svc).ListByWeek(rec, newReq(t, "GET", "/?week=2024-01-01", nil, uid))
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("svc validation -> 422", func(t *testing.T) {
		svc := &MockLogService{
			ListByWeekFunc: func(ctx context.Context, u uuid.UUID, w string) ([]model.DayLogSummary, error) {
				return nil, &model.ValidationError{Message: "bad", Field: "week"}
			},
		}
		rec := httptest.NewRecorder()
		NewLogHandler(svc).ListByWeek(rec, newReq(t, "GET", "/?week=bad", nil, uid))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("expected 422, got %d", rec.Code)
		}
	})
}

func TestLogHandler_GetByDate(t *testing.T) {
	uid := uuid.New()

	t.Run("success", func(t *testing.T) {
		svc := &MockLogService{
			GetByDateFunc: func(ctx context.Context, u uuid.UUID, d string) (*model.DayLog, error) {
				return &model.DayLog{Date: d}, nil
			},
		}
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "GET", "/", nil, uid), "date", "2024-01-01")
		NewLogHandler(svc).GetByDate(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("not found -> 404", func(t *testing.T) {
		svc := &MockLogService{
			GetByDateFunc: func(ctx context.Context, u uuid.UUID, d string) (*model.DayLog, error) {
				return nil, &model.NotFoundError{Message: "nope"}
			},
		}
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "GET", "/", nil, uid), "date", "2024-01-01")
		NewLogHandler(svc).GetByDate(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rec.Code)
		}
	})
}

func TestLogHandler_Create(t *testing.T) {
	uid := uuid.New()
	body := model.CreateDayLogRequest{Date: "2024-01-01", TypeID: "push", SubtypeID: "hypertrophy"}

	t.Run("success -> 201", func(t *testing.T) {
		svc := &MockLogService{
			CreateFunc: func(ctx context.Context, u uuid.UUID, r model.CreateDayLogRequest) (*model.DayLog, error) {
				return &model.DayLog{Date: r.Date}, nil
			},
		}
		rec := httptest.NewRecorder()
		NewLogHandler(svc).Create(rec, newReq(t, "POST", "/", body, uid))
		if rec.Code != http.StatusCreated {
			t.Errorf("expected 201, got %d", rec.Code)
		}
	})

	t.Run("bad json -> 400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		NewLogHandler(&MockLogService{}).Create(rec, newReq(t, "POST", "/", "x", uid))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("conflict -> 409", func(t *testing.T) {
		svc := &MockLogService{
			CreateFunc: func(ctx context.Context, u uuid.UUID, r model.CreateDayLogRequest) (*model.DayLog, error) {
				return nil, &model.ConflictError{Message: "dupe"}
			},
		}
		rec := httptest.NewRecorder()
		NewLogHandler(svc).Create(rec, newReq(t, "POST", "/", body, uid))
		if rec.Code != http.StatusConflict {
			t.Errorf("expected 409, got %d", rec.Code)
		}
	})
}

func TestLogHandler_Update(t *testing.T) {
	uid := uuid.New()
	body := model.UpdateDayLogRequest{}

	t.Run("success", func(t *testing.T) {
		svc := &MockLogService{
			UpdateFunc: func(ctx context.Context, u uuid.UUID, d string, r model.UpdateDayLogRequest) (*model.DayLog, error) {
				return &model.DayLog{Date: d}, nil
			},
		}
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "PUT", "/", body, uid), "date", "2024-01-01")
		NewLogHandler(svc).Update(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("bad json -> 400", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "PUT", "/", "junk", uid), "date", "2024-01-01")
		NewLogHandler(&MockLogService{}).Update(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("svc error -> 500", func(t *testing.T) {
		svc := &MockLogService{
			UpdateFunc: func(ctx context.Context, u uuid.UUID, d string, r model.UpdateDayLogRequest) (*model.DayLog, error) {
				return nil, errors.New("db")
			},
		}
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "PUT", "/", body, uid), "date", "2024-01-01")
		NewLogHandler(svc).Update(rec, req)
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", rec.Code)
		}
	})
}

func TestLogHandler_Delete(t *testing.T) {
	uid := uuid.New()

	t.Run("success -> 204", func(t *testing.T) {
		svc := &MockLogService{
			DeleteFunc: func(ctx context.Context, u uuid.UUID, d string) error {
				return nil
			},
		}
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "DELETE", "/", nil, uid), "date", "2024-01-01")
		NewLogHandler(svc).Delete(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Errorf("expected 204, got %d", rec.Code)
		}
	})

	t.Run("not found -> 404", func(t *testing.T) {
		svc := &MockLogService{
			DeleteFunc: func(ctx context.Context, u uuid.UUID, d string) error {
				return &model.NotFoundError{Message: "nope"}
			},
		}
		rec := httptest.NewRecorder()
		req := withURLParam(newReq(t, "DELETE", "/", nil, uid), "date", "2024-01-01")
		NewLogHandler(svc).Delete(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rec.Code)
		}
	})
}
