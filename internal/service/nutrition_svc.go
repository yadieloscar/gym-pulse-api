package service

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// NutritionService validates nutrition saves and reads against the athlete's
// local calendar and the documented bounds, then delegates to the DAO.
type NutritionService interface {
	SaveDay(ctx context.Context, userID uuid.UUID, date string, req model.SaveNutritionDayRequest) (*model.NutritionDay, model.NutritionDayOutcome, error)
	ListDays(ctx context.Context, userID uuid.UUID, from, to string) (*model.NutritionRange, error)
	GetTarget(ctx context.Context, userID uuid.UUID) (*model.NutritionTarget, error)
	SaveTarget(ctx context.Context, userID uuid.UUID, req model.SaveNutritionTargetRequest) (*model.NutritionTarget, model.NutritionTargetOutcome, error)
}

type nutritionService struct {
	nutrition dao.NutritionDAO
	profiles  dao.TrainingProfileDAO
	now       func() time.Time
}

func NewNutritionService(nutrition dao.NutritionDAO, profiles dao.TrainingProfileDAO) NutritionService {
	return &nutritionService{nutrition: nutrition, profiles: profiles, now: time.Now}
}

const (
	maxNutritionEnergyKcal = 20000
	maxNutritionMacroGrams = 2000
)

// localToday returns the athlete's current calendar date in their saved
// training-profile timezone as YYYY-MM-DD, as sport activities do.
func (s *nutritionService) localToday(ctx context.Context, userID uuid.UUID) (string, error) {
	return localTodayFromProfile(ctx, s.profiles, userID, s.now())
}

func (s *nutritionService) SaveDay(ctx context.Context, userID uuid.UUID, date string, req model.SaveNutritionDayRequest) (*model.NutritionDay, model.NutritionDayOutcome, error) {
	today, err := s.localToday(ctx, userID)
	if err != nil {
		return nil, model.NutritionDayCreated, err
	}
	req.OperationKey = strings.TrimSpace(req.OperationKey)
	if req.Source == "" {
		req.Source = model.NutritionSourceManual
	}
	if err := validateNutritionDate(date, today, "date"); err != nil {
		return nil, model.NutritionDayCreated, err
	}
	if err := validateNutritionDayRequest(req); err != nil {
		return nil, model.NutritionDayCreated, err
	}
	// Hash the request as the client sent it (after normalization that is
	// itself deterministic) so a verbatim retry replays rather than conflicts.
	hash, err := hashPayload(struct {
		Date    string
		Request model.SaveNutritionDayRequest
	}{date, req})
	if err != nil {
		return nil, model.NutritionDayCreated, err
	}
	day := &model.NutritionDay{
		Date:       date,
		EnergyKcal: req.EnergyKcal,
		ProteinG:   roundedGrams(req.ProteinG),
		CarbsG:     roundedGrams(req.CarbsG),
		FatG:       roundedGrams(req.FatG),
		Source:     req.Source,
	}
	expected := req.ExpectedRevision
	if req.Source == model.NutritionSourceAppleHealth {
		expected = nil
	}
	return s.nutrition.SaveDay(ctx, userID, day, expected, !req.HasValue(), req.OperationKey, hash)
}

func (s *nutritionService) ListDays(ctx context.Context, userID uuid.UUID, from, to string) (*model.NutritionRange, error) {
	if err := validateSportRange(from, to); err != nil {
		return nil, err
	}
	return s.nutrition.ListDays(ctx, userID, from, to)
}

func (s *nutritionService) GetTarget(ctx context.Context, userID uuid.UUID) (*model.NutritionTarget, error) {
	today, err := s.localToday(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.nutrition.GetCurrentTarget(ctx, userID, today)
}

func (s *nutritionService) SaveTarget(ctx context.Context, userID uuid.UUID, req model.SaveNutritionTargetRequest) (*model.NutritionTarget, model.NutritionTargetOutcome, error) {
	today, err := s.localToday(ctx, userID)
	if err != nil {
		return nil, model.NutritionTargetCreated, err
	}
	req.OperationKey = strings.TrimSpace(req.OperationKey)
	req.EffectiveFrom = strings.TrimSpace(req.EffectiveFrom)
	// Hash before defaulting effective_from: the default depends on the clock,
	// and a retry that omits it must still replay.
	hash, err := hashPayload(req)
	if err != nil {
		return nil, model.NutritionTargetCreated, err
	}
	if req.EffectiveFrom == "" {
		req.EffectiveFrom = today
	}
	if err := validateNutritionDate(req.EffectiveFrom, today, "effective_from"); err != nil {
		return nil, model.NutritionTargetCreated, err
	}
	if err := validateNutritionTargetRequest(req); err != nil {
		return nil, model.NutritionTargetCreated, err
	}
	target := &model.NutritionTarget{
		EffectiveFrom: req.EffectiveFrom,
		ProteinG:      math.Round(req.ProteinG*10) / 10,
		EnergyKcal:    req.EnergyKcal,
		CarbsG:        roundedGrams(req.CarbsG),
		FatG:          roundedGrams(req.FatG),
	}
	return s.nutrition.SaveTarget(ctx, userID, target, req.ExpectedRevision, today, req.OperationKey, hash)
}

func validateNutritionDate(value, today, field string) error {
	if _, err := model.ParseDate(value); err != nil {
		return &model.ValidationError{Message: field + " must be YYYY-MM-DD", Field: field}
	}
	if value > today {
		return &model.ValidationError{Message: "cannot log future dates", Field: field}
	}
	return nil
}

func validateNutritionDayRequest(req model.SaveNutritionDayRequest) error {
	if req.Source != model.NutritionSourceManual && req.Source != model.NutritionSourceAppleHealth {
		return &model.ValidationError{Message: "source must be manual or apple_health", Field: "source"}
	}
	if err := validateNutritionOperationKey(req.OperationKey); err != nil {
		return err
	}
	if err := validateEnergy(req.EnergyKcal, 0); err != nil {
		return err
	}
	for _, macro := range []struct {
		field string
		value *float64
	}{{"protein_g", req.ProteinG}, {"carbs_g", req.CarbsG}, {"fat_g", req.FatG}} {
		if err := validateGrams(macro.value, macro.field, false); err != nil {
			return err
		}
	}
	if req.Source == model.NutritionSourceAppleHealth && !req.HasValue() {
		return &model.ValidationError{Message: "a health save needs at least one value", Field: "source"}
	}
	if req.Source == model.NutritionSourceManual {
		if req.ExpectedRevision == nil {
			return &model.ValidationError{Message: "expected_revision is required", Field: "expected_revision"}
		}
		if *req.ExpectedRevision < 0 {
			return &model.ValidationError{Message: "expected_revision must be 0 or greater", Field: "expected_revision"}
		}
	}
	return nil
}

func validateNutritionTargetRequest(req model.SaveNutritionTargetRequest) error {
	if err := validateNutritionOperationKey(req.OperationKey); err != nil {
		return err
	}
	protein := req.ProteinG
	if err := validateGrams(&protein, "protein_g", true); err != nil {
		return err
	}
	if err := validateEnergy(req.EnergyKcal, 1); err != nil {
		return err
	}
	for _, macro := range []struct {
		field string
		value *float64
	}{{"carbs_g", req.CarbsG}, {"fat_g", req.FatG}} {
		if err := validateGrams(macro.value, macro.field, false); err != nil {
			return err
		}
	}
	if req.ExpectedRevision < 0 {
		return &model.ValidationError{Message: "expected_revision must be 0 or greater", Field: "expected_revision"}
	}
	return nil
}

func validateNutritionOperationKey(key string) error {
	if key == "" || len(key) > 128 {
		return &model.ValidationError{Message: "operation_key must be between 1 and 128 characters", Field: "operation_key"}
	}
	return nil
}

func validateEnergy(value *int, minimum int) error {
	if value == nil {
		return nil
	}
	if *value < minimum || *value > maxNutritionEnergyKcal {
		return &model.ValidationError{Message: "energy_kcal is out of range", Field: "energy_kcal"}
	}
	return nil
}

// validateGrams accepts 0–2,000 grams with at most one decimal; positive
// requires more than zero (the target's protein).
func validateGrams(value *float64, field string, positive bool) error {
	if value == nil {
		return nil
	}
	v := *value
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > maxNutritionMacroGrams || (positive && v <= 0) {
		return &model.ValidationError{Message: field + " is out of range", Field: field}
	}
	if math.Abs(v*10-math.Round(v*10)) > 1e-6 {
		return &model.ValidationError{Message: field + " allows at most one decimal", Field: field}
	}
	return nil
}

func roundedGrams(value *float64) *float64 {
	if value == nil {
		return nil
	}
	rounded := math.Round(*value*10) / 10
	return &rounded
}
