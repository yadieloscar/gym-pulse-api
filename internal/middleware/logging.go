package middleware

import (
	"log/slog"
	"net/http"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)
			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", wrapped.statusCode),
				slog.Duration("duration", duration),
				slog.String("remote", r.RemoteAddr),
			}
			if requestID := chimiddleware.GetReqID(r.Context()); requestID != "" {
				attrs = append(attrs, slog.String("request_id", requestID))
			}

			msg := r.Method + " " + r.URL.Path
			switch {
			case wrapped.statusCode >= 500:
				logger.LogAttrs(r.Context(), slog.LevelError, msg, attrs...)
			case wrapped.statusCode >= 400:
				logger.LogAttrs(r.Context(), slog.LevelWarn, msg, attrs...)
			default:
				logger.LogAttrs(r.Context(), slog.LevelInfo, msg, attrs...)
			}
		})
	}
}
