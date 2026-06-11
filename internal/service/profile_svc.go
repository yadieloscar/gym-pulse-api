package service

import (
	"context"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// ProfileService defines operations on user profiles.
type ProfileService interface {
	Get(ctx context.Context, userID uuid.UUID) (*model.UserProfile, error)
	Update(ctx context.Context, userID uuid.UUID, req model.UpdateProfileRequest) (*model.UserProfile, error)
}

type profileService struct {
	repo      dao.ProfileDAO
	validator *validator.Validate
}

// NewProfileService creates a new ProfileService.
func NewProfileService(repo dao.ProfileDAO, v *validator.Validate) ProfileService {
	return &profileService{repo: repo, validator: v}
}

func (s *profileService) Get(ctx context.Context, userID uuid.UUID) (*model.UserProfile, error) {
	return s.repo.Get(ctx, userID)
}

func (s *profileService) Update(ctx context.Context, userID uuid.UUID, req model.UpdateProfileRequest) (*model.UserProfile, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid profile data", Field: "body"}
	}

	if err := s.repo.Upsert(ctx, userID, &req); err != nil {
		return nil, err
	}

	return s.repo.Get(ctx, userID)
}
