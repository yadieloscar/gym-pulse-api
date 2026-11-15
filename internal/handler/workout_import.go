package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

const maxImportMultipartBytes = service.MaxImportFileBytes + 64<<10

// maxImportFieldBytes bounds each non-file form field.
const maxImportFieldBytes = 256

// WorkoutImportHandler serves Strong and Hevy CSV imports.
type WorkoutImportHandler struct {
	svc service.WorkoutImportService
}

// NewWorkoutImportHandler returns a WorkoutImportHandler.
func NewWorkoutImportHandler(svc service.WorkoutImportService) *WorkoutImportHandler {
	return &WorkoutImportHandler{svc: svc}
}

type importForm struct {
	file         []byte
	opts         model.ImportOptions
	operationKey string
}

// Preview godoc
// @Summary     Preview a Strong or Hevy CSV import
// @Description Parses the file and reports what an import would write. Writes nothing.
// @Tags        imports
// @Accept      multipart/form-data
// @Produce     json
// @Param       file formData file true "Strong or Hevy CSV export (max 10 MiB)"
// @Param       source_weight_unit formData string false "lb or kg when the file states no unit"
// @Success     200 {object} model.ImportPreview
// @Failure     413 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/imports/preview [post]
func (h *WorkoutImportHandler) Preview(w http.ResponseWriter, r *http.Request) {
	form, ok := readImportForm(w, r)
	if !ok {
		return
	}
	preview, err := h.svc.Preview(r.Context(), middleware.MustGetUserID(r.Context()), form.file, form.opts)
	respond(w, preview, err, http.StatusOK)
}

// Commit godoc
// @Summary     Import a Strong or Hevy CSV
// @Description Writes one day log per new date in one transaction. An identical retry returns the original import with 200.
// @Tags        imports
// @Accept      multipart/form-data
// @Produce     json
// @Param       Idempotency-Key header string true "Equal to operation_key"
// @Param       file formData file true "Strong or Hevy CSV export (max 10 MiB)"
// @Param       unmatched formData string true "keep or skip"
// @Param       source_weight_unit formData string false "lb or kg when the file states no unit"
// @Param       operation_key formData string true "Stable client operation key"
// @Success     201 {object} model.WorkoutImport
// @Failure     409 {object} map[string]string
// @Failure     422 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/imports [post]
func (h *WorkoutImportHandler) Commit(w http.ResponseWriter, r *http.Request) {
	form, ok := readImportForm(w, r)
	if !ok || !matchIdempotencyKey(w, r, form.operationKey) {
		return
	}
	imp, outcome, err := h.svc.Commit(r.Context(), middleware.MustGetUserID(r.Context()), form.file, form.opts, form.operationKey)
	status := http.StatusCreated
	if outcome == model.ImportReplayed {
		status = http.StatusOK
	}
	respond(w, imp, err, status)
}

// List godoc
// @Summary     List workout imports
// @Tags        imports
// @Produce     json
// @Success     200 {object} map[string][]model.WorkoutImport
// @Security    BearerAuth
// @Router      /api/v1/imports [get]
func (h *WorkoutImportHandler) List(w http.ResponseWriter, r *http.Request) {
	imports, err := h.svc.List(r.Context(), middleware.MustGetUserID(r.Context()))
	respond(w, map[string][]model.WorkoutImport{"imports": imports}, err, http.StatusOK)
}

// Delete godoc
// @Summary     Remove a workout import and every day it created
// @Tags        imports
// @Success     204
// @Failure     404 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/imports/{id} [delete]
func (h *WorkoutImportHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), middleware.MustGetUserID(r.Context()), id); err != nil {
		handleServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeImportMultipartError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "import file too large", "REQUEST_TOO_LARGE", map[string]int{"max_bytes": service.MaxImportFileBytes})
		return
	}
	writeError(w, http.StatusBadRequest, "invalid multipart body", "BAD_REQUEST", nil)
}

// readImportForm reads exactly one file part plus the option fields.
func readImportForm(w http.ResponseWriter, r *http.Request) (importForm, bool) {
	var form importForm
	r.Body = http.MaxBytesReader(w, r.Body, maxImportMultipartBytes)
	reader, err := r.MultipartReader()
	if err != nil {
		writeImportMultipartError(w, err)
		return form, false
	}
	files := 0
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			writeImportMultipartError(w, err)
			return form, false
		}
		if part.FileName() != "" {
			files++
			if files > 1 || part.FormName() != "file" {
				part.Close()
				writeError(w, http.StatusBadRequest, "exactly one file is required", "BAD_REQUEST", nil)
				return form, false
			}
			form.file, err = io.ReadAll(io.LimitReader(part, service.MaxImportFileBytes+1))
			part.Close()
			if err != nil {
				writeImportMultipartError(w, err)
				return form, false
			}
			if len(form.file) > service.MaxImportFileBytes {
				writeError(w, http.StatusRequestEntityTooLarge, "import file too large", "REQUEST_TOO_LARGE", map[string]int{"max_bytes": service.MaxImportFileBytes})
				return form, false
			}
			continue
		}
		value, err := io.ReadAll(io.LimitReader(part, maxImportFieldBytes+1))
		part.Close()
		if err != nil {
			writeImportMultipartError(w, err)
			return form, false
		}
		if len(value) > maxImportFieldBytes {
			writeError(w, http.StatusBadRequest, "form field too large", "BAD_REQUEST", map[string]string{"field": part.FormName()})
			return form, false
		}
		switch part.FormName() {
		case "source_weight_unit":
			form.opts.SourceWeightUnit = string(value)
		case "unmatched":
			form.opts.Unmatched = string(value)
		case "operation_key":
			form.operationKey = string(value)
		}
	}
	if files != 1 || len(form.file) == 0 {
		writeError(w, http.StatusBadRequest, "exactly one file is required", "BAD_REQUEST", nil)
		return form, false
	}
	return form, true
}
