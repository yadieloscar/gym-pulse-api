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

func TestBodyWeightService_LogWeight(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	todayStr := time.Now().Format("2006-01-02")

	validReq := model.CreateBodyWeightRequest{
		Date:   todayStr,
		Weight: 175.5,
		Unit:   "lb",
	}

	t.Run("success", func(t *testing.T) {
		upsertCalled := false
		repo := &MockBodyWeightDAO{
			UpsertFunc: func(ctx context.Context, id uuid.UUID, w *model.BodyWeight) (*model.BodyWeight, error) {
				if id != userID {
					t.Errorf("expected userID %s, got %s", userID, id)
				}
				if w.Date != todayStr {
					t.Errorf("expected date %s, got %s", todayStr, w.Date)
				}
				if w.Weight != 175.5 {
					t.Errorf("expected weight 175.5, got %f", w.Weight)
				}
				if w.Unit != "lb" {
					t.Errorf("expected unit lb, got %s", w.Unit)
				}
				upsertCalled = true
				return &model.BodyWeight{
					ID:       uuid.New(),
					UserID:   userID,
					Date:     w.Date,
					Weight:   w.Weight,
					Unit:     w.Unit,
					LoggedAt: time.Now(),
				}, nil
			},
		}

		v := validator.New()
		svc := NewBodyWeightService(repo, v)

		bw, err := svc.LogWeight(ctx, userID, validReq)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !upsertCalled {
			t.Error("expected upsert to be called")
		}
		if bw.Weight != 175.5 {
			t.Errorf("expected returned weight 175.5, got %f", bw.Weight)
		}
	})

	t.Run("validation failure - negative weight", func(t *testing.T) {
		req := model.CreateBodyWeightRequest{
			Date:   todayStr,
			Weight: -5,
			Unit:   "lb",
		}
		repo := &MockBodyWeightDAO{}
		v := validator.New()
		svc := NewBodyWeightService(repo, v)

		_, err := svc.LogWeight(ctx, userID, req)
		if err == nil {
			t.Fatal("expected validation error, got nil")
		}
	})

	t.Run("validation failure - invalid unit", func(t *testing.T) {
		req := model.CreateBodyWeightRequest{
			Date:   todayStr,
			Weight: 150,
			Unit:   "stone",
		}
		repo := &MockBodyWeightDAO{}
		v := validator.New()
		svc := NewBodyWeightService(repo, v)

		_, err := svc.LogWeight(ctx, userID, req)
		if err == nil {
			t.Fatal("expected validation error, got nil")
		}
	})

	t.Run("invalid date format", func(t *testing.T) {
		req := model.CreateBodyWeightRequest{
			Date:   "01-02-2026",
			Weight: 150,
			Unit:   "lb",
		}
		repo := &MockBodyWeightDAO{}
		v := validator.New()
		svc := NewBodyWeightService(repo, v)

		_, err := svc.LogWeight(ctx, userID, req)
		if err == nil {
			t.Fatal("expected parse date error, got nil")
		}
		var valErr *model.ValidationError
		if !errors.As(err, &valErr) {
			t.Errorf("expected ValidationError, got %T", err)
		}
	})

	t.Run("future date rejection", func(t *testing.T) {
		// UTC basis — matches model.UTCToday, stable at any wall-clock time.
		tomorrowStr := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
		req := model.CreateBodyWeightRequest{
			Date:   tomorrowStr,
			Weight: 150,
			Unit:   "lb",
		}
		repo := &MockBodyWeightDAO{}
		v := validator.New()
		svc := NewBodyWeightService(repo, v)

		_, err := svc.LogWeight(ctx, userID, req)
		if err == nil {
			t.Fatal("expected future date error, got nil")
		}
	})

	t.Run("db error", func(t *testing.T) {
		repo := &MockBodyWeightDAO{
			UpsertFunc: func(ctx context.Context, id uuid.UUID, w *model.BodyWeight) (*model.BodyWeight, error) {
				return nil, errors.New("db error")
			},
		}

		v := validator.New()
		svc := NewBodyWeightService(repo, v)

		_, err := svc.LogWeight(ctx, userID, validReq)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestBodyWeightService_ListWeights(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	t.Run("success", func(t *testing.T) {
		mockList := []model.BodyWeight{
			{ID: uuid.New(), UserID: userID, Date: "2026-06-01", Weight: 175.5, Unit: "lb"},
		}
		repo := &MockBodyWeightDAO{
			ListFunc: func(ctx context.Context, id uuid.UUID) ([]model.BodyWeight, error) {
				if id != userID {
					t.Errorf("expected userID %s, got %s", userID, id)
				}
				return mockList, nil
			},
		}

		v := validator.New()
		svc := NewBodyWeightService(repo, v)

		res, err := svc.ListWeights(ctx, userID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 {
			t.Errorf("expected 1 result, got %d", len(res))
		}
	})
}

func TestBodyWeightService_DeleteWeight(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	entryID := uuid.New()

	t.Run("success", func(t *testing.T) {
		deleteCalled := false
		repo := &MockBodyWeightDAO{
			DeleteFunc: func(ctx context.Context, uid, eid uuid.UUID) error {
				if uid != userID {
					t.Errorf("expected userID %s, got %s", userID, uid)
				}
				if eid != entryID {
					t.Errorf("expected entryID %s, got %s", entryID, eid)
				}
				deleteCalled = true
				return nil
			},
		}

		v := validator.New()
		svc := NewBodyWeightService(repo, v)

		err := svc.DeleteWeight(ctx, userID, entryID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !deleteCalled {
			t.Error("expected delete to be called")
		}
	})
}
