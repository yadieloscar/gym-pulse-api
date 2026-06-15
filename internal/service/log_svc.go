package service

import (
	"context"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type LogService interface {
	ListByWeek(ctx context.Context, userID uuid.UUID, weekParam string) ([]model.DayLogSummary, error)
	GetByDate(ctx context.Context, userID uuid.UUID, date string) (*model.DayLog, error)
	Create(ctx context.Context, userID uuid.UUID, req model.CreateDayLogRequest) (*model.DayLog, error)
	Update(ctx context.Context, userID uuid.UUID, date string, req model.UpdateDayLogRequest) (*model.DayLog, error)
	Delete(ctx context.Context, userID uuid.UUID, date string) error
	ExerciseHistory(ctx context.Context, userID uuid.UUID, idsParam string) ([]model.ExerciseHistory, error)
}

type logService struct {
	repo         dao.LogDAO
	templateRepo dao.TemplateDAO
	validator    *validator.Validate
}

func NewLogService(repo dao.LogDAO, templateRepo dao.TemplateDAO, v *validator.Validate) LogService {
	return &logService{repo: repo, templateRepo: templateRepo, validator: v}
}

func (s *logService) ListByWeek(ctx context.Context, userID uuid.UUID, weekParam string) ([]model.DayLogSummary, error) {
	t, err := model.ParseDate(weekParam)
	if err != nil {
		return nil, &model.ValidationError{Message: "invalid date format, expected YYYY-MM-DD", Field: "week"}
	}
	monday := model.MondayOfWeek(t)
	return s.repo.ListByWeek(ctx, userID, monday)
}

func (s *logService) GetByDate(ctx context.Context, userID uuid.UUID, date string) (*model.DayLog, error) {
	if _, err := model.ParseDate(date); err != nil {
		return nil, &model.ValidationError{Message: "invalid date format, expected YYYY-MM-DD", Field: "date"}
	}
	return s.repo.GetByDate(ctx, userID, date)
}

func (s *logService) Create(ctx context.Context, userID uuid.UUID, req model.CreateDayLogRequest) (*model.DayLog, error) {
	if err := s.validator.Struct(req); err != nil {
		return nil, &model.ValidationError{Message: "invalid log data", Field: "body"}
	}

	parsedDate, err := model.ParseDate(req.Date)
	if err != nil {
		return nil, &model.ValidationError{Message: "invalid date format, expected YYYY-MM-DD", Field: "date"}
	}

	today := time.Now().Truncate(24 * time.Hour)
	if parsedDate.After(today) {
		return nil, &model.ValidationError{Message: "cannot log future dates", Field: "date"}
	}

	if !model.IsValidTypeID(req.TypeID) {
		return nil, &model.ValidationError{Message: "invalid workout type: " + req.TypeID, Field: "type_id"}
	}
	if !model.IsValidSubtypeID(req.SubtypeID) {
		return nil, &model.ValidationError{Message: "invalid workout subtype: " + req.SubtypeID, Field: "subtype_id"}
	}

	if req.TypeID == "rest" {
		if req.TemplateID != nil {
			return nil, &model.ValidationError{Message: "rest days cannot have templates", Field: "template_id"}
		}
		if len(req.Overrides) > 0 {
			return nil, &model.ValidationError{Message: "rest days cannot have exercise overrides", Field: "overrides"}
		}
		if len(req.SetLogs) > 0 {
			return nil, &model.ValidationError{Message: "rest days cannot have set logs", Field: "set_logs"}
		}
	}

	if err := s.validateSetLogs(req.SetLogs); err != nil {
		return nil, err
	}

	// Verify template ownership if provided.
	if req.TemplateID != nil {
		_, err := s.templateRepo.GetByID(ctx, userID, *req.TemplateID)
		if err != nil {
			return nil, err
		}
	}

	dl := &model.DayLog{
		Date:         req.Date,
		TypeID:       req.TypeID,
		SubtypeID:    req.SubtypeID,
		TemplateID:   req.TemplateID,
		SessionNotes: req.SessionNotes,
		Overrides:    toOverrides(req.Overrides),
		SetLogs:      toSetLogs(req.SetLogs),
	}

	if err := s.repo.Create(ctx, userID, dl); err != nil {
		return nil, err
	}
	return dl, nil
}

func (s *logService) Update(ctx context.Context, userID uuid.UUID, date string, req model.UpdateDayLogRequest) (*model.DayLog, error) {
	if _, err := model.ParseDate(date); err != nil {
		return nil, &model.ValidationError{Message: "invalid date format, expected YYYY-MM-DD", Field: "date"}
	}

	replace, err := s.resolveReplacement(ctx, userID, req)
	if err != nil {
		return nil, err
	}

	// Mirror Create's invariant: a rest day carries no exercise overrides.
	// Without this, replacing a day to rest while sending overrides would
	// persist a rest log with overrides — a state Create forbids.
	if replace != nil && replace.TypeID == "rest" && len(req.Overrides) > 0 {
		return nil, &model.ValidationError{Message: "rest days cannot have exercise overrides", Field: "overrides"}
	}
	if replace != nil && replace.TypeID == "rest" && len(req.SetLogs) > 0 {
		return nil, &model.ValidationError{Message: "rest days cannot have set logs", Field: "set_logs"}
	}

	if err := s.validateSetLogs(req.SetLogs); err != nil {
		return nil, err
	}

	overrides := toOverrides(req.Overrides)
	setLogs := toSetLogs(req.SetLogs)
	if err := s.repo.Update(ctx, userID, date, overrides, setLogs, req.SessionNotes, replace); err != nil {
		return nil, err
	}

	return s.repo.GetByDate(ctx, userID, date)
}

// resolveReplacement validates the optional workout-replacement fields on an
// update. When a template is given it is authoritative: ownership is checked
// and type/subtype derive from the template, ignoring any drifting values in
// the request. Type-only replacement (quick logs) is validated against the
// known type/subtype ids.
func (s *logService) resolveReplacement(ctx context.Context, userID uuid.UUID, req model.UpdateDayLogRequest) (*model.LogReplacement, error) {
	if req.TemplateID == nil && req.TypeID == nil && req.SubtypeID == nil {
		// nil replacement is the documented "nothing to replace" signal the
		// DAO branches on; a sentinel error would conflate it with failure.
		return nil, nil //nolint:nilnil
	}

	if req.TemplateID != nil {
		tpl, err := s.templateRepo.GetByID(ctx, userID, *req.TemplateID)
		if err != nil {
			return nil, err
		}
		return &model.LogReplacement{TypeID: tpl.TypeID, SubtypeID: tpl.SubtypeID, TemplateID: req.TemplateID}, nil
	}

	if req.TypeID == nil || req.SubtypeID == nil {
		return nil, &model.ValidationError{Message: "replacing a workout requires type_id and subtype_id (or a template_id)", Field: "type_id"}
	}
	if !model.IsValidTypeID(*req.TypeID) {
		return nil, &model.ValidationError{Message: "invalid workout type: " + *req.TypeID, Field: "type_id"}
	}
	if !model.IsValidSubtypeID(*req.SubtypeID) {
		return nil, &model.ValidationError{Message: "invalid workout subtype: " + *req.SubtypeID, Field: "subtype_id"}
	}
	return &model.LogReplacement{TypeID: *req.TypeID, SubtypeID: *req.SubtypeID, TemplateID: nil}, nil
}

func (s *logService) Delete(ctx context.Context, userID uuid.UUID, date string) error {
	if _, err := model.ParseDate(date); err != nil {
		return &model.ValidationError{Message: "invalid date format, expected YYYY-MM-DD", Field: "date"}
	}
	return s.repo.Delete(ctx, userID, date)
}

// ExerciseHistory returns the most recent completed sets for each exercise id
// in the comma-separated idsParam (e.g. "uuid,uuid"). Unknown/empty ids yield
// no rows rather than an error.
func (s *logService) ExerciseHistory(ctx context.Context, userID uuid.UUID, idsParam string) ([]model.ExerciseHistory, error) {
	ids := []uuid.UUID{}
	for raw := range strings.SplitSeq(idsParam, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, &model.ValidationError{Message: "invalid exercise id: " + raw, Field: "ids"}
		}
		ids = append(ids, id)
	}
	return s.repo.ExerciseHistory(ctx, userID, ids)
}

// validateSetLogs enforces struct rules plus the invariant that a completed set
// records what was done (reps or a cardio duration).
func (s *logService) validateSetLogs(reqs []model.CreateSetLogRequest) error {
	for _, sl := range reqs {
		if err := s.validator.Struct(sl); err != nil {
			return &model.ValidationError{Message: "invalid set log data", Field: "set_logs"}
		}
		if sl.Completed && sl.ActualReps == nil && sl.DurationSeconds == nil {
			return &model.ValidationError{Message: "a completed set must record actual_reps or duration_seconds", Field: "set_logs"}
		}
	}
	return nil
}

func toSetLogs(reqs []model.CreateSetLogRequest) []model.SetLog {
	if reqs == nil {
		return []model.SetLog{}
	}
	sets := make([]model.SetLog, len(reqs))
	for i, r := range reqs {
		sets[i] = model.SetLog{
			ExerciseID:      r.ExerciseID,
			SetIndex:        r.SetIndex,
			TargetReps:      r.TargetReps,
			TargetWeight:    r.TargetWeight,
			ActualReps:      r.ActualReps,
			ActualWeight:    r.ActualWeight,
			DurationSeconds: r.DurationSeconds,
			Completed:       r.Completed,
		}
	}
	return sets
}

func toOverrides(reqs []model.CreateOverrideRequest) []model.ExerciseOverride {
	if reqs == nil {
		return []model.ExerciseOverride{}
	}
	overrides := make([]model.ExerciseOverride, len(reqs))
	for i, r := range reqs {
		overrides[i] = model.ExerciseOverride{
			ExerciseID:   r.ExerciseID,
			ActualSets:   r.ActualSets,
			ActualReps:   r.ActualReps,
			ActualWeight: r.ActualWeight,
			Notes:        r.Notes,
			Skipped:      r.Skipped,
		}
	}
	return overrides
}
