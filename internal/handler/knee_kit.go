package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// KneeKitHandler serves the daily knee checklist and pain log.
type KneeKitHandler struct{ svc service.KneeKitService }

// NewKneeKitHandler returns a KneeKitHandler.
func NewKneeKitHandler(svc service.KneeKitService) *KneeKitHandler {
	return &KneeKitHandler{svc: svc}
}

// SaveDay godoc
// @Summary     Create or replace one day's knee checklist and pain log
// @Description Full replacement of the date's record; expected_revision is 0 when none exists.
// @Description See docs/CONTRACTS.md "Knee Kit days".
// @Tags        knee-kit
// @Param       date path string true "Local calendar date YYYY-MM-DD"
// @Param       Idempotency-Key header string true "Equal to operation_key"
// @Param       body body model.SaveKneeKitDayRequest true "Checklist items and pain values"
// @Success     201 {object} model.KneeKitDay
// @Success     200 {object} model.KneeKitDay
// @Failure     404 {object} map[string]string
// @Failure     409 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/knee-kit/days/{date} [put]
func (h *KneeKitHandler) SaveDay(w http.ResponseWriter, r *http.Request) {
	var req model.SaveKneeKitDayRequest
	if !decodeMutation(w, r, &req, "") || !matchIdempotencyKey(w, r, req.OperationKey) {
		return
	}
	day, outcome, err := h.svc.SaveDay(r.Context(), middleware.MustGetUserID(r.Context()), chi.URLParam(r, "date"), req)
	status := http.StatusOK
	if outcome == model.KneeKitDayCreated {
		status = http.StatusCreated
	}
	respond(w, day, err, status)
}

// ListDays godoc
// @Summary     List knee checklist and pain-log days in a range
// @Tags        knee-kit
// @Param       from query string true "YYYY-MM-DD"
// @Param       to query string true "YYYY-MM-DD (at most 366 days after from)"
// @Success     200 {object} model.KneeKitRange
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/knee-kit/days [get]
func (h *KneeKitHandler) ListDays(w http.ResponseWriter, r *http.Request) {
	days, err := h.svc.ListDays(r.Context(), middleware.MustGetUserID(r.Context()),
		r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	respond(w, days, err, http.StatusOK)
}
