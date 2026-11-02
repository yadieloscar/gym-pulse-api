package service

import (
	"context"
	"errors"

	"github.com/gym-pulse/gym-pulse-api/internal/engine"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// RulesModelID names engine-built drafts in the roadmap, where the model path
// records the provider model id.
const RulesModelID = "gympulse-rules-v1"

// rulesDraftGenerator adapts the in-process engine to the DraftGenerator seam
// so the service, validator, handler, and route stay unchanged (spec 016 R2).
type rulesDraftGenerator struct{}

// NewRulesDraftGenerator returns the deterministic engine behind the seam.
func NewRulesDraftGenerator() DraftGenerator { return rulesDraftGenerator{} }

func (rulesDraftGenerator) Generate(_ context.Context, input GenerationInput) (*model.RawGeneratedDraft, GenerationUsage, error) {
	draft, explanation, err := engine.Build(engine.Input{
		PrimaryGoal: input.PrimaryGoal, SecondaryGoals: input.SecondaryGoals, Experience: input.Experience,
		UsualActivity: input.UsualActivity, Equipment: input.Equipment, AvailableDays: input.AvailableDays,
		SessionMinutes: input.SessionDurationMinutes, WorkoutCap: input.WorkoutCap,
		Avoid: input.Avoid, Focus: input.Focus, Variant: input.Variant, Catalog: input.Catalog,
	})
	if err != nil {
		if errors.Is(err, engine.ErrTooConstrained) {
			return nil, GenerationUsage{}, err
		}
		return nil, GenerationUsage{}, errors.Join(ErrGeneratorUnavailable, err)
	}
	raw := &model.RawGeneratedDraft{Name: draft.Name, Rationale: engine.Rationale(explanation)}
	for _, w := range draft.Workouts {
		workout := model.RawGeneratedWorkout{Name: w.Name}
		for _, e := range w.Exercises {
			rest := e.RestSeconds
			ex := model.RawGeneratedExercise{CatalogID: e.Entry.ID.String(), TargetSets: e.Sets, RestSeconds: &rest}
			if e.Entry.Modality == "cardio" {
				duration := e.DurationSeconds
				ex.TargetDurationSeconds = &duration
			} else {
				reps := e.Reps
				ex.TargetReps = &reps
			}
			workout.Exercises = append(workout.Exercises, ex)
		}
		raw.Workouts = append(raw.Workouts, workout)
	}
	return raw, GenerationUsage{}, nil
}
