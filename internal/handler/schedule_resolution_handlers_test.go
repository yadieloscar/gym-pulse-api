package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type resolutionScheduleService struct {
	guidedScheduleService
	err error
}

func (s resolutionScheduleService) Skip(context.Context, uuid.UUID, uuid.UUID, model.RevisionRequest) (*model.ScheduledWorkout, error) {
	if s.err != nil {
		return nil, s.err
	}
	skipped := model.ResolutionSkipped
	return &model.ScheduledWorkout{Status: model.WorkoutStatusMissed, Resolution: &skipped, Revision: 2}, nil
}

func (s resolutionScheduleService) Unskip(context.Context, uuid.UUID, uuid.UUID, model.RevisionRequest) (*model.ScheduledWorkout, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &model.ScheduledWorkout{Status: model.WorkoutStatusMissed, Revision: 3}, nil
}

func TestSkipHandlerReturnsResolvedWorkout(t *testing.T) {
	userID := uuid.New()
	params := map[string]string{"id": uuid.NewString()}
	handler := NewScheduleHandler(resolutionScheduleService{})
	rec := httptest.NewRecorder()
	handler.Skip(rec, guidedHandlerRequest(t, http.MethodPost, "/", model.RevisionRequest{OperationKey: "skip-1", ExpectedRevision: 1}, userID, "skip-1", params))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["resolution"] != model.ResolutionSkipped {
		t.Fatalf("resolution = %v", body["resolution"])
	}

	rec = httptest.NewRecorder()
	handler.Unskip(rec, guidedHandlerRequest(t, http.MethodPost, "/", model.RevisionRequest{OperationKey: "unskip-1", ExpectedRevision: 2}, userID, "unskip-1", params))
	if rec.Code != http.StatusOK {
		t.Fatalf("unskip status = %d", rec.Code)
	}
	body = map[string]any{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, present := body["resolution"]; present {
		t.Fatalf("unresolved workout serialized resolution: %v", body)
	}
}

func TestSkipHandlerErrors(t *testing.T) {
	userID := uuid.New()
	params := map[string]string{"id": uuid.NewString()}
	recovered := model.ResolutionRecovered
	cases := []struct {
		name   string
		err    error
		header string
		want   int
		code   string
	}{
		{name: "key mismatch", header: "other", want: http.StatusUnprocessableEntity, code: "VALIDATION_ERROR"},
		{name: "not found", err: &model.NotFoundError{Message: "scheduled workout not found"}, header: "skip-1", want: http.StatusNotFound, code: "NOT_FOUND"},
		{name: "resolution", err: &model.ResolutionConflictError{Message: "workout cannot be skipped", Status: model.WorkoutStatusMissed, Resolution: &recovered}, header: "skip-1", want: http.StatusConflict, code: "RESOLUTION_CONFLICT"},
		{name: "revision", err: &model.ConflictError{Message: "scheduled workout revision conflict", Expected: 1, Actual: 2}, header: "skip-1", want: http.StatusConflict, code: "REVISION_CONFLICT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewScheduleHandler(resolutionScheduleService{err: tc.err})
			for _, invoke := range []func(http.ResponseWriter, *http.Request){handler.Skip, handler.Unskip} {
				rec := httptest.NewRecorder()
				invoke(rec, guidedHandlerRequest(t, http.MethodPost, "/", model.RevisionRequest{OperationKey: "skip-1", ExpectedRevision: 1}, userID, tc.header, params))
				if rec.Code != tc.want {
					t.Fatalf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
				}
				var body struct {
					Code string `json:"code"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Code != tc.code {
					t.Fatalf("code = %q, want %q (%v)", body.Code, tc.code, err)
				}
			}
		})
	}
}
