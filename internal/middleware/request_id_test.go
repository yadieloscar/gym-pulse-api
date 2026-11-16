package middleware

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

var requestIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestRequestIDMiddleware(t *testing.T) {
	var seen []string
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, chimiddleware.GetReqID(r.Context()))
		w.WriteHeader(http.StatusNoContent)
	}))

	var headers []string
	for range 2 {
		request := httptest.NewRequest(http.MethodGet, "/health", nil)
		request.Header.Set("X-Request-ID", "forged\nlevel=ERROR msg=other-user")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		headers = append(headers, recorder.Header().Get("X-Request-ID"))
	}

	for i, header := range headers {
		if !requestIDPattern.MatchString(header) {
			t.Errorf("request %d: X-Request-ID = %q, want 32 lowercase hex characters", i, header)
		}
		if seen[i] != header {
			t.Errorf("request %d: context id %q != header %q", i, seen[i], header)
		}
	}
	if headers[0] == headers[1] {
		t.Errorf("two requests got the same id %q", headers[0])
	}
}
