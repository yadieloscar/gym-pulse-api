package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// memorySettingsDAO stores one account's settings in memory and applies a
// partial update the way the SQL patch does, so handler tests can run the
// real settings service and its validation.
type memorySettingsDAO struct {
	stored *model.UserSettings
}

func (d *memorySettingsDAO) Get(context.Context, uuid.UUID) (*model.UserSettings, error) {
	if d.stored == nil {
		defaults := model.DefaultUserSettings()
		return &defaults, nil
	}
	got := *d.stored
	return &got, nil
}

func (d *memorySettingsDAO) Upsert(_ context.Context, _ uuid.UUID, s *model.UserSettings) error {
	stored := *s
	d.stored = &stored
	return nil
}

func (d *memorySettingsDAO) Patch(_ context.Context, _ uuid.UUID, req model.UpdateUserSettingsRequest) (*model.UserSettings, error) {
	if d.stored == nil {
		defaults := model.DefaultUserSettings()
		d.stored = &defaults
	}
	if req.WeightUnit != nil {
		d.stored.WeightUnit = *req.WeightUnit
	}
	if req.WeeklyGoal != nil {
		d.stored.WeeklyGoal = *req.WeeklyGoal
	}
	if req.Palette != nil {
		d.stored.Palette = *req.Palette
	}
	got := *d.stored
	return &got, nil
}

// TestSettingsHandler_Palettes runs the handler over the real service, so the
// statuses come from the request validator rather than from a stub.
func TestSettingsHandler_Palettes(t *testing.T) {
	uid := uuid.New()
	newHandler := func() *SettingsHandler {
		return NewSettingsHandler(service.NewSettingsService(&memorySettingsDAO{}, validator.New()))
	}

	t.Run("no stored settings reads the voltLime default", func(t *testing.T) {
		rec := httptest.NewRecorder()
		newHandler().Get(rec, newReq(t, "GET", "/", nil, uid))
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var got model.UserSettings
		decodeBody(t, rec, &got)
		want := model.UserSettings{WeightUnit: "lb", WeeklyGoal: 5, Palette: "voltLime"}
		if got != want {
			t.Errorf("defaults = %+v, want %+v", got, want)
		}
	})

	t.Run("first write without a palette stores voltLime", func(t *testing.T) {
		h := newHandler()
		rec := httptest.NewRecorder()
		h.Update(rec, newReq(t, "PUT", "/", map[string]any{"weekly_goal": 3}, uid))
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var got model.UserSettings
		decodeBody(t, rec, &got)
		if got.Palette != "voltLime" || got.WeeklyGoal != 3 {
			t.Errorf("got %+v, want palette voltLime and weekly_goal 3", got)
		}
	})

	// irisViolet is new in contract 1.9.0. The other six must keep working for
	// older app builds, including the three the current app no longer offers.
	accepted := []string{"irisViolet", "voltLime", "obsidianEmber", "abyssCerulean", "mossChartreuse", "oliveCitrine", "slateMint"}
	for _, palette := range accepted {
		t.Run("PUT "+palette+" -> 200", func(t *testing.T) {
			h := newHandler()
			seed := httptest.NewRecorder()
			h.Update(seed, newReq(t, "PUT", "/", map[string]any{"weight_unit": "kg", "weekly_goal": 4}, uid))
			if seed.Code != http.StatusOK {
				t.Fatalf("seed status=%d body=%s", seed.Code, seed.Body.String())
			}

			rec := httptest.NewRecorder()
			h.Update(rec, newReq(t, "PUT", "/", map[string]any{"palette": palette}, uid))
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			var got model.UserSettings
			decodeBody(t, rec, &got)
			want := model.UserSettings{WeightUnit: "kg", WeeklyGoal: 4, Palette: palette}
			if got != want {
				t.Errorf("PUT response = %+v, want %+v", got, want)
			}

			read := httptest.NewRecorder()
			h.Get(read, newReq(t, "GET", "/", nil, uid))
			var stored model.UserSettings
			decodeBody(t, read, &stored)
			if stored != want {
				t.Errorf("GET after PUT = %+v, want %+v", stored, want)
			}
		})
	}

	for _, palette := range []string{"neonGrape", "irisviolet", "IrisViolet", ""} {
		t.Run("PUT unknown palette "+palette+" -> 422", func(t *testing.T) {
			h := newHandler()
			rec := httptest.NewRecorder()
			h.Update(rec, newReq(t, "PUT", "/", map[string]any{"palette": palette}, uid))
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			var got struct {
				Code    string            `json:"code"`
				Details map[string]string `json:"details"`
			}
			decodeBody(t, rec, &got)
			if got.Code != "VALIDATION_ERROR" || got.Details["field"] != "body" {
				t.Errorf("got code=%q details=%+v", got.Code, got.Details)
			}
		})
	}
}

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
			UpdateFunc: func(ctx context.Context, u uuid.UUID, r model.UpdateUserSettingsRequest) (*model.UserSettings, error) {
				return &model.UserSettings{WeightUnit: *r.WeightUnit, WeeklyGoal: *r.WeeklyGoal, Palette: "obsidianEmber"}, nil
			},
		}
		rec := httptest.NewRecorder()
		unit, goal := "kg", 1
		body := model.UpdateUserSettingsRequest{WeightUnit: &unit, WeeklyGoal: &goal}
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
			UpdateFunc: func(ctx context.Context, u uuid.UUID, r model.UpdateUserSettingsRequest) (*model.UserSettings, error) {
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
			UpdateFunc: func(ctx context.Context, u uuid.UUID, r model.UpdateUserSettingsRequest) (*model.UserSettings, error) {
				return nil, &model.NotFoundError{Message: "missing"}
			},
		}
		rec := httptest.NewRecorder()
		NewSettingsHandler(svc).Update(rec, newReq(t, "PUT", "/", model.UpdateUserSettingsRequest{}, uid))
		if rec.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("conflict -> 409", func(t *testing.T) {
		svc := &MockSettingsService{
			UpdateFunc: func(ctx context.Context, u uuid.UUID, r model.UpdateUserSettingsRequest) (*model.UserSettings, error) {
				return nil, &model.ConflictError{Message: "dupe"}
			},
		}
		rec := httptest.NewRecorder()
		NewSettingsHandler(svc).Update(rec, newReq(t, "PUT", "/", model.UpdateUserSettingsRequest{}, uid))
		if rec.Code != http.StatusConflict {
			t.Errorf("expected 409, got %d", rec.Code)
		}
	})
}
