package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoggingMiddleware(t *testing.T) {
	cases := []struct {
		name   string
		status int
		level  string
	}{
		{"info on 200", http.StatusOK, "INFO"},
		{"warn on 4xx", http.StatusBadRequest, "WARN"},
		{"error on 5xx", http.StatusInternalServerError, "ERROR"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte("body"))
			})
			handler := LoggingMiddleware(logger)(next)
			req := httptest.NewRequest("GET", "/something", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Errorf("status passthrough wrong: %d", rec.Code)
			}
			log := buf.String()
			if !strings.Contains(log, "level="+tc.level) {
				t.Errorf("expected level=%s, log: %s", tc.level, log)
			}
			if !strings.Contains(log, "/something") {
				t.Errorf("expected path in log, got %s", log)
			}
		})
	}

	t.Run("default 200 when handler does not call WriteHeader", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("hi"))
		})
		handler := LoggingMiddleware(logger)(next)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
		if !strings.Contains(buf.String(), "status=200") {
			t.Errorf("expected status=200 in log: %s", buf.String())
		}
	})
}
