package service

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type TrainingBlockService interface {
	List(ctx context.Context, userID uuid.UUID, status string, limit, offset int) (*model.TrainingBlockList, error)
	Get(ctx context.Context, userID, blockID uuid.UUID) (*model.TrainingBlock, error)
	Create(ctx context.Context, userID uuid.UUID, req model.CreateTrainingBlockRequest) (*model.TrainingBlock, error)
	AddExposure(ctx context.Context, userID, blockID uuid.UUID, req model.CreateTrainingExposureRequest) (*model.TrainingBlock, error)
	RecordNextMorning(ctx context.Context, userID, blockID, exposureID uuid.UUID, req model.RecordNextMorningRequest) (*model.TrainingBlock, error)
	Transition(ctx context.Context, userID, blockID uuid.UUID, req model.CreateTrainingTransitionRequest) (*model.TrainingBlock, error)
}

type trainingBlockService struct {
	blocks   dao.TrainingBlockDAO
	profiles dao.TrainingProfileDAO
	now      func() time.Time
}

func NewTrainingBlockService(blocks dao.TrainingBlockDAO, profiles dao.TrainingProfileDAO) TrainingBlockService {
	return &trainingBlockService{blocks: blocks, profiles: profiles, now: time.Now}
}

func (s *trainingBlockService) List(ctx context.Context, userID uuid.UUID, status string, limit, offset int) (*model.TrainingBlockList, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		status = model.TrainingBlockActive
	}
	if status != model.TrainingBlockActive && status != model.TrainingBlockCompleted && status != model.TrainingBlockArchived && status != "all" {
		return nil, &model.ValidationError{Message: "invalid training block status", Field: "status"}
	}
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return nil, &model.ValidationError{Message: "limit must be between 1 and 100", Field: "limit"}
	}
	if offset < 0 {
		return nil, &model.ValidationError{Message: "offset must be zero or greater", Field: "offset"}
	}
	return s.blocks.List(ctx, userID, status, limit, offset)
}

func (s *trainingBlockService) Get(ctx context.Context, userID, blockID uuid.UUID) (*model.TrainingBlock, error) {
	return s.blocks.Get(ctx, userID, blockID)
}

func (s *trainingBlockService) Create(ctx context.Context, userID uuid.UUID, req model.CreateTrainingBlockRequest) (*model.TrainingBlock, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Purpose = trimOptional(req.Purpose)
	req.OperationKey = strings.TrimSpace(req.OperationKey)
	for i := range req.Stages {
		req.Stages[i].Name = strings.TrimSpace(req.Stages[i].Name)
		req.Stages[i].Instructions = trimOptional(req.Stages[i].Instructions)
		req.Stages[i].LoadLevel = strings.TrimSpace(req.Stages[i].LoadLevel)
	}
	if err := validateCreateTrainingBlock(req); err != nil {
		return nil, err
	}
	hash, err := hashPayload(req)
	if err != nil {
		return nil, err
	}
	block, _, err := s.blocks.Create(ctx, userID, req, hash)
	return block, err
}

func (s *trainingBlockService) AddExposure(ctx context.Context, userID, blockID uuid.UUID, req model.CreateTrainingExposureRequest) (*model.TrainingBlock, error) {
	profile, err := s.profiles.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(profile.Timezone)
	if err != nil {
		return nil, &model.ValidationError{Message: "invalid saved timezone", Field: "timezone"}
	}
	req.PerformedOn = strings.TrimSpace(req.PerformedOn)
	req.ActivityLabel = strings.TrimSpace(req.ActivityLabel)
	req.LoadLevel = strings.TrimSpace(req.LoadLevel)
	req.SessionOutcome = strings.TrimSpace(req.SessionOutcome)
	req.Notes = trimOptional(req.Notes)
	req.OperationKey = strings.TrimSpace(req.OperationKey)
	if err := validateTrainingExposure(req, s.now().In(location)); err != nil {
		return nil, err
	}
	hash, err := hashPayload(req)
	if err != nil {
		return nil, err
	}
	block, _, err := s.blocks.AddExposure(ctx, userID, blockID, req, hash)
	return block, err
}

func (s *trainingBlockService) RecordNextMorning(ctx context.Context, userID, blockID, exposureID uuid.UUID, req model.RecordNextMorningRequest) (*model.TrainingBlock, error) {
	req.Response = strings.TrimSpace(req.Response)
	req.OperationKey = strings.TrimSpace(req.OperationKey)
	if req.Response != model.NextMorningBaseline && req.Response != model.NextMorningAboveBaseline {
		return nil, &model.ValidationError{Message: "invalid next-morning response", Field: "response"}
	}
	if err := validateRevisionOperation(req.ExpectedRevision, req.OperationKey); err != nil {
		return nil, err
	}
	hash, err := hashPayload(req)
	if err != nil {
		return nil, err
	}
	block, _, err := s.blocks.RecordNextMorning(ctx, userID, blockID, exposureID, req, hash)
	return block, err
}

func (s *trainingBlockService) Transition(ctx context.Context, userID, blockID uuid.UUID, req model.CreateTrainingTransitionRequest) (*model.TrainingBlock, error) {
	req.Action = strings.TrimSpace(req.Action)
	req.Reason = trimOptional(req.Reason)
	req.OperationKey = strings.TrimSpace(req.OperationKey)
	if req.Action != model.TransitionAdvance && req.Action != model.TransitionRegress && req.Action != model.TransitionComplete && req.Action != model.TransitionArchive {
		return nil, &model.ValidationError{Message: "invalid transition action", Field: "action"}
	}
	if req.Action == model.TransitionRegress {
		if req.ToStageID == nil {
			return nil, &model.ValidationError{Message: "to_stage_id is required for regression", Field: "to_stage_id"}
		}
		if req.Reason == nil {
			return nil, &model.ValidationError{Message: "reason is required for regression", Field: "reason"}
		}
	}
	if req.Reason != nil && utf8.RuneCountInString(*req.Reason) > 500 {
		return nil, &model.ValidationError{Message: "reason must not exceed 500 characters", Field: "reason"}
	}
	if err := validateRevisionOperation(req.ExpectedRevision, req.OperationKey); err != nil {
		return nil, err
	}
	hash, err := hashPayload(req)
	if err != nil {
		return nil, err
	}
	block, _, err := s.blocks.Transition(ctx, userID, blockID, req, hash)
	return block, err
}

func validateCreateTrainingBlock(req model.CreateTrainingBlockRequest) error {
	if runeLengthOutside(req.Name, 1, 120) {
		return &model.ValidationError{Message: "name must be between 1 and 120 characters", Field: "name"}
	}
	if req.Purpose != nil && utf8.RuneCountInString(*req.Purpose) > 500 {
		return &model.ValidationError{Message: "purpose must not exceed 500 characters", Field: "purpose"}
	}
	if len(req.Stages) < 2 || len(req.Stages) > 12 {
		return &model.ValidationError{Message: "stages must contain between 2 and 12 items", Field: "stages"}
	}
	for i, stage := range req.Stages {
		field := "stages[" + strconv.Itoa(i) + "]"
		if runeLengthOutside(stage.Name, 1, 120) {
			return &model.ValidationError{Message: "stage name must be between 1 and 120 characters", Field: field + ".name"}
		}
		if stage.Instructions != nil && utf8.RuneCountInString(*stage.Instructions) > 1000 {
			return &model.ValidationError{Message: "stage instructions must not exceed 1000 characters", Field: field + ".instructions"}
		}
		if !validTrainingLoad(stage.LoadLevel) {
			return &model.ValidationError{Message: "invalid stage load level", Field: field + ".load_level"}
		}
		if err := validateTrainingTargets(stage.TargetCount, stage.TargetDurationMinutes, stage.TargetIntensityPercent, field, "target_count", "target_duration_minutes", "target_intensity_percent"); err != nil {
			return err
		}
		if stage.RequiredQualifyingExposures < 1 || stage.RequiredQualifyingExposures > 20 {
			return &model.ValidationError{Message: "required qualifying exposures must be between 1 and 20", Field: field + ".required_qualifying_exposures"}
		}
	}
	return validateOperationKey(req.OperationKey)
}

func validateTrainingExposure(req model.CreateTrainingExposureRequest, localNow time.Time) error {
	date, err := model.ParseDate(req.PerformedOn)
	if err != nil {
		return &model.ValidationError{Message: "performed_on must be YYYY-MM-DD", Field: "performed_on"}
	}
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.UTC)
	if date.After(today) {
		return &model.ValidationError{Message: "cannot log future dates", Field: "performed_on"}
	}
	if runeLengthOutside(req.ActivityLabel, 1, 120) {
		return &model.ValidationError{Message: "activity_label must be between 1 and 120 characters", Field: "activity_label"}
	}
	if !validTrainingLoad(req.LoadLevel) {
		return &model.ValidationError{Message: "invalid load level", Field: "load_level"}
	}
	if req.SessionOutcome != model.SessionCompletedAsPlanned && req.SessionOutcome != model.SessionModified && req.SessionOutcome != model.SessionStopped {
		return &model.ValidationError{Message: "invalid session outcome", Field: "session_outcome"}
	}
	if err := validateTrainingTargets(req.PerformedCount, req.DurationMinutes, req.PerformedIntensityPercent, "", "performed_count", "duration_minutes", "performed_intensity_percent"); err != nil {
		return err
	}
	if req.Notes != nil && utf8.RuneCountInString(*req.Notes) > 1000 {
		return &model.ValidationError{Message: "notes must not exceed 1000 characters", Field: "notes"}
	}
	return validateRevisionOperation(req.ExpectedRevision, req.OperationKey)
}

func validateTrainingTargets(count, duration, intensity *int, prefix, countName, durationName, intensityName string) error {
	field := func(name string) string {
		if prefix == "" {
			return name
		}
		return prefix + "." + name
	}
	if count != nil && (*count < 1 || *count > 10000) {
		return &model.ValidationError{Message: "count must be between 1 and 10000", Field: field(countName)}
	}
	if duration != nil && (*duration < 1 || *duration > 1440) {
		return &model.ValidationError{Message: "duration must be between 1 and 1440", Field: field(durationName)}
	}
	if intensity != nil && (*intensity < 1 || *intensity > 100) {
		return &model.ValidationError{Message: "intensity must be between 1 and 100", Field: field(intensityName)}
	}
	return nil
}

func validateRevisionOperation(revision int64, operationKey string) error {
	if revision < 1 {
		return &model.ValidationError{Message: "expected_revision must be greater than zero", Field: "expected_revision"}
	}
	return validateOperationKey(operationKey)
}

func validateOperationKey(value string) error {
	if _, err := uuid.Parse(value); err != nil {
		return &model.ValidationError{Message: "operation_key must be a UUID", Field: "operation_key"}
	}
	return nil
}

func trimOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func validTrainingLoad(value string) bool {
	return value == model.TrainingLoadEasy || value == model.TrainingLoadDemanding
}

func runeLengthOutside(value string, minimum, maximum int) bool {
	length := utf8.RuneCountInString(value)
	return length < minimum || length > maximum
}
