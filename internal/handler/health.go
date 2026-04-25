package handler

import "net/http"

// HealthCheck godoc
// @Summary     Health check
// @Description Returns 200 when the server is up.
// @Tags        health
// @Produce     json
// @Success     200 {object} map[string]string
// @Router      /health [get]
func HealthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
