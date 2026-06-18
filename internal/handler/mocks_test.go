package handler

import (
	"context"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// MockTemplateService stubs service.TemplateService.
type MockTemplateService struct {
	ListFunc    func(ctx context.Context, userID uuid.UUID, typeFilter, subtypeFilter string) ([]model.TemplateSummary, error)
	GetByIDFunc func(ctx context.Context, userID, templateID uuid.UUID) (*model.WorkoutTemplate, error)
	CreateFunc  func(ctx context.Context, userID uuid.UUID, req model.CreateTemplateRequest) (*model.WorkoutTemplate, error)
	UpdateFunc  func(ctx context.Context, userID, templateID uuid.UUID, req model.CreateTemplateRequest) (*model.WorkoutTemplate, error)
	DeleteFunc  func(ctx context.Context, userID, templateID uuid.UUID) error
}

func (m *MockTemplateService) List(ctx context.Context, userID uuid.UUID, t, s string) ([]model.TemplateSummary, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, userID, t, s)
	}
	return nil, nil
}
func (m *MockTemplateService) GetByID(ctx context.Context, userID, tID uuid.UUID) (*model.WorkoutTemplate, error) {
	if m.GetByIDFunc != nil {
		return m.GetByIDFunc(ctx, userID, tID)
	}
	return nil, nil
}
func (m *MockTemplateService) Create(ctx context.Context, userID uuid.UUID, req model.CreateTemplateRequest) (*model.WorkoutTemplate, error) {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, userID, req)
	}
	return nil, nil
}
func (m *MockTemplateService) Update(ctx context.Context, userID, tID uuid.UUID, req model.CreateTemplateRequest) (*model.WorkoutTemplate, error) {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, userID, tID, req)
	}
	return nil, nil
}
func (m *MockTemplateService) Delete(ctx context.Context, userID, tID uuid.UUID) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, userID, tID)
	}
	return nil
}

type MockLogService struct {
	ListByWeekFunc      func(ctx context.Context, userID uuid.UUID, week string) ([]model.DayLogSummary, error)
	GetByDateFunc       func(ctx context.Context, userID uuid.UUID, date string) (*model.DayLog, error)
	CreateFunc          func(ctx context.Context, userID uuid.UUID, req model.CreateDayLogRequest) (*model.DayLog, error)
	UpdateFunc          func(ctx context.Context, userID uuid.UUID, date string, req model.UpdateDayLogRequest) (*model.DayLog, error)
	DeleteFunc          func(ctx context.Context, userID uuid.UUID, date string) error
	ExerciseHistoryFunc func(ctx context.Context, userID uuid.UUID, idsParam string) ([]model.ExerciseHistory, error)
	ExerciseRecordsFunc func(ctx context.Context, userID uuid.UUID, idsParam string) ([]model.ExerciseRecord, error)
}

func (m *MockLogService) ListByWeek(ctx context.Context, u uuid.UUID, w string) ([]model.DayLogSummary, error) {
	if m.ListByWeekFunc != nil {
		return m.ListByWeekFunc(ctx, u, w)
	}
	return nil, nil
}
func (m *MockLogService) GetByDate(ctx context.Context, u uuid.UUID, d string) (*model.DayLog, error) {
	if m.GetByDateFunc != nil {
		return m.GetByDateFunc(ctx, u, d)
	}
	return nil, nil
}
func (m *MockLogService) Create(ctx context.Context, u uuid.UUID, r model.CreateDayLogRequest) (*model.DayLog, error) {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, u, r)
	}
	return nil, nil
}
func (m *MockLogService) Update(ctx context.Context, u uuid.UUID, d string, r model.UpdateDayLogRequest) (*model.DayLog, error) {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, u, d, r)
	}
	return nil, nil
}
func (m *MockLogService) Delete(ctx context.Context, u uuid.UUID, d string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, u, d)
	}
	return nil
}
func (m *MockLogService) ExerciseHistory(ctx context.Context, u uuid.UUID, ids string) ([]model.ExerciseHistory, error) {
	if m.ExerciseHistoryFunc != nil {
		return m.ExerciseHistoryFunc(ctx, u, ids)
	}
	return []model.ExerciseHistory{}, nil
}
func (m *MockLogService) ExerciseRecords(ctx context.Context, u uuid.UUID, ids string) ([]model.ExerciseRecord, error) {
	if m.ExerciseRecordsFunc != nil {
		return m.ExerciseRecordsFunc(ctx, u, ids)
	}
	return []model.ExerciseRecord{}, nil
}

type MockStatsService struct {
	GetSummaryFunc      func(ctx context.Context, userID uuid.UUID) (*model.StatsSummary, error)
	GetDistributionFunc func(ctx context.Context, userID uuid.UUID) ([]model.TypeDistribution, error)
	GetVolumeFunc       func(ctx context.Context, userID uuid.UUID, weeksParam string) ([]model.WeeklyVolume, error)
}

func (m *MockStatsService) GetSummary(ctx context.Context, u uuid.UUID) (*model.StatsSummary, error) {
	if m.GetSummaryFunc != nil {
		return m.GetSummaryFunc(ctx, u)
	}
	return nil, nil
}
func (m *MockStatsService) GetDistribution(ctx context.Context, u uuid.UUID) ([]model.TypeDistribution, error) {
	if m.GetDistributionFunc != nil {
		return m.GetDistributionFunc(ctx, u)
	}
	return nil, nil
}
func (m *MockStatsService) GetVolume(ctx context.Context, u uuid.UUID, weeksParam string) ([]model.WeeklyVolume, error) {
	if m.GetVolumeFunc != nil {
		return m.GetVolumeFunc(ctx, u, weeksParam)
	}
	return []model.WeeklyVolume{}, nil
}

type MockSettingsService struct {
	GetFunc    func(ctx context.Context, userID uuid.UUID) (*model.UserSettings, error)
	UpdateFunc func(ctx context.Context, userID uuid.UUID, req model.UserSettings) (*model.UserSettings, error)
}

func (m *MockSettingsService) Get(ctx context.Context, u uuid.UUID) (*model.UserSettings, error) {
	if m.GetFunc != nil {
		return m.GetFunc(ctx, u)
	}
	return nil, nil
}
func (m *MockSettingsService) Update(ctx context.Context, u uuid.UUID, r model.UserSettings) (*model.UserSettings, error) {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, u, r)
	}
	return nil, nil
}

type MockProfileService struct {
	GetFunc    func(ctx context.Context, userID uuid.UUID) (*model.UserProfile, error)
	UpdateFunc func(ctx context.Context, userID uuid.UUID, req model.UpdateProfileRequest) (*model.UserProfile, error)
}

func (m *MockProfileService) Get(ctx context.Context, u uuid.UUID) (*model.UserProfile, error) {
	if m.GetFunc != nil {
		return m.GetFunc(ctx, u)
	}
	return nil, nil
}
func (m *MockProfileService) Update(ctx context.Context, u uuid.UUID, r model.UpdateProfileRequest) (*model.UserProfile, error) {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, u, r)
	}
	return nil, nil
}

type MockBodyWeightService struct {
	LogWeightFunc    func(ctx context.Context, userID uuid.UUID, req model.CreateBodyWeightRequest) (*model.BodyWeight, error)
	ListWeightsFunc  func(ctx context.Context, userID uuid.UUID) ([]model.BodyWeight, error)
	DeleteWeightFunc func(ctx context.Context, userID uuid.UUID, entryID uuid.UUID) error
}

func (m *MockBodyWeightService) LogWeight(ctx context.Context, u uuid.UUID, r model.CreateBodyWeightRequest) (*model.BodyWeight, error) {
	if m.LogWeightFunc != nil {
		return m.LogWeightFunc(ctx, u, r)
	}
	return nil, nil
}
func (m *MockBodyWeightService) ListWeights(ctx context.Context, u uuid.UUID) ([]model.BodyWeight, error) {
	if m.ListWeightsFunc != nil {
		return m.ListWeightsFunc(ctx, u)
	}
	return nil, nil
}
func (m *MockBodyWeightService) DeleteWeight(ctx context.Context, u uuid.UUID, e uuid.UUID) error {
	if m.DeleteWeightFunc != nil {
		return m.DeleteWeightFunc(ctx, u, e)
	}
	return nil
}

type MockExerciseCatalogService struct {
	ListFunc func(ctx context.Context, category string) ([]model.CatalogExercise, error)
}

func (m *MockExerciseCatalogService) List(ctx context.Context, category string) ([]model.CatalogExercise, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, category)
	}
	return []model.CatalogExercise{}, nil
}

type MockPlanService struct {
	GetFunc            func(ctx context.Context, userID uuid.UUID, from, to string) (*model.PlanResponse, error)
	PutWeeklyFunc      func(ctx context.Context, userID uuid.UUID, req model.PutWeeklyPlanRequest) ([]model.WeeklyPlanDay, error)
	PutOverrideFunc    func(ctx context.Context, userID uuid.UUID, date string, req model.PutPlanOverrideRequest) error
	DeleteOverrideFunc func(ctx context.Context, userID uuid.UUID, date string) error
}

func (m *MockPlanService) Get(ctx context.Context, userID uuid.UUID, from, to string) (*model.PlanResponse, error) {
	if m.GetFunc != nil {
		return m.GetFunc(ctx, userID, from, to)
	}
	return &model.PlanResponse{Weekly: []model.WeeklyPlanDay{}, Overrides: []model.PlanOverride{}}, nil
}

func (m *MockPlanService) PutWeekly(ctx context.Context, userID uuid.UUID, req model.PutWeeklyPlanRequest) ([]model.WeeklyPlanDay, error) {
	if m.PutWeeklyFunc != nil {
		return m.PutWeeklyFunc(ctx, userID, req)
	}
	return []model.WeeklyPlanDay{}, nil
}

func (m *MockPlanService) PutOverride(ctx context.Context, userID uuid.UUID, date string, req model.PutPlanOverrideRequest) error {
	if m.PutOverrideFunc != nil {
		return m.PutOverrideFunc(ctx, userID, date, req)
	}
	return nil
}

func (m *MockPlanService) DeleteOverride(ctx context.Context, userID uuid.UUID, date string) error {
	if m.DeleteOverrideFunc != nil {
		return m.DeleteOverrideFunc(ctx, userID, date)
	}
	return nil
}
