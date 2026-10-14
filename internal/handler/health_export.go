package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

type HealthExportHandler struct{ svc service.HealthExportService }

func NewHealthExportHandler(svc service.HealthExportService) *HealthExportHandler {
	return &HealthExportHandler{svc: svc}
}

// List returns owned health exports whose workout date falls in the range.
func (h *HealthExportHandler) List(w http.ResponseWriter, r *http.Request) {
	exports, err := h.svc.List(r.Context(), middleware.MustGetUserID(r.Context()),
		r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"health_exports": exports})
}

// Record remembers a finished workout's health-store identity once. A new
// record or a verbatim replay answers 201; the same identity already recorded
// answers 200 with the stored record.
func (h *HealthExportHandler) Record(w http.ResponseWriter, r *http.Request) {
	workoutID, ok := pathUUID(w, r, "workout_id")
	if !ok {
		return
	}
	var req model.RecordHealthExportRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	export, outcome, err := h.svc.Record(r.Context(), middleware.MustGetUserID(r.Context()), chi.URLParam(r, "workout_kind"), workoutID, req)
	status := http.StatusCreated
	if outcome == model.HealthExportExisting {
		status = http.StatusOK
	}
	respond(w, export, err, status)
}
