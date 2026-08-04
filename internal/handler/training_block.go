package handler

import (
	"net/http"
	"strconv"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

type TrainingBlockHandler struct{ svc service.TrainingBlockService }

func NewTrainingBlockHandler(svc service.TrainingBlockService) *TrainingBlockHandler {
	return &TrainingBlockHandler{svc: svc}
}

func (h *TrainingBlockHandler) List(w http.ResponseWriter, r *http.Request) {
	limit, ok := trainingBlockQueryInt(w, r, "limit", 0)
	if !ok {
		return
	}
	offset, ok := trainingBlockQueryInt(w, r, "offset", 0)
	if !ok {
		return
	}
	resources, err := h.svc.List(r.Context(), middleware.MustGetUserID(r.Context()), r.URL.Query().Get("status"), limit, offset)
	respond(w, resources, err, http.StatusOK)
}

func (h *TrainingBlockHandler) Get(w http.ResponseWriter, r *http.Request) {
	blockID, ok := pathUUID(w, r, "block_id")
	if !ok {
		return
	}
	resource, err := h.svc.Get(r.Context(), middleware.MustGetUserID(r.Context()), blockID)
	respond(w, resource, err, http.StatusOK)
}

func (h *TrainingBlockHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateTrainingBlockRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	resource, err := h.svc.Create(r.Context(), middleware.MustGetUserID(r.Context()), req)
	respond(w, resource, err, http.StatusCreated)
}

func (h *TrainingBlockHandler) AddExposure(w http.ResponseWriter, r *http.Request) {
	blockID, ok := pathUUID(w, r, "block_id")
	if !ok {
		return
	}
	var req model.CreateTrainingExposureRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	resource, err := h.svc.AddExposure(r.Context(), middleware.MustGetUserID(r.Context()), blockID, req)
	respond(w, resource, err, http.StatusOK)
}

func (h *TrainingBlockHandler) RecordNextMorning(w http.ResponseWriter, r *http.Request) {
	blockID, ok := pathUUID(w, r, "block_id")
	if !ok {
		return
	}
	exposureID, ok := pathUUID(w, r, "exposure_id")
	if !ok {
		return
	}
	var req model.RecordNextMorningRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	resource, err := h.svc.RecordNextMorning(r.Context(), middleware.MustGetUserID(r.Context()), blockID, exposureID, req)
	respond(w, resource, err, http.StatusOK)
}

func (h *TrainingBlockHandler) Transition(w http.ResponseWriter, r *http.Request) {
	blockID, ok := pathUUID(w, r, "block_id")
	if !ok {
		return
	}
	var req model.CreateTrainingTransitionRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	resource, err := h.svc.Transition(r.Context(), middleware.MustGetUserID(r.Context()), blockID, req)
	respond(w, resource, err, http.StatusOK)
}

func trainingBlockQueryInt(w http.ResponseWriter, r *http.Request, name string, defaultValue int) (int, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return defaultValue, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, name+" must be an integer", "VALIDATION_ERROR", map[string]string{"field": name})
		return 0, false
	}
	return value, true
}
