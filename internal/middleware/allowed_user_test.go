package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestRequireAllowedUser(t *testing.T) {
	owner := uuid.New()
	stranger := uuid.New()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	requestAs := func(userID string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
		if userID == "" {
			return req
		}
		return req.WithContext(context.WithValue(req.Context(), UserIDKey, userID))
	}

	tests := []struct {
		name     string
		allowed  []uuid.UUID
		userID   string
		wantCode int
		wantBody string
	}{
		{name: "empty list lets every verified user through", allowed: nil, userID: stranger.String(), wantCode: http.StatusNoContent},
		{name: "listed user continues", allowed: []uuid.UUID{owner}, userID: owner.String(), wantCode: http.StatusNoContent},
		{name: "unlisted user is forbidden", allowed: []uuid.UUID{owner}, userID: stranger.String(), wantCode: http.StatusForbidden, wantBody: "ACCOUNT_NOT_ALLOWED"},
		{name: "missing subject is unauthenticated", allowed: []uuid.UUID{owner}, userID: "", wantCode: http.StatusUnauthorized, wantBody: "AUTHENTICATION_REQUIRED"},
		{name: "malformed subject is unauthenticated", allowed: []uuid.UUID{owner}, userID: "not-a-uuid", wantCode: http.StatusUnauthorized, wantBody: "AUTHENTICATION_REQUIRED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			RequireAllowedUser(tt.allowed)(next).ServeHTTP(rec, requestAs(tt.userID))

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if tt.wantBody == "" {
				return
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["code"] != tt.wantBody {
				t.Fatalf("code = %q, want %q", body["code"], tt.wantBody)
			}
		})
	}
}
