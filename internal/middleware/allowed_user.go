package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

// RequireAllowedUser limits a route group to the given user ids. An empty
// list returns a pass-through so deployments without ALLOWED_USER_IDS keep
// today's behaviour. It must run after the auth middleware, which puts the
// verified subject in the request context.
func RequireAllowedUser(allowed []uuid.UUID) func(http.Handler) http.Handler {
	if len(allowed) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	set := make(map[uuid.UUID]struct{}, len(allowed))
	for _, id := range allowed {
		set[id] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := activeUserID(r.Context())
			if !ok {
				writeActiveUserError(w, http.StatusUnauthorized, "authentication required", "AUTHENTICATION_REQUIRED")
				return
			}
			if _, allowed := set[userID]; !allowed {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "this account does not have access", "code": "ACCOUNT_NOT_ALLOWED"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
