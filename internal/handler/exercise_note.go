package handler

import (
	"net/http"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// ExerciseNoteHandler serves the athlete's exercise notes. See
// docs/CONTRACTS.md "Exercise notes".
type ExerciseNoteHandler struct{ svc service.ExerciseNoteService }

// NewExerciseNoteHandler returns an ExerciseNoteHandler.
func NewExerciseNoteHandler(svc service.ExerciseNoteService) *ExerciseNoteHandler {
	return &ExerciseNoteHandler{svc: svc}
}

// List godoc
// @Summary     List the athlete's exercise notes
// @Description Returns every note the athlete keeps, ordered by exercise_key.
// @Tags        exercises
// @Success     200 {object} model.ExerciseNotesResponse
// @Security    BearerAuth
// @Router      /api/v1/exercise-notes [get]
func (h *ExerciseNoteHandler) List(w http.ResponseWriter, r *http.Request) {
	notes, err := h.svc.List(r.Context(), middleware.MustGetUserID(r.Context()))
	respond(w, notes, err, http.StatusOK)
}

// Save godoc
// @Summary     Save, edit, or clear the note for one exercise
// @Description Replaces the athlete's note for the named exercise; an empty note clears it. Idempotent, last write wins. See docs/CONTRACTS.md "Exercise notes".
// @Tags        exercises
// @Param       body body model.SaveExerciseNoteRequest true "Exercise name and note"
// @Success     200 {object} model.ExerciseNote
// @Failure     400 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/exercise-notes [put]
func (h *ExerciseNoteHandler) Save(w http.ResponseWriter, r *http.Request) {
	var req model.SaveExerciseNoteRequest
	if !decodeMutation(w, r, &req, "") {
		return
	}
	note, err := h.svc.Save(r.Context(), middleware.MustGetUserID(r.Context()), req)
	respond(w, note, err, http.StatusOK)
}
