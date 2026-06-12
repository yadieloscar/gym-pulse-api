package handler

import (
	"net/http"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// ExerciseCatalogHandler handles exercise catalog HTTP requests.
type ExerciseCatalogHandler struct {
	svc service.ExerciseCatalogService
}

// NewExerciseCatalogHandler creates a new ExerciseCatalogHandler.
func NewExerciseCatalogHandler(svc service.ExerciseCatalogService) *ExerciseCatalogHandler {
	return &ExerciseCatalogHandler{svc: svc}
}

// List godoc
// @Summary     List catalog exercises
// @Description Returns the curated exercise catalog, optionally filtered by workout type category.
// @Tags        exercises
// @Produce     json
// @Param       category query string false "Workout type id to filter by (e.g. push, pull, legs, cardio)"
// @Success     200 {object} map[string][]model.CatalogExercise
// @Failure     401 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/exercises [get]
func (h *ExerciseCatalogHandler) List(w http.ResponseWriter, r *http.Request) {
	exercises, err := h.svc.List(r.Context(), r.URL.Query().Get("category"))
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string][]model.CatalogExercise{"exercises": exercises})
}
