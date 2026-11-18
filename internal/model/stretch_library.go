package model

import "time"

// Limits on an athlete's own stretches. See docs/CONTRACTS.md "Stretch
// library".
const (
	MaxStretchMoves        = 50
	MaxStretchRoutines     = 30
	MaxStretchRoutineItems = 30
	MaxStretchNameLength   = 60
	MaxStretchCueLength    = 200
	MaxStretchAreasLength  = 60
	MinStretchHoldSeconds  = 5
	MaxStretchHoldSeconds  = 600
	MinStretchReps         = 1
	MaxStretchReps         = 100
	// OwnStretchIDPrefix starts the id of every move and routine an athlete
	// wrote, which keeps them apart from the ids the app bundles.
	OwnStretchIDPrefix = "my-"
)

// Stretch move modes.
const (
	StretchModeHold = "hold"
	StretchModeReps = "reps"
)

// StretchMove is one stretch the athlete wrote. A hold move carries
// HoldSeconds and no Reps; a reps move carries Reps and no HoldSeconds.
type StretchMove struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Cue         string `json:"cue"`
	Areas       string `json:"areas"`
	Mode        string `json:"mode"`
	Sided       bool   `json:"sided"`
	HoldSeconds *int   `json:"hold_seconds"`
	Reps        *int   `json:"reps"`
}

// StretchRoutineItem is one move in a routine. MoveID names one of the
// athlete's own moves or a move the app bundles. At most one of HoldSeconds
// and Reps is set; it replaces the move's own dose in this routine.
type StretchRoutineItem struct {
	MoveID      string `json:"move_id"`
	HoldSeconds *int   `json:"hold_seconds"`
	Reps        *int   `json:"reps"`
}

// StretchRoutine is an ordered list of moves the athlete put together.
type StretchRoutine struct {
	ID    string               `json:"id"`
	Name  string               `json:"name"`
	Items []StretchRoutineItem `json:"items"`
}

// StretchLibrary is everything one athlete wrote: their moves and routines.
// It is read and replaced whole. An athlete who never saved one has revision
// 0, empty lists, and a nil UpdatedAt. Moves and Routines are never nil.
type StretchLibrary struct {
	Revision  int64            `json:"revision"`
	Moves     []StretchMove    `json:"moves"`
	Routines  []StretchRoutine `json:"routines"`
	UpdatedAt *time.Time       `json:"updated_at"`
}

// EmptyStretchLibrary is the library of an athlete who never saved one.
func EmptyStretchLibrary() *StretchLibrary {
	return &StretchLibrary{Moves: []StretchMove{}, Routines: []StretchRoutine{}}
}

// SaveStretchLibraryRequest replaces the athlete's whole library.
// ExpectedRevision is the stored revision, 0 when no library exists. Moves
// and Routines must both be present; an empty list removes every entry.
type SaveStretchLibraryRequest struct {
	ExpectedRevision int64            `json:"expected_revision"`
	OperationKey     string           `json:"operation_key"`
	Moves            []StretchMove    `json:"moves"`
	Routines         []StretchRoutine `json:"routines"`
}
