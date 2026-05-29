package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// BodyWeightHandler handles body weight-related HTTP requests.
type BodyWeightHandler struct {
	svc service.BodyWeightService
}

// NewBodyWeightHandler creates a new BodyWeightHandler.
func NewBodyWeightHandler(svc service.BodyWeightService) *BodyWeightHandler {
	return &BodyWeightHandler{svc: svc}
}

// Create godoc
// @Summary     Log a body weight entry
// @Description Creates or updates a body weight entry for the given date. Only one entry per date is kept.
// @Tags        body-weight
// @Accept      json
// @Produce     json
// @Param       body body model.CreateBodyWeightRequest true "Body weight payload"
// @Success     201 {object} model.BodyWeight
// @Failure     400 {object} map[string]string
// @Failure     401 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/body/weight [post]
func (h *BodyWeightHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	var req model.CreateBodyWeightRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST", nil)
		return
	}

	entry, err := h.svc.LogWeight(r.Context(), userID, req)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, entry)
}

// List godoc
// @Summary     List body weight entries
// @Description Returns all body weight entries for the authenticated user, ordered by date descending.
// @Tags        body-weight
// @Produce     json
// @Success     200 {array} model.BodyWeight
// @Failure     401 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/body/weight [get]
func (h *BodyWeightHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	entries, err := h.svc.ListWeights(r.Context(), userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, entries)
}

// Delete godoc
// @Summary     Delete a body weight entry
// @Description Permanently deletes a body weight entry by its ID.
// @Tags        body-weight
// @Param       id path string true "Body weight entry UUID"
// @Success     204
// @Failure     400 {object} map[string]string
// @Failure     401 {object} map[string]string
// @Failure     404 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/body/weight/{id} [delete]
func (h *BodyWeightHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	entryID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid entry id", "BAD_REQUEST", nil)
		return
	}

	if err := h.svc.DeleteWeight(r.Context(), userID, entryID); err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
