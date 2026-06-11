package model

import (
	"time"

	"github.com/google/uuid"
)

// BodyWeight represents a daily body weight entry.
type BodyWeight struct {
	ID       uuid.UUID `json:"id"`
	UserID   uuid.UUID `json:"-"`
	Date     string    `json:"date"`
	Weight   float64   `json:"weight"`
	Unit     string    `json:"unit"`
	LoggedAt time.Time `json:"logged_at"`
}

// CreateBodyWeightRequest is the request body for POST /api/v1/body/weight.
type CreateBodyWeightRequest struct {
	Date   string  `json:"date" validate:"required"`
	Weight float64 `json:"weight" validate:"required,gt=0"`
	Unit   string  `json:"unit" validate:"required,oneof=lb kg"`
}
