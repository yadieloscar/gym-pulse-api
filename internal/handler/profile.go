package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// ProfileHandler handles profile-related HTTP requests.
type ProfileHandler struct {
	svc service.ProfileService
}

const maxAvatarBytes = 5 << 20
const maxAvatarMultipartBytes = maxAvatarBytes + 64<<10

func writeMultipartError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "avatar file too large", "REQUEST_TOO_LARGE", map[string]int{"max_bytes": maxAvatarBytes})
		return
	}
	writeError(w, http.StatusBadRequest, "invalid multipart body", "BAD_REQUEST", nil)
}

// UploadAvatar stores one authenticated user's durable JPEG or PNG avatar.
// @Summary Upload profile avatar
// @Tags profile
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "JPEG or PNG avatar (max 5 MiB)"
// @Success 200 {object} model.UserProfile
// @Router /api/v1/profile/avatar [put]
func (h *ProfileHandler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarMultipartBytes)
	reader, err := r.MultipartReader()
	if err != nil {
		writeMultipartError(w, err)
		return
	}
	var data []byte
	files := 0
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			writeMultipartError(w, err)
			return
		}
		if part.FileName() == "" {
			part.Close()
			continue
		}
		files++
		if files > 1 || part.FormName() != "file" {
			part.Close()
			writeError(w, http.StatusBadRequest, "exactly one file is required", "BAD_REQUEST", nil)
			return
		}
		data, err = io.ReadAll(io.LimitReader(part, maxAvatarBytes+1))
		part.Close()
		if err != nil {
			writeMultipartError(w, err)
			return
		}
		if len(data) > maxAvatarBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "avatar file too large", "REQUEST_TOO_LARGE", map[string]int{"max_bytes": maxAvatarBytes})
			return
		}
	}
	if files != 1 || len(data) == 0 {
		writeError(w, http.StatusBadRequest, "exactly one file is required", "BAD_REQUEST", nil)
		return
	}
	contentType := http.DetectContentType(data)
	if contentType != "image/jpeg" && contentType != "image/png" {
		writeError(w, http.StatusUnsupportedMediaType, "avatar must be JPEG or PNG", "UNSUPPORTED_MEDIA_TYPE", nil)
		return
	}
	profile, err := h.svc.UploadAvatar(r.Context(), middleware.MustGetUserID(r.Context()), contentType, data)
	if errors.Is(err, service.ErrAvatarStorageUnavailable) {
		writeError(w, http.StatusServiceUnavailable, "avatar storage unavailable", "STORAGE_UNAVAILABLE", nil)
		return
	}
	if errors.Is(err, service.ErrAvatarUploadFailed) {
		writeError(w, http.StatusBadGateway, "avatar upload failed", "STORAGE_UPLOAD_FAILED", nil)
		return
	}
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, profile)
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
// @Description Partially updates display name, avatar URL, or explicitly completes onboarding.
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
	if err := decodeJSON(w, r, &req); err != nil {
		writeDecodeError(w, err)
		return
	}

	profile, err := h.svc.Update(r.Context(), userID, req)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, profile)
}
