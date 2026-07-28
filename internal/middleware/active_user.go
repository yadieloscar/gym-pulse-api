package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// ActiveUserChecker is the consuming-boundary contract for proving that a
// verified JWT subject still exists in the authoritative auth identity root.
type ActiveUserChecker interface {
	Exists(ctx context.Context, userID uuid.UUID) (bool, error)
}

const activeUserCheckTimeout = 2 * time.Second

func RequireActiveUser(checker ActiveUserChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := activeUserID(r.Context())
			if !ok {
				writeActiveUserError(w, http.StatusUnauthorized, "authentication required", "AUTHENTICATION_REQUIRED")
				return
			}
			if checker == nil {
				writeActiveUserError(w, http.StatusServiceUnavailable, "authentication service temporarily unavailable", "AUTHENTICATION_UNAVAILABLE")
				return
			}

			checkContext, cancel := context.WithTimeout(r.Context(), activeUserCheckTimeout)
			exists, err := checker.Exists(checkContext, userID)
			cancel()
			if err != nil {
				writeActiveUserError(w, http.StatusServiceUnavailable, "authentication service temporarily unavailable", "AUTHENTICATION_UNAVAILABLE")
				return
			}
			if !exists {
				writeActiveUserError(w, http.StatusUnauthorized, "authentication required", "AUTHENTICATION_REQUIRED")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func activeUserID(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value(UserIDKey).(string)
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(userID)
	return id, err == nil
}

func writeActiveUserError(w http.ResponseWriter, status int, message, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message, "code": code})
}
