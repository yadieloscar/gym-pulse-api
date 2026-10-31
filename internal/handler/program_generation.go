package handler

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

type ProgramGenerationHandler struct {
	svc service.ProgramGenerationService
	// timeout is the generation deadline; the response write deadline is
	// extended past it for this route only, because the server's default
	// WriteTimeout (15 s) is shorter than a provider call.
	timeout time.Duration
}

// writeDeadlineMargin covers validation, repair, and JSON encoding after the
// provider call returns.
const writeDeadlineMargin = 5 * time.Second

func NewProgramGenerationHandler(svc service.ProgramGenerationService, timeout time.Duration) *ProgramGenerationHandler {
	return &ProgramGenerationHandler{svc: svc, timeout: timeout}
}

// Status reports whether program generation is available and how many drafts
// remain in the athlete's local day.
// @Summary Program generation status
// @Tags goal-training
// @Produce json
// @Success 200 {object} model.GenerationStatus
// @Security BearerAuth
// @Router /api/v1/program-generation [get]
func (h *ProgramGenerationHandler) Status(w http.ResponseWriter, r *http.Request) {
	status, err := h.svc.Status(r.Context(), middleware.MustGetUserID(r.Context()))
	respond(w, status, err, http.StatusOK)
}

// Generate builds one unsaved program draft from the training profile. The
// body is optional; nothing but the daily count is stored.
// @Summary Generate a program draft
// @Tags goal-training
// @Accept json
// @Produce json
// @Param body body model.GenerateProgramRequest false "Optional one-line request"
// @Success 200 {object} model.GenerateProgramResponse
// @Failure 422 {object} apiError
// @Failure 429 {object} apiError
// @Failure 503 {object} apiError
// @Security BearerAuth
// @Router /api/v1/program-generation [post]
func (h *ProgramGenerationHandler) Generate(w http.ResponseWriter, r *http.Request) {
	var req model.GenerateProgramRequest
	if err := decodeJSON(w, r, &req); err != nil && !errors.Is(err, io.EOF) {
		writeDecodeError(w, err)
		return
	}
	// Recorders and some middleware do not support deadlines; the server's
	// own timeout then applies, which the router test documents.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(h.timeout + writeDeadlineMargin))
	resp, err := h.svc.Generate(r.Context(), middleware.MustGetUserID(r.Context()), req)
	respond(w, resp, err, http.StatusOK)
}
