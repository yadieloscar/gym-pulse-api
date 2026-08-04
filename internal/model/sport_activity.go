package model

import (
	"time"

	"github.com/google/uuid"
)

// SportActivity is one completed athlete-owned sport occurrence.
type SportActivity struct {
	ID              uuid.UUID `json:"id"`
	Date            string    `json:"date"`
	SportID         string    `json:"sport_id"`
	SportName       string    `json:"sport_name"`
	DurationMinutes int       `json:"duration_minutes"`
	Notes           *string   `json:"notes,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// CreateSportActivityRequest records a completed sport. Date defaults to the
// athlete's current civil date in their saved training-profile timezone.
type CreateSportActivityRequest struct {
	Date            string  `json:"date,omitempty"`
	SportID         string  `json:"sport_id"`
	SportName       string  `json:"sport_name"`
	DurationMinutes int     `json:"duration_minutes"`
	Notes           *string `json:"notes,omitempty"`
	OperationKey    string  `json:"operation_key"`
}
