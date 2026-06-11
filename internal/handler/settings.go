package handler

import (
	"net/http"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

type SettingsHandler struct {
	svc service.SettingsService
}

func NewSettingsHandler(svc service.SettingsService) *SettingsHandler {
	return &SettingsHandler{svc: svc}
}

// Get godoc
// @Summary     Get user settings
// @Description Returns the authenticated user's settings (weight unit, weekly goal).
// @Tags        settings
// @Produce     json
// @Success     200 {object} model.UserSettings
// @Failure     401 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/settings [get]
func (h *SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	settings, err := h.svc.Get(r.Context(), userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, settings)
}

// Update godoc
// @Summary     Update user settings
// @Description Updates weight unit and weekly goal for the authenticated user.
// @Tags        settings
// @Accept      json
// @Produce     json
// @Param       body body model.UserSettings true "Settings payload"
// @Success     200 {object} model.UserSettings
// @Failure     400 {object} map[string]string
// @Failure     401 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/settings [put]
func (h *SettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	var req model.UserSettings
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST", nil)
		return
	}

	settings, err := h.svc.Update(r.Context(), userID, req)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, settings)
}
