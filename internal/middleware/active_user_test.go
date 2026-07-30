package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type activeUserCheckerStub struct {
	exists bool
	err    error
	calls  int
	userID uuid.UUID
}

func (c *activeUserCheckerStub) Exists(_ context.Context, userID uuid.UUID) (bool, error) {
	c.calls++
	c.userID = userID
	return c.exists, c.err
}

func TestRequireActiveUser(t *testing.T) {
	userID := uuid.New()
	requestWithUser := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
		ctx := context.WithValue(req.Context(), UserIDKey, userID.String())
		return req.WithContext(ctx)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	t.Run("active auth user continues", func(t *testing.T) {
		checker := &activeUserCheckerStub{exists: true}
		rec := httptest.NewRecorder()

		RequireActiveUser(checker)(next).ServeHTTP(rec, requestWithUser())

		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
		}
		if checker.calls != 1 || checker.userID != userID {
			t.Fatalf("checker calls = %d user = %s, want 1 and %s", checker.calls, checker.userID, userID)
		}
	})

	t.Run("deleted auth user is rejected", func(t *testing.T) {
		checker := &activeUserCheckerStub{exists: false}
		rec := httptest.NewRecorder()

		RequireActiveUser(checker)(next).ServeHTTP(rec, requestWithUser())

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		if body := rec.Body.String(); !strings.Contains(body, `"code":"AUTHENTICATION_REQUIRED"`) {
			t.Fatalf("unexpected body: %s", body)
		}
	})

	t.Run("database failure is fail-closed and retryable", func(t *testing.T) {
		checker := &activeUserCheckerStub{err: errors.New("database unavailable")}
		rec := httptest.NewRecorder()

		RequireActiveUser(checker)(next).ServeHTTP(rec, requestWithUser())

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
		}
		if body := rec.Body.String(); !strings.Contains(body, `"code":"AUTHENTICATION_UNAVAILABLE"`) || strings.Contains(body, "database unavailable") {
			t.Fatalf("unexpected body: %s", body)
		}
	})

	t.Run("missing checker is unavailable", func(t *testing.T) {
		rec := httptest.NewRecorder()
		RequireActiveUser(nil)(next).ServeHTTP(rec, requestWithUser())
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
		}
	})

	t.Run("missing verified identity is unauthorized without a lookup", func(t *testing.T) {
		checker := &activeUserCheckerStub{exists: true}
		rec := httptest.NewRecorder()

		RequireActiveUser(checker)(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		if checker.calls != 0 {
			t.Fatalf("checker calls = %d, want 0", checker.calls)
		}
	})
}
