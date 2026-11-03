package handler

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gym-pulse/gym-pulse-api/internal/middleware"
	"github.com/gym-pulse/gym-pulse-api/internal/service"
)

// ExportHandler serves the athlete's data export.
type ExportHandler struct {
	svc service.ExportService
}

// NewExportHandler creates a new ExportHandler.
func NewExportHandler(svc service.ExportService) *ExportHandler {
	return &ExportHandler{svc: svc}
}

// Export godoc
// @Summary     Export all of the athlete's data
// @Description Returns every workout with its sets, saved workouts, sport activities, body weights, nutrition days, targets, and food log from one consistent snapshot.
// @Tags        export
// @Produce     json
// @Success     200 {object} model.DataExport
// @Failure     401 {object} map[string]string
// @Failure     429 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/export [get]
func (h *ExportHandler) Export(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())
	export, err := h.svc.Export(r.Context(), userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	w.Header().Set("Content-Disposition", attachment(export.ExportedAt.Format("2006-01-02"), "json"))
	writeJSON(w, http.StatusOK, export)
}

// WorkoutSets godoc
// @Summary     Export workout sets as CSV
// @Description Returns one CSV row per recorded set, oldest workout first, for spreadsheets and other training apps.
// @Tags        export
// @Produce     text/csv
// @Success     200 {string} string
// @Failure     401 {object} map[string]string
// @Failure     429 {object} map[string]string
// @Security    BearerAuth
// @Router      /api/v1/export/workout-sets [get]
func (h *ExportHandler) WorkoutSets(w http.ResponseWriter, r *http.Request) {
	userID := middleware.MustGetUserID(r.Context())
	export, err := h.svc.Export(r.Context(), userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	// Render fully before writing so a failure still answers with a JSON error
	// instead of a truncated file.
	var body bytes.Buffer
	if err := service.WriteWorkoutSetsCSV(&body, export); err != nil {
		handleServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", attachment(export.ExportedAt.Format("2006-01-02"), "csv"))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body.Bytes()); err != nil {
		slog.Error("failed to write export csv", "error", err)
	}
}

func attachment(date, extension string) string {
	return fmt.Sprintf(`attachment; filename="gympulse-export-%s.%s"`, date, extension)
}
