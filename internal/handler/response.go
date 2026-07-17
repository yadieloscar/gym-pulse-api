package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type apiError struct {
	Error    string `json:"error"`
	Code     string `json:"code,omitempty"`
	Details  any    `json:"details,omitempty"`
	Resource any    `json:"resource,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message, code string, details any) {
	writeJSON(w, status, apiError{
		Error:   message,
		Code:    code,
		Details: details,
	})
}

func handleServiceError(w http.ResponseWriter, err error) {
	var notFound *model.NotFoundError
	if errors.As(err, &notFound) {
		writeError(w, http.StatusNotFound, notFound.Message, "NOT_FOUND", nil)
		return
	}

	var validation *model.ValidationError
	if errors.As(err, &validation) {
		writeError(w, http.StatusUnprocessableEntity, validation.Message, "VALIDATION_ERROR", map[string]string{
			"field": validation.Field,
		})
		return
	}

	var conflict *model.ConflictError
	if errors.As(err, &conflict) {
		code := "CONFLICT"
		var details any
		if conflict.Actual > 0 {
			code = "REVISION_CONFLICT"
			details = map[string]int64{"expected_revision": conflict.Expected, "actual_revision": conflict.Actual}
		} else if strings.Contains(conflict.Message, "idempotency") {
			code = "IDEMPOTENCY_CONFLICT"
		} else if strings.Contains(conflict.Message, "active session") {
			code = "ACTIVE_SESSION_CONFLICT"
		}
		writeJSON(w, http.StatusConflict, apiError{Error: conflict.Message, Code: code, Details: details, Resource: conflict.Authoritative})
		return
	}

	slog.Error("internal server error", "error", err)
	writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}
