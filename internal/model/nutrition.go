package model

import (
	"time"

	"github.com/google/uuid"
)

// NutritionSource names who last wrote a nutrition day: the athlete or a
// health-store import. An athlete-written day is never overwritten by an
// import.
type NutritionSource string

const (
	// NutritionSourceManual is a day the athlete typed or edited.
	NutritionSourceManual NutritionSource = "manual"
	// NutritionSourceAppleHealth is a day's totals read from Apple Health.
	NutritionSourceAppleHealth NutritionSource = "apple_health"
)

// NutritionDay is one athlete's totals for one calendar date. Any of the four
// values may be absent; a day with no values at all does not exist.
type NutritionDay struct {
	ID         uuid.UUID       `json:"id"`
	Date       string          `json:"date"`
	EnergyKcal *int            `json:"energy_kcal,omitempty"`
	ProteinG   *float64        `json:"protein_g,omitempty"`
	CarbsG     *float64        `json:"carbs_g,omitempty"`
	FatG       *float64        `json:"fat_g,omitempty"`
	Source     NutritionSource `json:"source"`
	Revision   int64           `json:"revision"`
	LoggedAt   time.Time       `json:"logged_at"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// NutritionTarget is the daily target the athlete chose, effective from a
// date. Optional values are always present in JSON, null when unset.
type NutritionTarget struct {
	ID            uuid.UUID `json:"id"`
	EffectiveFrom string    `json:"effective_from"`
	ProteinG      float64   `json:"protein_g"`
	EnergyKcal    *int      `json:"energy_kcal"`
	CarbsG        *float64  `json:"carbs_g"`
	FatG          *float64  `json:"fat_g"`
	Revision      int64     `json:"revision"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// SaveNutritionDayRequest saves, replaces, or removes the totals for the date
// in the address. An athlete save (Source empty or manual) carries
// ExpectedRevision: 0 when no entry exists. A Health save carries none.
type SaveNutritionDayRequest struct {
	EnergyKcal       *int            `json:"energy_kcal,omitempty"`
	ProteinG         *float64        `json:"protein_g,omitempty"`
	CarbsG           *float64        `json:"carbs_g,omitempty"`
	FatG             *float64        `json:"fat_g,omitempty"`
	Source           NutritionSource `json:"source,omitempty"`
	ExpectedRevision *int64          `json:"expected_revision,omitempty"`
	OperationKey     string          `json:"operation_key"`
}

// HasValue reports whether any of the four totals is present.
func (r SaveNutritionDayRequest) HasValue() bool {
	return r.EnergyKcal != nil || r.ProteinG != nil || r.CarbsG != nil || r.FatG != nil
}

// SaveNutritionTargetRequest saves the athlete's chosen target. EffectiveFrom
// defaults to the athlete's local today. ExpectedRevision is the current
// target's revision, 0 when none exists.
type SaveNutritionTargetRequest struct {
	EffectiveFrom    string   `json:"effective_from,omitempty"`
	ProteinG         float64  `json:"protein_g"`
	EnergyKcal       *int     `json:"energy_kcal,omitempty"`
	CarbsG           *float64 `json:"carbs_g,omitempty"`
	FatG             *float64 `json:"fat_g,omitempty"`
	ExpectedRevision int64    `json:"expected_revision"`
	OperationKey     string   `json:"operation_key"`
}

// NutritionRange is a range read: the days inside the range oldest first and
// the targets that applied to them (the one in effect on the first day, if
// any, plus every target starting inside the range), oldest first.
type NutritionRange struct {
	Days    []NutritionDay    `json:"days"`
	Targets []NutritionTarget `json:"targets"`
}

// NutritionDayOutcome says what a day save did. A verbatim idempotent retry
// returns the original outcome.
type NutritionDayOutcome int

const (
	// NutritionDayCreated means a new day was stored (201).
	NutritionDayCreated NutritionDayOutcome = iota
	// NutritionDayReplaced means an existing day was updated (200).
	NutritionDayReplaced
	// NutritionDayIgnored means a Health save met an athlete-written day and
	// wrote nothing; the stored day is returned (200).
	NutritionDayIgnored
	// NutritionDayRemoved means the day was deleted or there was nothing to
	// delete (204).
	NutritionDayRemoved
)

// NutritionTargetOutcome says what a target save did.
type NutritionTargetOutcome int

const (
	// NutritionTargetCreated means a new target row was stored (201).
	NutritionTargetCreated NutritionTargetOutcome = iota
	// NutritionTargetReplaced means the row for that effective date was updated (200).
	NutritionTargetReplaced
	// NutritionTargetUnchanged means the save matched the current target (200).
	NutritionTargetUnchanged
)
