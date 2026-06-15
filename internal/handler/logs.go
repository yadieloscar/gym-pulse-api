package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

type LogHandler struct {
	svc service.LogService
}

func NewLogHandler(svc service.LogService) *LogHandler {
	return &LogHandler{svc: svc}
}

// ListByWeek godoc
// @Summary     List day logs for a week
// @Description Returns all day logs in the week containing the given Monday date.
// @Tags        logs
// @Produce     json
// @Param       week query string true "Week start date (Monday) in YYYY-MM-DD format"
// @Success     200 {array}  model.DayLogSummary
// @Failure     400 {object} map[string]string
// @Failure     401 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/logs [get]
func (h *LogHandler) ListByWeek(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	weekParam := r.URL.Query().Get("week")
	if weekParam == "" {
		writeError(w, http.StatusBadRequest, "week query parameter is required", "BAD_REQUEST", nil)
		return
	}

	logs, err := h.svc.ListByWeek(r.Context(), userID, weekParam)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, logs)
}

// GetByDate godoc
// @Summary     Get a day log
// @Description Returns the full day log for a specific date including template and overrides.
// @Tags        logs
// @Produce     json
// @Param       date path string true "Date in YYYY-MM-DD format"
// @Success     200 {object} model.DayLog
// @Failure     401 {object} map[string]string
// @Failure     404 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/logs/{date} [get]
func (h *LogHandler) GetByDate(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())
	date := chi.URLParam(r, "date")

	log, err := h.svc.GetByDate(r.Context(), userID, date)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, log)
}

// Create godoc
// @Summary     Create a day log
// @Description Logs a workout session for the given date. Only one log per date is allowed.
// @Tags        logs
// @Accept      json
// @Produce     json
// @Param       body body model.CreateDayLogRequest true "Day log payload"
// @Success     201 {object} model.DayLog
// @Failure     400 {object} map[string]string
// @Failure     401 {object} map[string]string
// @Failure     409 {object} map[string]string "A log already exists for this date"
// @Security    BearerAuth
// @Router      /api/v1/logs [post]
func (h *LogHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	var req model.CreateDayLogRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST", nil)
		return
	}

	log, err := h.svc.Create(r.Context(), userID, req)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, log)
}

// Update godoc
// @Summary     Update a day log
// @Description Updates session notes and replaces the exercise override list for the given date.
// @Tags        logs
// @Accept      json
// @Produce     json
// @Param       date path string                    true "Date in YYYY-MM-DD format"
// @Param       body body model.UpdateDayLogRequest true "Update payload"
// @Success     200 {object} model.DayLog
// @Failure     400 {object} map[string]string
// @Failure     401 {object} map[string]string
// @Failure     404 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/logs/{date} [put]
func (h *LogHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())
	date := chi.URLParam(r, "date")

	var req model.UpdateDayLogRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST", nil)
		return
	}

	log, err := h.svc.Update(r.Context(), userID, date, req)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, log)
}

// ExerciseHistory godoc
// @Summary     Most recent completed sets per exercise
// @Description Returns, for each comma-separated exercise id, the completed sets from the most recent day that exercise was performed — powers "last time you did X" hints.
// @Tags        logs
// @Produce     json
// @Param       ids query string true "Comma-separated exercise UUIDs"
// @Success     200 {array}  model.ExerciseHistory
// @Failure     401 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/exercises/history [get]
func (h *LogHandler) ExerciseHistory(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	history, err := h.svc.ExerciseHistory(r.Context(), userID, r.URL.Query().Get("ids"))
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, history)
}

// Delete godoc
// @Summary     Delete a day log
// @Description Permanently deletes the day log for the given date.
// @Tags        logs
// @Param       date path string true "Date in YYYY-MM-DD format"
// @Success     204
// @Failure     401 {object} map[string]string
// @Failure     404 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/logs/{date} [delete]
func (h *LogHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())
	date := chi.URLParam(r, "date")

	if err := h.svc.Delete(r.Context(), userID, date); err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
