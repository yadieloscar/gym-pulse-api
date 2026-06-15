package service

import (
	"context"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// BodyWeightService defines operations on body weight entries.
type BodyWeightService interface {
	LogWeight(ctx context.Context, userID uuid.UUID, req model.CreateBodyWeightRequest) (*model.BodyWeight, error)
	ListWeights(ctx context.Context, userID uuid.UUID) ([]model.BodyWeight, error)
	DeleteWeight(ctx context.Context, userID uuid.UUID, entryID uuid.UUID) error
}

type bodyWeightService struct {
	repo      dao.BodyWeightDAO
	validator *validator.Validate
}

// NewBodyWeightService creates a new BodyWeightService.
func NewBodyWeightService(repo dao.BodyWeightDAO, v *validator.Validate) BodyWeightService {
	return &bodyWeightService{repo: repo, validator: v}
}

func (s *bodyWeightService) LogWeight(ctx context.Context, userID uuid.UUID, req model.CreateBodyWeightRequest) (*model.BodyWeight, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid body weight data", Field: "body"}
	}

	parsedDate, err := model.ParseDate(req.Date)
	if err != nil {
		return nil, &model.ValidationError{Message: "invalid date format, expected YYYY-MM-DD", Field: "date"}
	}

	// UTC calendar basis — see model.UTCToday.
	if parsedDate.After(model.UTCToday()) {
		return nil, &model.ValidationError{Message: "cannot log future dates", Field: "date"}
	}

	bw := &model.BodyWeight{
		Date:   req.Date,
		Weight: req.Weight,
		Unit:   req.Unit,
	}

	return s.repo.Upsert(ctx, userID, bw)
}

func (s *bodyWeightService) ListWeights(ctx context.Context, userID uuid.UUID) ([]model.BodyWeight, error) {
	return s.repo.List(ctx, userID)
}

func (s *bodyWeightService) DeleteWeight(ctx context.Context, userID uuid.UUID, entryID uuid.UUID) error {
	return s.repo.Delete(ctx, userID, entryID)
}
