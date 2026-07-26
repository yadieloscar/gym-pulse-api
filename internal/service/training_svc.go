package service

import (
	"context"
	"errors"
	"slices"
	"sort"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type TrainingProfileService interface {
	Get(ctx context.Context, userID uuid.UUID) (*model.TrainingProfile, error)
	Update(ctx context.Context, userID uuid.UUID, req model.UpdateTrainingProfileRequest) (*model.TrainingProfile, error)
}

type ProgramService interface {
	ListStarters(ctx context.Context, filter model.StarterProgramFilter) ([]model.StarterProgram, error)
	List(ctx context.Context, userID uuid.UUID) ([]model.Program, error)
	Get(ctx context.Context, userID, programID uuid.UUID) (*model.Program, error)
	Create(ctx context.Context, userID uuid.UUID, req model.CreateProgramRequest) (*model.Program, error)
	CloneStarter(ctx context.Context, userID uuid.UUID, req model.CloneStarterProgramRequest) (*model.Program, error)
	Update(ctx context.Context, userID, programID uuid.UUID, req model.UpdateProgramRequest) (*model.Program, error)
}

type trainingProfileService struct{ repo dao.TrainingProfileDAO }

type programService struct {
	starters    dao.StarterProgramDAO
	programs    dao.ProgramDAO
	idempotency dao.IdempotencyDAO
	validator   *validator.Validate
}

func NewTrainingProfileService(repo dao.TrainingProfileDAO) TrainingProfileService {
	return &trainingProfileService{repo: repo}
}

func NewProgramService(starters dao.StarterProgramDAO, programs dao.ProgramDAO, idempotency dao.IdempotencyDAO, v *validator.Validate) ProgramService {
	return &programService{starters: starters, programs: programs, idempotency: idempotency, validator: v}
}

func (s *trainingProfileService) Get(ctx context.Context, userID uuid.UUID) (*model.TrainingProfile, error) {
	return s.repo.Get(ctx, userID)
}

func (s *trainingProfileService) Update(ctx context.Context, userID uuid.UUID, req model.UpdateTrainingProfileRequest) (*model.TrainingProfile, error) {
	var profile model.TrainingProfile
	if req.ExpectedRevision == 0 {
		profile = model.TrainingProfile{Preferences: map[string]any{}}
	} else {
		current, err := s.repo.Get(ctx, userID)
		if err != nil {
			return nil, err
		}
		profile = *current
	}
	if req.PrimaryGoal != nil {
		profile.PrimaryGoal = *req.PrimaryGoal
	}
	if req.AvailableDays != nil {
		profile.AvailableDays = slices.Clone(*req.AvailableDays)
	}
	if req.UsualActivity != nil {
		profile.UsualActivity = *req.UsualActivity
	}
	if req.Experience != nil {
		profile.Experience = *req.Experience
	}
	if req.Equipment != nil {
		profile.Equipment = slices.Clone(*req.Equipment)
	}
	if req.SessionDurationMinutes != nil {
		profile.SessionDurationMinutes = *req.SessionDurationMinutes
	}
	if req.Timezone != nil {
		profile.Timezone = *req.Timezone
	}
	if req.Preferences != nil {
		profile.Preferences = *req.Preferences
	}
	if err := profile.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.Put(ctx, userID, &profile, req.ExpectedRevision); err != nil {
		return nil, err
	}
	return &profile, nil
}

func (s *programService) ListStarters(ctx context.Context, filter model.StarterProgramFilter) ([]model.StarterProgram, error) {
	if filter.PrimaryGoal != "" && !model.IsValidTrainingGoal(filter.PrimaryGoal) {
		return nil, &model.ValidationError{Message: "unknown training goal", Field: "primary_goal"}
	}
	if filter.AvailableDays < 0 || filter.AvailableDays > 7 {
		return nil, &model.ValidationError{Message: "available_days must be between 1 and 7", Field: "available_days"}
	}
	if filter.UsualActivity != "" && !slices.Contains(model.ValidActivityLevels, filter.UsualActivity) {
		return nil, &model.ValidationError{Message: "unknown activity level", Field: "usual_activity"}
	}
	seen := map[int]bool{}
	for _, day := range filter.AvailableWeekdays {
		if day < 1 || day > 7 || seen[day] {
			return nil, &model.ValidationError{Message: "available_weekdays must contain unique ISO weekdays", Field: "available_weekdays"}
		}
		seen[day] = true
	}
	programs, err := s.starters.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	activityTarget := map[string]int{"sedentary": 35, "light": 45, "moderate": 60, "high": 75}[filter.UsualActivity]
	sort.SliceStable(programs, func(i, j int) bool {
		score := func(p model.StarterProgram) int {
			value := 0
			if filter.PrimaryGoal != "" && p.PrimaryGoal == filter.PrimaryGoal {
				value += 100
			}
			if filter.AvailableDays > 0 && filter.AvailableDays >= p.MinDays && filter.AvailableDays <= p.MaxDays {
				value += 30
			}
			if filter.Experience != "" && slices.Contains(p.Experience, filter.Experience) {
				value += 20
			}
			if len(filter.Equipment) == 0 || len(p.Equipment) == 0 || allEquipmentAvailable(p.Equipment, filter.Equipment) {
				value += 15
			}
			if filter.SessionDurationMinutes == 0 || p.DurationMinutes <= filter.SessionDurationMinutes {
				value += 10
			}
			if activityTarget > 0 {
				difference := p.DurationMinutes - activityTarget
				if difference < 0 {
					difference = -difference
				}
				value += max(0, 10-difference/5)
			}
			if len(filter.AvailableWeekdays) > 0 {
				value += 5
			}
			return value
		}
		return score(programs[i]) > score(programs[j])
	})
	return programs, nil
}

func allEquipmentAvailable(required, available []string) bool {
	for _, item := range required {
		if !slices.Contains(available, item) && !slices.Contains(available, "full_gym") {
			return false
		}
	}
	return true
}

func (s *programService) List(ctx context.Context, userID uuid.UUID) ([]model.Program, error) {
	return s.programs.List(ctx, userID)
}

func (s *programService) Get(ctx context.Context, userID, programID uuid.UUID) (*model.Program, error) {
	return s.programs.Get(ctx, userID, programID)
}

func (s *programService) Create(ctx context.Context, userID uuid.UUID, req model.CreateProgramRequest) (*model.Program, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid program", Field: "body"}
	}
	if err := validateProgramShape(req.PrimaryGoal, req.Workouts); err != nil {
		return nil, err
	}
	p := &model.Program{Name: req.Name, PrimaryGoal: req.PrimaryGoal, Roadmap: req.Roadmap, Active: true, Workouts: req.Workouts}
	if err := s.programs.Create(ctx, userID, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *programService) CloneStarter(ctx context.Context, userID uuid.UUID, req model.CloneStarterProgramRequest) (*model.Program, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid starter selection", Field: "body"}
	}
	hash, err := hashPayload(req)
	if err != nil {
		return nil, err
	}
	if _, atomic := s.programs.(dao.IdempotentProgramDAO); !atomic {
		if replay, err := s.idempotency.Get(ctx, userID, "programs/from-starter", req.OperationKey); err == nil {
			if replay.RequestHash != hash {
				return nil, &model.ConflictError{Message: "idempotency key was already used with a different payload"}
			}
			if replay.ResourceID == nil {
				return nil, &model.ConflictError{Message: "idempotency record has no program"}
			}
			return s.programs.Get(ctx, userID, *replay.ResourceID)
		} else if !isNotFound(err) {
			return nil, err
		}
	}

	starter, err := s.starters.Get(ctx, req.StarterProgramID, req.StarterVersion)
	if err != nil {
		return nil, err
	}
	name := starter.Name
	if req.Name != nil {
		name = *req.Name
	}
	starterID, version := starter.ID, starter.Version
	p := &model.Program{
		StarterProgramID: &starterID, StarterVersion: &version, Name: name,
		PrimaryGoal: starter.PrimaryGoal, Roadmap: starter.Roadmap, Active: true,
		Workouts: cloneStarterWorkouts(starter.Workouts),
	}
	if repo, ok := s.programs.(dao.IdempotentProgramDAO); ok {
		result, _, err := repo.CreateIdempotent(ctx, userID, p, model.IdempotencyRecord{
			Scope: "programs/from-starter", OperationKey: req.OperationKey,
			RequestHash: hash, ResponseStatus: 201, ResourceType: "program",
		})
		return result, err
	}
	if err := s.programs.Create(ctx, userID, p); err != nil {
		return nil, err
	}
	if err := recordResource(ctx, s.idempotency, userID, "programs/from-starter", req.OperationKey, hash, "program", p.ID, p.Revision); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *programService) Update(ctx context.Context, userID, programID uuid.UUID, req model.UpdateProgramRequest) (*model.Program, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid program", Field: "body"}
	}
	if err := validateProgramShape(req.PrimaryGoal, req.Workouts); err != nil {
		return nil, err
	}
	p := &model.Program{ID: programID, Name: req.Name, PrimaryGoal: req.PrimaryGoal, Roadmap: req.Roadmap, Active: req.Active, Workouts: req.Workouts}
	if err := s.programs.Replace(ctx, userID, p, req.ExpectedRevision); err != nil {
		return nil, err
	}
	return p, nil
}

func cloneStarterWorkouts(src []model.ProgramWorkout) []model.ProgramWorkout {
	out := make([]model.ProgramWorkout, len(src))
	for i := range src {
		out[i] = src[i]
		out[i].ID = uuid.Nil
		out[i].Exercises = make([]model.ProgramExercise, len(src[i].Exercises))
		for j := range src[i].Exercises {
			out[i].Exercises[j] = src[i].Exercises[j]
			sourceID := src[i].Exercises[j].ID
			out[i].Exercises[j].ID = uuid.Nil
			out[i].Exercises[j].SourceStarterExerciseID = &sourceID
		}
	}
	return out
}

func validateProgramShape(goal string, workouts []model.ProgramWorkout) error {
	if !model.IsValidTrainingGoal(goal) {
		return &model.ValidationError{Message: "unknown training goal", Field: "primary_goal"}
	}
	for _, workout := range workouts {
		if workout.Name == "" || workout.SequencePosition < 1 || len(workout.Exercises) == 0 {
			return &model.ValidationError{Message: "invalid program workout", Field: "workouts"}
		}
		for _, exercise := range workout.Exercises {
			if exercise.Name == "" || exercise.ExerciseOrder < 1 || exercise.TargetSets < 1 || (exercise.Modality != "strength" && exercise.Modality != "cardio") {
				return &model.ValidationError{Message: "invalid program exercise", Field: "workouts"}
			}
		}
	}
	return nil
}

func isNotFound(err error) bool {
	var notFound *model.NotFoundError
	return errors.As(err, &notFound)
}
