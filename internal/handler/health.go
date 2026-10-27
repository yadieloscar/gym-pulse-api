package handler

import (
	"net/http"

	"github.com/gym-pulse/gym-pulse-api/internal/buildinfo"
)

// HealthCheck godoc
// @Summary     Health check
// @Description Returns 200 when the server is up, with the build version (source commit, or "dev").
// @Tags        health
// @Produce     json
// @Success     200 {object} map[string]string
// @Router      /health [get]
func HealthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": buildinfo.Version})
}
