package middleware

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/httprate"
)

// Per-user rate limits for the endpoints whose cost is dominated by external
// storage calls, advisory locks, or multi-row schedule rewrites. Limits are
// keyed by the authenticated subject, never by client IP: the API runs behind
// a TLS-terminating proxy, so RemoteAddr is shared by every user and an IP
// bucket would let one abusive client exhaust everyone else's quota.
const (
	// StorageBoundLimit bounds avatar uploads and account deletion per minute.
	StorageBoundLimit = 10
	// ScheduleRewriteLimit bounds schedule materialization, regeneration,
	// recovery, and plan transitions per minute.
	ScheduleRewriteLimit = 30
	rateLimitWindow      = time.Minute
)

// RateLimitPerUser returns a middleware that allows requestLimit requests per
// minute for each authenticated user. It must run inside the auth group so
// the user id is present; requests without one share a single bucket, which
// only ever happens when the middleware is misplaced.
func RateLimitPerUser(requestLimit int) func(http.Handler) http.Handler {
	return httprate.LimitBy(
		requestLimit,
		rateLimitWindow,
		func(r *http.Request) (string, error) {
			userID, _ := r.Context().Value(UserIDKey).(string)
			return userID, nil
		},
		httprate.WithLimitHandler(writeRateLimitError),
	)
}

func writeRateLimitError(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": "too many requests, retry shortly",
		"code":  "RATE_LIMITED",
	})
}
