package service

import (
	"context"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type LogService interface {
	ListByWeek(ctx context.Context, userID uuid.UUID, weekParam string) ([]model.DayLogSummary, error)
	GetByDate(ctx context.Context, userID uuid.UUID, date string) (*model.DayLog, error)
	Create(ctx context.Context, userID uuid.UUID, req model.CreateDayLogRequest) (*model.DayLog, error)
	Update(ctx context.Context, userID uuid.UUID, date string, req model.UpdateDayLogRequest) (*model.DayLog, error)
	Delete(ctx context.Context, userID uuid.UUID, date string) error
}

type logService struct {
	repo         dao.LogDAO
	templateRepo dao.TemplateDAO
	validator    *validator.Validate
}

func NewLogService(repo dao.LogDAO, templateRepo dao.TemplateDAO, v *validator.Validate) LogService {
	return &logService{repo: repo, templateRepo: templateRepo, validator: v}
}

func (s *logService) ListByWeek(ctx context.Context, userID uuid.UUID, weekParam string) ([]model.DayLogSummary, error) {
	t, err := model.ParseDate(weekParam)
	if err != nil {
		return nil, &model.ValidationError{Message: "invalid date format, expected YYYY-MM-DD", Field: "week"}
	}
	monday := model.MondayOfWeek(t)
	return s.repo.ListByWeek(ctx, userID, monday)
}

func (s *logService) GetByDate(ctx context.Context, userID uuid.UUID, date string) (*model.DayLog, error) {
	if _, err := model.ParseDate(date); err != nil {
		return nil, &model.ValidationError{Message: "invalid date format, expected YYYY-MM-DD", Field: "date"}
	}
	return s.repo.GetByDate(ctx, userID, date)
}

func (s *logService) Create(ctx context.Context, userID uuid.UUID, req model.CreateDayLogRequest) (*model.DayLog, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid log data", Field: "body"}
	}

	parsedDate, err := model.ParseDate(req.Date)
	if err != nil {
		return nil, &model.ValidationError{Message: "invalid date format, expected YYYY-MM-DD", Field: "date"}
	}

	today := time.Now().Truncate(24 * time.Hour)
	if parsedDate.After(today) {
		return nil, &model.ValidationError{Message: "cannot log future dates", Field: "date"}
	}

	if !model.IsValidTypeID(req.TypeID) {
		return nil, &model.ValidationError{Message: "invalid workout type: " + req.TypeID, Field: "type_id"}
	}
	if !model.IsValidSubtypeID(req.SubtypeID) {
		return nil, &model.ValidationError{Message: "invalid workout subtype: " + req.SubtypeID, Field: "subtype_id"}
	}

	// Verify template ownership if provided.
	if req.TemplateID != nil {
		_, err := s.templateRepo.GetByID(ctx, userID, *req.TemplateID)
		if err != nil {
			return nil, err
		}
	}

	dl := &model.DayLog{
		Date:         req.Date,
		TypeID:       req.TypeID,
		SubtypeID:    req.SubtypeID,
		TemplateID:   req.TemplateID,
		SessionNotes: req.SessionNotes,
		Overrides:    toOverrides(req.Overrides),
	}

	if err := s.repo.Create(ctx, userID, dl); err != nil {
		return nil, err
	}
	return dl, nil
}

func (s *logService) Update(ctx context.Context, userID uuid.UUID, date string, req model.UpdateDayLogRequest) (*model.DayLog, error) {
	if _, err := model.ParseDate(date); err != nil {
		return nil, &model.ValidationError{Message: "invalid date format, expected YYYY-MM-DD", Field: "date"}
	}

	overrides := toOverrides(req.Overrides)
	if err := s.repo.Update(ctx, userID, date, overrides, req.SessionNotes); err != nil {
		return nil, err
	}

	return s.repo.GetByDate(ctx, userID, date)
}

func (s *logService) Delete(ctx context.Context, userID uuid.UUID, date string) error {
	if _, err := model.ParseDate(date); err != nil {
		return &model.ValidationError{Message: "invalid date format, expected YYYY-MM-DD", Field: "date"}
	}
	return s.repo.Delete(ctx, userID, date)
}

func toOverrides(reqs []model.CreateOverrideRequest) []model.ExerciseOverride {
	if reqs == nil {
		return []model.ExerciseOverride{}
	}
	overrides := make([]model.ExerciseOverride, len(reqs))
	for i, r := range reqs {
		overrides[i] = model.ExerciseOverride{
			ExerciseID:   r.ExerciseID,
			ActualSets:   r.ActualSets,
			ActualReps:   r.ActualReps,
			ActualWeight: r.ActualWeight,
			Notes:        r.Notes,
			Skipped:      r.Skipped,
		}
	}
	return overrides
}
