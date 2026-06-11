package model

import (
	"time"

	"github.com/google/uuid"
)

// UserProfile represents a user's profile information.
type UserProfile struct {
	ID                  uuid.UUID `json:"id"`
	DisplayName         *string   `json:"display_name"`
	AvatarURL           *string   `json:"avatar_url"`
	OnboardingCompleted bool      `json:"onboarding_completed"`
	CreatedAt           time.Time `json:"created_at"`
}

// UpdateProfileRequest is the request body for PUT /api/v1/profile.
type UpdateProfileRequest struct {
	DisplayName *string `json:"display_name" validate:"omitempty,min=2,max=50"`
	AvatarURL   *string `json:"avatar_url" validate:"omitempty,url"`
}
