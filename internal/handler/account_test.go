package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

func TestAccountHandler_Delete(t *testing.T) {
	uid := uuid.New()

	t.Run("success -> 204 with the caller's user id", func(t *testing.T) {
		var got uuid.UUID
		svc := &MockAccountService{
			DeleteFunc: func(ctx context.Context, u uuid.UUID) error {
				got = u
				return nil
			},
		}
		h := NewAccountHandler(svc)
		rec := httptest.NewRecorder()
		h.Delete(rec, newReq(t, "DELETE", "/", nil, uid))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if got != uid {
			t.Errorf("deleted %s, expected %s", got, uid)
		}
	})

	t.Run("service error -> 500", func(t *testing.T) {
		svc := &MockAccountService{
			DeleteFunc: func(ctx context.Context, u uuid.UUID) error {
				return errors.New("boom")
			},
		}
		h := NewAccountHandler(svc)
		rec := httptest.NewRecorder()
		h.Delete(rec, newReq(t, "DELETE", "/", nil, uid))
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d", rec.Code)
		}
	})

	t.Run("incomplete deletion -> stable retryable 503", func(t *testing.T) {
		svc := &MockAccountService{
			DeleteFunc: func(ctx context.Context, u uuid.UUID) error {
				return service.ErrAccountDeletionIncomplete
			},
		}
		h := NewAccountHandler(svc)
		rec := httptest.NewRecorder()
		h.Delete(rec, newReq(t, "DELETE", "/", nil, uid))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if body := rec.Body.String(); !strings.Contains(body, `"code":"ACCOUNT_DELETION_INCOMPLETE"`) || strings.Contains(body, "supabase") {
			t.Fatalf("unexpected public error body: %s", body)
		}
	})
}
