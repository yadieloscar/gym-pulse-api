package handler

import (
	"net/http"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

type StatsHandler struct {
	svc service.StatsService
}

func NewStatsHandler(svc service.StatsService) *StatsHandler {
	return &StatsHandler{svc: svc}
}

// Summary godoc
// @Summary     Get stats summary
// @Description Returns this week's progress, current streak, and total workout count.
// @Tags        stats
// @Produce     json
// @Success     200 {object} model.StatsSummary
// @Failure     401 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/stats/summary [get]
func (h *StatsHandler) Summary(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	summary, err := h.svc.GetSummary(r.Context(), userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, summary)
}

// Distribution godoc
// @Summary     Get workout type distribution
// @Description Returns a breakdown of workout counts by type and subtype.
// @Tags        stats
// @Produce     json
// @Success     200 {object} map[string][]model.TypeDistribution
// @Failure     401 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/stats/distribution [get]
func (h *StatsHandler) Distribution(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	dist, err := h.svc.GetDistribution(r.Context(), userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"types": dist})
}

// Volume godoc
// @Summary     Weekly training volume
// @Description Total lifted volume (Σ weight × reps) per week for the last N weeks (default 8), oldest first; weeks with no data are 0.
// @Tags        stats
// @Produce     json
// @Param       weeks query int false "Number of weeks (default 8, max 52)"
// @Success     200 {array}  model.WeeklyVolume
// @Failure     401 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/stats/volume [get]
func (h *StatsHandler) Volume(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	volume, err := h.svc.GetVolume(r.Context(), userID, r.URL.Query().Get("weeks"))
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, volume)
}
