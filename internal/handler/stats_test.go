package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func TestStatsHandler_Summary(t *testing.T) {
	uid := uuid.New()

	t.Run("success", func(t *testing.T) {
		svc := &MockStatsService{
			GetSummaryFunc: func(ctx context.Context, u uuid.UUID) (*model.StatsSummary, error) {
				return &model.StatsSummary{
					ThisWeek:      model.WeekProgress{Completed: 2, Goal: 5},
					StreakWeeks:   3,
					StreakDays:    4,
					TotalWorkouts: 10,
				}, nil
			},
		}
		rec := httptest.NewRecorder()
		NewStatsHandler(svc).Summary(rec, newReq(t, "GET", "/", nil, uid))
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d", rec.Code)
		}
		var got model.StatsSummary
		decodeBody(t, rec, &got)
		if got.ThisWeek.Goal != 5 || got.TotalWorkouts != 10 {
			t.Errorf("unexpected: %+v", got)
		}
	})

	t.Run("service error -> 500", func(t *testing.T) {
		svc := &MockStatsService{
			GetSummaryFunc: func(ctx context.Context, u uuid.UUID) (*model.StatsSummary, error) {
				return nil, errors.New("db")
			},
		}
		rec := httptest.NewRecorder()
		NewStatsHandler(svc).Summary(rec, newReq(t, "GET", "/", nil, uid))
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", rec.Code)
		}
	})
}

func TestStatsHandler_Distribution(t *testing.T) {
	uid := uuid.New()

	t.Run("wraps under types key with array", func(t *testing.T) {
		svc := &MockStatsService{
			GetDistributionFunc: func(ctx context.Context, u uuid.UUID) ([]model.TypeDistribution, error) {
				return []model.TypeDistribution{{TypeID: "push", Count: 2}}, nil
			},
		}
		rec := httptest.NewRecorder()
		NewStatsHandler(svc).Distribution(rec, newReq(t, "GET", "/", nil, uid))
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d", rec.Code)
		}
		var got map[string][]model.TypeDistribution
		decodeBody(t, rec, &got)
		if _, ok := got["types"]; !ok {
			t.Fatalf("missing 'types' key: %s", rec.Body.String())
		}
		if len(got["types"]) != 1 || got["types"][0].TypeID != "push" {
			t.Errorf("unexpected: %+v", got)
		}
	})

	t.Run("nil distribution wrapped as types:null", func(t *testing.T) {
		svc := &MockStatsService{
			GetDistributionFunc: func(ctx context.Context, u uuid.UUID) ([]model.TypeDistribution, error) {
				return nil, nil
			},
		}
		rec := httptest.NewRecorder()
		NewStatsHandler(svc).Distribution(rec, newReq(t, "GET", "/", nil, uid))
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d", rec.Code)
		}
		// raw body should contain the "types" wrapper
		if !contains(rec.Body.String(), `"types"`) {
			t.Errorf("expected 'types' wrapper in body, got %s", rec.Body.String())
		}
	})

	t.Run("service error -> 500", func(t *testing.T) {
		svc := &MockStatsService{
			GetDistributionFunc: func(ctx context.Context, u uuid.UUID) ([]model.TypeDistribution, error) {
				return nil, errors.New("db")
			},
		}
		rec := httptest.NewRecorder()
		NewStatsHandler(svc).Distribution(rec, newReq(t, "GET", "/", nil, uid))
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", rec.Code)
		}
	})
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
