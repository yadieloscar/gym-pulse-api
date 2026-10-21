package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestRateLimitPerUser(t *testing.T) {
	limited := RateLimitPerUser(2)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	do := func(userID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/schedule/materialize", nil)
		req = req.WithContext(context.WithValue(req.Context(), UserIDKey, userID))
		rec := httptest.NewRecorder()
		limited.ServeHTTP(rec, req)
		return rec
	}

	alice, bob := uuid.NewString(), uuid.NewString()
	for i := range 2 {
		if rec := do(alice); rec.Code != http.StatusNoContent {
			t.Fatalf("request %d for alice = %d, want 204", i+1, rec.Code)
		}
	}

	rec := do(alice)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third request for alice = %d, want 429", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "RATE_LIMITED" {
		t.Fatalf("code = %q, want RATE_LIMITED", body["code"])
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("content type = %q, want application/json", rec.Header().Get("Content-Type"))
	}

	// Buckets are per subject: another user is unaffected by alice's burst.
	if rec := do(bob); rec.Code != http.StatusNoContent {
		t.Fatalf("first request for bob = %d, want 204", rec.Code)
	}
}
