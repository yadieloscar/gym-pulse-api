package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type apiError struct {
	Error   string      `json:"error"`
	Code    string      `json:"code,omitempty"`
	Details interface{} `json:"details,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message, code string, details interface{}) {
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
		writeError(w, http.StatusConflict, conflict.Message, "CONFLICT", nil)
		return
	}

	slog.Error("internal server error", "error", err)
	writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
}

func decodeJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close() //nolint:errcheck
	return json.NewDecoder(r.Body).Decode(v)
}
