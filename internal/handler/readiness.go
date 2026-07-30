package handler

import (
	"context"
	"net/http"
	"time"
)

const readinessTimeout = 2 * time.Second

type databasePinger interface {
	Ping(context.Context) error
}

// ReadinessHandler reports whether the API can reach its required database dependency.
// It is intentionally separate from /health, which only reports process liveness.
type ReadinessHandler struct {
	database databasePinger
}

func NewReadinessHandler(database databasePinger) *ReadinessHandler {
	return &ReadinessHandler{database: database}
}

// ServeHTTP godoc
// @Summary     Readiness check
// @Description Returns 200 when required dependencies are available, otherwise 503.
// @Tags        health
// @Produce     json
// @Success     200 {object} map[string]string
// @Failure     503 {object} map[string]string
// @Router      /ready [get]
func (h *ReadinessHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()

	if h == nil || h.database == nil || h.database.Ping(ctx) != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
