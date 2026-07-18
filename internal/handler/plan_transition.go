package handler

import (
	"net/http"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

type PlanTransitionHandler struct{ svc service.PlanTransitionService }

func NewPlanTransitionHandler(svc service.PlanTransitionService) *PlanTransitionHandler {
	return &PlanTransitionHandler{svc: svc}
}
func (h *PlanTransitionHandler) Preview(w http.ResponseWriter, r *http.Request) {
	var req model.PreviewPlanTransitionRequest
	if !decodeMutation(w, r, &req, "") {
		return
	}
	resource, err := h.svc.Preview(r.Context(), middleware.MustGetUserID(r.Context()), req)
	respond(w, resource, err, http.StatusOK)
}
func (h *PlanTransitionHandler) Apply(w http.ResponseWriter, r *http.Request) {
	var req model.ApplyPlanTransitionRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	resource, err := h.svc.Apply(r.Context(), middleware.MustGetUserID(r.Context()), req)
	respond(w, resource, err, http.StatusOK)
}
