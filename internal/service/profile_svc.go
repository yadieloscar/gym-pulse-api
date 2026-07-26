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
	UploadAvatar(ctx context.Context, userID uuid.UUID, contentType string, data []byte) (*model.UserProfile, error)
}

type profileService struct {
	repo      dao.ProfileDAO
	validator *validator.Validate
	storage   AvatarStorage
}

// NewProfileService creates a new ProfileService.
func NewProfileService(repo dao.ProfileDAO, v *validator.Validate, storage ...AvatarStorage) ProfileService {
	var avatarStorage AvatarStorage
	if len(storage) > 0 {
		avatarStorage = storage[0]
	}
	return &profileService{repo: repo, validator: v, storage: avatarStorage}
}

func (s *profileService) UploadAvatar(ctx context.Context, userID uuid.UUID, contentType string, data []byte) (*model.UserProfile, error) {
	if s.storage == nil {
		return nil, ErrAvatarStorageUnavailable
	}
	avatarURL, err := s.storage.Upload(ctx, userID.String()+"/avatar", contentType, data)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Upsert(ctx, userID, &model.UpdateProfileRequest{AvatarURL: &avatarURL}); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, userID)
}

func (s *profileService) Get(ctx context.Context, userID uuid.UUID) (*model.UserProfile, error) {
	return s.repo.Get(ctx, userID)
}

func (s *profileService) Update(ctx context.Context, userID uuid.UUID, req model.UpdateProfileRequest) (*model.UserProfile, error) {
	if req.OnboardingCompleted != nil && !*req.OnboardingCompleted {
		return nil, &model.ValidationError{Message: "onboarding_completed cannot be reset", Field: "onboarding_completed"}
	}
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid profile data", Field: "body"}
	}

	if err := s.repo.Upsert(ctx, userID, &req); err != nil {
		return nil, err
	}

	return s.repo.Get(ctx, userID)
}
