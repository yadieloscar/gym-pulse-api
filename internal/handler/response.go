package handler

import (
	"encoding/json"
	"errors"
	"io"
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
		switch {
		case conflict.Actual > 0:
			code = "REVISION_CONFLICT"
			details = map[string]int64{"expected_revision": conflict.Expected, "actual_revision": conflict.Actual}
		case strings.Contains(conflict.Message, "idempotency"):
			code = "IDEMPOTENCY_CONFLICT"
		case strings.Contains(conflict.Message, "active session"):
			code = "ACTIVE_SESSION_CONFLICT"
		}
		writeJSON(w, http.StatusConflict, apiError{Error: conflict.Message, Code: code, Details: details, Resource: conflict.Authoritative})
		return
	}

	slog.Error("internal server error", "error", err)
	writeError(w, http.StatusInternalServerError, "internal server error", "INTERNAL_ERROR", nil)
}

const maxJSONBodyBytes = 1 << 20

var errTrailingJSONValue = errors.New("request body must contain one JSON value")

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(v); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errTrailingJSONValue
		}
		return err
	}
	return nil
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large", "REQUEST_TOO_LARGE", map[string]int64{"max_bytes": maxJSONBodyBytes})
		return
	}
	writeError(w, http.StatusBadRequest, "invalid request body", "BAD_REQUEST", nil)
}
