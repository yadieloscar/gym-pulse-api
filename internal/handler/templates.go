package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

type TemplateHandler struct {
	svc service.TemplateService
}

func NewTemplateHandler(svc service.TemplateService) *TemplateHandler {
	return &TemplateHandler{svc: svc}
}

// List godoc
// @Summary     List workout templates
// @Description Returns all workout templates for the authenticated user, optionally filtered by type or subtype.
// @Tags        templates
// @Produce     json
// @Param       type    query string false "Filter by type (push, pull, legs, cardio, upper, lower, full, core, other)"
// @Param       subtype query string false "Filter by subtype (hypertrophy, strength, power, endurance, mobility, conditioning, skills, general)"
// @Success     200 {array}  model.TemplateSummary
// @Failure     401 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/templates [get]
func (h *TemplateHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())
	typeFilter := r.URL.Query().Get("type")
	subtypeFilter := r.URL.Query().Get("subtype")

	summaries, err := h.svc.List(r.Context(), userID, typeFilter, subtypeFilter)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, summaries)
}

// GetByID godoc
// @Summary     Get a workout template
// @Description Returns the full workout template including exercises.
// @Tags        templates
// @Produce     json
// @Param       id path string true "Template UUID"
// @Success     200 {object} model.WorkoutTemplate
// @Failure     400 {object} map[string]string
// @Failure     401 {object} map[string]string
// @Failure     404 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/templates/{id} [get]
func (h *TemplateHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	templateID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid template id", "INVALID_ID", nil)
		return
	}

	tmpl, err := h.svc.GetByID(r.Context(), userID, templateID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, tmpl)
}

// Create godoc
// @Summary     Create a workout template
// @Description Creates a new workout template with exercises.
// @Tags        templates
// @Accept      json
// @Produce     json
// @Param       body body model.CreateTemplateRequest true "Template payload"
// @Success     201 {object} model.WorkoutTemplate
// @Failure     400 {object} map[string]string
// @Failure     401 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/templates [post]
func (h *TemplateHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	var req model.CreateTemplateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST", nil)
		return
	}

	tmpl, err := h.svc.Create(r.Context(), userID, req)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, tmpl)
}

// Update godoc
// @Summary     Update a workout template
// @Description Replaces the template name, type, subtype, and full exercise list.
// @Tags        templates
// @Accept      json
// @Produce     json
// @Param       id   path string                      true "Template UUID"
// @Param       body body model.CreateTemplateRequest true "Template payload"
// @Success     200 {object} model.WorkoutTemplate
// @Failure     400 {object} map[string]string
// @Failure     401 {object} map[string]string
// @Failure     404 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/templates/{id} [put]
func (h *TemplateHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	templateID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid template id", "INVALID_ID", nil)
		return
	}

	var req model.CreateTemplateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST", nil)
		return
	}

	tmpl, err := h.svc.Update(r.Context(), userID, templateID, req)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, tmpl)
}

// Delete godoc
// @Summary     Delete a workout template
// @Description Permanently deletes a workout template and its exercises.
// @Tags        templates
// @Param       id path string true "Template UUID"
// @Success     204
// @Failure     400 {object} map[string]string
// @Failure     401 {object} map[string]string
// @Failure     404 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/templates/{id} [delete]
func (h *TemplateHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	templateID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid template id", "INVALID_ID", nil)
		return
	}

	if err := h.svc.Delete(r.Context(), userID, templateID); err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
