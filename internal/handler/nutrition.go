package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

type NutritionHandler struct{ svc service.NutritionService }

func NewNutritionHandler(svc service.NutritionService) *NutritionHandler {
	return &NutritionHandler{svc: svc}
}

// SaveDay godoc
// @Summary     Save, replace, or remove one day's nutrition totals
// @Description Athlete saves carry expected_revision; Health saves never overwrite an athlete day.
// @Description All four values absent removes the day. See docs/CONTRACTS.md "Nutrition".
// @Tags        nutrition
// @Param       date path string true "Local calendar date YYYY-MM-DD"
// @Param       Idempotency-Key header string true "Equal to operation_key"
// @Param       body body model.SaveNutritionDayRequest true "Totals"
// @Success     201 {object} model.NutritionDay
// @Success     200 {object} model.NutritionDay
// @Success     204
// @Failure     404 {object} map[string]string
// @Failure     409 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/nutrition/days/{date} [put]
func (h *NutritionHandler) SaveDay(w http.ResponseWriter, r *http.Request) {
	var req model.SaveNutritionDayRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	day, outcome, err := h.svc.SaveDay(r.Context(), middleware.MustGetUserID(r.Context()), chi.URLParam(r, "date"), req)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	switch outcome {
	case model.NutritionDayCreated:
		writeJSON(w, http.StatusCreated, day)
	case model.NutritionDayRemoved:
		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSON(w, http.StatusOK, day)
	}
}

// ListDays godoc
// @Summary     List nutrition days in a range with the targets that applied
// @Tags        nutrition
// @Param       from query string true "YYYY-MM-DD"
// @Param       to query string true "YYYY-MM-DD (at most 366 days after from)"
// @Success     200 {object} model.NutritionRange
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/nutrition/days [get]
func (h *NutritionHandler) ListDays(w http.ResponseWriter, r *http.Request) {
	days, err := h.svc.ListDays(r.Context(), middleware.MustGetUserID(r.Context()),
		r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	respond(w, days, err, http.StatusOK)
}

// GetTarget godoc
// @Summary     Read the nutrition target in effect today
// @Tags        nutrition
// @Success     200 {object} model.NutritionTarget
// @Failure     404 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/nutrition/target [get]
func (h *NutritionHandler) GetTarget(w http.ResponseWriter, r *http.Request) {
	target, err := h.svc.GetTarget(r.Context(), middleware.MustGetUserID(r.Context()))
	respond(w, target, err, http.StatusOK)
}

// SaveTarget godoc
// @Summary     Save the athlete's chosen nutrition target
// @Description expected_revision is the current target's revision, 0 when none exists.
// @Tags        nutrition
// @Param       Idempotency-Key header string true "Equal to operation_key"
// @Param       body body model.SaveNutritionTargetRequest true "Target"
// @Success     201 {object} model.NutritionTarget
// @Success     200 {object} model.NutritionTarget
// @Failure     404 {object} map[string]string
// @Failure     409 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/nutrition/target [put]
func (h *NutritionHandler) SaveTarget(w http.ResponseWriter, r *http.Request) {
	var req model.SaveNutritionTargetRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	target, outcome, err := h.svc.SaveTarget(r.Context(), middleware.MustGetUserID(r.Context()), req)
	status := http.StatusOK
	if outcome == model.NutritionTargetCreated {
		status = http.StatusCreated
	}
	respond(w, target, err, status)
}
