package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type fakeExportService struct {
	export *model.DataExport
	err    error
	gotID  uuid.UUID
}

func (f *fakeExportService) Export(_ context.Context, userID uuid.UUID) (*model.DataExport, error) {
	f.gotID = userID
	return f.export, f.err
}

func TestExportHandler_ExportJSON(t *testing.T) {
	uid := uuid.New()
	export := model.NewDataExport(time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC))
	export.BodyWeights = append(export.BodyWeights, model.ExportBodyWeight{Date: "2026-11-01", Weight: 80.5, Unit: "kg"})
	svc := &fakeExportService{export: export}
	rec := httptest.NewRecorder()
	NewExportHandler(svc).Export(rec, newReq(t, http.MethodGet, "/api/v1/export", nil, uid))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if svc.gotID != uid {
		t.Fatal("export not scoped to the authenticated user")
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="gympulse-export-2026-11-03.json"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schema_version", "exported_at", "profile", "settings", "training_profile", "workouts", "templates", "sport_activities", "body_weights", "nutrition"} {
		if _, ok := body[key]; !ok {
			t.Errorf("missing top-level %q", key)
		}
	}
	// Empty collections are arrays, never null.
	if !strings.Contains(rec.Body.String(), `"workouts":[]`) || !strings.Contains(rec.Body.String(), `"food_entries":[]`) {
		t.Errorf("empty collections must encode as []: %s", rec.Body.String())
	}
}

func TestExportHandler_WorkoutSetsCSV(t *testing.T) {
	export := model.NewDataExport(time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC))
	export.Workouts = append(export.Workouts, model.ExportWorkout{
		Date: "2026-11-02", Name: "Legs", Source: model.ExportWorkoutSession, Status: "completed",
		Sets: []model.ExportSet{{ExerciseName: "Squat", ExerciseCategory: "legs", ExerciseModality: "strength", SetIndex: 1, Completed: true}},
	})
	rec := httptest.NewRecorder()
	NewExportHandler(&fakeExportService{export: export}).WorkoutSets(rec, newReq(t, http.MethodGet, "/api/v1/export/workout-sets", nil, uuid.New()))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/csv; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="gympulse-export-2026-11-03.csv"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "date,workout_name,") || !strings.Contains(lines[1], "Squat") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestExportHandler_ReadFailureIsStableJSONError(t *testing.T) {
	for name, call := range map[string]func(*ExportHandler, http.ResponseWriter, *http.Request){
		"json": (*ExportHandler).Export,
		"csv":  (*ExportHandler).WorkoutSets,
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			call(NewExportHandler(&fakeExportService{err: errors.New("db down")}), rec, newReq(t, http.MethodGet, "/", nil, uuid.New()))
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d", rec.Code)
			}
			if rec.Header().Get("Content-Disposition") != "" {
				t.Fatal("a failed export must not be offered as a file")
			}
			var body apiError
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Code == "" {
				t.Fatalf("body = %q", rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "db down") {
				t.Fatal("internal cause leaked to the client")
			}
		})
	}
}
