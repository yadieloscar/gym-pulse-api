package service

import (
	"context"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type PlanTransitionService interface {
	Preview(ctx context.Context, userID uuid.UUID, req model.PreviewPlanTransitionRequest) (*model.PlanTransitionPreview, error)
	Apply(ctx context.Context, userID uuid.UUID, req model.ApplyPlanTransitionRequest) (*model.PlanTransitionPreview, error)
}

type planTransitionService struct {
	starters    dao.StarterProgramDAO
	programs    dao.ProgramDAO
	transitions dao.PlanTransitionDAO
	validator   *validator.Validate
}

func NewPlanTransitionService(starters dao.StarterProgramDAO, programs dao.ProgramDAO, transitions dao.PlanTransitionDAO, v *validator.Validate) PlanTransitionService {
	return &planTransitionService{starters: starters, programs: programs, transitions: transitions, validator: v}
}

func (s *planTransitionService) Preview(ctx context.Context, userID uuid.UUID, req model.PreviewPlanTransitionRequest) (*model.PlanTransitionPreview, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid plan transition preview", Field: "body"}
	}
	profile := req.ProposedProfile.TrainingProfile()
	if profile.Preferences == nil {
		profile.Preferences = map[string]any{}
	}
	if err := profile.Validate(); err != nil {
		return nil, err
	}
	if err := model.ValidateDateRange(req.From, req.To); err != nil {
		return nil, err
	}
	filter := model.StarterProgramFilter{PrimaryGoal: profile.PrimaryGoal, AvailableDays: len(profile.AvailableDays), AvailableWeekdays: profile.AvailableDays, UsualActivity: profile.UsualActivity, Experience: profile.Experience, Equipment: profile.Equipment, SessionDurationMinutes: profile.SessionDurationMinutes}
	candidates, err := s.starters.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	var recommended *model.StarterProgram
	if len(candidates) > 0 {
		recommended = &candidates[0]
	}
	var target *model.Program
	if req.ProgramID != nil {
		target, err = s.programs.Get(ctx, userID, *req.ProgramID)
	} else {
		starter := recommended
		if req.StarterProgramID != nil {
			version := 1
			if req.StarterVersion != nil {
				version = *req.StarterVersion
			}
			starter, err = s.starters.Get(ctx, *req.StarterProgramID, version)
		}
		if err == nil && starter == nil {
			err = &model.NotFoundError{Message: "no matching starter program"}
		}
		if err == nil {
			starterID, version := starter.ID, starter.Version
			target = &model.Program{StarterProgramID: &starterID, StarterVersion: &version, Name: starter.Name, PrimaryGoal: starter.PrimaryGoal, Roadmap: starter.Roadmap, Active: true, Workouts: cloneStarterWorkouts(starter.Workouts)}
		}
	}
	if err != nil {
		return nil, err
	}
	workouts, err := model.MaterializeProgramForWeekdays(target, profile.AvailableDays, req.From, req.To)
	if err != nil {
		return nil, err
	}
	token, err := hashPayload(struct {
		UserID          uuid.UUID
		Request         model.PreviewPlanTransitionRequest
		TargetStarterID *uuid.UUID
		TargetID        uuid.UUID
	}{userID, req, target.StarterProgramID, target.ID})
	if err != nil {
		return nil, err
	}
	preview := &model.PlanTransitionPreview{PreviewToken: token, ProposedProfile: req.ProposedProfile, TargetProgram: *target, RecommendedStarterProgram: recommended, ScheduledWorkouts: workouts}
	if len(workouts) > 0 {
		first := workouts[0].Date
		preview.FirstAffectedDate = &first
	}
	return preview, nil
}

func (s *planTransitionService) Apply(ctx context.Context, userID uuid.UUID, req model.ApplyPlanTransitionRequest) (*model.PlanTransitionPreview, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid plan transition apply", Field: "body"}
	}
	preview, err := s.Preview(ctx, userID, req.PreviewPlanTransitionRequest)
	if err != nil {
		return nil, err
	}
	if preview.PreviewToken != req.PreviewToken {
		return nil, &model.ConflictError{Message: "plan transition preview is stale"}
	}
	hash, err := hashPayload(req)
	if err != nil {
		return nil, err
	}
	profile := req.ProposedProfile.TrainingProfile()
	programID, workouts, replay, err := s.transitions.Apply(ctx, userID, req.ExpectedProfileRevision, &profile, &preview.TargetProgram, req.From, req.To, req.OperationKey, hash)
	if err != nil {
		return nil, err
	}
	if replay {
		program, getErr := s.programs.Get(ctx, userID, programID)
		if getErr != nil {
			return nil, getErr
		}
		preview.TargetProgram = *program
	} else {
		preview.TargetProgram.ID = programID
		preview.ScheduledWorkouts = workouts
	}
	return preview, nil
}
