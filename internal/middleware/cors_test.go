package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("default wildcard origins when empty", func(t *testing.T) {
		h := CORSMiddleware(nil)(next)
		req := httptest.NewRequest("OPTIONS", "/x", nil)
		req.Header.Set("Origin", "https://example.com")
		req.Header.Set("Access-Control-Request-Method", "GET")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Header().Get("Access-Control-Allow-Origin") == "" {
			t.Error("missing CORS header")
		}
	})

	t.Run("explicit allowed origins", func(t *testing.T) {
		h := CORSMiddleware([]string{"https://app.example.com"})(next)
		req := httptest.NewRequest("OPTIONS", "/x", nil)
		req.Header.Set("Origin", "https://app.example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
			t.Errorf("expected origin echoed, got %q", rec.Header().Get("Access-Control-Allow-Origin"))
		}
	})

	t.Run("simple GET passes through", func(t *testing.T) {
		h := CORSMiddleware([]string{"*"})(next)
		req := httptest.NewRequest("GET", "/x", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})
}
