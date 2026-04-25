package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type StatsService interface {
	GetSummary(ctx context.Context, userID uuid.UUID) (*model.StatsSummary, error)
	GetDistribution(ctx context.Context, userID uuid.UUID) ([]model.TypeDistribution, error)
}

type statsService struct {
	statsRepo    dao.StatsDAO
	settingsRepo dao.SettingsDAO
}

func NewStatsService(statsRepo dao.StatsDAO, settingsRepo dao.SettingsDAO) StatsService {
	return &statsService{statsRepo: statsRepo, settingsRepo: settingsRepo}
}

func (s *statsService) GetSummary(ctx context.Context, userID uuid.UUID) (*model.StatsSummary, error) {
	settings, err := s.settingsRepo.Get(ctx, userID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	monday := model.MondayOfWeek(now)
	sunday := monday.AddDate(0, 0, 6)

	thisWeekCount, err := s.statsRepo.GetWeeklyCount(ctx, userID, monday, sunday)
	if err != nil {
		return nil, err
	}

	totalWorkouts, err := s.statsRepo.GetTotalWorkouts(ctx, userID)
	if err != nil {
		return nil, err
	}

	streak, err := s.calculateStreak(ctx, userID, settings.WeeklyGoal)
	if err != nil {
		return nil, err
	}

	return &model.StatsSummary{
		ThisWeek: model.WeekProgress{
			Completed: thisWeekCount,
			Goal:      settings.WeeklyGoal,
		},
		StreakWeeks:   streak,
		TotalWorkouts: totalWorkouts,
	}, nil
}

func (s *statsService) GetDistribution(ctx context.Context, userID uuid.UUID) ([]model.TypeDistribution, error) {
	return s.statsRepo.GetDistribution(ctx, userID)
}

func (s *statsService) calculateStreak(ctx context.Context, userID uuid.UUID, goal int) (int, error) {
	streak := 0
	currentMonday := model.MondayOfWeek(time.Now())
	checkWeek := currentMonday

	for {
		sunday := checkWeek.AddDate(0, 0, 6)
		count, err := s.statsRepo.GetWeeklyCount(ctx, userID, checkWeek, sunday)
		if err != nil {
			return 0, err
		}

		if checkWeek.Equal(currentMonday) {
			// Current week: grace period.
			if count > 0 {
				streak++
			}
			// Move to previous week regardless.
			checkWeek = checkWeek.AddDate(0, 0, -7)
			continue
		}

		// Past weeks: must meet goal.
		if count >= goal {
			streak++
			checkWeek = checkWeek.AddDate(0, 0, -7)
		} else {
			break
		}
	}

	return streak, nil
}
