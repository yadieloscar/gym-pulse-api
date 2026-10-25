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

type sportHandlerService struct {
	activities []model.SportActivity
	err        error
	created    model.CreateSportActivityRequest
	outcome    model.SportActivityCreateOutcome
}

func (s *sportHandlerService) List(context.Context, uuid.UUID, string, string) ([]model.SportActivity, error) {
	return s.activities, s.err
}
func (s *sportHandlerService) Get(context.Context, uuid.UUID, uuid.UUID) (*model.SportActivity, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &s.activities[0], nil
}
func (s *sportHandlerService) Create(_ context.Context, _ uuid.UUID, req model.CreateSportActivityRequest) (*model.SportActivity, model.SportActivityCreateOutcome, error) {
	s.created = req
	if s.err != nil {
		return nil, model.SportActivityCreated, s.err
	}
	return &s.activities[0], s.outcome, nil
}

func TestSportActivityHandlerSuccess(t *testing.T) {
	activity := model.SportActivity{ID: uuid.New(), Date: "2026-08-02", SportID: "basketball", SportName: "Basketball", DurationMinutes: 60}
	svc := &sportHandlerService{activities: []model.SportActivity{activity}}
	handler := NewSportActivityHandler(svc)
	userID := uuid.New()

	list := httptest.NewRecorder()
	handler.List(list, guidedHandlerRequest(t, http.MethodGet, "/?from=2026-08-01&to=2026-08-02", nil, userID, "", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	var listed []model.SportActivity
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil || len(listed) != 1 {
		t.Fatalf("list body=%s error=%v", list.Body.String(), err)
	}

	get := httptest.NewRecorder()
	handler.Get(get, guidedHandlerRequest(t, http.MethodGet, "/", nil, userID, "", map[string]string{"id": activity.ID.String()}))
	if get.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", get.Code, get.Body.String())
	}

	create := httptest.NewRecorder()
	req := model.CreateSportActivityRequest{SportID: "basketball", SportName: "Basketball", DurationMinutes: 60, OperationKey: "sport-op"}
	handler.Create(create, guidedHandlerRequest(t, http.MethodPost, "/", req, userID, "sport-op", nil))
	if create.Code != http.StatusCreated || svc.created.OperationKey != "sport-op" {
		t.Fatalf("create status=%d body=%s request=%+v", create.Code, create.Body.String(), svc.created)
	}
}

func TestSportActivityHandlerErrors(t *testing.T) {
	activity := model.SportActivity{ID: uuid.New()}
	svc := &sportHandlerService{activities: []model.SportActivity{activity}, err: &model.NotFoundError{Message: "sport activity not found"}}
	handler := NewSportActivityHandler(svc)
	userID := uuid.New()

	missing := httptest.NewRecorder()
	handler.Get(missing, guidedHandlerRequest(t, http.MethodGet, "/", nil, userID, "", map[string]string{"id": uuid.NewString()}))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("not found status=%d body=%s", missing.Code, missing.Body.String())
	}

	badID := httptest.NewRecorder()
	handler.Get(badID, guidedHandlerRequest(t, http.MethodGet, "/", nil, userID, "", map[string]string{"id": "bad"}))
	if badID.Code != http.StatusBadRequest {
		t.Fatalf("bad id status=%d", badID.Code)
	}

	badKey := httptest.NewRecorder()
	req := model.CreateSportActivityRequest{OperationKey: "body-key"}
	handler.Create(badKey, guidedHandlerRequest(t, http.MethodPost, "/", req, userID, "header-key", nil))
	if badKey.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad key status=%d body=%s", badKey.Code, badKey.Body.String())
	}

	badBody := httptest.NewRecorder()
	r := guidedHandlerRequest(t, http.MethodPost, "/", req, userID, "body-key", nil)
	r.Body = http.NoBody
	handler.Create(badBody, r)
	if badBody.Code != http.StatusBadRequest {
		t.Fatalf("bad body status=%d body=%s", badBody.Code, badBody.Body.String())
	}
}

func TestSportActivityHandlerCreateStatusFollowsOutcome(t *testing.T) {
	external := "9B3C2E1A-6F0D-4A51-9C7E-2B1F0D8E4A77"
	meters := 8120
	activity := model.SportActivity{ID: uuid.New(), Date: "2026-10-11", SportID: "running", SportName: "Running", DurationMinutes: 42,
		Source: model.ActivitySourceAppleHealth, ExternalID: &external, DistanceMeters: &meters}
	req := model.CreateSportActivityRequest{SportID: "running", SportName: "Running", DurationMinutes: 42, OperationKey: "hk:9B3C",
		Source: model.ActivitySourceAppleHealth, ExternalID: &external, DistanceMeters: &meters}
	userID := uuid.New()

	for _, test := range []struct {
		name    string
		outcome model.SportActivityCreateOutcome
		status  int
	}{
		{"created", model.SportActivityCreated, http.StatusCreated},
		{"replayed", model.SportActivityReplayed, http.StatusCreated},
		{"existing import", model.SportActivityExisting, http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc := &sportHandlerService{activities: []model.SportActivity{activity}, outcome: test.outcome}
			rec := httptest.NewRecorder()
			NewSportActivityHandler(svc).Create(rec, guidedHandlerRequest(t, http.MethodPost, "/", req, userID, "hk:9B3C", nil))
			if rec.Code != test.status {
				t.Fatalf("status=%d want %d body=%s", rec.Code, test.status, rec.Body.String())
			}
			var body model.SportActivity
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.ID != activity.ID || body.Source != model.ActivitySourceAppleHealth || body.ExternalID == nil || body.DistanceMeters == nil {
				t.Fatalf("body lost import fields: %s", rec.Body.String())
			}
			if svc.created.Source != model.ActivitySourceAppleHealth || svc.created.ExternalID == nil {
				t.Fatalf("request fields not decoded: %+v", svc.created)
			}
		})
	}
}

func TestSportActivityHandlerListOmitsImportFieldsForManualRows(t *testing.T) {
	manual := model.SportActivity{ID: uuid.New(), Date: "2026-08-02", SportID: "basketball", SportName: "Basketball", DurationMinutes: 60, Source: model.ActivitySourceManual}
	svc := &sportHandlerService{activities: []model.SportActivity{manual}}
	rec := httptest.NewRecorder()
	NewSportActivityHandler(svc).List(rec, guidedHandlerRequest(t, http.MethodGet, "/?from=2026-08-01&to=2026-08-02", nil, uuid.New(), "", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"source":"manual"`) || strings.Contains(body, "external_id") || strings.Contains(body, "distance_meters") {
		t.Fatalf("manual row shape wrong: %s", body)
	}
}

func TestSportActivityHandlerValidationFieldSurfaces(t *testing.T) {
	svc := &sportHandlerService{err: &model.ValidationError{Message: "external_id is required for imported activities", Field: "external_id"}}
	rec := httptest.NewRecorder()
	req := model.CreateSportActivityRequest{SportID: "running", SportName: "Running", DurationMinutes: 42, OperationKey: "op", Source: model.ActivitySourceAppleHealth}
	NewSportActivityHandler(svc).Create(rec, guidedHandlerRequest(t, http.MethodPost, "/", req, uuid.New(), "op", nil))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"field":"external_id"`) || !strings.Contains(rec.Body.String(), "VALIDATION_ERROR") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSportSessionConflictIncludesStatusAndResource(t *testing.T) {
	rec := httptest.NewRecorder()
	handleServiceError(rec, &model.SportSessionConflictError{
		Message: "sport session is already completed", Status: model.WorkoutStatusCompleted,
		Authoritative: &model.ScheduledWorkout{Kind: model.UnitKindSport, Status: model.WorkoutStatusCompleted, Revision: 2},
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d want 409", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`"code":"SPORT_SESSION_CONFLICT"`, `"status":"completed"`, `"resource"`, `"kind":"sport"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q: %s", want, body)
		}
	}
}

func TestSportActivityHandlerCreatePassesAndReturnsScheduledWorkoutLink(t *testing.T) {
	sessionID := uuid.New()
	activity := model.SportActivity{ID: uuid.New(), Date: "2026-10-31", SportID: "cycling", SportName: "Cycling", DurationMinutes: 95, Source: model.ActivitySourceManual, ScheduledWorkoutID: &sessionID}
	svc := &sportHandlerService{activities: []model.SportActivity{activity}}
	handler := NewSportActivityHandler(svc)
	req := model.CreateSportActivityRequest{SportID: "cycling", SportName: "Cycling", DurationMinutes: 95, OperationKey: "ride-op", ScheduledWorkoutID: &sessionID}
	rec := httptest.NewRecorder()
	handler.Create(rec, guidedHandlerRequest(t, http.MethodPost, "/", req, uuid.New(), "ride-op", nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if svc.created.ScheduledWorkoutID == nil || *svc.created.ScheduledWorkoutID != sessionID {
		t.Fatalf("handler dropped scheduled_workout_id: %+v", svc.created)
	}
	if !strings.Contains(rec.Body.String(), `"scheduled_workout_id":"`+sessionID.String()+`"`) {
		t.Fatalf("response must carry the link: %s", rec.Body.String())
	}
	for _, failure := range []struct {
		err    error
		status int
		want   string
	}{
		{&model.NotFoundError{Message: "scheduled workout not found"}, http.StatusNotFound, `"NOT_FOUND"`},
		{&model.ValidationError{Message: "must reference a sport session", Field: "scheduled_workout_id"}, http.StatusUnprocessableEntity, `"field":"scheduled_workout_id"`},
		{&model.ValidationError{Message: "sport mismatch", Field: "sport_id"}, http.StatusUnprocessableEntity, `"field":"sport_id"`},
	} {
		svc.err = failure.err
		rec := httptest.NewRecorder()
		handler.Create(rec, guidedHandlerRequest(t, http.MethodPost, "/", req, uuid.New(), "ride-op", nil))
		if rec.Code != failure.status || !strings.Contains(rec.Body.String(), failure.want) {
			t.Fatalf("err=%v status=%d body=%s", failure.err, rec.Code, rec.Body.String())
		}
	}
}
