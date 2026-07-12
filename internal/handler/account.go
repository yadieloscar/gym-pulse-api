package handler

import (
	"net/http"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

type AccountHandler struct {
	svc service.AccountService
}

func NewAccountHandler(svc service.AccountService) *AccountHandler {
	return &AccountHandler{svc: svc}
}

// Delete godoc
// @Summary     Delete account
// @Description Permanently deletes the authenticated user's account: all
// @Description application data and, when the server is configured with
// @Description Supabase admin credentials, the auth user itself. Irreversible.
// @Tags        account
// @Success     204
// @Failure     401 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/account [delete]
func (h *AccountHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	if err := h.svc.Delete(r.Context(), userID); err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
