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

type sportHandlerService struct {
	activities []model.SportActivity
	err        error
	created    model.CreateSportActivityRequest
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
func (s *sportHandlerService) Create(_ context.Context, _ uuid.UUID, req model.CreateSportActivityRequest) (*model.SportActivity, error) {
	s.created = req
	if s.err != nil {
		return nil, s.err
	}
	return &s.activities[0], nil
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
