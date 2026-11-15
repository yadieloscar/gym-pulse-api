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

type kneeKitHandlerService struct {
	day      *model.KneeKitDay
	outcome  model.KneeKitDayOutcome
	listed   *model.KneeKitRange
	err      error
	date     string
	saved    model.SaveKneeKitDayRequest
	userID   uuid.UUID
	from, to string
}

func (s *kneeKitHandlerService) SaveDay(_ context.Context, userID uuid.UUID, date string, req model.SaveKneeKitDayRequest) (*model.KneeKitDay, model.KneeKitDayOutcome, error) {
	s.userID, s.date, s.saved = userID, date, req
	if s.err != nil {
		return nil, model.KneeKitDayCreated, s.err
	}
	return s.day, s.outcome, nil
}

func (s *kneeKitHandlerService) ListDays(_ context.Context, userID uuid.UUID, from, to string) (*model.KneeKitRange, error) {
	s.userID, s.from, s.to = userID, from, to
	return s.listed, s.err
}

func kneeKitDayRequest(t *testing.T, body any, key string, userID uuid.UUID) *http.Request {
	t.Helper()
	return guidedHandlerRequest(t, http.MethodPut, "/", body, userID, key, map[string]string{"date": "2026-11-14"})
}

const kneeKitWireBody = `{"completed_items":["morning-check","isometric-hold"],"morning_pain":3,"activity_pain":null,"sharp_pain":false,"limping":false,"expected_revision":0,"operation_key":"knee-day-1"}`

func TestKneeKitHandlerSaveDayStatusFollowsOutcome(t *testing.T) {
	pain := 3
	day := &model.KneeKitDay{ID: uuid.New(), Date: "2026-11-14", CompletedItems: []string{"isometric-hold", "morning-check"}, MorningPain: &pain, Revision: 1}
	for _, test := range []struct {
		name    string
		outcome model.KneeKitDayOutcome
		status  int
	}{
		{"created", model.KneeKitDayCreated, http.StatusCreated},
		{"replaced", model.KneeKitDayReplaced, http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			userID := uuid.New()
			svc := &kneeKitHandlerService{day: day, outcome: test.outcome}
			rec := httptest.NewRecorder()
			NewKneeKitHandler(svc).SaveDay(rec, kneeKitDayRequest(t, kneeKitWireBody, "knee-day-1", userID))
			if rec.Code != test.status {
				t.Fatalf("status=%d want %d body=%s", rec.Code, test.status, rec.Body.String())
			}
			if svc.userID != userID || svc.date != "2026-11-14" || len(svc.saved.CompletedItems) != 2 || svc.saved.MorningPain == nil || *svc.saved.MorningPain != 3 ||
				svc.saved.ActivityPain != nil || svc.saved.ExpectedRevision == nil || *svc.saved.ExpectedRevision != 0 {
				t.Fatalf("identity, date, or body not routed: user=%s date=%s req=%+v", svc.userID, svc.date, svc.saved)
			}
			var got model.KneeKitDay
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.ID != day.ID || got.Revision != 1 {
				t.Fatalf("body=%s err=%v", rec.Body.String(), err)
			}
			for _, present := range []string{`"completed_items":["isometric-hold","morning-check"]`, `"morning_pain":3`, `"activity_pain":null`, `"sharp_pain":false`, `"limping":false`} {
				if !strings.Contains(rec.Body.String(), present) {
					t.Errorf("response missing %s: %s", present, rec.Body.String())
				}
			}
		})
	}
}

func TestKneeKitHandlerSaveDayErrors(t *testing.T) {
	stored := &model.KneeKitDay{ID: uuid.New(), Date: "2026-11-14", CompletedItems: []string{}, Revision: 3}
	save := func(svc *kneeKitHandlerService, body any, key string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		NewKneeKitHandler(svc).SaveDay(rec, kneeKitDayRequest(t, body, key, uuid.New()))
		return rec
	}

	stale := save(&kneeKitHandlerService{err: &model.ConflictError{Message: "resource revision conflict", Expected: 0, Actual: 3, Authoritative: stored}}, kneeKitWireBody, "knee-day-1")
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), `"code":"REVISION_CONFLICT"`) ||
		!strings.Contains(stale.Body.String(), `"actual_revision":3`) || !strings.Contains(stale.Body.String(), `"resource":{`) || !strings.Contains(stale.Body.String(), `"revision":3`) {
		t.Fatalf("stale status=%d body=%s", stale.Code, stale.Body.String())
	}

	gone := save(&kneeKitHandlerService{err: &model.ConflictError{Message: "resource revision conflict", Expected: 2, Actual: 0}}, kneeKitWireBody, "knee-day-1")
	if gone.Code != http.StatusConflict || !strings.Contains(gone.Body.String(), `"code":"REVISION_CONFLICT"`) || !strings.Contains(gone.Body.String(), `"actual_revision":0`) || strings.Contains(gone.Body.String(), `"resource"`) {
		t.Fatalf("missing-row conflict status=%d body=%s", gone.Code, gone.Body.String())
	}

	reused := save(&kneeKitHandlerService{err: &model.ConflictError{Message: "idempotency key was already used with a different payload"}}, kneeKitWireBody, "knee-day-1")
	if reused.Code != http.StatusConflict || !strings.Contains(reused.Body.String(), `"code":"IDEMPOTENCY_CONFLICT"`) {
		t.Fatalf("idempotency status=%d body=%s", reused.Code, reused.Body.String())
	}

	invalid := save(&kneeKitHandlerService{err: &model.ValidationError{Message: "completed_items must not repeat an entry", Field: "completed_items"}}, kneeKitWireBody, "knee-day-1")
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), `"code":"VALIDATION_ERROR"`) || !strings.Contains(invalid.Body.String(), `"field":"completed_items"`) {
		t.Fatalf("invalid status=%d body=%s", invalid.Code, invalid.Body.String())
	}

	noProfile := save(&kneeKitHandlerService{err: &model.NotFoundError{Message: "training profile not found"}}, kneeKitWireBody, "knee-day-1")
	if noProfile.Code != http.StatusNotFound || !strings.Contains(noProfile.Body.String(), `"code":"NOT_FOUND"`) {
		t.Fatalf("no profile status=%d body=%s", noProfile.Code, noProfile.Body.String())
	}

	// The header must equal operation_key; the service is never reached.
	for _, key := range []string{"other-key", ""} {
		svc := &kneeKitHandlerService{}
		badKey := save(svc, kneeKitWireBody, key)
		if badKey.Code != http.StatusUnprocessableEntity || !strings.Contains(badKey.Body.String(), `"field":"Idempotency-Key"`) || svc.date != "" {
			t.Fatalf("key %q status=%d body=%s", key, badKey.Code, badKey.Body.String())
		}
	}

	for name, body := range map[string]string{
		"truncated":        `{"completed_items":`,
		"fractional pain":  `{"completed_items":[],"morning_pain":3.5,"expected_revision":0,"operation_key":"knee-day-1"}`,
		"items not array":  `{"completed_items":"morning-check","expected_revision":0,"operation_key":"knee-day-1"}`,
		"flag not boolean": `{"completed_items":[],"sharp_pain":"no","expected_revision":0,"operation_key":"knee-day-1"}`,
	} {
		svc := &kneeKitHandlerService{}
		if rec := save(svc, body, "knee-day-1"); rec.Code != http.StatusBadRequest || svc.date != "" {
			t.Errorf("%s: status=%d body=%s", name, rec.Code, rec.Body.String())
		}
	}
}

func TestKneeKitHandlerListDays(t *testing.T) {
	userID := uuid.New()
	svc := &kneeKitHandlerService{listed: &model.KneeKitRange{Days: []model.KneeKitDay{}}}
	empty := httptest.NewRecorder()
	NewKneeKitHandler(svc).ListDays(empty, guidedHandlerRequest(t, http.MethodGet, "/?from=2026-11-01&to=2026-11-14", nil, userID, "", nil))
	if empty.Code != http.StatusOK || strings.TrimSpace(empty.Body.String()) != `{"days":[]}` || svc.from != "2026-11-01" || svc.to != "2026-11-14" || svc.userID != userID {
		t.Fatalf("empty status=%d body=%s from=%s to=%s", empty.Code, empty.Body.String(), svc.from, svc.to)
	}

	pain := 2
	listed := httptest.NewRecorder()
	NewKneeKitHandler(&kneeKitHandlerService{listed: &model.KneeKitRange{Days: []model.KneeKitDay{
		{ID: uuid.New(), Date: "2026-11-13", CompletedItems: []string{}, Revision: 2},
		{ID: uuid.New(), Date: "2026-11-14", CompletedItems: []string{"wall-sit"}, ActivityPain: &pain, Limping: true, Revision: 1},
	}}}).ListDays(listed, guidedHandlerRequest(t, http.MethodGet, "/?from=2026-11-01&to=2026-11-14", nil, uuid.New(), "", nil))
	var body model.KneeKitRange
	if err := json.Unmarshal(listed.Body.Bytes(), &body); err != nil || len(body.Days) != 2 || body.Days[0].Date != "2026-11-13" || body.Days[1].ActivityPain == nil || !body.Days[1].Limping {
		t.Fatalf("list body=%s err=%v", listed.Body.String(), err)
	}
	if !strings.Contains(listed.Body.String(), `"completed_items":[]`) || !strings.Contains(listed.Body.String(), `"morning_pain":null`) {
		t.Fatalf("empty items and absent pain must be [] and null: %s", listed.Body.String())
	}

	invalid := httptest.NewRecorder()
	NewKneeKitHandler(&kneeKitHandlerService{err: &model.ValidationError{Message: "date range must not exceed 366 days", Field: "to"}}).ListDays(invalid, guidedHandlerRequest(t, http.MethodGet, "/?from=2025-01-01&to=2026-11-14", nil, uuid.New(), "", nil))
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), `"field":"to"`) {
		t.Fatalf("invalid status=%d body=%s", invalid.Code, invalid.Body.String())
	}
}
