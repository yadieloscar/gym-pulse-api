package handler

import (
	"net/http"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// StretchLibraryHandler serves the stretch moves and routines an athlete
// wrote. See docs/CONTRACTS.md "Stretch library".
type StretchLibraryHandler struct{ svc service.StretchLibraryService }

// NewStretchLibraryHandler returns a StretchLibraryHandler.
func NewStretchLibraryHandler(svc service.StretchLibraryService) *StretchLibraryHandler {
	return &StretchLibraryHandler{svc: svc}
}

// Get godoc
// @Summary     Read the athlete's own stretch moves and routines
// @Description An athlete who never saved a library gets revision 0, empty lists, and a null updated_at.
// @Tags        stretching
// @Success     200 {object} model.StretchLibrary
// @Security    BearerAuth
// @Router      /api/v1/stretching/library [get]
func (h *StretchLibraryHandler) Get(w http.ResponseWriter, r *http.Request) {
	library, err := h.svc.Get(r.Context(), middleware.MustGetUserID(r.Context()))
	respond(w, library, err, http.StatusOK)
}

// Save godoc
// @Summary     Replace the athlete's own stretch moves and routines
// @Description The whole library is replaced. expected_revision is the stored revision, 0 when none exists. See docs/CONTRACTS.md "Stretch library".
// @Tags        stretching
// @Param       Idempotency-Key header string true "Equal to operation_key"
// @Param       body body model.SaveStretchLibraryRequest true "Every move and routine the athlete keeps"
// @Success     200 {object} model.StretchLibrary
// @Failure     409 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/stretching/library [put]
func (h *StretchLibraryHandler) Save(w http.ResponseWriter, r *http.Request) {
	var req model.SaveStretchLibraryRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	library, err := h.svc.Save(r.Context(), middleware.MustGetUserID(r.Context()), req)
	respond(w, library, err, http.StatusOK)
}
