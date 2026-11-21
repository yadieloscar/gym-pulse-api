package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// WeekOffService lists, takes, and ends an athlete's weeks off. A week is
// named by its Monday. Taking and ending are idempotent by that date, so
// they carry no operation key. See docs/CONTRACTS.md "Weeks off".
type WeekOffService interface {
	// List returns the weeks off that start within from..to, oldest first.
	List(ctx context.Context, userID uuid.UUID, from, to string) (*model.WeeksOffResponse, error)
	// Take marks the week starting on weekStart as off. The week must be a
	// Monday, not before the athlete's current week, and at most
	// model.MaxWeekOffAheadWeeks weeks after it.
	Take(ctx context.Context, userID uuid.UUID, weekStart string) (*model.WeekOff, error)
	// End removes the week off starting on weekStart. A week before the
	// athlete's current week cannot be ended.
	End(ctx context.Context, userID uuid.UUID, weekStart string) error
}

type weekOffService struct {
	weeks dao.WeekOffDAO
	now   func() time.Time
}

// NewWeekOffService returns a WeekOffService.
func NewWeekOffService(weeks dao.WeekOffDAO) WeekOffService {
	return &weekOffService{weeks: weeks, now: time.Now}
}

func (s *weekOffService) List(ctx context.Context, userID uuid.UUID, from, to string) (*model.WeeksOffResponse, error) {
	fromDate, err := model.ParseDate(from)
	if err != nil {
		return nil, &model.ValidationError{Message: "from must be YYYY-MM-DD", Field: "from"}
	}
	toDate, err := model.ParseDate(to)
	if err != nil {
		return nil, &model.ValidationError{Message: "to must be YYYY-MM-DD", Field: "to"}
	}
	if toDate.Before(fromDate) {
		return nil, &model.ValidationError{Message: "to must not be before " + from, Field: "to"}
	}
	if toDate.After(fromDate.AddDate(0, 0, model.MaxWeeksOffRangeDays-1)) {
		return nil, &model.ValidationError{Message: "date range must not exceed 731 inclusive days", Field: "range"}
	}
	weeks, err := s.weeks.List(ctx, userID, from, to)
	if err != nil {
		return nil, err
	}
	return &model.WeeksOffResponse{WeeksOff: weeks}, nil
}

func (s *weekOffService) Take(ctx context.Context, userID uuid.UUID, weekStart string) (*model.WeekOff, error) {
	week, current, err := s.resolveWeek(ctx, userID, weekStart)
	if err != nil {
		return nil, err
	}
	if week.After(current.AddDate(0, 0, 7*model.MaxWeekOffAheadWeeks)) {
		return nil, &model.ValidationError{Message: "week_start must be within 8 weeks of the current week", Field: "week_start"}
	}
	return s.weeks.Put(ctx, userID, weekStart)
}

func (s *weekOffService) End(ctx context.Context, userID uuid.UUID, weekStart string) error {
	if _, _, err := s.resolveWeek(ctx, userID, weekStart); err != nil {
		return err
	}
	return s.weeks.Delete(ctx, userID, weekStart)
}

// resolveWeek parses weekStart and returns it with the Monday of the
// athlete's current week. It rejects a date that is not a Monday or that is
// before the current week. The current week is judged in the athlete's saved
// timezone, or in UTC when they have no training profile.
func (s *weekOffService) resolveWeek(ctx context.Context, userID uuid.UUID, weekStart string) (week, current time.Time, err error) {
	week, err = model.ParseDate(weekStart)
	if err != nil {
		return time.Time{}, time.Time{}, &model.ValidationError{Message: "week_start must be YYYY-MM-DD", Field: "week_start"}
	}
	if week.Weekday() != time.Monday {
		return time.Time{}, time.Time{}, &model.ValidationError{Message: "week_start must be a Monday", Field: "week_start"}
	}
	timezone, err := s.weeks.Timezone(ctx, userID)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	location := time.UTC
	if timezone != "" {
		location, err = time.LoadLocation(timezone)
		if err != nil {
			return time.Time{}, time.Time{}, &model.ValidationError{Message: "invalid saved timezone", Field: "timezone"}
		}
	}
	current = mondayOf(s.now().In(location))
	if week.Before(current) {
		return time.Time{}, time.Time{}, &model.ValidationError{Message: "week_start must not be before the current week", Field: "week_start"}
	}
	return week, current, nil
}

// mondayOf returns the Monday of t's calendar week as a UTC midnight, the
// form model.ParseDate returns.
func mondayOf(t time.Time) time.Time {
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
}
