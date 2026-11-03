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

type mobilityHandlerService struct {
	sessions []model.MobilitySession
	err      error
	created  model.CreateMobilitySessionRequest
	outcome  model.MobilitySessionCreateOutcome
	from, to string
	deleted  uuid.UUID
}

func (s *mobilityHandlerService) List(_ context.Context, _ uuid.UUID, from, to string) ([]model.MobilitySession, error) {
	s.from, s.to = from, to
	return s.sessions, s.err
}

func (s *mobilityHandlerService) Get(context.Context, uuid.UUID, uuid.UUID) (*model.MobilitySession, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &s.sessions[0], nil
}

func (s *mobilityHandlerService) Create(_ context.Context, _ uuid.UUID, req model.CreateMobilitySessionRequest) (*model.MobilitySession, model.MobilitySessionCreateOutcome, error) {
	s.created = req
	if s.err != nil {
		return nil, model.MobilitySessionCreated, s.err
	}
	return &s.sessions[0], s.outcome, nil
}

func (s *mobilityHandlerService) Delete(_ context.Context, _ uuid.UUID, id uuid.UUID) error {
	s.deleted = id
	return s.err
}

func mobilityEntry() model.MobilitySession {
	return model.MobilitySession{ID: uuid.New(), Date: "2026-11-03", RoutineID: "hips-and-hamstrings", RoutineName: "Hips and hamstrings",
		Context: model.MobilityContextCooldown, DurationSeconds: 540, MovesCompleted: 7, MovesTotal: 8, ContentVersion: "1", OperationKey: "mob-op"}
}

func TestMobilitySessionHandlerCreateStatusFollowsOutcome(t *testing.T) {
	entry := mobilityEntry()
	req := model.CreateMobilitySessionRequest{RoutineID: "hips-and-hamstrings", RoutineName: "Hips and hamstrings", Context: model.MobilityContextCooldown,
		DurationSeconds: 540, MovesCompleted: 7, MovesTotal: 8, ContentVersion: "1", OperationKey: "mob-op"}
	for _, test := range []struct {
		name    string
		outcome model.MobilitySessionCreateOutcome
		status  int
	}{
		{"created", model.MobilitySessionCreated, http.StatusCreated},
		{"replayed", model.MobilitySessionReplayed, http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc := &mobilityHandlerService{sessions: []model.MobilitySession{entry}, outcome: test.outcome}
			rec := httptest.NewRecorder()
			NewMobilitySessionHandler(svc).Create(rec, guidedHandlerRequest(t, http.MethodPost, "/", req, uuid.New(), "mob-op", nil))
			if rec.Code != test.status {
				t.Fatalf("status=%d want %d body=%s", rec.Code, test.status, rec.Body.String())
			}
			if svc.created != req {
				t.Fatalf("request not decoded: %+v", svc.created)
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"id", "date", "routine_id", "routine_name", "context", "duration_seconds", "moves_completed", "moves_total", "content_version", "operation_key", "created_at"} {
				if _, ok := body[key]; !ok {
					t.Errorf("response missing %q: %s", key, rec.Body.String())
				}
			}
			if _, ok := body["request_hash"]; ok {
				t.Fatal("request_hash must not be exposed")
			}
		})
	}
}

func TestMobilitySessionHandlerCreateErrors(t *testing.T) {
	userID := uuid.New()
	req := model.CreateMobilitySessionRequest{OperationKey: "body-key"}

	mismatch := httptest.NewRecorder()
	svc := &mobilityHandlerService{}
	NewMobilitySessionHandler(svc).Create(mismatch, guidedHandlerRequest(t, http.MethodPost, "/", req, userID, "header-key", nil))
	if mismatch.Code != http.StatusUnprocessableEntity || !strings.Contains(mismatch.Body.String(), `"field":"Idempotency-Key"`) {
		t.Fatalf("mismatch status=%d body=%s", mismatch.Code, mismatch.Body.String())
	}
	missing := httptest.NewRecorder()
	NewMobilitySessionHandler(svc).Create(missing, guidedHandlerRequest(t, http.MethodPost, "/", req, userID, "", nil))
	if missing.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing header status=%d", missing.Code)
	}

	badBody := httptest.NewRecorder()
	r := guidedHandlerRequest(t, http.MethodPost, "/", req, userID, "body-key", nil)
	r.Body = http.NoBody
	NewMobilitySessionHandler(svc).Create(badBody, r)
	if badBody.Code != http.StatusBadRequest {
		t.Fatalf("bad body status=%d", badBody.Code)
	}

	for _, failure := range []struct {
		err    error
		status int
		want   string
	}{
		{&model.ValidationError{Message: "moves_completed must not exceed moves_total", Field: "moves_completed"}, http.StatusUnprocessableEntity, `"field":"moves_completed"`},
		{&model.ConflictError{Message: "idempotency key was already used with a different payload"}, http.StatusConflict, `"code":"IDEMPOTENCY_CONFLICT"`},
		{&model.NotFoundError{Message: "training profile not found"}, http.StatusNotFound, `"NOT_FOUND"`},
	} {
		rec := httptest.NewRecorder()
		NewMobilitySessionHandler(&mobilityHandlerService{err: failure.err}).Create(rec, guidedHandlerRequest(t, http.MethodPost, "/", req, userID, "body-key", nil))
		if rec.Code != failure.status || !strings.Contains(rec.Body.String(), failure.want) {
			t.Fatalf("err=%v status=%d body=%s", failure.err, rec.Code, rec.Body.String())
		}
	}
}

func TestMobilitySessionHandlerListGetDelete(t *testing.T) {
	entry := mobilityEntry()
	svc := &mobilityHandlerService{sessions: []model.MobilitySession{entry}}
	handler := NewMobilitySessionHandler(svc)
	userID := uuid.New()
	params := map[string]string{"id": entry.ID.String()}

	list := httptest.NewRecorder()
	handler.List(list, guidedHandlerRequest(t, http.MethodGet, "/?from=2026-11-01&to=2026-11-03", nil, userID, "", nil))
	if list.Code != http.StatusOK || svc.from != "2026-11-01" || svc.to != "2026-11-03" || !strings.HasPrefix(list.Body.String(), "[") {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}

	empty := httptest.NewRecorder()
	NewMobilitySessionHandler(&mobilityHandlerService{sessions: []model.MobilitySession{}}).List(empty, guidedHandlerRequest(t, http.MethodGet, "/?from=2026-11-01&to=2026-11-03", nil, userID, "", nil))
	if strings.TrimSpace(empty.Body.String()) != "[]" {
		t.Fatalf("empty list must be [] not %s", empty.Body.String())
	}

	get := httptest.NewRecorder()
	handler.Get(get, guidedHandlerRequest(t, http.MethodGet, "/", nil, userID, "", params))
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), entry.ID.String()) {
		t.Fatalf("get status=%d body=%s", get.Code, get.Body.String())
	}

	del := httptest.NewRecorder()
	handler.Delete(del, guidedHandlerRequest(t, http.MethodDelete, "/", nil, userID, "", params))
	if del.Code != http.StatusNoContent || svc.deleted != entry.ID || del.Body.Len() != 0 {
		t.Fatalf("delete status=%d body=%s", del.Code, del.Body.String())
	}

	for _, invoke := range []func(http.ResponseWriter, *http.Request){handler.Get, handler.Delete} {
		bad := httptest.NewRecorder()
		invoke(bad, guidedHandlerRequest(t, http.MethodGet, "/", nil, userID, "", map[string]string{"id": "bad"}))
		if bad.Code != http.StatusBadRequest {
			t.Fatalf("bad id status=%d", bad.Code)
		}
	}

	notFound := &mobilityHandlerService{err: &model.NotFoundError{Message: "mobility session not found"}}
	for _, invoke := range []func(http.ResponseWriter, *http.Request){NewMobilitySessionHandler(notFound).Get, NewMobilitySessionHandler(notFound).Delete} {
		rec := httptest.NewRecorder()
		invoke(rec, guidedHandlerRequest(t, http.MethodGet, "/", nil, userID, "", params))
		if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "NOT_FOUND") {
			t.Fatalf("not found status=%d body=%s", rec.Code, rec.Body.String())
		}
	}

	badRange := httptest.NewRecorder()
	NewMobilitySessionHandler(&mobilityHandlerService{err: &model.ValidationError{Message: "date range must not exceed 366 days", Field: "to"}}).List(badRange, guidedHandlerRequest(t, http.MethodGet, "/?from=2025-01-01&to=2026-11-03", nil, userID, "", nil))
	if badRange.Code != http.StatusUnprocessableEntity {
		t.Fatalf("range status=%d", badRange.Code)
	}
}
