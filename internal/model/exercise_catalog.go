package model

import (
	"slices"

	"github.com/google/uuid"
)

// Programming metadata values (migration 032). The engine reads them; the
// catalog stays read-only.
const (
	PatternSquat          = "squat"
	PatternHinge          = "hinge"
	PatternLunge          = "lunge"
	PatternHorizontalPush = "horizontal_push"
	PatternVerticalPush   = "vertical_push"
	PatternHorizontalPull = "horizontal_pull"
	PatternVerticalPull   = "vertical_pull"
	PatternCarry          = "carry"
	PatternCore           = "core"
	PatternExplosive      = "explosive"
	PatternLocomotion     = "locomotion"
	PatternIsolation      = "isolation"

	EquipmentBodyweight = "bodyweight"
	EquipmentFullGym    = "full_gym"

	ImpactLow  = "low"
	ImpactHigh = "high"
)

// MovementPatterns lists every catalog pattern value, in a stable order.
var MovementPatterns = []string{
	PatternSquat, PatternHinge, PatternLunge, PatternHorizontalPush, PatternVerticalPush,
	PatternHorizontalPull, PatternVerticalPull, PatternCarry, PatternCore, PatternExplosive,
	PatternLocomotion, PatternIsolation,
}

// IsMovementPattern reports whether value is a catalog pattern.
func IsMovementPattern(value string) bool { return slices.Contains(MovementPatterns, value) }

// CatalogExercise is a curated entry in the exercise catalog.
// Mechanic is nil for cardio entries (and serialized as JSON null).
type CatalogExercise struct {
	ID                uuid.UUID `json:"id"`
	Name              string    `json:"name"`
	Category          string    `json:"category"`
	Modality          string    `json:"modality"`
	Mechanic          *string   `json:"mechanic"`
	SortOrder         int       `json:"sort_order"`
	Pattern           string    `json:"pattern"`
	MuscleGroup       string    `json:"muscle_group"`
	EquipmentRequired []string  `json:"equipment_required"`
	Impact            string    `json:"impact"`
	// SubPattern refines isolation and push entries (migration 034); nil otherwise.
	SubPattern *string `json:"sub_pattern"`
}

// Sub-pattern values the engine's slots select by.
const (
	SubPatternInclinePush        = "incline_push"
	SubPatternChestIsolation     = "chest_isolation"
	SubPatternShoulderIsolation  = "shoulder_isolation"
	SubPatternRearDelt           = "rear_delt"
	SubPatternTriceps            = "triceps"
	SubPatternBiceps             = "biceps"
	SubPatternQuadIsolation      = "quad_isolation"
	SubPatternHamstringIsolation = "hamstring_isolation"
	SubPatternGluteIsolation     = "glute_isolation"
	SubPatternCalf               = "calf"
)

// HasSubPattern reports whether the entry carries the given sub-pattern.
func (e CatalogExercise) HasSubPattern(value string) bool {
	return e.SubPattern != nil && *e.SubPattern == value
}

// EligibleFor reports whether the athlete's equipment covers the entry: every
// required value is available, or the athlete has a full gym, or the entry
// needs only bodyweight. An entry with no requirement recorded is never
// eligible, so an unseeded row cannot slip into a draft.
func (e CatalogExercise) EligibleFor(equipment []string) bool {
	if len(e.EquipmentRequired) == 0 {
		return false
	}
	if slices.Equal(e.EquipmentRequired, []string{EquipmentBodyweight}) || slices.Contains(equipment, EquipmentFullGym) {
		return true
	}
	for _, required := range e.EquipmentRequired {
		if required != EquipmentBodyweight && !slices.Contains(equipment, required) {
			return false
		}
	}
	return true
}

// IsCompound reports a strength entry with a compound mechanic.
func (e CatalogExercise) IsCompound() bool { return e.Mechanic != nil && *e.Mechanic == "compound" }
