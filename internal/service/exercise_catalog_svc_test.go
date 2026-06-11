package service

import (
	"context"
	"errors"
	"testing"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

var errCatalogDB = errors.New("db down")

func TestExerciseCatalogService_List(t *testing.T) {
	sample := []model.CatalogExercise{
		{Name: "Barbell Bench Press", Category: "push", Modality: "strength"},
	}

	tests := []struct {
		name      string
		category  string
		daoResult []model.CatalogExercise
		daoErr    error
		wantErr   bool
		wantLen   int
	}{
		{name: "all categories", category: "", daoResult: sample, wantLen: 1},
		{name: "valid category filter", category: "push", daoResult: sample, wantLen: 1},
		{name: "another valid category", category: "cardio", daoResult: []model.CatalogExercise{}, wantLen: 0},
		{name: "invalid category rejected", category: "biceps", wantErr: true},
		{name: "dao error propagates", category: "", daoErr: errCatalogDB, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &MockExerciseCatalogDAO{
				ListFunc: func(ctx context.Context, category string) ([]model.CatalogExercise, error) {
					if tt.daoErr != nil {
						return nil, tt.daoErr
					}
					return tt.daoResult, nil
				},
			}
			got, err := NewExerciseCatalogService(repo).List(context.Background(), tt.category)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Errorf("len=%d want %d", len(got), tt.wantLen)
			}
		})
	}

	t.Run("invalid category is a ValidationError", func(t *testing.T) {
		_, err := NewExerciseCatalogService(&MockExerciseCatalogDAO{}).List(context.Background(), "nope")
		var vErr *model.ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("want ValidationError, got %T: %v", err, err)
		}
	})
}
