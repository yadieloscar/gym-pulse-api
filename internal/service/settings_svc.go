package service

import (
	"context"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/repository"
)

type SettingsService interface {
	Get(ctx context.Context, userID uuid.UUID) (*model.UserSettings, error)
	Update(ctx context.Context, userID uuid.UUID, req model.UserSettings) (*model.UserSettings, error)
}

type settingsService struct {
	repo      repository.SettingsRepository
	validator *validator.Validate
}

func NewSettingsService(repo repository.SettingsRepository, v *validator.Validate) SettingsService {
	return &settingsService{repo: repo, validator: v}
}

func (s *settingsService) Get(ctx context.Context, userID uuid.UUID) (*model.UserSettings, error) {
	return s.repo.Get(ctx, userID)
}

func (s *settingsService) Update(ctx context.Context, userID uuid.UUID, req model.UserSettings) (*model.UserSettings, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid settings", Field: "body"}
	}

	if err := s.repo.Upsert(ctx, userID, &req); err != nil {
		return nil, err
	}

	return &req, nil
}
