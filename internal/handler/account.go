package handler

import (
	"errors"
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
// @Description Permanently deletes the authenticated user's account:
// @Description avatar object, application data, and Supabase auth user.
// @Description Irreversible; incomplete deletion can be retried safely.
// @Tags        account
// @Success     204
// @Failure     401 {object} map[string]string
// @Failure     503 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/account [delete]
func (h *AccountHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())

	if err := h.svc.Delete(r.Context(), userID); err != nil {
		if errors.Is(err, service.ErrAccountDeletionIncomplete) {
			writeError(w, http.StatusServiceUnavailable, "account deletion temporarily unavailable", "ACCOUNT_DELETION_INCOMPLETE", nil)
			return
		}
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
