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

func uuidPtr(u uuid.UUID) *uuid.UUID { return &u }

// templateRepo that owns exactly one template id.
func planTemplateRepo(owned uuid.UUID) *MockTemplateDAO {
	return &MockTemplateDAO{
		GetByIDFunc: func(ctx context.Context, userID, templateID uuid.UUID) (*model.WorkoutTemplate, error) {
			if templateID == owned {
				return &model.WorkoutTemplate{ID: templateID, TypeID: "push", SubtypeID: "hypertrophy"}, nil
			}
			return nil, &model.NotFoundError{Message: "template not found"}
		},
	}
}

func TestPlanService_PutWeekly(t *testing.T) {
	owned := uuid.New()
	uid := uuid.New()

	tests := []struct {
		name    string
		days    []model.WeeklyPlanDay
		wantErr bool
	}{
		{name: "valid sparse plan", days: []model.WeeklyPlanDay{
			{Weekday: 1, TemplateID: uuidPtr(owned)},
			{Weekday: 7, Rest: true},
		}},
		{name: "empty plan clears everything", days: []model.WeeklyPlanDay{}},
		{name: "weekday out of range", days: []model.WeeklyPlanDay{{Weekday: 8, Rest: true}}, wantErr: true},
		{name: "weekday zero", days: []model.WeeklyPlanDay{{Weekday: 0, Rest: true}}, wantErr: true},
		{name: "duplicate weekday", days: []model.WeeklyPlanDay{
			{Weekday: 3, Rest: true}, {Weekday: 3, TemplateID: uuidPtr(owned)},
		}, wantErr: true},
		{name: "rest with template rejected", days: []model.WeeklyPlanDay{
			{Weekday: 2, TemplateID: uuidPtr(owned), Rest: true},
		}, wantErr: true},
		{name: "foreign template rejected", days: []model.WeeklyPlanDay{
			{Weekday: 2, TemplateID: uuidPtr(uuid.New())},
		}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stored []model.WeeklyPlanDay
			repo := &MockPlanDAO{
				PutWeeklyFunc: func(ctx context.Context, u uuid.UUID, days []model.WeeklyPlanDay) error {
					stored = days
					return nil
				},
				GetWeeklyFunc: func(ctx context.Context, u uuid.UUID) ([]model.WeeklyPlanDay, error) {
					return stored, nil
				},
			}
			svc := NewPlanService(repo, planTemplateRepo(owned), validator.New())
			got, err := svc.PutWeekly(context.Background(), uid, model.PutWeeklyPlanRequest{Days: tt.days})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.days) {
				t.Errorf("stored %d days, want %d", len(got), len(tt.days))
			}
		})
	}
}

func TestPlanService_Overrides(t *testing.T) {
	owned := uuid.New()
	uid := uuid.New()
	svc := func(repo *MockPlanDAO) PlanService {
		return NewPlanService(repo, planTemplateRepo(owned), validator.New())
	}

	t.Run("valid override upserts", func(t *testing.T) {
		called := false
		repo := &MockPlanDAO{UpsertOverrideFunc: func(ctx context.Context, u uuid.UUID, date string, o model.PutPlanOverrideRequest) error {
			called = true
			return nil
		}}
		err := svc(repo).PutOverride(context.Background(), uid, "2026-06-15", model.PutPlanOverrideRequest{TemplateID: uuidPtr(owned)})
		if err != nil || !called {
			t.Fatalf("err=%v called=%v", err, called)
		}
	})

	t.Run("bad date rejected", func(t *testing.T) {
		err := svc(&MockPlanDAO{}).PutOverride(context.Background(), uid, "june-15", model.PutPlanOverrideRequest{Rest: true})
		var vErr *model.ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("want ValidationError, got %v", err)
		}
	})

	t.Run("rest with template rejected", func(t *testing.T) {
		err := svc(&MockPlanDAO{}).PutOverride(context.Background(), uid, "2026-06-15", model.PutPlanOverrideRequest{TemplateID: uuidPtr(owned), Rest: true})
		var vErr *model.ValidationError
		if !errors.As(err, &vErr) {
			t.Fatalf("want ValidationError, got %v", err)
		}
	})

	t.Run("foreign template rejected", func(t *testing.T) {
		err := svc(&MockPlanDAO{}).PutOverride(context.Background(), uid, "2026-06-15", model.PutPlanOverrideRequest{TemplateID: uuidPtr(uuid.New())})
		var nfErr *model.NotFoundError
		if !errors.As(err, &nfErr) {
			t.Fatalf("want NotFoundError, got %v", err)
		}
	})

	t.Run("delete validates date then delegates", func(t *testing.T) {
		repo := &MockPlanDAO{DeleteOverrideFunc: func(ctx context.Context, u uuid.UUID, date string) error {
			return &model.NotFoundError{Message: "plan override not found"}
		}}
		err := svc(repo).DeleteOverride(context.Background(), uid, "2026-06-15")
		var nfErr *model.NotFoundError
		if !errors.As(err, &nfErr) {
			t.Fatalf("want NotFoundError passthrough, got %v", err)
		}
		if err := svc(&MockPlanDAO{}).DeleteOverride(context.Background(), uid, "not-a-date"); err == nil {
			t.Fatal("bad date should error")
		}
	})
}

func TestPlanService_Get(t *testing.T) {
	uid := uuid.New()
	owned := uuid.New()

	t.Run("explicit window forwarded to dao", func(t *testing.T) {
		var gotFrom, gotTo time.Time
		repo := &MockPlanDAO{GetOverridesFunc: func(ctx context.Context, u uuid.UUID, from, to time.Time) ([]model.PlanOverride, error) {
			gotFrom, gotTo = from, to
			return []model.PlanOverride{}, nil
		}}
		_, err := NewPlanService(repo, planTemplateRepo(owned), validator.New()).Get(context.Background(), uid, "2026-06-01", "2026-06-30")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotFrom.Format("2006-01-02") != "2026-06-01" || gotTo.Format("2006-01-02") != "2026-06-30" {
			t.Errorf("window %v..%v", gotFrom, gotTo)
		}
	})

	t.Run("default window is ±4 weeks", func(t *testing.T) {
		var gotFrom, gotTo time.Time
		repo := &MockPlanDAO{GetOverridesFunc: func(ctx context.Context, u uuid.UUID, from, to time.Time) ([]model.PlanOverride, error) {
			gotFrom, gotTo = from, to
			return []model.PlanOverride{}, nil
		}}
		_, err := NewPlanService(repo, planTemplateRepo(owned), validator.New()).Get(context.Background(), uid, "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if d := gotTo.Sub(gotFrom); d != 56*24*time.Hour {
			t.Errorf("window span = %v, want 8 weeks", d)
		}
	})

	t.Run("inverted window rejected", func(t *testing.T) {
		_, err := NewPlanService(&MockPlanDAO{}, planTemplateRepo(owned), validator.New()).Get(context.Background(), uid, "2026-06-30", "2026-06-01")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("bad from date rejected", func(t *testing.T) {
		_, err := NewPlanService(&MockPlanDAO{}, planTemplateRepo(owned), validator.New()).Get(context.Background(), uid, "soon", "")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
