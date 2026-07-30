package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type readinessPinger struct {
	err error
}

func (p readinessPinger) Ping(context.Context) error {
	return p.err
}

func TestReadinessHandler(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		pinger databasePinger
		status int
		body   string
	}{
		{name: "ready", pinger: readinessPinger{}, status: http.StatusOK, body: "{\"status\":\"ready\"}\n"},
		{name: "database unavailable", pinger: readinessPinger{err: errors.New("unavailable")}, status: http.StatusServiceUnavailable, body: "{\"status\":\"unavailable\"}\n"},
		{name: "missing dependency", pinger: nil, status: http.StatusServiceUnavailable, body: "{\"status\":\"unavailable\"}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			recorder := httptest.NewRecorder()
			NewReadinessHandler(tc.pinger).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ready", nil))

			if recorder.Code != tc.status {
				t.Fatalf("status = %d, want %d", recorder.Code, tc.status)
			}
			if got := recorder.Body.String(); got != tc.body {
				t.Fatalf("body = %q, want %q", got, tc.body)
			}
		})
	}
}
