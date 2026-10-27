package handler

import (
	"net/http"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

type SportActivityHandler struct{ svc service.SportActivityService }

func NewSportActivityHandler(svc service.SportActivityService) *SportActivityHandler {
	return &SportActivityHandler{svc: svc}
}

// List returns owned completed sports in deterministic newest-first order.
func (h *SportActivityHandler) List(w http.ResponseWriter, r *http.Request) {
	activities, err := h.svc.List(r.Context(), middleware.MustGetUserID(r.Context()),
		r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	respond(w, activities, err, http.StatusOK)
}

// Get returns one owned completed sport.
func (h *SportActivityHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	activity, err := h.svc.Get(r.Context(), middleware.MustGetUserID(r.Context()), id)
	respond(w, activity, err, http.StatusOK)
}

// Create records a completed sport and preserves participation atomically.
// A new activity or an identical idempotent replay answers 201; a re-imported
// health workout answers 200 with the activity it already produced.
func (h *SportActivityHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateSportActivityRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	activity, outcome, err := h.svc.Create(r.Context(), middleware.MustGetUserID(r.Context()), req)
	status := http.StatusCreated
	if outcome == model.SportActivityExisting {
		status = http.StatusOK
	}
	respond(w, activity, err, status)
}

// Link counts an owned unlinked activity toward an owned open sport session.
func (h *SportActivityHandler) Link(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req model.LinkSportActivityRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	result, err := h.svc.Link(r.Context(), middleware.MustGetUserID(r.Context()), id, req)
	respond(w, result, err, http.StatusOK)
}

// Unlink clears an activity's session link; the session reopens or is missed again.
func (h *SportActivityHandler) Unlink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var req model.UnlinkSportActivityRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	result, err := h.svc.Unlink(r.Context(), middleware.MustGetUserID(r.Context()), id, req)
	respond(w, result, err, http.StatusOK)
}
