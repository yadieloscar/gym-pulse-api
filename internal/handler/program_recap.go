package handler

import (
	"net/http"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// ProgramRecapHandler serves a program's recap. See docs/CONTRACTS.md
// "Program recap".
type ProgramRecapHandler struct{ svc service.ProgramRecapService }

// NewProgramRecapHandler returns a ProgramRecapHandler.
func NewProgramRecapHandler(svc service.ProgramRecapService) *ProgramRecapHandler {
	return &ProgramRecapHandler{svc: svc}
}

// Get godoc
// @Summary     Recap of one program
// @Description Weeks on target, workouts done, and how the main lifts moved, computed from the program's sessions. Read-only. See docs/CONTRACTS.md "Program recap".
// @Tags        goal-training
// @Produce     json
// @Param       id path string true "Program id"
// @Success     200 {object} model.ProgramRecap
// @Failure     400 {object} map[string]string
// @Failure     404 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/programs/{id}/recap [get]
func (h *ProgramRecapHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	recap, err := h.svc.Get(r.Context(), middleware.MustGetUserID(r.Context()), id)
	respond(w, recap, err, http.StatusOK)
}
