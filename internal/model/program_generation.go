package model

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Program generation builds an unsaved program draft from the training
// profile. The draft mirrors the custom-program create body so the app can
// submit it unchanged; see docs/CONTRACTS.md "Program generation".

const (
	// GenerationDisabled says no provider key is configured.
	GenerationDisabled = "disabled"
	// GenerationUnavailable covers provider errors, timeouts, refusals, and
	// drafts that failed validation.
	GenerationUnavailable = "unavailable"

	// ProgramGenerationOrigin marks a draft's roadmap provenance.
	ProgramGenerationOrigin = "generated"

	maxDraftWorkouts         = 6
	minDraftExercises        = 3
	maxDraftExercises        = 8
	maxDraftNameRunes        = 80
	maxDraftRationaleRunes   = 500
	maxDraftNotesRunes       = 200
	maxGenerationRequestLen  = 200
	defaultDraftReps         = 10
	defaultDraftDurationSecs = 600
	defaultDraftRestSecs     = 90
)

// GenerationLimitError says the athlete already received the day's drafts.
type GenerationLimitError struct {
	Limit    int
	ResetsAt time.Time
}

func (e *GenerationLimitError) Error() string { return "daily program generation limit reached" }

// GenerationUnavailableError says no draft could be produced. Reason is
// GenerationDisabled or GenerationUnavailable.
type GenerationUnavailableError struct{ Reason string }

func (e *GenerationUnavailableError) Error() string { return "program generation is unavailable" }

// InvalidDraftError says the provider's answer could not be repaired into a
// valid draft. The service maps it to GenerationUnavailable.
type InvalidDraftError struct{ Message string }

func (e *InvalidDraftError) Error() string { return "invalid generated draft: " + e.Message }

// GenerationStatus is the GET /program-generation answer.
type GenerationStatus struct {
	Enabled        bool       `json:"enabled"`
	Limit          int        `json:"limit"`
	RemainingToday int        `json:"remaining_today"`
	ResetsAt       *time.Time `json:"resets_at"`
}

// GenerateProgramRequest is the optional POST body.
type GenerateProgramRequest struct {
	Request *string `json:"request"`
}

// GenerateProgramResponse carries the unsaved draft.
type GenerateProgramResponse struct {
	Draft          ProgramDraft `json:"draft"`
	Rationale      string       `json:"rationale"`
	Request        *string      `json:"request"`
	RemainingToday int          `json:"remaining_today"`
	ResetsAt       time.Time    `json:"resets_at"`
}

// ProgramDraft is the custom-program create body without operation_key and
// without server ids.
type ProgramDraft struct {
	Name        string         `json:"name"`
	PrimaryGoal string         `json:"primary_goal"`
	Roadmap     map[string]any `json:"roadmap"`
	Workouts    []DraftWorkout `json:"workouts"`
}

type DraftWorkout struct {
	Kind             UnitKind        `json:"kind"`
	Name             string          `json:"name"`
	PreferredWeekday int             `json:"preferred_weekday"`
	SequencePosition int             `json:"sequence_position"`
	Exercises        []DraftExercise `json:"exercises"`
}

type DraftExercise struct {
	CatalogID             uuid.UUID `json:"catalog_id"`
	Name                  string    `json:"name"`
	Category              string    `json:"category"`
	Modality              string    `json:"modality"`
	ExerciseOrder         int       `json:"exercise_order"`
	TargetSets            int       `json:"target_sets"`
	TargetReps            *int      `json:"target_reps"`
	TargetWeight          *float64  `json:"target_weight"`
	TargetDurationSeconds *int      `json:"target_duration_seconds"`
	RestSeconds           int       `json:"rest_seconds"`
	Notes                 *string   `json:"notes"`
}

// ProgramWorkouts converts the draft into the create-request shape.
func (d ProgramDraft) ProgramWorkouts() []ProgramWorkout {
	workouts := make([]ProgramWorkout, 0, len(d.Workouts))
	for _, w := range d.Workouts {
		weekday := w.PreferredWeekday
		exercises := make([]ProgramExercise, 0, len(w.Exercises))
		for _, e := range w.Exercises {
			catalogID := e.CatalogID
			rest := e.RestSeconds
			exercises = append(exercises, ProgramExercise{
				CatalogID: &catalogID, Name: e.Name, Category: e.Category, Modality: e.Modality,
				ExerciseOrder: e.ExerciseOrder, TargetSets: e.TargetSets, TargetReps: e.TargetReps,
				TargetWeight: e.TargetWeight, TargetDurationSeconds: e.TargetDurationSeconds,
				RestSeconds: &rest, Notes: e.Notes,
			})
		}
		workouts = append(workouts, ProgramWorkout{Kind: w.Kind, Name: w.Name, PreferredWeekday: &weekday, SequencePosition: w.SequencePosition, Exercises: exercises})
	}
	return workouts
}

// RawGeneratedDraft is the provider's JSON answer before repair.
type RawGeneratedDraft struct {
	Name      string                `json:"name"`
	Rationale string                `json:"rationale"`
	Workouts  []RawGeneratedWorkout `json:"workouts"`
}

type RawGeneratedWorkout struct {
	Name      string                 `json:"name"`
	Exercises []RawGeneratedExercise `json:"exercises"`
}

type RawGeneratedExercise struct {
	CatalogID             string  `json:"catalog_id"`
	TargetSets            int     `json:"target_sets"`
	TargetReps            *int    `json:"target_reps"`
	TargetDurationSeconds *int    `json:"target_duration_seconds"`
	RestSeconds           *int    `json:"rest_seconds"`
	Notes                 *string `json:"notes"`
}

// GeneratedProgram is a repaired draft with its rationale.
type GeneratedProgram struct {
	Draft     ProgramDraft
	Rationale string
}

// NormalizeGenerationRequest trims the optional steer and bounds it to 1..200
// characters. Nil stays nil.
func NormalizeGenerationRequest(request *string) (*string, error) {
	if request == nil {
		return request, nil // absent stays absent; the nil pointer is the value
	}
	trimmed := strings.TrimSpace(*request)
	if length := utf8.RuneCountInString(trimmed); length < 1 || length > maxGenerationRequestLen {
		return nil, &ValidationError{Message: "request must be between 1 and 200 characters", Field: "request"}
	}
	return &trimmed, nil
}

// DraftWorkoutCap is how many workouts a draft may hold for the profile.
func DraftWorkoutCap(availableDays []int) int {
	return min(len(availableDays), maxDraftWorkouts)
}

// GenerationSchema is the JSON schema the provider must answer with. Every
// catalog_id is an enum of the catalog so an invented exercise cannot parse.
func GenerationSchema(catalogIDs []string, workoutCap int) map[string]any {
	ids := make([]any, 0, len(catalogIDs))
	for _, id := range catalogIDs {
		ids = append(ids, id)
	}
	exercise := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"catalog_id", "target_sets", "target_reps", "target_duration_seconds", "rest_seconds", "notes"},
		"properties": map[string]any{
			"catalog_id":              map[string]any{"type": "string", "enum": ids},
			"target_sets":             map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
			"target_reps":             map[string]any{"type": []any{"integer", "null"}, "minimum": 1, "maximum": 50},
			"target_duration_seconds": map[string]any{"type": []any{"integer", "null"}, "minimum": 30, "maximum": 3600},
			"rest_seconds":            map[string]any{"type": "integer", "minimum": 0, "maximum": 600},
			"notes":                   map[string]any{"type": []any{"string", "null"}, "maxLength": maxDraftNotesRunes},
		},
	}
	workout := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"name", "exercises"},
		"properties": map[string]any{
			"name":      map[string]any{"type": "string", "minLength": 1, "maxLength": maxDraftNameRunes},
			"exercises": map[string]any{"type": "array", "minItems": minDraftExercises, "maxItems": maxDraftExercises, "items": exercise},
		},
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"name", "rationale", "workouts"},
		"properties": map[string]any{
			"name":      map[string]any{"type": "string", "minLength": 1, "maxLength": maxDraftNameRunes},
			"rationale": map[string]any{"type": "string", "maxLength": maxDraftRationaleRunes},
			"workouts":  map[string]any{"type": "array", "minItems": 1, "maxItems": workoutCap, "items": workout},
		},
	}
}

// RepairGeneratedDraft validates the provider's answer against the profile
// and catalog, repairs what is safe (unknown or duplicate exercises, surplus
// workouts, out-of-range targets), and rejects the rest.
func RepairGeneratedDraft(profile *TrainingProfile, catalog []CatalogExercise, raw *RawGeneratedDraft, modelID string, request *string) (*GeneratedProgram, error) {
	if profile == nil || raw == nil {
		return nil, &InvalidDraftError{Message: "missing profile or draft"}
	}
	byID := make(map[uuid.UUID]CatalogExercise, len(catalog))
	for _, entry := range catalog {
		byID[entry.ID] = entry
	}
	days := slices.Clone(profile.AvailableDays)
	slices.Sort(days)
	days = slices.Compact(days)
	limit := DraftWorkoutCap(days)
	if limit == 0 {
		return nil, &InvalidDraftError{Message: "profile has no available days"}
	}
	if len(raw.Workouts) == 0 {
		return nil, &InvalidDraftError{Message: "draft has no workouts"}
	}
	rawWorkouts := raw.Workouts
	if len(rawWorkouts) > limit {
		rawWorkouts = rawWorkouts[:limit]
	}

	workouts := make([]DraftWorkout, 0, len(rawWorkouts))
	for i, rw := range rawWorkouts {
		seen := map[uuid.UUID]bool{}
		exercises := make([]DraftExercise, 0, len(rw.Exercises))
		for _, re := range rw.Exercises {
			id, err := uuid.Parse(strings.TrimSpace(re.CatalogID))
			if err != nil {
				continue
			}
			entry, ok := byID[id]
			if !ok || seen[id] {
				continue
			}
			seen[id] = true
			exercises = append(exercises, repairExercise(entry, re, len(exercises)+1))
			if len(exercises) == maxDraftExercises {
				break
			}
		}
		if len(exercises) < minDraftExercises {
			return nil, &InvalidDraftError{Message: fmt.Sprintf("workout %d has fewer than %d catalog exercises", i+1, minDraftExercises)}
		}
		name := truncateRunes(strings.TrimSpace(rw.Name), maxDraftNameRunes)
		if name == "" {
			name = fmt.Sprintf("Workout %d", i+1)
		}
		workouts = append(workouts, DraftWorkout{Kind: UnitKindExercises, Name: name, PreferredWeekday: days[i], SequencePosition: i + 1, Exercises: exercises})
	}

	name := truncateRunes(strings.TrimSpace(raw.Name), maxDraftNameRunes)
	if name == "" {
		name = "Generated program"
	}
	roadmap := map[string]any{"origin": ProgramGenerationOrigin, "model": modelID, "request": nil}
	if request != nil {
		roadmap["request"] = *request
	}
	return &GeneratedProgram{
		Draft:     ProgramDraft{Name: name, PrimaryGoal: profile.PrimaryGoal, Roadmap: roadmap, Workouts: workouts},
		Rationale: truncateRunes(strings.TrimSpace(raw.Rationale), maxDraftRationaleRunes),
	}, nil
}

func repairExercise(entry CatalogExercise, re RawGeneratedExercise, order int) DraftExercise {
	exercise := DraftExercise{
		CatalogID: entry.ID, Name: entry.Name, Category: entry.Category, Modality: entry.Modality,
		ExerciseOrder: order, TargetSets: clampInt(re.TargetSets, 1, 10), RestSeconds: defaultDraftRestSecs,
	}
	if re.RestSeconds != nil {
		exercise.RestSeconds = clampInt(*re.RestSeconds, 0, 600)
	}
	switch entry.Modality {
	case "cardio":
		duration := defaultDraftDurationSecs
		if re.TargetDurationSeconds != nil {
			duration = clampInt(*re.TargetDurationSeconds, 30, 3600)
		}
		exercise.TargetDurationSeconds = &duration
	default:
		reps := defaultDraftReps
		if re.TargetReps != nil {
			reps = clampInt(*re.TargetReps, 1, 50)
		}
		exercise.TargetReps = &reps
	}
	if re.Notes != nil {
		if notes := truncateRunes(strings.TrimSpace(*re.Notes), maxDraftNotesRunes); notes != "" {
			exercise.Notes = &notes
		}
	}
	return exercise
}

func clampInt(value, low, high int) int {
	return max(low, min(high, value))
}

func truncateRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:limit]))
}

// LocalDate is the athlete's civil date in the profile timezone.
func LocalDate(now time.Time, timezone string) (string, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return "", &ValidationError{Message: "invalid saved timezone", Field: "timezone"}
	}
	return now.In(location).Format("2006-01-02"), nil
}

// NextLocalMidnight is when the daily generation count resets.
func NextLocalMidnight(now time.Time, timezone string) (time.Time, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, &ValidationError{Message: "invalid saved timezone", Field: "timezone"}
	}
	local := now.In(location)
	return time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, location), nil
}

// Generation attempt statuses stored in program_generations.
const (
	GenerationPending   = "pending"
	GenerationSucceeded = "succeeded"
	GenerationFailed    = "failed"
)
