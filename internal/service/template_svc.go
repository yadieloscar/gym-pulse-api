package service

import (
	"context"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type TemplateService interface {
	List(ctx context.Context, userID uuid.UUID, typeFilter, subtypeFilter string) ([]model.TemplateSummary, error)
	GetByID(ctx context.Context, userID, templateID uuid.UUID) (*model.WorkoutTemplate, error)
	Create(ctx context.Context, userID uuid.UUID, req model.CreateTemplateRequest) (*model.WorkoutTemplate, error)
	Update(ctx context.Context, userID, templateID uuid.UUID, req model.CreateTemplateRequest) (*model.WorkoutTemplate, error)
	Delete(ctx context.Context, userID, templateID uuid.UUID) error
}

type templateService struct {
	repo      dao.TemplateDAO
	validator *validator.Validate
}

func NewTemplateService(repo dao.TemplateDAO, v *validator.Validate) TemplateService {
	return &templateService{repo: repo, validator: v}
}

func (s *templateService) List(ctx context.Context, userID uuid.UUID, typeFilter, subtypeFilter string) ([]model.TemplateSummary, error) {
	if typeFilter != "" && !model.IsValidTypeID(typeFilter) {
		return nil, &model.ValidationError{Message: "invalid type: " + typeFilter, Field: "type"}
	}
	if subtypeFilter != "" && !model.IsValidSubtypeID(subtypeFilter) {
		return nil, &model.ValidationError{Message: "invalid subtype: " + subtypeFilter, Field: "subtype"}
	}
	return s.repo.List(ctx, userID, typeFilter, subtypeFilter)
}

func (s *templateService) GetByID(ctx context.Context, userID, templateID uuid.UUID) (*model.WorkoutTemplate, error) {
	return s.repo.GetByID(ctx, userID, templateID)
}

func (s *templateService) Create(ctx context.Context, userID uuid.UUID, req model.CreateTemplateRequest) (*model.WorkoutTemplate, error) {
	if err := s.validateRequest(req); err != nil {
		return nil, err
	}

	t := &model.WorkoutTemplate{
		Name:      req.Name,
		TypeID:    req.TypeID,
		SubtypeID: req.SubtypeID,
		Exercises: toExercises(req.Exercises),
	}

	if err := s.repo.Create(ctx, userID, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *templateService) Update(ctx context.Context, userID, templateID uuid.UUID, req model.CreateTemplateRequest) (*model.WorkoutTemplate, error) {
	if err := s.validateRequest(req); err != nil {
		return nil, err
	}

	// Fetch existing to preserve created_at.
	existing, err := s.repo.GetByID(ctx, userID, templateID)
	if err != nil {
		return nil, err
	}

	existing.Name = req.Name
	existing.TypeID = req.TypeID
	existing.SubtypeID = req.SubtypeID
	existing.Exercises = toExercises(req.Exercises)

	if err := s.repo.Update(ctx, userID, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

func (s *templateService) Delete(ctx context.Context, userID, templateID uuid.UUID) error {
	return s.repo.Delete(ctx, userID, templateID)
}

func (s *templateService) validateRequest(req model.CreateTemplateRequest) error {
	if err := s.validator.Struct(req); err != nil {
		return &model.ValidationError{Message: "invalid template data", Field: "body"}
	}
	if !model.IsValidTypeID(req.TypeID) {
		return &model.ValidationError{Message: "invalid workout type: " + req.TypeID, Field: "type_id"}
	}
	if !model.IsValidSubtypeID(req.SubtypeID) {
		return &model.ValidationError{Message: "invalid workout subtype: " + req.SubtypeID, Field: "subtype_id"}
	}
	for _, e := range req.Exercises {
		if err := s.validator.Struct(e); err != nil {
			return &model.ValidationError{Message: "invalid exercise data", Field: "exercises"}
		}
	}
	return nil
}

func toExercises(reqs []model.CreateExerciseRequest) []model.Exercise {
	exercises := make([]model.Exercise, len(reqs))
	for i, r := range reqs {
		exercises[i] = model.Exercise{
			Name:        r.Name,
			SortOrder:   i,
			Sets:        r.Sets,
			Reps:        r.Reps,
			Weight:      r.Weight,
			RestSeconds: r.RestSeconds,
			Notes:       r.Notes,
		}
	}
	return exercises
}
