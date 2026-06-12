package service

import (
	"context"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// ExerciseCatalogService defines read operations on the exercise catalog.
type ExerciseCatalogService interface {
	List(ctx context.Context, category string) ([]model.CatalogExercise, error)
}

type exerciseCatalogService struct {
	repo dao.ExerciseCatalogDAO
}

// NewExerciseCatalogService creates a new ExerciseCatalogService.
func NewExerciseCatalogService(repo dao.ExerciseCatalogDAO) ExerciseCatalogService {
	return &exerciseCatalogService{repo: repo}
}

func (s *exerciseCatalogService) List(ctx context.Context, category string) ([]model.CatalogExercise, error) {
	if category != "" && !model.IsValidTypeID(category) {
		return nil, &model.ValidationError{Message: "invalid category: " + category, Field: "category"}
	}
	return s.repo.List(ctx, category)
}
