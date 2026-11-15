package model

import (
	"time"

	"github.com/google/uuid"
)

// KneeKitMaxCompletedItems bounds the checklist items stored for one day.
const KneeKitMaxCompletedItems = 12

// KneeKitDay is one athlete's knee checklist and pain log for one local
// calendar date. It is a stored fact: the API computes no guidance or streak
// from it and no other read model consumes it. CompletedItems is always an
// array, sorted ascending; pain values are null when not logged.
type KneeKitDay struct {
	ID             uuid.UUID `json:"id"`
	Date           string    `json:"date"`
	CompletedItems []string  `json:"completed_items"`
	MorningPain    *int      `json:"morning_pain"`
	ActivityPain   *int      `json:"activity_pain"`
	SharpPain      bool      `json:"sharp_pain"`
	Limping        bool      `json:"limping"`
	Revision       int64     `json:"revision"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// SaveKneeKitDayRequest fully replaces the record for the date in the
// address. CompletedItems is required and may be empty; a nil slice means the
// field was absent. ExpectedRevision is 0 when the client believes no record
// exists, otherwise the stored revision.
type SaveKneeKitDayRequest struct {
	CompletedItems   []string `json:"completed_items"`
	MorningPain      *int     `json:"morning_pain"`
	ActivityPain     *int     `json:"activity_pain"`
	SharpPain        bool     `json:"sharp_pain"`
	Limping          bool     `json:"limping"`
	ExpectedRevision *int64   `json:"expected_revision"`
	OperationKey     string   `json:"operation_key"`
}

// KneeKitRange is a range read: the owned days inside the range, oldest first.
type KneeKitRange struct {
	Days []KneeKitDay `json:"days"`
}

// KneeKitDayOutcome says what a day save did. A verbatim idempotent retry
// returns the original outcome.
type KneeKitDayOutcome int

const (
	// KneeKitDayCreated means a new record was stored (201).
	KneeKitDayCreated KneeKitDayOutcome = iota
	// KneeKitDayReplaced means the existing record was replaced (200).
	KneeKitDayReplaced
)
