package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type healthExportHandlerService struct {
	exports  []model.HealthExport
	outcome  model.HealthExportOutcome
	err      error
	kind     string
	workout  uuid.UUID
	recorded model.RecordHealthExportRequest
}

func (s *healthExportHandlerService) Record(_ context.Context, _ uuid.UUID, kind string, workoutID uuid.UUID, req model.RecordHealthExportRequest) (*model.HealthExport, model.HealthExportOutcome, error) {
	s.kind, s.workout, s.recorded = kind, workoutID, req
	if s.err != nil {
		return nil, model.HealthExportRecorded, s.err
	}
	return &s.exports[0], s.outcome, nil
}

func (s *healthExportHandlerService) List(context.Context, uuid.UUID, string, string) ([]model.HealthExport, error) {
	return s.exports, s.err
}

func TestHealthExportHandlerRecordStatusFollowsOutcome(t *testing.T) {
	workoutID := uuid.New()
	export := model.HealthExport{ID: uuid.New(), WorkoutKind: model.HealthExportKindScheduledWorkout, WorkoutID: workoutID, Date: "2026-10-13", Source: model.HealthExportSourceAppleHealth, ExternalID: "B5D7"}
	req := model.RecordHealthExportRequest{Source: model.HealthExportSourceAppleHealth, ExternalID: "B5D7", OperationKey: "hk-export-1"}
	for _, test := range []struct {
		name    string
		outcome model.HealthExportOutcome
		status  int
	}{
		{"recorded", model.HealthExportRecorded, http.StatusCreated},
		{"replayed", model.HealthExportReplayed, http.StatusCreated},
		{"existing", model.HealthExportExisting, http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc := &healthExportHandlerService{exports: []model.HealthExport{export}, outcome: test.outcome}
			rec := httptest.NewRecorder()
			NewHealthExportHandler(svc).Record(rec, guidedHandlerRequest(t, http.MethodPut, "/", req, uuid.New(), "hk-export-1", map[string]string{"workout_kind": "scheduled_workout", "workout_id": workoutID.String()}))
			if rec.Code != test.status {
				t.Fatalf("status=%d want %d body=%s", rec.Code, test.status, rec.Body.String())
			}
			var body model.HealthExport
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.ID != export.ID || body.ExternalID != "B5D7" || svc.kind != "scheduled_workout" || svc.workout != workoutID {
				t.Fatalf("body or routing wrong: %s kind=%s", rec.Body.String(), svc.kind)
			}
		})
	}
}

func TestHealthExportHandlerErrors(t *testing.T) {
	workoutID := uuid.New()
	req := model.RecordHealthExportRequest{Source: model.HealthExportSourceAppleHealth, ExternalID: "OTHER", OperationKey: "hk-export-2"}
	stored := model.HealthExport{ID: uuid.New(), ExternalID: "B5D7"}

	conflict := httptest.NewRecorder()
	NewHealthExportHandler(&healthExportHandlerService{err: &model.ConflictError{Message: "health export identity already recorded", Authoritative: stored}}).
		Record(conflict, guidedHandlerRequest(t, http.MethodPut, "/", req, uuid.New(), "hk-export-2", map[string]string{"workout_kind": "day_log", "workout_id": workoutID.String()}))
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), `"code":"HEALTH_EXPORT_CONFLICT"`) || !strings.Contains(conflict.Body.String(), `"external_id":"B5D7"`) {
		t.Fatalf("conflict status=%d body=%s", conflict.Code, conflict.Body.String())
	}

	missing := httptest.NewRecorder()
	NewHealthExportHandler(&healthExportHandlerService{err: &model.NotFoundError{Message: "finished workout not found"}}).
		Record(missing, guidedHandlerRequest(t, http.MethodPut, "/", req, uuid.New(), "hk-export-2", map[string]string{"workout_kind": "day_log", "workout_id": workoutID.String()}))
	if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), "NOT_FOUND") {
		t.Fatalf("missing status=%d body=%s", missing.Code, missing.Body.String())
	}

	invalid := httptest.NewRecorder()
	NewHealthExportHandler(&healthExportHandlerService{err: &model.ValidationError{Message: "workout_kind must be scheduled_workout or day_log", Field: "workout_kind"}}).
		Record(invalid, guidedHandlerRequest(t, http.MethodPut, "/", req, uuid.New(), "hk-export-2", map[string]string{"workout_kind": "session", "workout_id": workoutID.String()}))
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), `"field":"workout_kind"`) {
		t.Fatalf("invalid status=%d body=%s", invalid.Code, invalid.Body.String())
	}

	badID := httptest.NewRecorder()
	NewHealthExportHandler(&healthExportHandlerService{}).Record(badID, guidedHandlerRequest(t, http.MethodPut, "/", req, uuid.New(), "hk-export-2", map[string]string{"workout_kind": "day_log", "workout_id": "bad"}))
	if badID.Code != http.StatusBadRequest {
		t.Fatalf("bad id status=%d", badID.Code)
	}

	badKey := httptest.NewRecorder()
	NewHealthExportHandler(&healthExportHandlerService{}).Record(badKey, guidedHandlerRequest(t, http.MethodPut, "/", req, uuid.New(), "header-key", map[string]string{"workout_kind": "day_log", "workout_id": workoutID.String()}))
	if badKey.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad key status=%d body=%s", badKey.Code, badKey.Body.String())
	}
}

func TestHealthExportHandlerList(t *testing.T) {
	empty := httptest.NewRecorder()
	NewHealthExportHandler(&healthExportHandlerService{exports: []model.HealthExport{}}).List(empty, guidedHandlerRequest(t, http.MethodGet, "/?from=2026-10-01&to=2026-10-14", nil, uuid.New(), "", nil))
	if empty.Code != http.StatusOK || strings.TrimSpace(empty.Body.String()) != `{"health_exports":[]}` {
		t.Fatalf("empty list status=%d body=%s", empty.Code, empty.Body.String())
	}

	export := model.HealthExport{ID: uuid.New(), WorkoutKind: model.HealthExportKindDayLog, WorkoutID: uuid.New(), Date: "2026-10-13", Source: model.HealthExportSourceAppleHealth, ExternalID: "B5D7"}
	listed := httptest.NewRecorder()
	NewHealthExportHandler(&healthExportHandlerService{exports: []model.HealthExport{export}}).List(listed, guidedHandlerRequest(t, http.MethodGet, "/?from=2026-10-01&to=2026-10-14", nil, uuid.New(), "", nil))
	var body struct {
		HealthExports []model.HealthExport `json:"health_exports"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &body); err != nil || len(body.HealthExports) != 1 || body.HealthExports[0].Date != "2026-10-13" {
		t.Fatalf("list body=%s error=%v", listed.Body.String(), err)
	}

	invalid := httptest.NewRecorder()
	NewHealthExportHandler(&healthExportHandlerService{err: &model.ValidationError{Message: "date range must not exceed 366 days", Field: "to"}}).List(invalid, guidedHandlerRequest(t, http.MethodGet, "/?from=2025-01-01&to=2026-10-14", nil, uuid.New(), "", nil))
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid range status=%d", invalid.Code)
	}
}
