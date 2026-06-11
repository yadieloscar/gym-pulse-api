package handler

import (
	"net/http"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// ProfileHandler handles profile-related HTTP requests.
type ProfileHandler struct {
	svc service.ProfileService
}

// NewProfileHandler creates a new ProfileHandler.
func NewProfileHandler(svc service.ProfileService) *ProfileHandler {
	return &ProfileHandler{svc: svc}
}

// Get godoc
// @Summary     Get user profile
// @Description Returns the authenticated user's profile (display name, avatar, onboarding status).
// @Tags        profile
// @Produce     json
// @Success     200 {object} model.UserProfile
// @Failure     401 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/profile [get]
func (h *ProfileHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	profile, err := h.svc.Get(r.Context(), userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, profile)
}

// Update godoc
// @Summary     Update user profile
// @Description Updates display name and avatar URL for the authenticated user. Sets onboarding_completed to true.
// @Tags        profile
// @Accept      json
// @Produce     json
// @Param       body body model.UpdateProfileRequest true "Profile update payload"
// @Success     200 {object} model.UserProfile
// @Failure     400 {object} map[string]string
// @Failure     401 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/profile [put]
func (h *ProfileHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	var req model.UpdateProfileRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST", nil)
		return
	}

	profile, err := h.svc.Update(r.Context(), userID, req)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, profile)
}
