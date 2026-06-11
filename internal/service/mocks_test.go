package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type MockProfileDAO struct {
	GetFunc    func(ctx context.Context, userID uuid.UUID) (*model.UserProfile, error)
	UpsertFunc func(ctx context.Context, userID uuid.UUID, profile *model.UpdateProfileRequest) error
}

func (m *MockProfileDAO) Get(ctx context.Context, userID uuid.UUID) (*model.UserProfile, error) {
	if m.GetFunc != nil {
		return m.GetFunc(ctx, userID)
	}
	return nil, nil
}

func (m *MockProfileDAO) Upsert(ctx context.Context, userID uuid.UUID, profile *model.UpdateProfileRequest) error {
	if m.UpsertFunc != nil {
		return m.UpsertFunc(ctx, userID, profile)
	}
	return nil
}

type MockBodyWeightDAO struct {
	UpsertFunc func(ctx context.Context, userID uuid.UUID, w *model.BodyWeight) (*model.BodyWeight, error)
	ListFunc   func(ctx context.Context, userID uuid.UUID) ([]model.BodyWeight, error)
	DeleteFunc func(ctx context.Context, userID uuid.UUID, entryID uuid.UUID) error
}

func (m *MockBodyWeightDAO) Upsert(ctx context.Context, userID uuid.UUID, w *model.BodyWeight) (*model.BodyWeight, error) {
	if m.UpsertFunc != nil {
		return m.UpsertFunc(ctx, userID, w)
	}
	return nil, nil
}

func (m *MockBodyWeightDAO) List(ctx context.Context, userID uuid.UUID) ([]model.BodyWeight, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, userID)
	}
	return nil, nil
}

func (m *MockBodyWeightDAO) Delete(ctx context.Context, userID uuid.UUID, entryID uuid.UUID) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, userID, entryID)
	}
	return nil
}

type MockLogDAO struct {
	ListByWeekFunc func(ctx context.Context, userID uuid.UUID, weekStart time.Time) ([]model.DayLogSummary, error)
	GetByDateFunc  func(ctx context.Context, userID uuid.UUID, date string) (*model.DayLog, error)
	CreateFunc     func(ctx context.Context, userID uuid.UUID, l *model.DayLog) error
	UpdateFunc     func(ctx context.Context, userID uuid.UUID, date string, overrides []model.ExerciseOverride, sessionNotes *string) error
	DeleteFunc     func(ctx context.Context, userID uuid.UUID, date string) error
}

func (m *MockLogDAO) ListByWeek(ctx context.Context, userID uuid.UUID, weekStart time.Time) ([]model.DayLogSummary, error) {
	if m.ListByWeekFunc != nil {
		return m.ListByWeekFunc(ctx, userID, weekStart)
	}
	return nil, nil
}

func (m *MockLogDAO) GetByDate(ctx context.Context, userID uuid.UUID, date string) (*model.DayLog, error) {
	if m.GetByDateFunc != nil {
		return m.GetByDateFunc(ctx, userID, date)
	}
	return nil, nil
}

func (m *MockLogDAO) Create(ctx context.Context, userID uuid.UUID, l *model.DayLog) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, userID, l)
	}
	return nil
}

func (m *MockLogDAO) Update(ctx context.Context, userID uuid.UUID, date string, overrides []model.ExerciseOverride, sessionNotes *string) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, userID, date, overrides, sessionNotes)
	}
	return nil
}

func (m *MockLogDAO) Delete(ctx context.Context, userID uuid.UUID, date string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, userID, date)
	}
	return nil
}

type MockTemplateDAO struct {
	ListFunc    func(ctx context.Context, userID uuid.UUID, typeFilter, subtypeFilter string) ([]model.TemplateSummary, error)
	GetByIDFunc func(ctx context.Context, userID, templateID uuid.UUID) (*model.WorkoutTemplate, error)
	CreateFunc  func(ctx context.Context, userID uuid.UUID, t *model.WorkoutTemplate) error
	UpdateFunc  func(ctx context.Context, userID uuid.UUID, t *model.WorkoutTemplate) error
	DeleteFunc  func(ctx context.Context, userID, templateID uuid.UUID) error
}

func (m *MockTemplateDAO) List(ctx context.Context, userID uuid.UUID, typeFilter, subtypeFilter string) ([]model.TemplateSummary, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, userID, typeFilter, subtypeFilter)
	}
	return nil, nil
}

func (m *MockTemplateDAO) GetByID(ctx context.Context, userID, templateID uuid.UUID) (*model.WorkoutTemplate, error) {
	if m.GetByIDFunc != nil {
		return m.GetByIDFunc(ctx, userID, templateID)
	}
	return nil, nil
}

func (m *MockTemplateDAO) Create(ctx context.Context, userID uuid.UUID, t *model.WorkoutTemplate) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, userID, t)
	}
	return nil
}

func (m *MockTemplateDAO) Update(ctx context.Context, userID uuid.UUID, t *model.WorkoutTemplate) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, userID, t)
	}
	return nil
}

func (m *MockTemplateDAO) Delete(ctx context.Context, userID, templateID uuid.UUID) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, userID, templateID)
	}
	return nil
}

type MockStatsDAO struct {
	GetWeeklyCountFunc   func(ctx context.Context, userID uuid.UUID, weekStart, weekEnd time.Time) (int, error)
	GetTotalWorkoutsFunc func(ctx context.Context, userID uuid.UUID) (int, error)
	GetDistributionFunc  func(ctx context.Context, userID uuid.UUID) ([]model.TypeDistribution, error)
	GetDayStreakFunc     func(ctx context.Context, userID uuid.UUID) (int, error)
}

func (m *MockStatsDAO) GetWeeklyCount(ctx context.Context, userID uuid.UUID, weekStart, weekEnd time.Time) (int, error) {
	if m.GetWeeklyCountFunc != nil {
		return m.GetWeeklyCountFunc(ctx, userID, weekStart, weekEnd)
	}
	return 0, nil
}

func (m *MockStatsDAO) GetTotalWorkouts(ctx context.Context, userID uuid.UUID) (int, error) {
	if m.GetTotalWorkoutsFunc != nil {
		return m.GetTotalWorkoutsFunc(ctx, userID)
	}
	return 0, nil
}

func (m *MockStatsDAO) GetDistribution(ctx context.Context, userID uuid.UUID) ([]model.TypeDistribution, error) {
	if m.GetDistributionFunc != nil {
		return m.GetDistributionFunc(ctx, userID)
	}
	return nil, nil
}

func (m *MockStatsDAO) GetDayStreak(ctx context.Context, userID uuid.UUID) (int, error) {
	if m.GetDayStreakFunc != nil {
		return m.GetDayStreakFunc(ctx, userID)
	}
	return 0, nil
}

type MockSettingsDAO struct {
	GetFunc    func(ctx context.Context, userID uuid.UUID) (*model.UserSettings, error)
	UpsertFunc func(ctx context.Context, userID uuid.UUID, settings *model.UserSettings) error
}

func (m *MockSettingsDAO) Get(ctx context.Context, userID uuid.UUID) (*model.UserSettings, error) {
	if m.GetFunc != nil {
		return m.GetFunc(ctx, userID)
	}
	return nil, nil
}

func (m *MockSettingsDAO) Upsert(ctx context.Context, userID uuid.UUID, settings *model.UserSettings) error {
	if m.UpsertFunc != nil {
		return m.UpsertFunc(ctx, userID, settings)
	}
	return nil
}

type MockExerciseCatalogDAO struct {
	ListFunc func(ctx context.Context, category string) ([]model.CatalogExercise, error)
}

func (m *MockExerciseCatalogDAO) List(ctx context.Context, category string) ([]model.CatalogExercise, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, category)
	}
	return []model.CatalogExercise{}, nil
}
