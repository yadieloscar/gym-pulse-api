package model

import "github.com/google/uuid"

// MaxProgramRecapLifts is how many lifts a program recap carries.
const MaxProgramRecapLifts = 5

// ProgramRecap sums up one program: its weeks on target, the workouts done,
// and how its main lifts moved. It is computed from the program's scheduled
// sessions on every read and never stored. See docs/CONTRACTS.md "Program
// recap".
type ProgramRecap struct {
	ProgramID uuid.UUID `json:"program_id"`
	Name      string    `json:"name"`
	Active    bool      `json:"active"`
	// FirstDate and LastDate are the program's first and last session dated
	// today or earlier, YYYY-MM-DD. Both are nil when it has none.
	FirstDate    *string           `json:"first_date"`
	LastDate     *string           `json:"last_date"`
	Weeks        ProgramRecapWeeks `json:"weeks"`
	WorkoutsDone int               `json:"workouts_done"`
	// Lifts is never nil.
	Lifts []ProgramRecapLift `json:"lifts"`
}

// ProgramRecapWeeks counts the program's weeks and how many reached the
// weekly target. A week off that fell short is in neither number.
type ProgramRecapWeeks struct {
	Total    int `json:"total"`
	OnTarget int `json:"on_target"`
}

// ProgramRecapLift is one lift's heaviest set on the first and on the last
// day it was lifted in the program. Days is how many days it was lifted.
type ProgramRecapLift struct {
	Name  string          `json:"name"`
	Days  int             `json:"days"`
	First ProgramRecapSet `json:"first"`
	Last  ProgramRecapSet `json:"last"`
}

// ProgramRecapSet is a set the athlete lifted on Date (YYYY-MM-DD).
type ProgramRecapSet struct {
	Weight float64 `json:"weight"`
	Reps   int     `json:"reps"`
	Date   string  `json:"date"`
}
