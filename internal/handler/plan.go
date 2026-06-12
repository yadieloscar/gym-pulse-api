package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// PlanHandler handles weekly plan HTTP requests.
type PlanHandler struct {
	svc service.PlanService
}

// NewPlanHandler creates a new PlanHandler.
func NewPlanHandler(svc service.PlanService) *PlanHandler {
	return &PlanHandler{svc: svc}
}

// Get godoc
// @Summary     Get the training plan
// @Description Returns the recurring weekly plan plus per-date overrides in the requested window (default: ±4 weeks around today). Effective-day resolution is the client's responsibility.
// @Tags        plan
// @Produce     json
// @Param       from query string false "Window start (YYYY-MM-DD)"
// @Param       to   query string false "Window end (YYYY-MM-DD)"
// @Success     200 {object} model.PlanResponse
// @Failure     401 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/plan [get]
func (h *PlanHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	plan, err := h.svc.Get(r.Context(), userID, r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, plan)
}

// PutWeekly godoc
// @Summary     Replace the recurring weekly plan
// @Description Fully replaces the recurring plan. Sparse is allowed; weekdays missing from the request become unplanned.
// @Tags        plan
// @Accept      json
// @Produce     json
// @Param       body body model.PutWeeklyPlanRequest true "Weekly plan"
// @Success     200 {array} model.WeeklyPlanDay
// @Failure     400 {object} map[string]string
// @Failure     401 {object} map[string]string
// @Failure     404 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/plan/weekly [put]
func (h *PlanHandler) PutWeekly(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	var req model.PutWeeklyPlanRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST", nil)
		return
	}

	days, err := h.svc.PutWeekly(r.Context(), userID, req)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, days)
}

// PutOverride godoc
// @Summary     Upsert a one-day plan override
// @Description Overrides the weekly plan for one date without changing the recurring plan.
// @Tags        plan
// @Accept      json
// @Produce     json
// @Param       date path string true "Date (YYYY-MM-DD)"
// @Param       body body model.PutPlanOverrideRequest true "Override"
// @Success     204
// @Failure     400 {object} map[string]string
// @Failure     401 {object} map[string]string
// @Failure     404 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/plan/overrides/{date} [put]
func (h *PlanHandler) PutOverride(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	var req model.PutPlanOverrideRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST", nil)
		return
	}

	if err := h.svc.PutOverride(r.Context(), userID, chi.URLParam(r, "date"), req); err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DeleteOverride godoc
// @Summary     Remove a one-day plan override
// @Description Deletes the override; the date falls back to the recurring weekly plan.
// @Tags        plan
// @Param       date path string true "Date (YYYY-MM-DD)"
// @Success     204
// @Failure     401 {object} map[string]string
// @Failure     404 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/plan/overrides/{date} [delete]
func (h *PlanHandler) DeleteOverride(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	if err := h.svc.DeleteOverride(r.Context(), userID, chi.URLParam(r, "date")); err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
