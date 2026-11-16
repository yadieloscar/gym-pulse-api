package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// RequestIDMiddleware gives every request a random identifier and returns it
// in X-Request-ID so a support report can be correlated with one structured
// server log entry.
//
// A client-supplied X-Request-ID is ignored: accepting it would let callers
// write arbitrary text into the logs and forge correlation with another
// user's request. The identifier carries no host name or sequence number.
// It is stored under chi's key, so chimiddleware.GetReqID still reads it.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := newRequestID()
		w.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), chimiddleware.RequestIDKey, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newRequestID() string {
	var b [16]byte
	// crypto/rand.Read never returns an error on supported platforms.
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
