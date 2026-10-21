package middleware

import (
	"net/http"
	"strings"
)

// SecurityHeaders sets conservative response headers for a JSON API that is
// consumed by native mobile clients and, in development, by the Swagger UI.
//
// Authenticated API responses carry user data, so they are marked no-store to
// keep shared caches and proxies from retaining them. nosniff and DENY cost
// nothing and close the browser-side vectors that remain if a response is ever
// rendered in a web view.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
