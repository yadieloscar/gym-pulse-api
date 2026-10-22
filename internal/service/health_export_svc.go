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

type HealthExportService interface {
	// Record remembers the health-store identity of a finished workout once.
	Record(ctx context.Context, userID uuid.UUID, kind string, workoutID uuid.UUID, req model.RecordHealthExportRequest) (*model.HealthExport, model.HealthExportOutcome, error)
	List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.HealthExport, error)
}

type healthExportService struct {
	exports dao.HealthExportDAO
	now     func() time.Time
}

func NewHealthExportService(exports dao.HealthExportDAO) HealthExportService {
	return &healthExportService{exports: exports, now: time.Now}
}

func (s *healthExportService) Record(ctx context.Context, userID uuid.UUID, kind string, workoutID uuid.UUID, req model.RecordHealthExportRequest) (*model.HealthExport, model.HealthExportOutcome, error) {
	req.ExternalID = strings.TrimSpace(req.ExternalID)
	req.OperationKey = strings.TrimSpace(req.OperationKey)
	exportKind, err := validateHealthExportRequest(kind, req)
	if err != nil {
		return nil, model.HealthExportRecorded, err
	}
	// Hash the request as the client sent it. Server-defaulted fields such as
	// exported_at depend on the clock, so including them would make a verbatim
	// retry hash differently and surface as an idempotency conflict.
	hash, err := hashPayload(struct {
		Kind      string
		WorkoutID uuid.UUID
		Request   model.RecordHealthExportRequest
	}{kind, workoutID, req})
	if err != nil {
		return nil, model.HealthExportRecorded, err
	}
	now := s.now().UTC()
	exportedAt := now
	if req.ExportedAt != nil && !req.ExportedAt.After(now) {
		exportedAt = req.ExportedAt.UTC()
	}
	export := &model.HealthExport{
		WorkoutKind: exportKind,
		WorkoutID:   workoutID,
		Source:      req.Source,
		ExternalID:  req.ExternalID,
		ExportedAt:  exportedAt,
	}
	return s.exports.Record(ctx, userID, export, req.OperationKey, hash)
}

func (s *healthExportService) List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.HealthExport, error) {
	if err := validateSportRange(from, to); err != nil {
		return nil, err
	}
	return s.exports.List(ctx, userID, from, to)
}

func validateHealthExportRequest(kind string, req model.RecordHealthExportRequest) (model.HealthExportKind, error) {
	exportKind := model.HealthExportKind(kind)
	if exportKind != model.HealthExportKindScheduledWorkout && exportKind != model.HealthExportKindDayLog {
		return "", &model.ValidationError{Message: "workout_kind must be scheduled_workout or day_log", Field: "workout_kind"}
	}
	if req.Source != model.HealthExportSourceAppleHealth {
		return "", &model.ValidationError{Message: "source must be apple_health", Field: "source"}
	}
	if length := utf8.RuneCountInString(req.ExternalID); length < 1 || length > 128 {
		return "", &model.ValidationError{Message: "external_id must be between 1 and 128 characters", Field: "external_id"}
	}
	if req.OperationKey == "" || len(req.OperationKey) > 128 {
		return "", &model.ValidationError{Message: "operation_key must be between 1 and 128 characters", Field: "operation_key"}
	}
	return exportKind, nil
}
