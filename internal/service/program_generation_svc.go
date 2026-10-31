package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
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
	generator DraftGenerator // nil when no provider key is configured
	limit     int
	model     string
	now       func() time.Time
}

// NewProgramGenerationService wires the DAOs and the optional generator. A nil
// generator reports the feature as disabled.
func NewProgramGenerationService(profiles dao.TrainingProfileDAO, catalog dao.ExerciseCatalogDAO, ledger dao.ProgramGenerationDAO, generator DraftGenerator, limit int, modelID string) ProgramGenerationService {
	return &programGenerationService{profiles: profiles, catalog: catalog, ledger: ledger, generator: generator, limit: limit, model: modelID, now: time.Now}
}

func (s *programGenerationService) Status(ctx context.Context, userID uuid.UUID) (*model.GenerationStatus, error) {
	status := &model.GenerationStatus{Enabled: s.generator != nil, Limit: s.limit, RemainingToday: s.limit}
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
	profile, err := s.profiles.Get(ctx, userID)
	var notFound *model.NotFoundError
	if errors.As(err, &notFound) {
		return nil, &model.ValidationError{Message: "save a training profile before building a program", Field: "training_profile"}
	}
	if err != nil {
		return nil, err
	}
	localDate, resetsAt, err := s.localDay(profile)
	if err != nil {
		return nil, err
	}
	reservation, err := s.ledger.Reserve(ctx, userID, localDate, s.limit, s.model)
	var limitErr *model.GenerationLimitError
	if errors.As(err, &limitErr) {
		limitErr.ResetsAt = resetsAt
		return nil, limitErr
	}
	if err != nil {
		return nil, err
	}

	catalog, err := s.catalog.List(ctx, "")
	if err != nil {
		s.complete(ctx, reservation.ID, model.GenerationFailed)
		return nil, err
	}
	input := GenerationInput{
		PrimaryGoal: profile.PrimaryGoal, AvailableDays: profile.AvailableDays, UsualActivity: profile.UsualActivity,
		Experience: profile.Experience, Equipment: profile.Equipment, SessionDurationMinutes: profile.SessionDurationMinutes,
		Catalog: catalog, Request: request, Model: s.model, WorkoutCap: model.DraftWorkoutCap(profile.AvailableDays),
	}
	started := s.now()
	raw, usage, err := s.generator.Generate(ctx, input)
	if err != nil {
		s.complete(ctx, reservation.ID, model.GenerationFailed)
		slog.WarnContext(ctx, "program generation failed", "model", s.model, "duration_ms", s.now().Sub(started).Milliseconds(), "error", err)
		return nil, &model.GenerationUnavailableError{Reason: model.GenerationUnavailable}
	}
	generated, err := model.RepairGeneratedDraft(profile, catalog, raw, s.model, request)
	if err != nil {
		s.complete(ctx, reservation.ID, model.GenerationFailed)
		slog.WarnContext(ctx, "program generation produced an invalid draft", "model", s.model, "error", err)
		return nil, &model.GenerationUnavailableError{Reason: model.GenerationUnavailable}
	}
	s.complete(ctx, reservation.ID, model.GenerationSucceeded)
	slog.InfoContext(ctx, "program generation succeeded", "model", s.model, "duration_ms", s.now().Sub(started).Milliseconds(),
		"input_tokens", usage.InputTokens, "output_tokens", usage.OutputTokens, "workouts", len(generated.Draft.Workouts))
	return &model.GenerateProgramResponse{
		Draft: generated.Draft, Rationale: generated.Rationale, Request: request,
		RemainingToday: max(0, s.limit-reservation.Used), ResetsAt: resetsAt,
	}, nil
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
