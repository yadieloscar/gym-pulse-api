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
	Create(ctx context.Context, userID uuid.UUID, req model.CreateSportActivityRequest) (*model.SportActivity, error)
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

func (s *sportActivityService) Create(ctx context.Context, userID uuid.UUID, req model.CreateSportActivityRequest) (*model.SportActivity, error) {
	profile, err := s.profiles.Get(ctx, userID)
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(profile.Timezone)
	if err != nil {
		return nil, &model.ValidationError{Message: "invalid saved timezone", Field: "timezone"}
	}

	req.SportID = strings.TrimSpace(req.SportID)
	req.SportName = strings.TrimSpace(req.SportName)
	req.OperationKey = strings.TrimSpace(req.OperationKey)
	if req.Notes != nil {
		trimmed := strings.TrimSpace(*req.Notes)
		if trimmed == "" {
			req.Notes = nil
		} else {
			req.Notes = &trimmed
		}
	}
	localNow := s.now().In(location)
	if req.Date == "" {
		req.Date = localNow.Format("2006-01-02")
	}
	if err := validateSportRequest(req, localNow); err != nil {
		return nil, err
	}

	hash, err := hashPayload(req)
	if err != nil {
		return nil, err
	}
	activity := &model.SportActivity{
		Date:            req.Date,
		SportID:         req.SportID,
		SportName:       req.SportName,
		DurationMinutes: req.DurationMinutes,
		Notes:           req.Notes,
	}
	result, _, err := s.activities.Create(ctx, userID, activity, profile.Timezone, req.OperationKey, hash)
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
	return nil
}
