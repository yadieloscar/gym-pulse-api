package service

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// MobilitySessionService records and reads finished stretching routines. It
// never touches participation, stats, or any other training read model.
type MobilitySessionService interface {
	List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.MobilitySession, error)
	Get(ctx context.Context, userID, sessionID uuid.UUID) (*model.MobilitySession, error)
	// Create records an entry and reports whether it was created or replayed.
	Create(ctx context.Context, userID uuid.UUID, req model.CreateMobilitySessionRequest) (*model.MobilitySession, model.MobilitySessionCreateOutcome, error)
	Delete(ctx context.Context, userID, sessionID uuid.UUID) error
}

type mobilitySessionService struct {
	sessions dao.MobilitySessionDAO
	profiles dao.TrainingProfileDAO
	now      func() time.Time
}

// NewMobilitySessionService returns a MobilitySessionService.
func NewMobilitySessionService(sessions dao.MobilitySessionDAO, profiles dao.TrainingProfileDAO) MobilitySessionService {
	return &mobilitySessionService{sessions: sessions, profiles: profiles, now: time.Now}
}

func (s *mobilitySessionService) List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.MobilitySession, error) {
	if err := validateSportRange(from, to); err != nil {
		return nil, err
	}
	return s.sessions.List(ctx, userID, from, to)
}

func (s *mobilitySessionService) Get(ctx context.Context, userID, sessionID uuid.UUID) (*model.MobilitySession, error) {
	return s.sessions.Get(ctx, userID, sessionID)
}

func (s *mobilitySessionService) Delete(ctx context.Context, userID, sessionID uuid.UUID) error {
	return s.sessions.Delete(ctx, userID, sessionID)
}

func (s *mobilitySessionService) Create(ctx context.Context, userID uuid.UUID, req model.CreateMobilitySessionRequest) (*model.MobilitySession, model.MobilitySessionCreateOutcome, error) {
	profile, err := s.profiles.Get(ctx, userID)
	if err != nil {
		return nil, model.MobilitySessionCreated, err
	}
	location, err := time.LoadLocation(profile.Timezone)
	if err != nil {
		return nil, model.MobilitySessionCreated, &model.ValidationError{Message: "invalid saved timezone", Field: "timezone"}
	}

	req.Date = strings.TrimSpace(req.Date)
	req.RoutineID = strings.TrimSpace(req.RoutineID)
	req.RoutineName = strings.TrimSpace(req.RoutineName)
	req.ContentVersion = strings.TrimSpace(req.ContentVersion)
	req.OperationKey = strings.TrimSpace(req.OperationKey)
	// Hash the request as sent, before the date default, so a retry that
	// crosses local midnight with an omitted date still replays.
	hash, err := hashPayload(req)
	if err != nil {
		return nil, model.MobilitySessionCreated, err
	}
	localNow := s.now().In(location)
	if req.Date == "" {
		req.Date = localNow.Format("2006-01-02")
	}
	if err := validateMobilityRequest(req, localNow); err != nil {
		return nil, model.MobilitySessionCreated, err
	}

	session := &model.MobilitySession{
		Date:            req.Date,
		RoutineID:       req.RoutineID,
		RoutineName:     req.RoutineName,
		Context:         req.Context,
		DurationSeconds: req.DurationSeconds,
		MovesCompleted:  req.MovesCompleted,
		MovesTotal:      req.MovesTotal,
		ContentVersion:  req.ContentVersion,
		OperationKey:    req.OperationKey,
	}
	return s.sessions.Create(ctx, userID, session, hash)
}

func validateMobilityRequest(req model.CreateMobilitySessionRequest, localNow time.Time) error {
	date, err := model.ParseDate(req.Date)
	if err != nil {
		return &model.ValidationError{Message: "date must be YYYY-MM-DD", Field: "date"}
	}
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.UTC)
	if date.After(today) {
		return &model.ValidationError{Message: "cannot log future dates", Field: "date"}
	}
	if !sportIDPattern.MatchString(req.RoutineID) || len(req.RoutineID) > 64 {
		return &model.ValidationError{Message: "routine_id must be a lowercase slug of at most 64 characters", Field: "routine_id"}
	}
	if n := utf8.RuneCountInString(req.RoutineName); n < 1 || n > 80 {
		return &model.ValidationError{Message: "routine_name must be between 1 and 80 characters", Field: "routine_name"}
	}
	if !req.Context.Valid() {
		return &model.ValidationError{Message: "context must be warmup, cooldown, rest_day, or anytime", Field: "context"}
	}
	if req.DurationSeconds < 1 || req.DurationSeconds > 7200 {
		return &model.ValidationError{Message: "duration_seconds must be between 1 and 7200", Field: "duration_seconds"}
	}
	if req.MovesCompleted < 1 || req.MovesCompleted > 100 {
		return &model.ValidationError{Message: "moves_completed must be between 1 and 100", Field: "moves_completed"}
	}
	if req.MovesTotal < 1 || req.MovesTotal > 100 {
		return &model.ValidationError{Message: "moves_total must be between 1 and 100", Field: "moves_total"}
	}
	if req.MovesCompleted > req.MovesTotal {
		return &model.ValidationError{Message: "moves_completed must not exceed moves_total", Field: "moves_completed"}
	}
	if n := utf8.RuneCountInString(req.ContentVersion); n < 1 || n > 32 {
		return &model.ValidationError{Message: "content_version must be between 1 and 32 characters", Field: "content_version"}
	}
	if req.OperationKey == "" || len(req.OperationKey) > 128 {
		return &model.ValidationError{Message: "operation_key must be between 1 and 128 characters", Field: "operation_key"}
	}
	return nil
}
