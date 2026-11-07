package middleware

import (
	"net/http"
	"strings"
)

// strictTransportSecurity pins clients to HTTPS for a year. Release
// environments terminate TLS at the platform edge, so the header is only
// meaningful there; development and test serve plain HTTP and must not set it
// or a browser would refuse the local API until the policy expires.
const strictTransportSecurity = "max-age=31536000; includeSubDomains"

// SecurityHeaders sets conservative response headers for a JSON API that is
// consumed by native mobile clients and, in development, by the Swagger UI.
//
// Authenticated API responses carry user data, so they are marked no-store to
// keep shared caches and proxies from retaining them. nosniff and DENY cost
// nothing and close the browser-side vectors that remain if a response is ever
// rendered in a web view. strictTransport adds HSTS; pass true only for
// environments served exclusively over HTTPS.
func SecurityHeaders(strictTransport bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			if strictTransport {
				h.Set("Strict-Transport-Security", strictTransportSecurity)
			}
			if strings.HasPrefix(r.URL.Path, "/api/") {
				h.Set("Cache-Control", "no-store")
			}
			next.ServeHTTP(w, r)
		})
	}
}
