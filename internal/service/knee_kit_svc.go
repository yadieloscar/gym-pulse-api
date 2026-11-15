package service

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// KneeKitService validates knee checklist and pain-log saves against the
// athlete's local calendar and the documented bounds, then delegates to the
// DAO. It stores facts only: it computes no guidance or streak and never
// touches participation, stats, or any other training read model.
type KneeKitService interface {
	SaveDay(ctx context.Context, userID uuid.UUID, date string, req model.SaveKneeKitDayRequest) (*model.KneeKitDay, model.KneeKitDayOutcome, error)
	ListDays(ctx context.Context, userID uuid.UUID, from, to string) (*model.KneeKitRange, error)
}

type kneeKitService struct {
	days     dao.KneeKitDAO
	profiles dao.TrainingProfileDAO
	now      func() time.Time
}

// NewKneeKitService returns a KneeKitService.
func NewKneeKitService(days dao.KneeKitDAO, profiles dao.TrainingProfileDAO) KneeKitService {
	return &kneeKitService{days: days, profiles: profiles, now: time.Now}
}

const (
	maxKneeKitItemLength = 64
	maxKneeKitPain       = 10
)

func (s *kneeKitService) SaveDay(ctx context.Context, userID uuid.UUID, date string, req model.SaveKneeKitDayRequest) (*model.KneeKitDay, model.KneeKitDayOutcome, error) {
	today, err := localTodayFromProfile(ctx, s.profiles, userID, s.now())
	if err != nil {
		return nil, model.KneeKitDayCreated, err
	}
	req.OperationKey = strings.TrimSpace(req.OperationKey)
	if err := validateNutritionDate(date, today, "date"); err != nil {
		return nil, model.KneeKitDayCreated, err
	}
	if err := validateKneeKitDayRequest(req); err != nil {
		return nil, model.KneeKitDayCreated, err
	}
	// The items are a set: store and hash them sorted so the same ticks in a
	// different order are the same payload and a verbatim retry replays.
	req.CompletedItems = slices.Sorted(slices.Values(req.CompletedItems))
	if req.CompletedItems == nil {
		req.CompletedItems = []string{}
	}
	hash, err := hashPayload(struct {
		Date    string
		Request model.SaveKneeKitDayRequest
	}{date, req})
	if err != nil {
		return nil, model.KneeKitDayCreated, err
	}
	day := &model.KneeKitDay{
		Date:           date,
		CompletedItems: req.CompletedItems,
		MorningPain:    req.MorningPain,
		ActivityPain:   req.ActivityPain,
		SharpPain:      req.SharpPain,
		Limping:        req.Limping,
	}
	return s.days.SaveDay(ctx, userID, day, *req.ExpectedRevision, req.OperationKey, hash)
}

func (s *kneeKitService) ListDays(ctx context.Context, userID uuid.UUID, from, to string) (*model.KneeKitRange, error) {
	if err := validateSportRange(from, to); err != nil {
		return nil, err
	}
	days, err := s.days.ListDays(ctx, userID, from, to)
	if err != nil {
		return nil, err
	}
	if days == nil {
		days = []model.KneeKitDay{}
	}
	return &model.KneeKitRange{Days: days}, nil
}

func validateKneeKitDayRequest(req model.SaveKneeKitDayRequest) error {
	if req.CompletedItems == nil {
		return &model.ValidationError{Message: "completed_items is required", Field: "completed_items"}
	}
	if len(req.CompletedItems) > model.KneeKitMaxCompletedItems {
		return &model.ValidationError{Message: "completed_items allows at most 12 entries", Field: "completed_items"}
	}
	seen := make(map[string]bool, len(req.CompletedItems))
	for _, item := range req.CompletedItems {
		if len(item) > maxKneeKitItemLength || !sportIDPattern.MatchString(item) {
			return &model.ValidationError{Message: "completed_items entries must be lowercase slugs of at most 64 characters", Field: "completed_items"}
		}
		if seen[item] {
			return &model.ValidationError{Message: "completed_items must not repeat an entry", Field: "completed_items"}
		}
		seen[item] = true
	}
	for _, pain := range []struct {
		field string
		value *int
	}{{"morning_pain", req.MorningPain}, {"activity_pain", req.ActivityPain}} {
		if pain.value != nil && (*pain.value < 0 || *pain.value > maxKneeKitPain) {
			return &model.ValidationError{Message: pain.field + " must be between 0 and 10", Field: pain.field}
		}
	}
	if req.ExpectedRevision == nil {
		return &model.ValidationError{Message: "expected_revision is required", Field: "expected_revision"}
	}
	if *req.ExpectedRevision < 0 {
		return &model.ValidationError{Message: "expected_revision must be 0 or greater", Field: "expected_revision"}
	}
	return validateNutritionOperationKey(req.OperationKey)
}
