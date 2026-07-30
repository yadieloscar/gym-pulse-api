package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

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

var (
	ErrAvatarIdentityInactive    = errors.New("avatar identity inactive")
	ErrAvatarIdentityCheckFailed = errors.New("avatar identity check failed")
)

const (
	avatarCommitCheckTimeout = 2 * time.Second
)

type ProfileActiveUserChecker interface {
	Exists(ctx context.Context, userID uuid.UUID) (bool, error)
}

type profileService struct {
	repo      dao.ProfileDAO
	validator *validator.Validate
	storage   AvatarStorage
	active    ProfileActiveUserChecker
	locker    UserOperationLocker
}

// NewProfileService creates a new ProfileService.
func NewProfileService(repo dao.ProfileDAO, v *validator.Validate, storage ...AvatarStorage) ProfileService {
	var avatarStorage AvatarStorage
	if len(storage) > 0 {
		avatarStorage = storage[0]
	}
	return &profileService{repo: repo, validator: v, storage: avatarStorage}
}

func NewProfileServiceWithUserBoundary(
	repo dao.ProfileDAO,
	v *validator.Validate,
	storage AvatarStorage,
	active ProfileActiveUserChecker,
	locker UserOperationLocker,
) ProfileService {
	return &profileService{
		repo: repo, validator: v, storage: storage, active: active, locker: locker,
	}
}

func (s *profileService) UploadAvatar(ctx context.Context, userID uuid.UUID, contentType string, data []byte) (*model.UserProfile, error) {
	var profile *model.UserProfile
	upload := func(ctx context.Context) error {
		if s.active != nil {
			exists, err := s.active.Exists(ctx, userID)
			if err != nil {
				return fmt.Errorf("%w: %w", ErrAvatarIdentityCheckFailed, err)
			}
			if !exists {
				return ErrAvatarIdentityInactive
			}
		}
		if s.storage == nil {
			return ErrAvatarStorageUnavailable
		}

		current, err := s.repo.Get(ctx, userID)
		if err != nil {
			return fmt.Errorf("loading current profile before avatar upload: %w", err)
		}
		if current == nil {
			current = &model.UserProfile{ID: userID}
		}

		objectPath := nextAvatarObjectPath(userID, current.AvatarURL)
		avatarURL, err := s.storage.Upload(ctx, objectPath, contentType, data)
		if err != nil {
			return err
		}
		committed, err := s.repo.ReplaceAvatar(ctx, userID, avatarURL)
		if err != nil {
			persistErr := fmt.Errorf("persisting avatar profile: %w", err)
			confirmed, confirmErr := s.confirmAvatarCommit(ctx, userID)
			if confirmErr != nil {
				// The write result is ambiguous. Keep the newly uploaded object:
				// deleting it could remove the avatar the database now references.
				return errors.Join(
					persistErr,
					fmt.Errorf("confirming avatar profile commit: %w", confirmErr),
				)
			}
			if confirmed != nil && confirmed.AvatarURL != nil && *confirmed.AvatarURL == avatarURL {
				profile = confirmed
				slog.WarnContext(ctx, "avatar profile write returned an error but the commit was confirmed")
				return nil
			}
			// A read that still shows the previous URL is not proof that an
			// errored autocommit cannot commit afterward. Retain the bounded
			// inactive object; a retry safely overwrites the inactive slot,
			// and account deletion eventually removes every bounded path.
			return persistErr
		}

		profile = committed
		return nil
	}
	if s.locker != nil {
		if err := s.locker.WithUserLock(ctx, userID, upload); err != nil {
			return nil, err
		}
		return profile, nil
	}
	if err := upload(ctx); err != nil {
		return nil, err
	}
	return profile, nil
}

func (s *profileService) confirmAvatarCommit(ctx context.Context, userID uuid.UUID) (*model.UserProfile, error) {
	confirmCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), avatarCommitCheckTimeout)
	defer cancel()
	return s.repo.Get(confirmCtx, userID)
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
