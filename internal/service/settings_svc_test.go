package service

import (
	"context"
	"errors"
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func TestSettingsService_Get(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	t.Run("success", func(t *testing.T) {
		want := &model.UserSettings{WeightUnit: "lb", WeeklyGoal: 5}
		repo := &MockSettingsDAO{
			GetFunc: func(ctx context.Context, id uuid.UUID) (*model.UserSettings, error) {
				if id != userID {
					t.Errorf("userID mismatch")
				}
				return want, nil
			},
		}
		svc := NewSettingsService(repo, validator.New())
		got, err := svc.Get(ctx, userID)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got.WeightUnit != "lb" || got.WeeklyGoal != 5 {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repo := &MockSettingsDAO{
			GetFunc: func(ctx context.Context, id uuid.UUID) (*model.UserSettings, error) {
				return nil, errors.New("db down")
			},
		}
		svc := NewSettingsService(repo, validator.New())
		if _, err := svc.Get(ctx, userID); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestSettingsService_Update(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	cases := []struct {
		name    string
		req     model.UserSettings
		wantErr bool
		isVal   bool
	}{
		{"valid lb 5", model.UserSettings{WeightUnit: "lb", WeeklyGoal: 5}, false, false},
		{"valid kg 1 (regression: weekly_goal=1)", model.UserSettings{WeightUnit: "kg", WeeklyGoal: 1}, false, false},
		{"valid kg 7", model.UserSettings{WeightUnit: "kg", WeeklyGoal: 7}, false, false},
		{"missing weight_unit", model.UserSettings{WeeklyGoal: 3}, true, true},
		{"missing weekly_goal", model.UserSettings{WeightUnit: "lb"}, true, true},
		{"invalid unit", model.UserSettings{WeightUnit: "stone", WeeklyGoal: 3}, true, true},
		{"goal too high", model.UserSettings{WeightUnit: "lb", WeeklyGoal: 8}, true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &MockSettingsDAO{
				UpsertFunc: func(ctx context.Context, id uuid.UUID, s *model.UserSettings) error {
					return nil
				},
			}
			svc := NewSettingsService(repo, validator.New())
			_, err := svc.Update(ctx, userID, tc.req)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.isVal {
				var ve *model.ValidationError
				if !errors.As(err, &ve) {
					t.Fatalf("expected ValidationError, got %T", err)
				}
				if ve.Field != "body" {
					t.Errorf("expected field 'body', got %q", ve.Field)
				}
			}
		})
	}

	t.Run("upsert error propagates", func(t *testing.T) {
		repo := &MockSettingsDAO{
			UpsertFunc: func(ctx context.Context, id uuid.UUID, s *model.UserSettings) error {
				return errors.New("boom")
			},
		}
		svc := NewSettingsService(repo, validator.New())
		_, err := svc.Update(ctx, userID, model.UserSettings{WeightUnit: "lb", WeeklyGoal: 3})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
