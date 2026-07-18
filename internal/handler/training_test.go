package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func TestTrainingMutationRequiresMatchingIdempotencyKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	if matchIdempotencyKey(rec, req, "operation-1") {
		t.Fatal("missing Idempotency-Key was accepted")
	}
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d want 422", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Idempotency-Key", "operation-1")
	rec = httptest.NewRecorder()
	if !matchIdempotencyKey(rec, req, "operation-1") {
		t.Fatal("matching Idempotency-Key was rejected")
	}
}

func TestTrainingRevisionConflictIncludesAuthoritativeResource(t *testing.T) {
	rec := httptest.NewRecorder()
	handleServiceError(rec, &model.ConflictError{
		Message: "scheduled workout revision conflict", Expected: 2, Actual: 3,
		Authoritative: &model.ScheduledWorkout{Revision: 3},
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d want 409", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"REVISION_CONFLICT", "expected_revision", "actual_revision", "resource"} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q: %s", want, body)
		}
	}
}
