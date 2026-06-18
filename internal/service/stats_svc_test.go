package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func TestStatsService_GetSummary(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	t.Run("success - current week counts, streaks", func(t *testing.T) {
		settingsRepo := &MockSettingsDAO{
			GetFunc: func(ctx context.Context, id uuid.UUID) (*model.UserSettings, error) {
				return &model.UserSettings{WeightUnit: "lb", WeeklyGoal: 3}, nil
			},
		}
		// Streak: current week >=1 + 2 past weeks meeting goal, then break.
		// Compare by week offset (in days) from "this Monday" to avoid sub-second clock skew.
		currentMonday := model.MondayOfWeek(time.Now())
		// daysAgo bucketizes by 7-day intervals relative to currentMonday's date.
		daysAgo := func(start time.Time) int {
			diff := currentMonday.Sub(start)
			return int(diff.Hours()/24 + 0.5)
		}
		statsRepo := &MockStatsDAO{
			GetWeeklyCountFunc: func(ctx context.Context, id uuid.UUID, start, end time.Time) (int, error) {
				switch daysAgo(start) {
				case 0:
					return 2, nil
				case 7:
					return 3, nil
				case 14:
					return 4, nil
				default:
					return 0, nil
				}
			},
			GetTotalWorkoutsFunc: func(ctx context.Context, id uuid.UUID) (int, error) {
				return 42, nil
			},
			GetDayStreakFunc: func(ctx context.Context, id uuid.UUID) (int, error) {
				return 5, nil
			},
		}
		svc := NewStatsService(statsRepo, settingsRepo)
		got, err := svc.GetSummary(ctx, userID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ThisWeek.Completed != 2 || got.ThisWeek.Goal != 3 {
			t.Errorf("this_week mismatch: %+v", got.ThisWeek)
		}
		if got.StreakWeeks != 3 {
			t.Errorf("expected streak 3, got %d", got.StreakWeeks)
		}
		if got.StreakDays != 5 {
			t.Errorf("expected day streak 5, got %d", got.StreakDays)
		}
		if got.TotalWorkouts != 42 {
			t.Errorf("expected total 42, got %d", got.TotalWorkouts)
		}
	})

	t.Run("streak zero when current week below goal and past below", func(t *testing.T) {
		settingsRepo := &MockSettingsDAO{
			GetFunc: func(ctx context.Context, id uuid.UUID) (*model.UserSettings, error) {
				return &model.UserSettings{WeightUnit: "lb", WeeklyGoal: 3}, nil
			},
		}
		statsRepo := &MockStatsDAO{
			GetWeeklyCountFunc: func(ctx context.Context, id uuid.UUID, start, end time.Time) (int, error) {
				return 0, nil
			},
		}
		svc := NewStatsService(statsRepo, settingsRepo)
		got, err := svc.GetSummary(ctx, userID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.StreakWeeks != 0 {
			t.Errorf("expected 0, got %d", got.StreakWeeks)
		}
	})

	t.Run("settings repo error", func(t *testing.T) {
		settingsRepo := &MockSettingsDAO{
			GetFunc: func(ctx context.Context, id uuid.UUID) (*model.UserSettings, error) {
				return nil, errors.New("db")
			},
		}
		svc := NewStatsService(&MockStatsDAO{}, settingsRepo)
		if _, err := svc.GetSummary(ctx, userID); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("weekly count error", func(t *testing.T) {
		settingsRepo := &MockSettingsDAO{
			GetFunc: func(ctx context.Context, id uuid.UUID) (*model.UserSettings, error) {
				return &model.UserSettings{WeightUnit: "lb", WeeklyGoal: 3}, nil
			},
		}
		statsRepo := &MockStatsDAO{
			GetWeeklyCountFunc: func(ctx context.Context, id uuid.UUID, start, end time.Time) (int, error) {
				return 0, errors.New("db")
			},
		}
		svc := NewStatsService(statsRepo, settingsRepo)
		if _, err := svc.GetSummary(ctx, userID); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("total workouts error", func(t *testing.T) {
		settingsRepo := &MockSettingsDAO{
			GetFunc: func(ctx context.Context, id uuid.UUID) (*model.UserSettings, error) {
				return &model.UserSettings{WeightUnit: "lb", WeeklyGoal: 3}, nil
			},
		}
		statsRepo := &MockStatsDAO{
			GetWeeklyCountFunc: func(ctx context.Context, id uuid.UUID, start, end time.Time) (int, error) {
				return 1, nil
			},
			GetTotalWorkoutsFunc: func(ctx context.Context, id uuid.UUID) (int, error) {
				return 0, errors.New("db")
			},
		}
		svc := NewStatsService(statsRepo, settingsRepo)
		if _, err := svc.GetSummary(ctx, userID); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("day streak error", func(t *testing.T) {
		currentMonday := model.MondayOfWeek(time.Now())
		isCurrentWeek := func(start time.Time) bool {
			diff := currentMonday.Sub(start).Hours() / 24
			return diff > -0.5 && diff < 0.5
		}
		settingsRepo := &MockSettingsDAO{
			GetFunc: func(ctx context.Context, id uuid.UUID) (*model.UserSettings, error) {
				return &model.UserSettings{WeightUnit: "lb", WeeklyGoal: 3}, nil
			},
		}
		statsRepo := &MockStatsDAO{
			GetWeeklyCountFunc: func(ctx context.Context, id uuid.UUID, start, end time.Time) (int, error) {
				if isCurrentWeek(start) {
					return 1, nil
				}
				return 0, nil
			},
			GetTotalWorkoutsFunc: func(ctx context.Context, id uuid.UUID) (int, error) {
				return 1, nil
			},
			GetDayStreakFunc: func(ctx context.Context, id uuid.UUID) (int, error) {
				return 0, errors.New("db")
			},
		}
		svc := NewStatsService(statsRepo, settingsRepo)
		if _, err := svc.GetSummary(ctx, userID); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("calculateStreak inner error on past week", func(t *testing.T) {
		currentMonday := model.MondayOfWeek(time.Now())
		isCurrentWeek := func(start time.Time) bool {
			diff := currentMonday.Sub(start).Hours() / 24
			return diff > -0.5 && diff < 0.5
		}
		settingsRepo := &MockSettingsDAO{
			GetFunc: func(ctx context.Context, id uuid.UUID) (*model.UserSettings, error) {
				return &model.UserSettings{WeightUnit: "lb", WeeklyGoal: 3}, nil
			},
		}
		statsRepo := &MockStatsDAO{
			GetWeeklyCountFunc: func(ctx context.Context, id uuid.UUID, start, end time.Time) (int, error) {
				if isCurrentWeek(start) {
					return 1, nil
				}
				return 0, errors.New("db")
			},
			GetTotalWorkoutsFunc: func(ctx context.Context, id uuid.UUID) (int, error) {
				return 1, nil
			},
		}
		svc := NewStatsService(statsRepo, settingsRepo)
		if _, err := svc.GetSummary(ctx, userID); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestStatsService_GetDistribution(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	t.Run("success", func(t *testing.T) {
		want := []model.TypeDistribution{{TypeID: "push", Count: 3}}
		repo := &MockStatsDAO{
			GetDistributionFunc: func(ctx context.Context, id uuid.UUID) ([]model.TypeDistribution, error) {
				return want, nil
			},
		}
		svc := NewStatsService(repo, &MockSettingsDAO{})
		got, err := svc.GetDistribution(ctx, userID)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if len(got) != 1 || got[0].TypeID != "push" {
			t.Errorf("unexpected: %+v", got)
		}
	})

	t.Run("error", func(t *testing.T) {
		repo := &MockStatsDAO{
			GetDistributionFunc: func(ctx context.Context, id uuid.UUID) ([]model.TypeDistribution, error) {
				return nil, errors.New("db")
			},
		}
		svc := NewStatsService(repo, &MockSettingsDAO{})
		if _, err := svc.GetDistribution(ctx, userID); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestStatsService_GetVolume(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	t.Run("pads to a continuous N-week series, oldest first", func(t *testing.T) {
		// DAO returns volume for only the current week; the rest pad to 0.
		thisMonday := model.MondayOfWeek(time.Now()).Format("2006-01-02")
		repo := &MockStatsDAO{
			GetWeeklyVolumeFunc: func(ctx context.Context, uid uuid.UUID, since time.Time) ([]model.WeeklyVolume, error) {
				return []model.WeeklyVolume{{WeekStart: thisMonday, Volume: 5000}}, nil
			},
		}
		svc := NewStatsService(repo, &MockSettingsDAO{})

		series, err := svc.GetVolume(ctx, userID, "4")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(series) != 4 {
			t.Fatalf("expected 4 weeks, got %d", len(series))
		}
		if series[3].WeekStart != thisMonday || series[3].Volume != 5000 {
			t.Errorf("last week: got %+v", series[3])
		}
		if series[0].Volume != 0 {
			t.Errorf("padded week should be 0, got %v", series[0].Volume)
		}
	})

	t.Run("defaults to 8 weeks and clamps to 52", func(t *testing.T) {
		repo := &MockStatsDAO{}
		svc := NewStatsService(repo, &MockSettingsDAO{})
		def, _ := svc.GetVolume(ctx, userID, "")
		if len(def) != 8 {
			t.Errorf("default weeks: got %d want 8", len(def))
		}
		big, _ := svc.GetVolume(ctx, userID, "999")
		if len(big) != 52 {
			t.Errorf("clamp: got %d want 52", len(big))
		}
	})

	t.Run("rejects a non-positive weeks param", func(t *testing.T) {
		svc := NewStatsService(&MockStatsDAO{}, &MockSettingsDAO{})
		_, err := svc.GetVolume(ctx, userID, "0")
		var verr *model.ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("expected ValidationError, got %v", err)
		}
	})
}
