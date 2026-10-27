package service

import (
	"context"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

var sportIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type SportActivityService interface {
	List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.SportActivity, error)
	Get(ctx context.Context, userID, activityID uuid.UUID) (*model.SportActivity, error)
	// Create records a sport and reports whether it was created, replayed, or
	// already existed as an imported activity.
	Create(ctx context.Context, userID uuid.UUID, req model.CreateSportActivityRequest) (*model.SportActivity, model.SportActivityCreateOutcome, error)
	// Link counts an owned unlinked activity toward an owned open sport session.
	Link(ctx context.Context, userID, activityID uuid.UUID, req model.LinkSportActivityRequest) (*model.SportSessionLinkResult, error)
	// Unlink clears an activity's session link so the athlete can repair a wrong match.
	Unlink(ctx context.Context, userID, activityID uuid.UUID, req model.UnlinkSportActivityRequest) (*model.SportSessionLinkResult, error)
}

type sportActivityService struct {
	activities dao.SportActivityDAO
	profiles   dao.TrainingProfileDAO
	now        func() time.Time
}

func NewSportActivityService(activities dao.SportActivityDAO, profiles dao.TrainingProfileDAO) SportActivityService {
	return &sportActivityService{activities: activities, profiles: profiles, now: time.Now}
}

func (s *sportActivityService) List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.SportActivity, error) {
	if err := validateSportRange(from, to); err != nil {
		return nil, err
	}
	return s.activities.List(ctx, userID, from, to)
}

func (s *sportActivityService) Get(ctx context.Context, userID, activityID uuid.UUID) (*model.SportActivity, error) {
	return s.activities.Get(ctx, userID, activityID)
}

func (s *sportActivityService) Create(ctx context.Context, userID uuid.UUID, req model.CreateSportActivityRequest) (*model.SportActivity, model.SportActivityCreateOutcome, error) {
	profile, err := s.profiles.Get(ctx, userID)
	if err != nil {
		return nil, model.SportActivityCreated, err
	}
	location, err := time.LoadLocation(profile.Timezone)
	if err != nil {
		return nil, model.SportActivityCreated, &model.ValidationError{Message: "invalid saved timezone", Field: "timezone"}
	}

	req.SportID = strings.TrimSpace(req.SportID)
	req.SportName = strings.TrimSpace(req.SportName)
	req.OperationKey = strings.TrimSpace(req.OperationKey)
	req.Notes = trimOptional(req.Notes)
	req.ExternalID = trimOptional(req.ExternalID)
	if req.Source == "" {
		req.Source = model.ActivitySourceManual
	}
	localNow := s.now().In(location)
	if req.Date == "" {
		req.Date = localNow.Format("2006-01-02")
	}
	if err := validateSportRequest(req, localNow); err != nil {
		return nil, model.SportActivityCreated, err
	}

	hash, err := hashPayload(req)
	if err != nil {
		return nil, model.SportActivityCreated, err
	}
	activity := &model.SportActivity{
		Date:            req.Date,
		SportID:         req.SportID,
		SportName:       req.SportName,
		DurationMinutes: req.DurationMinutes,
		Notes:           req.Notes,
		Source:          req.Source,
		ExternalID:      req.ExternalID,
		DistanceMeters:  req.DistanceMeters,
		EnergyKcal:      req.EnergyKcal,
		AvgHeartRateBPM: req.AvgHeartRateBPM,
		// The DAO resolves the session inside the create transaction: a
		// foreign id is a 404, an exercise workout or sport mismatch a 422,
		// and a completed session a SPORT_SESSION_CONFLICT.
		ScheduledWorkoutID: req.ScheduledWorkoutID,
		PerceivedEffort:    req.PerceivedEffort,
	}
	return s.activities.Create(ctx, userID, activity, profile.Timezone, req.OperationKey, hash)
}

func validLinkKey(operationKey string, expectedRevision int64) error {
	if operationKey == "" || len(operationKey) > 128 {
		return &model.ValidationError{Message: "operation_key must be between 1 and 128 characters", Field: "operation_key"}
	}
	if expectedRevision < 1 {
		return &model.ValidationError{Message: "expected_revision must be at least 1", Field: "expected_revision"}
	}
	return nil
}

func (s *sportActivityService) Link(ctx context.Context, userID, activityID uuid.UUID, req model.LinkSportActivityRequest) (*model.SportSessionLinkResult, error) {
	req.OperationKey = strings.TrimSpace(req.OperationKey)
	if req.ScheduledWorkoutID == uuid.Nil {
		return nil, &model.ValidationError{Message: "scheduled_workout_id is required", Field: "scheduled_workout_id"}
	}
	if err := validLinkKey(req.OperationKey, req.ExpectedRevision); err != nil {
		return nil, err
	}
	hash, err := hashPayload(struct {
		ActivityID uuid.UUID
		Request    model.LinkSportActivityRequest
	}{activityID, req})
	if err != nil {
		return nil, err
	}
	result, _, err := s.activities.Link(ctx, userID, activityID, req, hash)
	return result, err
}

func (s *sportActivityService) Unlink(ctx context.Context, userID, activityID uuid.UUID, req model.UnlinkSportActivityRequest) (*model.SportSessionLinkResult, error) {
	req.OperationKey = strings.TrimSpace(req.OperationKey)
	if err := validLinkKey(req.OperationKey, req.ExpectedRevision); err != nil {
		return nil, err
	}
	profile, err := s.profiles.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(profile.Timezone)
	if err != nil {
		return nil, &model.ValidationError{Message: "invalid saved timezone", Field: "timezone"}
	}
	hash, err := hashPayload(struct {
		ActivityID uuid.UUID
		Request    model.UnlinkSportActivityRequest
	}{activityID, req})
	if err != nil {
		return nil, err
	}
	localToday := s.now().In(location).Format("2006-01-02")
	result, _, err := s.activities.Unlink(ctx, userID, activityID, req, localToday, profile.Timezone, hash)
	return result, err
}

func validateSportRange(from, to string) error {
	if err := model.ValidateDateRange(from, to); err != nil {
		return err
	}
	fromDate, _ := model.ParseDate(from)
	toDate, _ := model.ParseDate(to)
	if toDate.Sub(fromDate) > 365*24*time.Hour {
		return &model.ValidationError{Message: "date range must not exceed 366 days", Field: "to"}
	}
	return nil
}

func validateSportRequest(req model.CreateSportActivityRequest, localNow time.Time) error {
	date, err := model.ParseDate(req.Date)
	if err != nil {
		return &model.ValidationError{Message: "date must be YYYY-MM-DD", Field: "date"}
	}
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.UTC)
	if date.After(today) {
		return &model.ValidationError{Message: "cannot log future dates", Field: "date"}
	}
	if !sportIDPattern.MatchString(req.SportID) || len(req.SportID) > 64 {
		return &model.ValidationError{Message: "invalid sport identifier", Field: "sport_id"}
	}
	nameLength := utf8.RuneCountInString(req.SportName)
	if nameLength < 1 || nameLength > 80 || (req.SportID == "other" && strings.EqualFold(req.SportName, "other")) {
		return &model.ValidationError{Message: "sport name must be between 1 and 80 characters", Field: "sport_name"}
	}
	if req.DurationMinutes < 1 || req.DurationMinutes > 1440 {
		return &model.ValidationError{Message: "duration_minutes must be between 1 and 1440", Field: "duration_minutes"}
	}
	if req.Notes != nil && utf8.RuneCountInString(*req.Notes) > 2000 {
		return &model.ValidationError{Message: "notes must not exceed 2000 characters", Field: "notes"}
	}
	if req.OperationKey == "" || len(req.OperationKey) > 128 {
		return &model.ValidationError{Message: "operation_key must be between 1 and 128 characters", Field: "operation_key"}
	}
	if req.PerceivedEffort != nil && (*req.PerceivedEffort < 1 || *req.PerceivedEffort > 10) {
		return &model.ValidationError{Message: "perceived_effort must be between 1 and 10", Field: "perceived_effort"}
	}
	return validateSportOrigin(req)
}

func validateSportOrigin(req model.CreateSportActivityRequest) error {
	switch req.Source {
	case model.ActivitySourceManual:
		if req.ExternalID != nil {
			return &model.ValidationError{Message: "external_id is only allowed for imported activities", Field: "external_id"}
		}
	case model.ActivitySourceAppleHealth:
		if req.ExternalID == nil {
			return &model.ValidationError{Message: "external_id is required for imported activities", Field: "external_id"}
		}
		if utf8.RuneCountInString(*req.ExternalID) > 128 {
			return &model.ValidationError{Message: "external_id must be between 1 and 128 characters", Field: "external_id"}
		}
	default:
		return &model.ValidationError{Message: "source must be manual or apple_health", Field: "source"}
	}
	for _, bound := range []struct {
		value    *int
		min, max int
		field    string
	}{
		{req.DistanceMeters, 1, 1000000, "distance_meters"},
		{req.EnergyKcal, 1, 20000, "energy_kcal"},
		{req.AvgHeartRateBPM, 30, 250, "avg_heart_rate_bpm"},
	} {
		if bound.value != nil && (*bound.value < bound.min || *bound.value > bound.max) {
			return &model.ValidationError{Message: bound.field + " is out of range", Field: bound.field}
		}
	}
	return nil
}
