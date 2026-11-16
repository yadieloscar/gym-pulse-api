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
		req     model.UpdateUserSettingsRequest
		wantErr bool
		isVal   bool
	}{
		{"empty preserves all", model.UpdateUserSettingsRequest{}, false, false},
		{"valid palette only", model.UpdateUserSettingsRequest{Palette: stringPtr("abyssCerulean")}, false, false},
		{"valid moss palette", model.UpdateUserSettingsRequest{Palette: stringPtr("mossChartreuse")}, false, false},
		{"valid volt palette", model.UpdateUserSettingsRequest{Palette: stringPtr("voltLime")}, false, false},
		{"valid olive palette", model.UpdateUserSettingsRequest{Palette: stringPtr("oliveCitrine")}, false, false},
		{"valid ember palette", model.UpdateUserSettingsRequest{Palette: stringPtr("obsidianEmber")}, false, false},
		{"valid slate palette", model.UpdateUserSettingsRequest{Palette: stringPtr("slateMint")}, false, false},
		{"valid iris palette", model.UpdateUserSettingsRequest{Palette: stringPtr("irisViolet")}, false, false},
		{"palette is case sensitive", model.UpdateUserSettingsRequest{Palette: stringPtr("irisviolet")}, true, true},
		{"unknown palette", model.UpdateUserSettingsRequest{Palette: stringPtr("neonGrape")}, true, true},
		{"valid kg 1", model.UpdateUserSettingsRequest{WeightUnit: stringPtr("kg"), WeeklyGoal: settingsIntPtr(1)}, false, false},
		{"invalid unit", model.UpdateUserSettingsRequest{WeightUnit: stringPtr("stone")}, true, true},
		{"goal too high", model.UpdateUserSettingsRequest{WeeklyGoal: settingsIntPtr(8)}, true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &MockSettingsDAO{
				PatchFunc: func(ctx context.Context, id uuid.UUID, req model.UpdateUserSettingsRequest) (*model.UserSettings, error) {
					v := model.DefaultUserSettings()
					return &v, nil
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
			PatchFunc: func(ctx context.Context, id uuid.UUID, req model.UpdateUserSettingsRequest) (*model.UserSettings, error) {
				return nil, errors.New("boom")
			},
		}
		svc := NewSettingsService(repo, validator.New())
		_, err := svc.Update(ctx, userID, model.UpdateUserSettingsRequest{WeightUnit: stringPtr("lb"), WeeklyGoal: settingsIntPtr(3)})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func stringPtr(v string) *string { return &v }
func settingsIntPtr(v int) *int  { return &v }

// TestSettingsService_UpdatePassesPaletteToDAO checks that an accepted palette
// reaches the DAO unchanged and that a rejected one never does.
func TestSettingsService_UpdatePassesPaletteToDAO(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	t.Run("irisViolet reaches the DAO", func(t *testing.T) {
		var seen *string
		repo := &MockSettingsDAO{
			PatchFunc: func(_ context.Context, id uuid.UUID, req model.UpdateUserSettingsRequest) (*model.UserSettings, error) {
				if id != userID {
					t.Errorf("userID = %v, want %v", id, userID)
				}
				seen = req.Palette
				return &model.UserSettings{WeightUnit: "lb", WeeklyGoal: 5, Palette: *req.Palette}, nil
			},
		}
		got, err := NewSettingsService(repo, validator.New()).Update(ctx, userID, model.UpdateUserSettingsRequest{Palette: stringPtr("irisViolet")})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if seen == nil || *seen != "irisViolet" || got.Palette != "irisViolet" {
			t.Errorf("DAO saw %v, service returned %+v", seen, got)
		}
	})

	t.Run("unknown palette never reaches the DAO", func(t *testing.T) {
		repo := &MockSettingsDAO{
			PatchFunc: func(context.Context, uuid.UUID, model.UpdateUserSettingsRequest) (*model.UserSettings, error) {
				t.Error("DAO called for an invalid palette")
				return nil, nil
			},
		}
		if _, err := NewSettingsService(repo, validator.New()).Update(ctx, userID, model.UpdateUserSettingsRequest{Palette: stringPtr("neonGrape")}); err == nil {
			t.Fatal("expected a validation error")
		}
	})
}
