package service

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/nutritionplan"
)

// NutritionPlanService runs the macro calculator and keeps the athlete's
// saved plan. The arithmetic lives in internal/nutritionplan; this service
// maps wire types, names validation failures, and delegates storage.
type NutritionPlanService interface {
	// Options returns the foods, ranges, and defaults the calculator uses.
	Options() *model.CalculatorOptions
	// Calculate computes a plan and stores nothing.
	Calculate(input model.CalculatorInput) (*model.NutritionPlan, error)
	// SaveProfile recomputes the targets from the request and stores the
	// input with them as the athlete's plan.
	SaveProfile(ctx context.Context, userID uuid.UUID, req model.SaveNutritionProfileRequest) (*model.SavedNutritionPlan, model.NutritionProfileOutcome, error)
	// GetPlan returns the saved plan built from its stored targets.
	GetPlan(ctx context.Context, userID uuid.UUID) (*model.SavedNutritionPlan, error)
}

type nutritionPlanService struct {
	profiles dao.NutritionProfileDAO
}

func NewNutritionPlanService(profiles dao.NutritionProfileDAO) NutritionPlanService {
	return &nutritionPlanService{profiles: profiles}
}

const foodSourceUSDA = "usda"

func (s *nutritionPlanService) Options() *model.CalculatorOptions {
	fatDefault := nutritionplan.DefaultFatPerLb
	options := &model.CalculatorOptions{
		Goals:          make([]model.CalculatorGoalOption, 0, len(nutritionplan.Goals)),
		ActivityLevels: make([]model.CalculatorActivityOption, 0, len(nutritionplan.ActivityLevels)),
		ProteinGPerLb:  model.CalculatorRateRange{Min: nutritionplan.MinProteinPerLb, Max: nutritionplan.MaxProteinPerLb, Step: 0.05},
		FatGPerLb:      model.CalculatorRateRange{Min: nutritionplan.MinFatPerLb, Max: nutritionplan.MaxFatPerLb, Step: 0.01, Default: &fatDefault},
		Exclusions:     append([]string{}, nutritionplan.ExclusionTags...),
		Foods:          make([]model.CalculatorFood, 0, len(nutritionplan.Foods)),
		Meals:          make([]model.CalculatorMealOption, 0, len(nutritionplan.MealSlots)),
	}
	for _, g := range nutritionplan.Goals {
		options.Goals = append(options.Goals, model.CalculatorGoalOption{
			ID:                    g.ID,
			MinAdjustmentKcal:     g.MinAdjustmentKcal,
			MaxAdjustmentKcal:     g.MaxAdjustmentKcal,
			DefaultAdjustmentKcal: g.DefaultAdjustment,
			DefaultProteinGPerLb:  g.DefaultProteinPerLb,
		})
	}
	for _, l := range nutritionplan.ActivityLevels {
		option := model.CalculatorActivityOption{ID: l.ID, Multiplier: l.Multiplier, MinSessions: l.MinSessions}
		if l.MaxSessions >= 0 {
			maxSessions := l.MaxSessions
			option.MaxSessions = &maxSessions
		}
		options.ActivityLevels = append(options.ActivityLevels, option)
	}
	for _, f := range nutritionplan.Foods {
		options.Foods = append(options.Foods, model.CalculatorFood{
			ID:         f.ID,
			Name:       f.Name,
			Role:       f.Role,
			Tags:       append([]string{}, f.Tags...),
			Unit:       f.Unit,
			Step:       f.Step,
			Selectable: f.Selectable,
			Source:     foodSourceUSDA,
			FDCID:      f.FDCID,
			Per100G:    model.PlanProvided{EnergyKcal: int(f.Kcal), ProteinG: f.Protein, CarbsG: f.Carbs, FatG: f.Fat},
		})
	}
	for _, m := range nutritionplan.MealSlots {
		options.Meals = append(options.Meals, model.CalculatorMealOption{Slot: m.Slot, Name: m.Name})
	}
	return options
}

func (s *nutritionPlanService) Calculate(input model.CalculatorInput) (*model.NutritionPlan, error) {
	resolved, err := resolveCalculatorInput(input)
	if err != nil {
		return nil, err
	}
	plan := planToModel(nutritionplan.Build(resolved, nutritionplan.Calculate(resolved)))
	return &plan, nil
}

func (s *nutritionPlanService) SaveProfile(ctx context.Context, userID uuid.UUID, req model.SaveNutritionProfileRequest) (*model.SavedNutritionPlan, model.NutritionProfileOutcome, error) {
	req.OperationKey = strings.TrimSpace(req.OperationKey)
	if err := validateNutritionOperationKey(req.OperationKey); err != nil {
		return nil, model.NutritionProfileCreated, err
	}
	if req.ExpectedRevision < 0 {
		return nil, model.NutritionProfileCreated, &model.ValidationError{Message: "expected_revision must be 0 or greater", Field: "expected_revision"}
	}
	resolved, err := resolveCalculatorInput(req.CalculatorInput)
	if err != nil {
		return nil, model.NutritionProfileCreated, err
	}
	// Hash the request as sent so a verbatim retry replays even if a later
	// release changes a default.
	hash, err := hashPayload(req)
	if err != nil {
		return nil, model.NutritionProfileCreated, err
	}
	targets := nutritionplan.Calculate(resolved)
	profile := &model.NutritionProfile{
		Input:      inputToModel(resolved),
		BMRKcal:    targets.BMRKcal,
		TDEEKcal:   targets.TDEEKcal,
		TargetKcal: targets.TargetKcal,
		ProteinG:   targets.ProteinG,
		CarbsG:     targets.CarbsG,
		FatG:       targets.FatG,
	}
	stored, outcome, err := s.profiles.Save(ctx, userID, profile, req.ExpectedRevision, req.OperationKey, hash)
	if err != nil {
		// A stale revision carries the stored row; the contract promises the
		// stored plan under resource.
		var conflict *model.ConflictError
		if errors.As(err, &conflict) {
			if current, ok := conflict.Authoritative.(*model.NutritionProfile); ok && current != nil {
				conflict.Authoritative = savedPlan(current)
			}
		}
		return nil, model.NutritionProfileCreated, err
	}
	return savedPlan(stored), outcome, nil
}

func (s *nutritionPlanService) GetPlan(ctx context.Context, userID uuid.UUID) (*model.SavedNutritionPlan, error) {
	profile, err := s.profiles.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	return savedPlan(profile), nil
}

// savedPlan builds the plan for a stored profile from its stored targets, so
// a saved plan's numbers move only when the athlete saves again.
func savedPlan(profile *model.NutritionProfile) *model.SavedNutritionPlan {
	in := storedInput(profile.Input)
	targets := nutritionplan.Targets{
		BMRKcal:        profile.BMRKcal,
		TDEEKcal:       profile.TDEEKcal,
		AdjustmentKcal: *in.AdjustmentKcal,
		TargetKcal:     profile.TargetKcal,
		ProteinG:       profile.ProteinG,
		CarbsG:         profile.CarbsG,
		FatG:           profile.FatG,
	}
	return &model.SavedNutritionPlan{
		NutritionPlan: planToModel(nutritionplan.Build(in, targets)),
		Revision:      profile.Revision,
		CreatedAt:     profile.CreatedAt,
		UpdatedAt:     profile.UpdatedAt,
	}
}

// storedInput turns a stored input into an engine input. Stored values were
// validated when saved; a food or tag a later release no longer knows is
// dropped rather than failing the read.
func storedInput(stored model.CalculatorInput) nutritionplan.Input {
	in := inputFromModel(stored)
	knownFood := func(role string) func(string) bool {
		return func(id string) bool {
			f, ok := nutritionplan.FoodByID(id)
			return ok && f.Selectable && f.Role == role
		}
	}
	in.Exclusions = keepKnown(in.Exclusions, func(tag string) bool {
		return slices.Contains(nutritionplan.ExclusionTags, tag)
	})
	in.PreferredProteins = keepKnown(in.PreferredProteins, knownFood(nutritionplan.RoleProtein))
	in.PreferredCarbs = keepKnown(in.PreferredCarbs, knownFood(nutritionplan.RoleCarb))
	if resolved, err := nutritionplan.Resolve(in); err == nil {
		return resolved
	}
	// A stored swap no longer valid (its food was removed or is now
	// excluded): keep the plan and drop the swaps.
	in.MealChoices = nil
	if resolved, err := nutritionplan.Resolve(in); err == nil {
		return resolved
	}
	// Stored scalars are constrained by the table, so this is unreachable;
	// fill what Build dereferences rather than panic on a read.
	if in.AdjustmentKcal == nil {
		in.AdjustmentKcal = new(int)
	}
	if in.ProteinPerLb == nil {
		in.ProteinPerLb = new(float64)
	}
	if in.FatPerLb == nil {
		in.FatPerLb = new(float64)
	}
	return in
}

func keepKnown(values []string, known func(string) bool) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if known(v) {
			out = append(out, v)
		}
	}
	return out
}

// resolveCalculatorInput validates the wire input and fills its defaults,
// mapping the engine's field error to the API's validation error.
func resolveCalculatorInput(input model.CalculatorInput) (nutritionplan.Input, error) {
	resolved, err := nutritionplan.Resolve(inputFromModel(input))
	if err != nil {
		var fieldErr *nutritionplan.FieldError
		if errors.As(err, &fieldErr) {
			return resolved, &model.ValidationError{Message: fieldErr.Message, Field: fieldErr.Field}
		}
		return resolved, err
	}
	return resolved, nil
}

func inputFromModel(m model.CalculatorInput) nutritionplan.Input {
	in := nutritionplan.Input{
		UnitSystem:        m.UnitSystem,
		Weight:            m.Weight,
		Height:            m.Height,
		AgeYears:          m.AgeYears,
		Sex:               m.Sex,
		StrengthDays:      m.StrengthDays,
		CardioDays:        m.CardioDays,
		Goal:              m.Goal,
		AdjustmentKcal:    m.CalorieAdjustmentKcal,
		ProteinPerLb:      m.ProteinGPerLb,
		FatPerLb:          m.FatGPerLb,
		CarbCycling:       m.CarbCycling,
		PreferredProteins: m.PreferredProteins,
		PreferredCarbs:    m.PreferredCarbs,
		Exclusions:        m.Exclusions,
	}
	if m.ActivityLevel != nil {
		in.ActivityLevel = *m.ActivityLevel
		if in.ActivityLevel == "" {
			// An explicit empty string is not "follow the suggestion".
			in.ActivityLevel = "invalid"
		}
	}
	for _, c := range m.MealChoices {
		in.MealChoices = append(in.MealChoices, nutritionplan.MealChoice{Slot: c.Slot, Protein: c.Protein, Carb: c.Carb, Fat: c.Fat})
	}
	return in
}

func inputToModel(in nutritionplan.Input) model.CalculatorInput {
	m := model.CalculatorInput{
		UnitSystem:            in.UnitSystem,
		Weight:                in.Weight,
		Height:                in.Height,
		AgeYears:              in.AgeYears,
		Sex:                   in.Sex,
		StrengthDays:          in.StrengthDays,
		CardioDays:            in.CardioDays,
		Goal:                  in.Goal,
		CalorieAdjustmentKcal: in.AdjustmentKcal,
		ProteinGPerLb:         in.ProteinPerLb,
		FatGPerLb:             in.FatPerLb,
		CarbCycling:           in.CarbCycling,
		PreferredProteins:     append([]string{}, in.PreferredProteins...),
		PreferredCarbs:        append([]string{}, in.PreferredCarbs...),
		Exclusions:            append([]string{}, in.Exclusions...),
		MealChoices:           make([]model.MealChoice, 0, len(in.MealChoices)),
	}
	if in.ActivityLevel != "" {
		level := in.ActivityLevel
		m.ActivityLevel = &level
	}
	for _, c := range in.MealChoices {
		m.MealChoices = append(m.MealChoices, model.MealChoice{Slot: c.Slot, Protein: c.Protein, Carb: c.Carb, Fat: c.Fat})
	}
	return m
}

func planToModel(p nutritionplan.Plan) model.NutritionPlan {
	out := model.NutritionPlan{
		Input:                 inputToModel(p.Input),
		Activity:              model.PlanActivity{SuggestedLevel: p.SuggestedLevel.ID, Level: p.Level.ID, Multiplier: p.Level.Multiplier},
		BMRKcal:               p.Targets.BMRKcal,
		TDEEKcal:              p.Targets.TDEEKcal,
		CalorieAdjustmentKcal: p.Targets.AdjustmentKcal,
		TargetKcal:            p.Targets.TargetKcal,
		Macros: model.PlanMacros{
			Protein: model.PlanMacro{G: p.Protein.G, Kcal: p.Protein.Kcal, Pct: p.Protein.Pct},
			Carbs:   model.PlanMacro{G: p.Carbs.G, Kcal: p.Carbs.Kcal, Pct: p.Carbs.Pct},
			Fat:     model.PlanMacro{G: p.Fat.G, Kcal: p.Fat.Kcal, Pct: p.Fat.Pct},
		},
		Notices:       append([]string{}, p.Notices...),
		MealTemplates: make([]model.PlanMealTemplate, 0, len(p.Templates)),
	}
	if p.Cycling != nil {
		out.CarbCycling = &model.PlanCarbCycling{
			TrainingDays:      p.Cycling.TrainingDays,
			RestDays:          p.Cycling.RestDays,
			Training:          dayToModel(p.Cycling.Training),
			Rest:              dayToModel(p.Cycling.Rest),
			WeeklyAverageKcal: p.Cycling.WeeklyAverageKcal,
		}
	}
	for _, t := range p.Templates {
		template := model.PlanMealTemplate{
			DayType:       t.DayType,
			PlanDayMacros: dayToModel(t.Day),
			Meals:         make([]model.PlanMeal, 0, len(t.Meals)),
			Provided:      providedToModel(t.Provided),
		}
		for _, m := range t.Meals {
			meal := model.PlanMeal{
				Slot:     m.Slot,
				Name:     m.Name,
				Target:   dayToModel(m.Target),
				Items:    make([]model.PlanMealItem, 0, len(m.Items)),
				Provided: providedToModel(m.Provided),
			}
			for _, item := range m.Items {
				meal.Items = append(meal.Items, model.PlanMealItem{
					Food:       item.Food.ID,
					Name:       item.Food.Name,
					Role:       item.Role,
					Amount:     item.Amount,
					Unit:       item.Food.Unit,
					Grams:      item.Grams,
					EnergyKcal: item.EnergyKcal,
					ProteinG:   item.ProteinG,
					CarbsG:     item.CarbsG,
					FatG:       item.FatG,
					Source:     foodSourceUSDA,
					FDCID:      item.Food.FDCID,
				})
			}
			template.Meals = append(template.Meals, meal)
		}
		out.MealTemplates = append(out.MealTemplates, template)
	}
	return out
}

func dayToModel(d nutritionplan.DayMacros) model.PlanDayMacros {
	return model.PlanDayMacros{EnergyKcal: d.EnergyKcal, ProteinG: d.ProteinG, CarbsG: d.CarbsG, FatG: d.FatG}
}

func providedToModel(p nutritionplan.Provided) model.PlanProvided {
	return model.PlanProvided{EnergyKcal: p.EnergyKcal, ProteinG: p.ProteinG, CarbsG: p.CarbsG, FatG: p.FatG}
}
