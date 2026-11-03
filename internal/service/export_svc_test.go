package service

import (
	"context"
	"encoding/csv"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type fakeExportDAO struct {
	export *model.DataExport
	err    error
	gotID  uuid.UUID
	gotAt  time.Time
}

func (f *fakeExportDAO) Export(_ context.Context, userID uuid.UUID, exportedAt time.Time) (*model.DataExport, error) {
	f.gotID, f.gotAt = userID, exportedAt
	if f.err != nil {
		return nil, f.err
	}
	f.export.ExportedAt = exportedAt
	return f.export, nil
}

func floatPtr(v float64) *float64 { return &v }

func TestExportService_ScopesToUserAndOrdersWorkoutsByDate(t *testing.T) {
	userID := uuid.New()
	export := model.NewDataExport(time.Time{})
	export.Workouts = []model.ExportWorkout{
		{Name: "session late", Date: "2026-03-02", Source: model.ExportWorkoutSession},
		{Name: "session early", Date: "2026-01-05", Source: model.ExportWorkoutSession},
		{Name: "legacy same day", Date: "2026-03-02", Source: model.ExportWorkoutDayLog},
	}
	repo := &fakeExportDAO{export: export}
	fixed := time.Date(2026, 11, 3, 14, 5, 6, 789, time.FixedZone("x", 3600))
	svc := &exportService{repo: repo, now: func() time.Time { return fixed }}

	got, err := svc.Export(t.Context(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if repo.gotID != userID {
		t.Fatalf("export read for %s, want the authenticated user %s", repo.gotID, userID)
	}
	if want := time.Date(2026, 11, 3, 13, 5, 6, 0, time.UTC); !repo.gotAt.Equal(want) || repo.gotAt.Location() != time.UTC {
		t.Fatalf("exported_at = %v, want %v in UTC", repo.gotAt, want)
	}
	var names []string
	for _, w := range got.Workouts {
		names = append(names, w.Name)
	}
	if strings.Join(names, ",") != "session early,session late,legacy same day" {
		t.Fatalf("workouts out of order: %v", names)
	}
	if got.SchemaVersion != model.ExportSchemaVersion {
		t.Fatalf("schema_version = %d", got.SchemaVersion)
	}
}

func TestExportService_WrapsReadFailure(t *testing.T) {
	cause := errors.New("connection reset")
	svc := NewExportService(&fakeExportDAO{err: cause})
	if _, err := svc.Export(t.Context(), uuid.New()); !errors.Is(err, cause) {
		t.Fatalf("got %v, want wrapped %v", err, cause)
	}
}

func TestWriteWorkoutSetsCSV(t *testing.T) {
	export := model.NewDataExport(time.Now())
	export.Settings = &model.ExportSettings{WeightUnit: "kg", WeeklyGoal: 3}
	export.Workouts = []model.ExportWorkout{
		{
			Date: "2026-10-01", Name: "Push, heavy", Source: model.ExportWorkoutSession, Status: "completed", Planned: true,
			Notes: stringPtr("=HYPERLINK(\"x\")"),
			Sets: []model.ExportSet{
				{ExerciseName: "Bench Press", ExerciseCategory: "push", ExerciseModality: "strength", SetIndex: 1,
					TargetReps: intPtr(5), TargetWeight: floatPtr(80), ActualReps: intPtr(5), ActualWeight: floatPtr(82.5), Completed: true},
				{ExerciseName: "-Dips", ExerciseCategory: "push", ExerciseModality: "strength", SetIndex: 1,
					ActualReps: intPtr(10), Completed: true, Extra: true},
			},
		},
		{Date: "2026-10-02", Name: "rest", Source: model.ExportWorkoutDayLog, Status: "rest"},
		{Date: "2026-10-03", Name: "Row", Source: model.ExportWorkoutDayLog, Status: "logged",
			Sets: []model.ExportSet{{ExerciseName: "Rower", ExerciseCategory: "cardio", ExerciseModality: "cardio", SetIndex: 1, DurationSeconds: intPtr(600), Completed: true}}},
		{Date: "2026-10-04", Name: "Empty", Source: model.ExportWorkoutSession, Status: "active", Notes: stringPtr("felt off")},
	}

	var b strings.Builder
	if err := WriteWorkoutSetsCSV(&b, export); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(b.String())).ReadAll()
	if err != nil {
		t.Fatalf("output is not valid CSV: %v", err)
	}
	if strings.Join(rows[0], ",") != strings.Join(WorkoutSetsCSVHeader, ",") {
		t.Fatalf("header = %v", rows[0])
	}
	if len(rows) != 5 {
		t.Fatalf("got %d rows, want header + 4 (rest day skipped): %v", len(rows), rows)
	}
	col := map[string]int{}
	for i, name := range WorkoutSetsCSVHeader {
		col[name] = i
	}
	for _, row := range rows {
		if len(row) != len(WorkoutSetsCSVHeader) {
			t.Fatalf("row has %d columns, want %d: %v", len(row), len(WorkoutSetsCSVHeader), row)
		}
	}
	bench := rows[1]
	for name, want := range map[string]string{
		"date": "2026-10-01", "workout_name": "Push, heavy", "workout_source": "session", "planned": "true",
		"exercise_name": "Bench Press", "set_index": "1", "target_reps": "5", "target_weight": "80",
		"actual_reps": "5", "actual_weight": "82.5", "weight_unit": "kg", "completed": "true", "extra": "false",
		"workout_notes": "'=HYPERLINK(\"x\")",
	} {
		if bench[col[name]] != want {
			t.Errorf("bench %s = %q, want %q", name, bench[col[name]], want)
		}
	}
	dips := rows[2]
	if dips[col["exercise_name"]] != "'-Dips" || dips[col["weight_unit"]] != "" || dips[col["extra"]] != "true" {
		t.Errorf("dips row = %v", dips)
	}
	rower := rows[3]
	if rower[col["duration_seconds"]] != "600" || rower[col["workout_source"]] != "day_log" {
		t.Errorf("rower row = %v", rower)
	}
	empty := rows[4]
	if empty[col["workout_name"]] != "Empty" || empty[col["exercise_name"]] != "" || empty[col["workout_notes"]] != "felt off" {
		t.Errorf("set-less workout row = %v", empty)
	}
}

func TestWriteWorkoutSetsCSV_EmptyExportIsHeaderOnly(t *testing.T) {
	var b strings.Builder
	if err := WriteWorkoutSetsCSV(&b, model.NewDataExport(time.Now())); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(b.String()); got != strings.Join(WorkoutSetsCSVHeader, ",") {
		t.Fatalf("got %q", got)
	}
}
