package service

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// Import limits. A file is rejected before any write when it exceeds one.
const (
	MaxImportFileBytes = 10 << 20
	maxImportRows      = 100_000
	maxImportDays      = 5_000
)

// errImportValue marks a field that cannot be read as the expected value.
var errImportValue = errors.New("unreadable import value")

// importLayout is one published export header. Columns are matched as an
// exact set so a reordered export still parses, but an unknown or missing
// column is rejected rather than guessed. Sources are cited in
// specs/026-workout-history-import/spec.md.
type importLayout struct {
	name    string
	source  model.ImportSource
	columns []string
	// weightUnit is the unit the header states; "" means each row states it
	// (unitColumn) or the file states none.
	weightUnit   string
	weightColumn string
	unitColumn   string
	distance     string
}

var importLayouts = []importLayout{
	{name: "strong_seconds", source: model.ImportSourceStrong, weightUnit: "kg", weightColumn: "Weight (kg)", distance: "Distance (meters)",
		columns: []string{"Workout #", "Date", "Workout Name", "Duration (sec)", "Exercise Name", "Set Order", "Weight (kg)", "Reps", "RPE", "Distance (meters)", "Seconds", "Notes", "Workout Notes"}},
	{name: "strong_legacy", source: model.ImportSourceStrong, weightColumn: "Weight", distance: "Distance",
		columns: []string{"Date", "Workout Name", "Duration", "Exercise Name", "Set Order", "Weight", "Reps", "Distance", "Seconds", "Notes", "Workout Notes", "RPE"}},
	{name: "strong_units", source: model.ImportSourceStrong, weightColumn: "Weight", unitColumn: "Weight Unit", distance: "Distance",
		columns: []string{"Date", "Workout Name", "Exercise Name", "Set Order", "Weight", "Weight Unit", "Reps", "RPE", "Distance", "Distance Unit", "Seconds", "Notes", "Workout Notes", "Workout Duration"}},
	{name: "strong_units_old", source: model.ImportSourceStrong, weightColumn: "Weight", unitColumn: "Weight Unit", distance: "Distance",
		columns: []string{"Date", "Workout Name", "Exercise Name", "Set Order", "Weight", "Weight Unit", "Reps", "Distance", "Distance Unit", "Seconds", "Notes", "Workout Notes"}},
}

func init() {
	// Hevy names its weight and distance columns after the athlete's units,
	// independently of each other.
	for _, weight := range []struct{ column, unit string }{{"weight_kg", "kg"}, {"weight_lbs", "lb"}} {
		for _, distance := range []string{"distance_km", "distance_miles"} {
			importLayouts = append(importLayouts, importLayout{
				name: "hevy", source: model.ImportSourceHevy, weightUnit: weight.unit, weightColumn: weight.column, distance: distance,
				columns: []string{"title", "start_time", "end_time", "description", "exercise_title", "superset_id", "exercise_notes",
					"set_index", "set_type", weight.column, "reps", distance, "duration_seconds", "rpe"},
			})
		}
	}
}

// importRow is one performed set read from the file.
type importRow struct {
	line         int
	workoutKey   string
	start        time.Time // wall clock, in UTC
	end          *time.Time
	workoutName  string
	workoutNotes string
	exercise     string
	warmup       bool
	weight       *float64
	weightUnit   string // "" when the file states none
	reps         *int
	seconds      *int
	hasDistance  bool
}

// importFile is a parsed export.
type importFile struct {
	source     model.ImportSource
	weightUnit string // "" when rows state it or the file states none
	rowUnits   bool
	rows       []importRow
}

// parseImportFile reads a Strong or Hevy CSV export. It never guesses: an
// unknown header or an unreadable value fails the whole file.
func parseImportFile(data []byte) (*importFile, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	firstLine, _, _ := bytes.Cut(data, []byte("\n"))
	delimiter := ','
	if bytes.Count(firstLine, []byte(";")) > bytes.Count(firstLine, []byte(",")) {
		delimiter = ';'
	}
	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = delimiter
	reader.ReuseRecord = true
	header, err := reader.Read()
	if err != nil {
		return nil, unrecognizedImport()
	}
	layout, columns, ok := matchImportLayout(header)
	if !ok {
		return nil, unrecognizedImport()
	}
	reader.FieldsPerRecord = len(header)
	file := &importFile{source: layout.source, weightUnit: layout.weightUnit, rowUnits: layout.unitColumn != ""}
	parser := rowParser{layout: layout, columns: columns, decimalComma: delimiter == ';'}
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		line, _ := reader.FieldPos(0)
		if err != nil {
			return nil, invalidImportRow(line, "the row is not valid CSV")
		}
		if len(file.rows) >= maxImportRows {
			return nil, &model.ImportFileError{Code: model.ImportCodeTooLarge, Message: "the file has too many rows",
				Details: map[string]any{"max_rows": maxImportRows}}
		}
		row, keep, err := parser.parse(line, record)
		if err != nil {
			return nil, err
		}
		if keep {
			file.rows = append(file.rows, row)
		}
	}
	return file, nil
}

func matchImportLayout(header []string) (importLayout, map[string]int, bool) {
	names := make([]string, len(header))
	for i, name := range header {
		names[i] = strings.TrimSpace(name)
	}
	sorted := slices.Clone(names)
	slices.Sort(sorted)
	for _, layout := range importLayouts {
		want := slices.Clone(layout.columns)
		slices.Sort(want)
		if slices.Equal(sorted, want) {
			columns := make(map[string]int, len(names))
			for i, name := range names {
				columns[name] = i
			}
			return layout, columns, true
		}
	}
	return importLayout{}, nil, false
}

func unrecognizedImport() error {
	return &model.ImportFileError{
		Code:    model.ImportCodeUnrecognized,
		Message: "the file is not a Strong or Hevy workout export",
		Details: map[string]any{"expected": []string{
			"Strong: Settings > Export Strong Data",
			"Hevy: Settings > Export & Import Data > Export Workouts",
		}},
	}
}

func invalidImportRow(line int, reason string) error {
	return &model.ImportFileError{
		Code:    model.ImportCodeInvalidRow,
		Message: fmt.Sprintf("line %d: %s", line, reason),
		Details: map[string]any{"line": line, "reason": reason},
	}
}

type rowParser struct {
	layout       importLayout
	columns      map[string]int
	decimalComma bool
}

func (p rowParser) field(record []string, name string) string {
	return strings.TrimSpace(record[p.columns[name]])
}

// parse returns the row and whether it is a set. Rest-timer and note rows
// are not sets.
func (p rowParser) parse(line int, record []string) (importRow, bool, error) {
	row := importRow{line: line, weightUnit: p.layout.weightUnit}
	var setOrder string
	var err error
	if p.layout.source == model.ImportSourceHevy {
		row.workoutName = p.field(record, "title")
		row.workoutNotes = p.field(record, "description")
		row.exercise = p.field(record, "exercise_title")
		if row.start, err = parseHevyTime(p.field(record, "start_time")); err != nil {
			return row, false, invalidImportRow(line, "start_time is not a date")
		}
		if end, err := parseHevyTime(p.field(record, "end_time")); err == nil && !end.Before(row.start) {
			row.end = &end
		}
		row.workoutKey = p.field(record, "start_time") + "\x00" + row.workoutName
		switch p.field(record, "set_type") {
		case "normal", "failure", "dropset":
		case "warmup":
			row.warmup = true
		default:
			return row, false, invalidImportRow(line, "set_type is not normal, warmup, failure, or dropset")
		}
		if row.seconds, err = p.integer(p.field(record, "duration_seconds")); err != nil {
			return row, false, invalidImportRow(line, "duration_seconds is not a number")
		}
	} else {
		row.workoutName = p.field(record, "Workout Name")
		row.workoutNotes = p.field(record, "Workout Notes")
		row.exercise = p.field(record, "Exercise Name")
		date := p.field(record, "Date")
		if row.start, err = parseStrongTime(date); err != nil {
			return row, false, invalidImportRow(line, "Date is not a date")
		}
		row.workoutKey = date + "\x00" + row.workoutName
		if _, ok := p.columns["Workout #"]; ok {
			row.workoutKey += "\x00" + p.field(record, "Workout #")
		}
		if duration, ok := p.strongDuration(record); ok {
			end := row.start.Add(duration)
			row.end = &end
		}
		setOrder = p.field(record, "Set Order")
		switch setOrder {
		case "Rest Timer", "Note":
			return row, false, nil
		case "W":
			row.warmup = true
		case "D", "F":
		default:
			if _, err := strconv.Atoi(setOrder); err != nil {
				return row, false, invalidImportRow(line, "Set Order is not a set number, W, D, or F")
			}
		}
		if row.seconds, err = p.integer(p.field(record, "Seconds")); err != nil {
			return row, false, invalidImportRow(line, "Seconds is not a number")
		}
		if p.layout.unitColumn != "" {
			switch unit := p.field(record, p.layout.unitColumn); unit {
			case "kg":
				row.weightUnit = "kg"
			case "lbs", "lb":
				row.weightUnit = "lb"
			case "":
			default:
				return row, false, invalidImportRow(line, "Weight Unit is not kg or lbs")
			}
		}
	}
	if row.exercise == "" {
		return row, false, invalidImportRow(line, "the exercise name is empty")
	}
	if row.weight, err = p.decimal(p.field(record, p.layout.weightColumn)); err != nil {
		return row, false, invalidImportRow(line, "the weight is not a number")
	}
	if row.weight != nil && *row.weight < 0 {
		return row, false, invalidImportRow(line, "the weight is negative")
	}
	repsColumn := "Reps"
	if p.layout.source == model.ImportSourceHevy {
		repsColumn = "reps"
	}
	if row.reps, err = p.integer(p.field(record, repsColumn)); err != nil {
		return row, false, invalidImportRow(line, "reps is not a number")
	}
	distance, err := p.decimal(p.field(record, p.layout.distance))
	if err != nil {
		return row, false, invalidImportRow(line, "the distance is not a number")
	}
	row.hasDistance = distance != nil
	return row, true, nil
}

// decimal parses an optional non-negative number. Empty and zero are absent:
// Strong's legacy layout writes 0 for "not recorded".
func (p rowParser) decimal(value string) (*float64, error) {
	if value == "" {
		return nil, nil //nolint:nilnil // an empty field is absent, not an error
	}
	if p.decimalComma && strings.Count(value, ",") == 1 && !strings.Contains(value, ".") {
		value = strings.Replace(value, ",", ".", 1)
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil, err
	}
	if number == 0 {
		return nil, nil //nolint:nilnil // zero means not recorded
	}
	return &number, nil
}

// integer parses an optional whole count; "120.0" is accepted as 120.
func (p rowParser) integer(value string) (*int, error) {
	number, err := p.decimal(value)
	if err != nil || number == nil {
		return nil, err
	}
	if *number < 0 || *number > 1_000_000 || *number != float64(int(*number)) {
		return nil, fmt.Errorf("%w: %q is not a whole number", errImportValue, value)
	}
	whole := int(*number)
	return &whole, nil
}

var durationPart = regexp.MustCompile(`(\d+)\s*(h|min|m|s)`)

func (p rowParser) strongDuration(record []string) (time.Duration, bool) {
	if _, ok := p.columns["Duration (sec)"]; ok {
		seconds, err := p.integer(p.field(record, "Duration (sec)"))
		if err != nil || seconds == nil {
			return 0, false
		}
		return time.Duration(*seconds) * time.Second, true
	}
	column := "Duration"
	if _, ok := p.columns["Workout Duration"]; ok {
		column = "Workout Duration"
	} else if _, ok := p.columns[column]; !ok {
		return 0, false
	}
	var total time.Duration
	matches := durationPart.FindAllStringSubmatch(p.field(record, column), -1)
	for _, match := range matches {
		amount, _ := strconv.Atoi(match[1])
		switch match[2] {
		case "h":
			total += time.Duration(amount) * time.Hour
		case "m", "min":
			total += time.Duration(amount) * time.Minute
		case "s":
			total += time.Duration(amount) * time.Second
		}
	}
	return total, len(matches) > 0
}

func parseStrongTime(value string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%w: %q is not a Strong date", errImportValue, value)
}

func parseHevyTime(value string) (time.Time, error) {
	for _, layout := range []string{"2 Jan 2006, 15:04", "Jan 2, 2006, 3:04 PM", "2 Jan 2006 15:04"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%w: %q is not a Hevy date", errImportValue, value)
}
