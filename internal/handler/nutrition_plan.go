package handler

import (
	"net/http"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// NutritionPlanHandler serves the macro calculator and the saved plan.
type NutritionPlanHandler struct{ svc service.NutritionPlanService }

func NewNutritionPlanHandler(svc service.NutritionPlanService) *NutritionPlanHandler {
	return &NutritionPlanHandler{svc: svc}
}

// Options godoc
// @Summary     Read the macro calculator's foods, ranges, and defaults
// @Tags        nutrition
// @Success     200 {object} model.CalculatorOptions
// @Security    BearerAuth
// @Router      /api/v1/nutrition/calculator/options [get]
func (h *NutritionPlanHandler) Options(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.Options())
}

// Calculate godoc
// @Summary     Calculate estimated energy, macro targets, and a four-meal template
// @Description Stores nothing. See docs/CONTRACTS.md "Macro calculator".
// @Tags        nutrition
// @Param       body body model.CalculatorInput true "The athlete's numbers and preferences"
// @Success     200 {object} model.NutritionPlan
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/nutrition/calculate [post]
func (h *NutritionPlanHandler) Calculate(w http.ResponseWriter, r *http.Request) {
	var req model.CalculatorInput
	if !decodeMutation(w, r, &req, "") {
		return
	}
	plan, err := h.svc.Calculate(req)
	respond(w, plan, err, http.StatusOK)
}

// SaveProfile godoc
// @Summary     Save the calculator input as the athlete's plan
// @Description The service recomputes the targets. expected_revision is the saved plan's revision, 0 when none exists.
// @Tags        nutrition
// @Param       Idempotency-Key header string true "Equal to operation_key"
// @Param       body body model.SaveNutritionProfileRequest true "The athlete's numbers and preferences"
// @Success     201 {object} model.SavedNutritionPlan
// @Success     200 {object} model.SavedNutritionPlan
// @Failure     409 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/nutrition/profile [put]
func (h *NutritionPlanHandler) SaveProfile(w http.ResponseWriter, r *http.Request) {
	var req model.SaveNutritionProfileRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	plan, outcome, err := h.svc.SaveProfile(r.Context(), middleware.MustGetUserID(r.Context()), req)
	status := http.StatusOK
	if outcome == model.NutritionProfileCreated {
		status = http.StatusCreated
	}
	respond(w, plan, err, status)
}

// GetPlan godoc
// @Summary     Read the athlete's saved plan
// @Tags        nutrition
// @Success     200 {object} model.SavedNutritionPlan
// @Failure     404 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/nutrition/plan [get]
func (h *NutritionPlanHandler) GetPlan(w http.ResponseWriter, r *http.Request) {
	plan, err := h.svc.GetPlan(r.Context(), middleware.MustGetUserID(r.Context()))
	respond(w, plan, err, http.StatusOK)
}
