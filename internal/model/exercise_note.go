package model

import (
	"strings"
	"time"
)

// MaxExerciseNameLength is the longest exercise name, in characters, that a
// note can be saved for. It matches the longest exercise name a scheduled
// workout accepts.
const MaxExerciseNameLength = 120

// MaxExerciseNoteLength is the longest note in characters. A note is a short
// reminder such as "seat height 4", not a journal.
const MaxExerciseNoteLength = 200

// MaxExerciseNotes is how many notes one athlete may keep, so the list stays
// one bounded read.
const MaxExerciseNotes = 500

// ExerciseNote is the athlete's one private note for an exercise. ExerciseKey
// identifies the exercise across saved workouts, programs, and scheduled
// workouts; see ExerciseNoteKey. Note is "" only in the answer to a save
// that cleared the note. See docs/CONTRACTS.md "Exercise notes".
type ExerciseNote struct {
	ExerciseKey  string    `json:"exercise_key"`
	ExerciseName string    `json:"exercise_name"`
	Note         string    `json:"note"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ExerciseNotesResponse is the body of GET /exercise-notes. ExerciseNotes is
// never nil.
type ExerciseNotesResponse struct {
	ExerciseNotes []ExerciseNote `json:"exercise_notes"`
}

// SaveExerciseNoteRequest is the body of PUT /exercise-notes. An empty Note
// clears the athlete's note for the exercise.
type SaveExerciseNoteRequest struct {
	ExerciseName string `json:"exercise_name"`
	Note         string `json:"note"`
}

// CleanExerciseName returns name trimmed with each run of whitespace
// collapsed to one space.
func CleanExerciseName(name string) string {
	return strings.Join(strings.Fields(name), " ")
}

// ExerciseNoteKey returns the key a note is stored under for an exercise
// name: the cleaned name, lowercased. Names that differ only by case or
// spacing share a key.
func ExerciseNoteKey(name string) string {
	return strings.ToLower(CleanExerciseName(name))
}
