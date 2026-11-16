package service

import (
	"context"
	"time"

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
	profiles    dao.TrainingProfileDAO
	transitions dao.PlanTransitionDAO
	validator   *validator.Validate
	now         func() time.Time
}

func NewPlanTransitionService(starters dao.StarterProgramDAO, programs dao.ProgramDAO, profiles dao.TrainingProfileDAO, transitions dao.PlanTransitionDAO, v *validator.Validate) PlanTransitionService {
	return &planTransitionService{starters: starters, programs: programs, profiles: profiles, transitions: transitions, validator: v, now: time.Now}
}

// localToday returns the calendar date in the given IANA timezone, and the
// UTC date when the timezone cannot be loaded.
func localToday(now time.Time, timezone string) string {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		location = time.UTC
	}
	return now.In(location).Format(time.DateOnly)
}

// transitionPlan is a resolved start: what the preview returns and what the
// apply needs to carry it out.
type transitionPlan struct {
	preview *model.PlanTransitionPreview
	profile model.TrainingProfile
	// through is the last date the start fills.
	through string
	to      string
}

func (s *planTransitionService) Preview(ctx context.Context, userID uuid.UUID, req model.PreviewPlanTransitionRequest) (*model.PlanTransitionPreview, error) {
	plan, err := s.resolve(ctx, userID, req)
	if err != nil {
		return nil, err
	}
	return plan.preview, nil
}

// effectiveProfile returns the proposed profile when the request carries one
// and the stored profile otherwise. A start needs one of the two: the profile
// supplies the timezone that decides today and the days an unplaced program
// is placed on.
func (s *planTransitionService) effectiveProfile(ctx context.Context, userID uuid.UUID, req model.PreviewPlanTransitionRequest) (model.TrainingProfile, error) {
	if req.ProposedProfile != nil {
		profile := req.ProposedProfile.TrainingProfile()
		if profile.Preferences == nil {
			profile.Preferences = map[string]any{}
		}
		return profile, profile.Validate()
	}
	stored, err := s.profiles.Get(ctx, userID)
	if isNotFound(err) {
		return model.TrainingProfile{}, &model.ValidationError{Field: "available_days", Message: "a training profile is required to start a program"}
	}
	if err != nil {
		return model.TrainingProfile{}, err
	}
	return *stored, nil
}

// activeProgram returns the program the athlete follows. found is false when
// there is none.
func (s *planTransitionService) activeProgram(ctx context.Context, userID uuid.UUID) (p *model.Program, found bool, err error) {
	programs, err := s.programs.List(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	for i := range programs {
		if programs[i].Active {
			return &programs[i], true, nil
		}
	}
	return nil, false, nil
}

func (s *planTransitionService) resolve(ctx context.Context, userID uuid.UUID, req model.PreviewPlanTransitionRequest) (*transitionPlan, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid plan transition preview", Field: "body"}
	}
	profile, err := s.effectiveProfile(ctx, userID, req)
	if err != nil {
		return nil, err
	}
	fromDate, err := model.ParseDate(req.From)
	if err != nil {
		return nil, &model.ValidationError{Field: "from", Message: "from must be YYYY-MM-DD"}
	}
	to := req.To
	if to == "" {
		to = fromDate.AddDate(0, 0, model.DefaultTransitionPreviewDays-1).Format(time.DateOnly)
	}
	if err := model.ValidateDateRange(req.From, to); err != nil {
		return nil, err
	}
	// No session is ever created in the past. A request dated before today
	// (a device a day behind the profile timezone) starts today instead.
	today := localToday(s.now(), profile.Timezone)
	start := max(req.From, today)
	to = max(to, start)

	filter := model.StarterProgramFilter{PrimaryGoal: profile.PrimaryGoal, AvailableDays: len(profile.AvailableDays), AvailableWeekdays: profile.AvailableDays, UsualActivity: profile.UsualActivity, Experience: profile.Experience, Equipment: profile.Equipment, SessionDurationMinutes: profile.SessionDurationMinutes}
	candidates, err := s.starters.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	var recommended *model.StarterProgram
	if len(candidates) > 0 {
		recommended = &candidates[0]
	}
	active, hasActive, err := s.activeProgram(ctx, userID)
	if err != nil {
		return nil, err
	}

	// The target is the named program, else the named starter, else the
	// program the athlete already follows, else the recommended starter. A
	// profile change alone therefore never replaces the active program.
	var target *model.Program
	switch {
	case req.ProgramID != nil:
		target, err = s.programs.Get(ctx, userID, *req.ProgramID)
	case req.StarterProgramID != nil:
		version := 1
		if req.StarterVersion != nil {
			version = *req.StarterVersion
		}
		var starter *model.StarterProgram
		if starter, err = s.starters.Get(ctx, *req.StarterProgramID, version); err == nil {
			target = programFromStarter(starter)
		}
	case hasActive:
		target = active
	case recommended != nil:
		target = programFromStarter(recommended)
	default:
		err = &model.NotFoundError{Message: "no matching starter program"}
	}
	if err != nil {
		return nil, err
	}
	// A target that is already the active, placed program changes no session.
	unchanged := hasActive && active.ID == target.ID && target.Placed()
	if len(target.Workouts) > 0 && !target.Placed() {
		if err := model.PlaceProgram(target, profile.AvailableDays, start); err != nil {
			return nil, err
		}
	}
	workouts, err := model.LayoutSessions(target, start, to)
	if err != nil {
		return nil, err
	}
	token, err := hashPayload(struct {
		UserID          uuid.UUID
		Request         model.PreviewPlanTransitionRequest
		TargetStarterID *uuid.UUID
		TargetID        uuid.UUID
		StartDate       string
	}{userID, req, target.StarterProgramID, target.ID, start})
	if err != nil {
		return nil, err
	}
	preview := &model.PlanTransitionPreview{
		PreviewToken: token, ProposedProfile: req.ProposedProfile, TargetProgram: *target,
		RecommendedStarterProgram: recommended, ScheduledWorkouts: workouts, StartDate: start,
		OffAvailability: model.OffAvailability(target, profile.AvailableDays),
	}
	if hasActive && active.ID != target.ID {
		preview.ReplacesProgram = &model.ProgramRef{ID: active.ID, Name: active.Name}
	}
	if len(workouts) > 0 && !unchanged {
		first := workouts[0].Date
		preview.FirstAffectedDate = &first
	}
	horizon := s.now()
	if parsed, parseErr := model.ParseDate(today); parseErr == nil {
		horizon = parsed
	}
	through := max(horizon.AddDate(0, 0, model.ScheduleHorizonDays).Format(time.DateOnly), to)
	return &transitionPlan{preview: preview, profile: profile, through: through, to: to}, nil
}

func programFromStarter(starter *model.StarterProgram) *model.Program {
	starterID, version := starter.ID, starter.Version
	return &model.Program{StarterProgramID: &starterID, StarterVersion: &version, Name: starter.Name, PrimaryGoal: starter.PrimaryGoal, Roadmap: starter.Roadmap, Active: true, Workouts: cloneStarterWorkouts(starter.Workouts)}
}

func (s *planTransitionService) Apply(ctx context.Context, userID uuid.UUID, req model.ApplyPlanTransitionRequest) (*model.PlanTransitionPreview, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid plan transition apply", Field: "body"}
	}
	plan, err := s.resolve(ctx, userID, req.PreviewPlanTransitionRequest)
	if err != nil {
		return nil, err
	}
	preview := plan.preview
	if preview.PreviewToken != req.PreviewToken {
		return nil, &model.ConflictError{Message: "plan transition preview is stale"}
	}
	hash, err := hashPayload(req)
	if err != nil {
		return nil, err
	}
	apply := dao.PlanTransitionApply{
		ExpectedProfileRevision: req.ExpectedProfileRevision, Target: &preview.TargetProgram,
		AvailableDays: plan.profile.AvailableDays, Start: preview.StartDate, Through: plan.through,
		From: preview.StartDate, To: plan.to, OperationKey: req.OperationKey, RequestHash: hash,
	}
	if req.ProposedProfile != nil {
		profile := plan.profile
		apply.Profile = &profile
	}
	result, err := s.transitions.Apply(ctx, userID, apply)
	if err != nil {
		return nil, err
	}
	program, err := s.programs.Get(ctx, userID, result.ProgramID)
	if err != nil {
		return nil, err
	}
	preview.TargetProgram = *program
	if !result.Replay {
		preview.ScheduledWorkouts = result.Workouts
	}
	return preview, nil
}
