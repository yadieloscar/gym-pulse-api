package service

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

const (
	defaultVolumeWeeks = 8
	maxVolumeWeeks     = 52
)

type StatsService interface {
	GetSummary(ctx context.Context, userID uuid.UUID) (*model.StatsSummary, error)
	GetDistribution(ctx context.Context, userID uuid.UUID) ([]model.TypeDistribution, error)
	GetVolume(ctx context.Context, userID uuid.UUID, weeksParam string) ([]model.WeeklyVolume, error)
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

	dayStreak, err := s.statsRepo.GetDayStreak(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &model.StatsSummary{
		ThisWeek: model.WeekProgress{
			Completed: thisWeekCount,
			Goal:      settings.WeeklyGoal,
		},
		StreakWeeks:   streak,
		StreakDays:    dayStreak,
		TotalWorkouts: totalWorkouts,
	}, nil
}

func (s *statsService) GetDistribution(ctx context.Context, userID uuid.UUID) ([]model.TypeDistribution, error) {
	return s.statsRepo.GetDistribution(ctx, userID)
}

// GetVolume returns a continuous weeks-long series of weekly volume (oldest
// first), padding weeks with no logged volume to 0 so the chart has no gaps.
func (s *statsService) GetVolume(ctx context.Context, userID uuid.UUID, weeksParam string) ([]model.WeeklyVolume, error) {
	weeks := defaultVolumeWeeks
	if weeksParam != "" {
		n, err := strconv.Atoi(weeksParam)
		if err != nil || n < 1 {
			return nil, &model.ValidationError{Message: "weeks must be a positive integer", Field: "weeks"}
		}
		weeks = min(n, maxVolumeWeeks)
	}

	currentMonday := model.MondayOfWeek(time.Now())
	since := currentMonday.AddDate(0, 0, -7*(weeks-1))

	rows, err := s.statsRepo.GetWeeklyVolume(ctx, userID, since)
	if err != nil {
		return nil, err
	}
	byWeek := make(map[string]float64, len(rows))
	for _, v := range rows {
		byWeek[v.WeekStart] = v.Volume
	}

	series := make([]model.WeeklyVolume, weeks)
	for i := range weeks {
		monday := currentMonday.AddDate(0, 0, -7*(weeks-1-i))
		key := monday.Format("2006-01-02")
		series[i] = model.WeeklyVolume{WeekStart: key, Volume: byWeek[key]}
	}
	return series, nil
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
