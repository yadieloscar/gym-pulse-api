package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func TestProfileService_Get(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	t.Run("success", func(t *testing.T) {
		displayName := "John Doe"
		mockProfile := &model.UserProfile{
			ID:                  userID,
			DisplayName:         &displayName,
			OnboardingCompleted: true,
			CreatedAt:           time.Now(),
		}

		repo := &MockProfileDAO{
			GetFunc: func(ctx context.Context, id uuid.UUID) (*model.UserProfile, error) {
				if id != userID {
					t.Errorf("expected userID %s, got %s", userID, id)
				}
				return mockProfile, nil
			},
		}

		v := validator.New()
		svc := NewProfileService(repo, v)

		p, err := svc.Get(ctx, userID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.ID != userID {
			t.Errorf("expected ID %s, got %s", userID, p.ID)
		}
		if p.DisplayName == nil || *p.DisplayName != displayName {
			t.Errorf("expected DisplayName %s", displayName)
		}
	})

	t.Run("db error", func(t *testing.T) {
		repo := &MockProfileDAO{
			GetFunc: func(ctx context.Context, id uuid.UUID) (*model.UserProfile, error) {
				return nil, errors.New("db error")
			},
		}

		v := validator.New()
		svc := NewProfileService(repo, v)

		_, err := svc.Get(ctx, userID)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestProfileService_Update(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	displayName := "Jane Doe"
	validReq := model.UpdateProfileRequest{
		DisplayName: &displayName,
	}

	t.Run("success", func(t *testing.T) {
		upsertCalled := false
		repo := &MockProfileDAO{
			UpsertFunc: func(ctx context.Context, id uuid.UUID, req *model.UpdateProfileRequest) error {
				if id != userID {
					t.Errorf("expected userID %s, got %s", userID, id)
				}
				if req.DisplayName == nil || *req.DisplayName != displayName {
					t.Errorf("expected DisplayName %s", displayName)
				}
				upsertCalled = true
				return nil
			},
			GetFunc: func(ctx context.Context, id uuid.UUID) (*model.UserProfile, error) {
				return &model.UserProfile{
					ID:                  userID,
					DisplayName:         &displayName,
					OnboardingCompleted: true,
				}, nil
			},
		}

		v := validator.New()
		svc := NewProfileService(repo, v)

		p, err := svc.Update(ctx, userID, validReq)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !upsertCalled {
			t.Error("expected upsert to be called")
		}
		if p.DisplayName == nil || *p.DisplayName != displayName {
			t.Errorf("expected updated display name %s", displayName)
		}
	})

	t.Run("validation failure - short display name", func(t *testing.T) {
		shortName := "a"
		req := model.UpdateProfileRequest{
			DisplayName: &shortName,
		}

		repo := &MockProfileDAO{}
		v := validator.New()
		svc := NewProfileService(repo, v)

		_, err := svc.Update(ctx, userID, req)
		if err == nil {
			t.Fatal("expected validation error, got nil")
		}
		var valErr *model.ValidationError
		if !errors.As(err, &valErr) {
			t.Errorf("expected ValidationError, got %T", err)
		}
	})

	t.Run("validation failure - long display name", func(t *testing.T) {
		longName := "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz" // 52 chars
		req := model.UpdateProfileRequest{
			DisplayName: &longName,
		}

		repo := &MockProfileDAO{}
		v := validator.New()
		svc := NewProfileService(repo, v)

		_, err := svc.Update(ctx, userID, req)
		if err == nil {
			t.Fatal("expected validation error, got nil")
		}
	})

	t.Run("upsert error", func(t *testing.T) {
		repo := &MockProfileDAO{
			UpsertFunc: func(ctx context.Context, id uuid.UUID, req *model.UpdateProfileRequest) error {
				return errors.New("upsert failed")
			},
		}

		v := validator.New()
		svc := NewProfileService(repo, v)

		_, err := svc.Update(ctx, userID, validReq)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
