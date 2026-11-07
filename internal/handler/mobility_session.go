package handler

import (
	"net/http"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// MobilitySessionHandler serves recorded stretching routines.
type MobilitySessionHandler struct {
	svc service.MobilitySessionService
}

// NewMobilitySessionHandler returns a MobilitySessionHandler.
func NewMobilitySessionHandler(svc service.MobilitySessionService) *MobilitySessionHandler {
	return &MobilitySessionHandler{svc: svc}
}

// List returns owned entries in a date range, newest first.
func (h *MobilitySessionHandler) List(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.svc.List(r.Context(), middleware.MustGetUserID(r.Context()),
		r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	respond(w, sessions, err, http.StatusOK)
}

// Get returns one owned entry.
func (h *MobilitySessionHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	session, err := h.svc.Get(r.Context(), middleware.MustGetUserID(r.Context()), id)
	respond(w, session, err, http.StatusOK)
}

// Create records a finished routine: 201 for a new entry, 200 with the
// original entry for an identical retry.
func (h *MobilitySessionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateMobilitySessionRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	session, outcome, err := h.svc.Create(r.Context(), middleware.MustGetUserID(r.Context()), req)
	status := http.StatusCreated
	if outcome == model.MobilitySessionReplayed {
		status = http.StatusOK
	}
	respond(w, session, err, status)
}

// Delete removes an owned entry.
func (h *MobilitySessionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), middleware.MustGetUserID(r.Context()), id); err != nil {
		handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
