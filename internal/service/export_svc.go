package service

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/dao"
	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// WorkoutSetsCSVHeader is the first row of the workout-sets CSV. Columns are
// only ever appended so spreadsheets built on the export keep working.
var WorkoutSetsCSVHeader = []string{
	"date", "workout_name", "workout_source", "workout_status", "planned",
	"exercise_name", "exercise_category", "exercise_modality", "set_index",
	"target_reps", "target_weight", "actual_reps", "actual_weight", "weight_unit",
	"duration_seconds", "completed", "extra", "workout_notes",
}

// ExportService builds an athlete's data export.
type ExportService interface {
	Export(ctx context.Context, userID uuid.UUID) (*model.DataExport, error)
}

type exportService struct {
	repo dao.ExportDAO
	now  func() time.Time
}

// NewExportService creates a new ExportService.
func NewExportService(repo dao.ExportDAO) ExportService {
	return &exportService{repo: repo, now: time.Now}
}

func (s *exportService) Export(ctx context.Context, userID uuid.UUID) (*model.DataExport, error) {
	export, err := s.repo.Export(ctx, userID, s.now().UTC().Truncate(time.Second))
	if err != nil {
		return nil, fmt.Errorf("exporting user data: %w", err)
	}
	// Sessions and legacy day logs are read separately; the athlete sees one
	// history, oldest first.
	slices.SortStableFunc(export.Workouts, func(a, b model.ExportWorkout) int {
		return strings.Compare(a.Date, b.Date)
	})
	return export, nil
}

// WriteWorkoutSetsCSV writes one row per recorded set, oldest workout first.
// A workout with no sets still gets one row so its date and notes survive.
func WriteWorkoutSetsCSV(w io.Writer, export *model.DataExport) error {
	unit := ""
	if export.Settings != nil {
		unit = export.Settings.WeightUnit
	}
	out := csv.NewWriter(w)
	if err := out.Write(WorkoutSetsCSVHeader); err != nil {
		return fmt.Errorf("writing csv header: %w", err)
	}
	for _, workout := range export.Workouts {
		if workout.Status == "rest" {
			continue
		}
		prefix := []string{
			workout.Date, csvText(workout.Name), string(workout.Source), workout.Status, strconv.FormatBool(workout.Planned),
		}
		notes := csvText(deref(workout.Notes))
		if len(workout.Sets) == 0 {
			row := append(slices.Clone(prefix), "", "", "", "", "", "", "", "", "", "", "", "", notes)
			if err := out.Write(row); err != nil {
				return fmt.Errorf("writing csv row: %w", err)
			}
			continue
		}
		for _, set := range workout.Sets {
			row := append(slices.Clone(prefix),
				csvText(set.ExerciseName), csvText(set.ExerciseCategory), set.ExerciseModality,
				strconv.Itoa(set.SetIndex),
				csvInt(set.TargetReps), csvFloat(set.TargetWeight),
				csvInt(set.ActualReps), csvFloat(set.ActualWeight), weightUnit(set, unit),
				csvInt(set.DurationSeconds),
				strconv.FormatBool(set.Completed), strconv.FormatBool(set.Extra), notes,
			)
			if err := out.Write(row); err != nil {
				return fmt.Errorf("writing csv row: %w", err)
			}
		}
	}
	out.Flush()
	if err := out.Error(); err != nil {
		return fmt.Errorf("flushing csv: %w", err)
	}
	return nil
}

// weightUnit names the unit only on rows that carry a weight.
func weightUnit(set model.ExportSet, unit string) string {
	if set.TargetWeight == nil && set.ActualWeight == nil {
		return ""
	}
	return unit
}

// csvText neutralizes athlete-authored text that a spreadsheet would
// otherwise evaluate as a formula (CWE-1236).
func csvText(value string) string {
	if value != "" && strings.ContainsRune("=+-@\t\r", rune(value[0])) {
		return "'" + value
	}
	return value
}

func csvInt(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

func csvFloat(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', -1, 64)
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
