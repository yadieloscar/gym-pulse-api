package service

import (
	"context"
	"errors"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// DraftGenerator produces a raw program draft from the generation input. The
// Anthropic adapter implements it; tests use a fake.
type DraftGenerator interface {
	Generate(ctx context.Context, input GenerationInput) (*model.RawGeneratedDraft, GenerationUsage, error)
}

// GenerationInput is everything the provider receives. Nothing else about the
// athlete leaves the API (spec FR-007).
type GenerationInput struct {
	PrimaryGoal            string
	AvailableDays          []int
	UsualActivity          string
	Experience             string
	Equipment              []string
	SessionDurationMinutes int
	Catalog                []model.CatalogExercise
	Request                *string
	Model                  string
	WorkoutCap             int
}

// GenerationUsage is what the service logs about a call.
type GenerationUsage struct {
	InputTokens  int64
	OutputTokens int64
}

var (
	// ErrGeneratorUnavailable covers provider errors, timeouts, and unparsable answers.
	ErrGeneratorUnavailable = errors.New("program generator unavailable")
	// ErrGeneratorRefused says the provider declined to answer.
	ErrGeneratorRefused = errors.New("program generator refused")
)
