package service

import (
	"context"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// PlanService manages the recurring weekly plan and per-date overrides.
// Effective-day resolution (override ?? weekly[weekday]) is client-side.
type PlanService interface {
	Get(ctx context.Context, userID uuid.UUID, from, to string) (*model.PlanResponse, error)
	PutWeekly(ctx context.Context, userID uuid.UUID, req model.PutWeeklyPlanRequest) ([]model.WeeklyPlanDay, error)
	PutOverride(ctx context.Context, userID uuid.UUID, date string, req model.PutPlanOverrideRequest) error
	DeleteOverride(ctx context.Context, userID uuid.UUID, date string) error
}

type planService struct {
	repo         dao.PlanDAO
	templateRepo dao.TemplateDAO
	validator    *validator.Validate
}

// NewPlanService creates a new PlanService.
func NewPlanService(repo dao.PlanDAO, templateRepo dao.TemplateDAO, v *validator.Validate) PlanService {
	return &planService{repo: repo, templateRepo: templateRepo, validator: v}
}

// Default override window when from/to are omitted: current week ±4 weeks.
const defaultWindowWeeks = 4

func (s *planService) Get(ctx context.Context, userID uuid.UUID, from, to string) (*model.PlanResponse, error) {
	now := time.Now().UTC()
	fromT := now.AddDate(0, 0, -7*defaultWindowWeeks)
	toT := now.AddDate(0, 0, 7*defaultWindowWeeks)

	var err error
	if from != "" {
		if fromT, err = model.ParseDate(from); err != nil {
			return nil, &model.ValidationError{Message: "invalid from date, expected YYYY-MM-DD", Field: "from"}
		}
	}
	if to != "" {
		if toT, err = model.ParseDate(to); err != nil {
			return nil, &model.ValidationError{Message: "invalid to date, expected YYYY-MM-DD", Field: "to"}
		}
	}
	if toT.Before(fromT) {
		return nil, &model.ValidationError{Message: "to must not be before from", Field: "to"}
	}

	weekly, err := s.repo.GetWeekly(ctx, userID)
	if err != nil {
		return nil, err
	}
	overrides, err := s.repo.GetOverrides(ctx, userID, fromT, toT)
	if err != nil {
		return nil, err
	}
	return &model.PlanResponse{Weekly: weekly, Overrides: overrides}, nil
}

func (s *planService) PutWeekly(ctx context.Context, userID uuid.UUID, req model.PutWeeklyPlanRequest) ([]model.WeeklyPlanDay, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid weekly plan", Field: "days"}
	}

	seen := map[int]bool{}
	for _, d := range req.Days {
		if seen[d.Weekday] {
			return nil, &model.ValidationError{Message: "duplicate weekday in plan", Field: "days"}
		}
		seen[d.Weekday] = true
		if err := s.validateAssignment(ctx, userID, d.TemplateID, d.Rest); err != nil {
			return nil, err
		}
	}

	if err := s.repo.PutWeekly(ctx, userID, req.Days); err != nil {
		return nil, err
	}
	return s.repo.GetWeekly(ctx, userID)
}

func (s *planService) PutOverride(ctx context.Context, userID uuid.UUID, date string, req model.PutPlanOverrideRequest) error {
	if _, err := model.ParseDate(date); err != nil {
		return &model.ValidationError{Message: "invalid date format, expected YYYY-MM-DD", Field: "date"}
	}
	if err := s.validateAssignment(ctx, userID, req.TemplateID, req.Rest); err != nil {
		return err
	}
	return s.repo.UpsertOverride(ctx, userID, date, req)
}

func (s *planService) DeleteOverride(ctx context.Context, userID uuid.UUID, date string) error {
	if _, err := model.ParseDate(date); err != nil {
		return &model.ValidationError{Message: "invalid date format, expected YYYY-MM-DD", Field: "date"}
	}
	return s.repo.DeleteOverride(ctx, userID, date)
}

// validateAssignment enforces the plan-slot rules shared by weekly days and
// overrides: rest excludes a template, and an assigned template must exist
// and belong to the user.
func (s *planService) validateAssignment(ctx context.Context, userID uuid.UUID, templateID *uuid.UUID, rest bool) error {
	if rest && templateID != nil {
		return &model.ValidationError{Message: "a rest day cannot have a template", Field: "template_id"}
	}
	if templateID != nil {
		if _, err := s.templateRepo.GetByID(ctx, userID, *templateID); err != nil {
			return err
		}
	}
	return nil
}
