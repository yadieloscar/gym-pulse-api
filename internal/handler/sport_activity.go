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
func (h *SportActivityHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateSportActivityRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	activity, err := h.svc.Create(r.Context(), middleware.MustGetUserID(r.Context()), req)
	respond(w, activity, err, http.StatusCreated)
}
