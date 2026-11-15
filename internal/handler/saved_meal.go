package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// SavedMealHandler serves an athlete's saved meals and meal logs under
// /api/v1/nutrition. See docs/CONTRACTS.md "Saved meals".
type SavedMealHandler struct{ svc service.SavedMealService }

func NewSavedMealHandler(svc service.SavedMealService) *SavedMealHandler {
	return &SavedMealHandler{svc: svc}
}

// List godoc
// @Summary     The athlete's saved meals, most recently logged first
// @Description Item values and totals are computed from each food's current facts; a meal stores none.
// @Tags        nutrition
// @Success     200 {object} model.SavedMeals
// @Security    BearerAuth
// @Router      /api/v1/nutrition/meals [get]
func (h *SavedMealHandler) List(w http.ResponseWriter, r *http.Request) {
	meals, err := h.svc.ListMeals(r.Context(), middleware.MustGetUserID(r.Context()))
	respond(w, meals, err, http.StatusOK)
}

// Create godoc
// @Summary     Save a meal: a name and 1 to 20 foods, each with a serving and amount
// @Tags        nutrition
// @Param       Idempotency-Key header string true "Equal to operation_key"
// @Param       body body model.SaveMealRequest true "Name and items"
// @Success     201 {object} model.SavedMeal
// @Failure     409 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/nutrition/meals [post]
func (h *SavedMealHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.SaveMealRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	meal, _, err := h.svc.CreateMeal(r.Context(), middleware.MustGetUserID(r.Context()), req)
	respond(w, meal, err, http.StatusCreated)
}

// Update godoc
// @Summary     Replace a saved meal's name and items; days already logged do not change
// @Tags        nutrition
// @Param       id path string true "Saved meal id"
// @Param       Idempotency-Key header string true "Equal to operation_key"
// @Param       body body model.SaveMealRequest true "Name and items with expected_revision"
// @Success     200 {object} model.SavedMeal
// @Failure     404 {object} map[string]string
// @Failure     409 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/nutrition/meals/{id} [put]
func (h *SavedMealHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := foodIDParam(w, r, "saved meal not found")
	if !ok {
		return
	}
	var req model.SaveMealRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	meal, _, err := h.svc.UpdateMeal(r.Context(), middleware.MustGetUserID(r.Context()), id, req)
	respond(w, meal, err, http.StatusOK)
}

// Delete godoc
// @Summary     Delete a saved meal; entries it logged stay on their days
// @Tags        nutrition
// @Param       id path string true "Saved meal id"
// @Param       expected_revision query int true "Current revision"
// @Param       Idempotency-Key header string true "Operation key"
// @Success     204
// @Failure     404 {object} map[string]string
// @Failure     409 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/nutrition/meals/{id} [delete]
func (h *SavedMealHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := foodIDParam(w, r, "saved meal not found")
	if !ok {
		return
	}
	revision, key, ok := deleteParams(w, r)
	if !ok {
		return
	}
	if _, err := h.svc.DeleteMeal(r.Context(), middleware.MustGetUserID(r.Context()), id, revision, key); err != nil {
		handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Log godoc
// @Summary     Log a saved meal to a day as one food entry per item, all or nothing
// @Description Optional items replace the saved items for this log only. Typed totals already on the day are converted into one quick_add entry (converted).
// @Tags        nutrition
// @Param       date path string true "Local calendar date YYYY-MM-DD"
// @Param       Idempotency-Key header string true "Equal to operation_key"
// @Param       body body model.LogMealRequest true "meal, optional items"
// @Success     201 {object} model.MealLogged
// @Failure     404 {object} map[string]string
// @Failure     409 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/nutrition/days/{date}/meal-logs [post]
func (h *SavedMealHandler) Log(w http.ResponseWriter, r *http.Request) {
	var req model.LogMealRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	logged, _, err := h.svc.LogMeal(r.Context(), middleware.MustGetUserID(r.Context()), chi.URLParam(r, "date"), req)
	respond(w, logged, err, http.StatusCreated)
}
