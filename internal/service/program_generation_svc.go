package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/engine"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// ProgramGenerationService builds unsaved program drafts from the training
// profile. It never writes programs, schedules, or the profile; its only
// persistence is the daily-limit ledger.
type ProgramGenerationService interface {
	Status(ctx context.Context, userID uuid.UUID) (*model.GenerationStatus, error)
	Generate(ctx context.Context, userID uuid.UUID, req model.GenerateProgramRequest) (*model.GenerateProgramResponse, error)
}

type programGenerationService struct {
	profiles  dao.TrainingProfileDAO
	catalog   dao.ExerciseCatalogDAO
	ledger    dao.ProgramGenerationDAO
	generator DraftGenerator // nil only when the model provider is configured without a key
	provider  string         // model.GenerationProviderRules or model.GenerationProviderModel
	limit     int
	model     string
	now       func() time.Time
}

// NewProgramGenerationService wires the DAOs and the generator. provider says
// whether drafts come from the rules engine (no ledger) or the model (ledger
// and daily limit). A nil generator reports the feature as disabled.
func NewProgramGenerationService(profiles dao.TrainingProfileDAO, catalog dao.ExerciseCatalogDAO, ledger dao.ProgramGenerationDAO, generator DraftGenerator, provider string, limit int, modelID string) ProgramGenerationService {
	if provider == "" {
		provider = model.GenerationProviderRules
	}
	return &programGenerationService{profiles: profiles, catalog: catalog, ledger: ledger, generator: generator, provider: provider, limit: limit, model: modelID, now: time.Now}
}

func (s *programGenerationService) rules() bool { return s.provider == model.GenerationProviderRules }

func (s *programGenerationService) Status(ctx context.Context, userID uuid.UUID) (*model.GenerationStatus, error) {
	status := &model.GenerationStatus{Enabled: s.generator != nil, Provider: s.provider, Limit: s.limit, RemainingToday: s.limit}
	if s.rules() {
		// The ledger bounds provider cost; engine drafts are free and unlimited.
		return status, nil
	}
	profile, err := s.profiles.Get(ctx, userID)
	var notFound *model.NotFoundError
	if errors.As(err, &notFound) {
		return status, nil
	}
	if err != nil {
		return nil, err
	}
	localDate, resetsAt, err := s.localDay(profile)
	if err != nil {
		return nil, err
	}
	used, err := s.ledger.CountToday(ctx, userID, localDate)
	if err != nil {
		return nil, err
	}
	status.RemainingToday = max(0, s.limit-used)
	status.ResetsAt = &resetsAt
	return status, nil
}

func (s *programGenerationService) Generate(ctx context.Context, userID uuid.UUID, req model.GenerateProgramRequest) (*model.GenerateProgramResponse, error) {
	if s.generator == nil {
		return nil, &model.GenerationUnavailableError{Reason: model.GenerationDisabled}
	}
	request, err := model.NormalizeGenerationRequest(req.Request)
	if err != nil {
		return nil, err
	}
	profile, err := s.resolveProfile(ctx, userID, req.ProposedProfile)
	if err != nil {
		return nil, err
	}
	secondary, err := model.NormalizeSecondaryGoals(profile.PrimaryGoal, req.SecondaryGoals)
	if err != nil {
		return nil, err
	}
	constraints, err := model.NormalizeGenerationConstraints(req.Avoid, req.Focus, req.Variant)
	if err != nil {
		return nil, err
	}
	localDate, resetsAt, err := s.localDay(profile)
	if err != nil {
		return nil, err
	}
	// The ledger and its daily limit bound provider cost; the rules engine has none.
	var reservation *dao.ProgramGenerationReservation
	if !s.rules() {
		reservation, err = s.ledger.Reserve(ctx, userID, localDate, s.limit, s.model)
		var limitErr *model.GenerationLimitError
		if errors.As(err, &limitErr) {
			limitErr.ResetsAt = resetsAt
			return nil, limitErr
		}
		if err != nil {
			return nil, err
		}
	}
	fail := func() {
		if reservation != nil {
			s.complete(ctx, reservation.ID, model.GenerationFailed)
		}
	}

	catalog, err := s.catalog.List(ctx, "")
	if err != nil {
		fail()
		return nil, err
	}
	input := GenerationInput{
		PrimaryGoal: profile.PrimaryGoal, SecondaryGoals: secondary, AvailableDays: profile.AvailableDays, UsualActivity: profile.UsualActivity,
		Experience: profile.Experience, Equipment: profile.Equipment, SessionDurationMinutes: profile.SessionDurationMinutes,
		Catalog: catalog, Request: request, Model: s.model, WorkoutCap: model.DraftWorkoutCap(profile.AvailableDays),
		Avoid: constraints.Avoid, Focus: constraints.Focus, Variant: constraints.Variant,
	}
	modelID := s.model
	if s.rules() {
		modelID = RulesModelID
	}
	started := s.now()
	raw, usage, err := s.generator.Generate(ctx, input)
	if errors.Is(err, engine.ErrTooConstrained) {
		return nil, &model.ValidationError{Message: "Those limits leave too little to build a workout from; drop one and try again.", Field: "avoid"}
	}
	if err != nil {
		fail()
		slog.WarnContext(ctx, "program generation failed", "provider", s.provider, "model", modelID, "duration_ms", s.now().Sub(started).Milliseconds(), "error", err)
		return nil, &model.GenerationUnavailableError{Reason: model.GenerationUnavailable}
	}
	generated, err := model.RepairGeneratedDraft(profile, catalog, raw, modelID, request)
	if err != nil {
		fail()
		slog.WarnContext(ctx, "program generation produced an invalid draft", "provider", s.provider, "model", modelID, "error", err)
		return nil, &model.GenerationUnavailableError{Reason: model.GenerationUnavailable}
	}
	remaining := s.limit
	if reservation != nil {
		s.complete(ctx, reservation.ID, model.GenerationSucceeded)
		remaining = max(0, s.limit-reservation.Used)
	}
	slog.InfoContext(ctx, "program generation succeeded", "provider", s.provider, "model", modelID, "duration_ms", s.now().Sub(started).Milliseconds(),
		"input_tokens", usage.InputTokens, "output_tokens", usage.OutputTokens, "workouts", len(generated.Draft.Workouts))
	var focus *string
	if constraints.Focus != "" {
		focus = &constraints.Focus
	}
	return &model.GenerateProgramResponse{
		Draft: generated.Draft, Rationale: generated.Rationale, Request: request, SecondaryGoals: secondary,
		Provider: s.provider, Avoid: constraints.Avoid, Focus: focus,
		RemainingToday: remaining, ResetsAt: resetsAt,
	}, nil
}

// resolveProfile uses the proposed profile when one is sent (onboarding builds
// before the profile is saved), otherwise the saved profile. A proposed profile
// is validated exactly like a saved one and is never written.
func (s *programGenerationService) resolveProfile(ctx context.Context, userID uuid.UUID, proposed *model.ProposedTrainingProfile) (*model.TrainingProfile, error) {
	if proposed != nil {
		profile := proposed.TrainingProfile()
		if err := profile.Validate(); err != nil {
			var validation *model.ValidationError
			if errors.As(err, &validation) {
				return nil, &model.ValidationError{Message: validation.Message, Field: "proposed_profile." + validation.Field}
			}
			return nil, err
		}
		return &profile, nil
	}
	profile, err := s.profiles.Get(ctx, userID)
	var notFound *model.NotFoundError
	if errors.As(err, &notFound) {
		return nil, &model.ValidationError{Message: "save a training profile before building a program", Field: "training_profile"}
	}
	if err != nil {
		return nil, err
	}
	return profile, nil
}

// complete records the outcome even when the request context was cancelled,
// so an abandoned call never keeps charging the athlete past the window.
func (s *programGenerationService) complete(ctx context.Context, id uuid.UUID, status string) {
	completeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.ledger.Complete(completeCtx, id, status); err != nil {
		slog.WarnContext(ctx, "program generation outcome not recorded", "status", status, "error", err)
	}
}

func (s *programGenerationService) localDay(profile *model.TrainingProfile) (string, time.Time, error) {
	now := s.now()
	localDate, err := model.LocalDate(now, profile.Timezone)
	if err != nil {
		return "", time.Time{}, err
	}
	resetsAt, err := model.NextLocalMidnight(now, profile.Timezone)
	if err != nil {
		return "", time.Time{}, err
	}
	return localDate, resetsAt, nil
}
