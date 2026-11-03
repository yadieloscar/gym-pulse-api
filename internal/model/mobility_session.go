package model

import (
	"time"

	"github.com/google/uuid"
)

// MobilityContext says where an athlete started a stretching routine.
type MobilityContext string

const (
	// MobilityContextWarmup is a routine started before training.
	MobilityContextWarmup MobilityContext = "warmup"
	// MobilityContextCooldown is a routine started after training.
	MobilityContextCooldown MobilityContext = "cooldown"
	// MobilityContextRestDay is a routine started on a rest day.
	MobilityContextRestDay MobilityContext = "rest_day"
	// MobilityContextAnytime is a routine started outside the other contexts.
	MobilityContextAnytime MobilityContext = "anytime"
)

// Valid reports whether c is a documented context.
func (c MobilityContext) Valid() bool {
	switch c {
	case MobilityContextWarmup, MobilityContextCooldown, MobilityContextRestDay, MobilityContextAnytime:
		return true
	}
	return false
}

// MobilitySession is one finished stretching routine owned by an athlete. It is
// a stored fact only: no stat, participation, record, volume, plan, or schedule
// read model consumes it.
type MobilitySession struct {
	ID              uuid.UUID       `json:"id"`
	Date            string          `json:"date"`
	RoutineID       string          `json:"routine_id"`
	RoutineName     string          `json:"routine_name"`
	Context         MobilityContext `json:"context"`
	DurationSeconds int             `json:"duration_seconds"`
	MovesCompleted  int             `json:"moves_completed"`
	MovesTotal      int             `json:"moves_total"`
	ContentVersion  string          `json:"content_version"`
	OperationKey    string          `json:"operation_key"`
	CreatedAt       time.Time       `json:"created_at"`
}

// CreateMobilitySessionRequest records a finished routine. Date defaults to the
// athlete's current civil date in their saved training-profile timezone.
type CreateMobilitySessionRequest struct {
	Date            string          `json:"date,omitempty"`
	RoutineID       string          `json:"routine_id"`
	RoutineName     string          `json:"routine_name"`
	Context         MobilityContext `json:"context"`
	DurationSeconds int             `json:"duration_seconds"`
	MovesCompleted  int             `json:"moves_completed"`
	MovesTotal      int             `json:"moves_total"`
	ContentVersion  string          `json:"content_version"`
	OperationKey    string          `json:"operation_key"`
}

// MobilitySessionCreateOutcome says what a create request did.
type MobilitySessionCreateOutcome int

const (
	// MobilitySessionCreated means a new entry was recorded.
	MobilitySessionCreated MobilitySessionCreateOutcome = iota
	// MobilitySessionReplayed means an identical retry returned the original entry.
	MobilitySessionReplayed
)
