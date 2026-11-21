package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// WeekOffHandler serves the weeks an athlete took off. See
// docs/CONTRACTS.md "Weeks off".
type WeekOffHandler struct{ svc service.WeekOffService }

// NewWeekOffHandler returns a WeekOffHandler.
func NewWeekOffHandler(svc service.WeekOffService) *WeekOffHandler {
	return &WeekOffHandler{svc: svc}
}

// List godoc
// @Summary     List the athlete's weeks off
// @Description Returns the weeks off whose Monday falls within from..to, oldest first.
// @Tags        schedule
// @Param       from query string true "First date, YYYY-MM-DD"
// @Param       to   query string true "Last date, YYYY-MM-DD"
// @Success     200 {object} model.WeeksOffResponse
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/weeks-off [get]
func (h *WeekOffHandler) List(w http.ResponseWriter, r *http.Request) {
	weeks, err := h.svc.List(r.Context(), middleware.MustGetUserID(r.Context()), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	respond(w, weeks, err, http.StatusOK)
}

// Take godoc
// @Summary     Take a week off
// @Description Marks the Monday-to-Sunday week as off. Idempotent by week_start; there is no body. See docs/CONTRACTS.md "Weeks off".
// @Tags        schedule
// @Param       week_start path string true "The week's Monday, YYYY-MM-DD"
// @Success     200 {object} model.WeekOff
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/weeks-off/{week_start} [put]
func (h *WeekOffHandler) Take(w http.ResponseWriter, r *http.Request) {
	week, err := h.svc.Take(r.Context(), middleware.MustGetUserID(r.Context()), chi.URLParam(r, "week_start"))
	respond(w, week, err, http.StatusOK)
}

// End godoc
// @Summary     End a week off
// @Description Removes the week off. Idempotent: a week that is not off also returns 204.
// @Tags        schedule
// @Param       week_start path string true "The week's Monday, YYYY-MM-DD"
// @Success     204
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/weeks-off/{week_start} [delete]
func (h *WeekOffHandler) End(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.End(r.Context(), middleware.MustGetUserID(r.Context()), chi.URLParam(r, "week_start")); err != nil {
		handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
