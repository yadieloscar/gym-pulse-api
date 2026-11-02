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

func TestExerciseCatalogHandler_List(t *testing.T) {
	uid := uuid.New()

	t.Run("success returns exercises wrapper", func(t *testing.T) {
		svc := &MockExerciseCatalogService{
			ListFunc: func(ctx context.Context, category string) ([]model.CatalogExercise, error) {
				return []model.CatalogExercise{
					{Name: "Barbell Squat", Category: "legs", Modality: "strength", Pattern: "squat", MuscleGroup: "quads", EquipmentRequired: []string{"barbell"}, Impact: "low"},
					{Name: "Treadmill Run", Category: "cardio", Modality: "cardio", Pattern: "locomotion", MuscleGroup: "full_body", EquipmentRequired: []string{"cardio_machine"}, Impact: "high"},
				}, nil
			},
		}
		rec := httptest.NewRecorder()
		NewExerciseCatalogHandler(svc).List(rec, newReq(t, "GET", "/", nil, uid))
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var got map[string][]model.CatalogExercise
		decodeBody(t, rec, &got)
		if len(got["exercises"]) != 2 {
			t.Errorf("exercises len=%d want 2", len(got["exercises"]))
		}
		// Programming metadata (spec 016 FR-005) rides on the same entry shape.
		if first := got["exercises"][0]; first.Pattern != "squat" || first.MuscleGroup != "quads" || len(first.EquipmentRequired) != 1 || first.EquipmentRequired[0] != "barbell" || first.Impact != "low" {
			t.Errorf("programming metadata missing from the wire shape: %+v", first)
		}
	})

	t.Run("category query param is forwarded", func(t *testing.T) {
		var captured string
		svc := &MockExerciseCatalogService{
			ListFunc: func(ctx context.Context, category string) ([]model.CatalogExercise, error) {
				captured = category
				return []model.CatalogExercise{}, nil
			},
		}
		rec := httptest.NewRecorder()
		NewExerciseCatalogHandler(svc).List(rec, newReq(t, "GET", "/?category=push", nil, uid))
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d", rec.Code)
		}
		if captured != "push" {
			t.Errorf("forwarded category=%q want push", captured)
		}
	})

	t.Run("validation error -> 422", func(t *testing.T) {
		svc := &MockExerciseCatalogService{
			ListFunc: func(ctx context.Context, category string) ([]model.CatalogExercise, error) {
				return nil, &model.ValidationError{Message: "invalid category", Field: "category"}
			},
		}
		rec := httptest.NewRecorder()
		NewExerciseCatalogHandler(svc).List(rec, newReq(t, "GET", "/?category=bogus", nil, uid))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("status=%d want 422", rec.Code)
		}
	})

	t.Run("service error -> 500", func(t *testing.T) {
		svc := &MockExerciseCatalogService{
			ListFunc: func(ctx context.Context, category string) ([]model.CatalogExercise, error) {
				return nil, errors.New("db down")
			},
		}
		rec := httptest.NewRecorder()
		NewExerciseCatalogHandler(svc).List(rec, newReq(t, "GET", "/", nil, uid))
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status=%d want 500", rec.Code)
		}
	})
}
