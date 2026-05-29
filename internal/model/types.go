package model

import (
	"slices"
	"time"
)

var ValidTypeIDs = []string{
	"push", "pull", "legs", "cardio",
	"upper", "lower", "full", "core", "other", "rest",
}

var ValidSubtypeIDs = []string{
	"hypertrophy", "strength", "power", "endurance",
	"mobility", "conditioning", "skills", "general",
}

func IsValidTypeID(s string) bool {
	return slices.Contains(ValidTypeIDs, s)
}

func IsValidSubtypeID(s string) bool {
	return slices.Contains(ValidSubtypeIDs, s)
}

// NotFoundError indicates a resource was not found (or not owned by user).
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string { return e.Message }

// ValidationError indicates invalid input.
type ValidationError struct {
	Message string
	Field   string
}

func (e *ValidationError) Error() string { return e.Message }

// ConflictError indicates a uniqueness constraint violation.
type ConflictError struct {
	Message string
}

func (e *ConflictError) Error() string { return e.Message }

// UserSettings holds per-user preferences.
type UserSettings struct {
	WeightUnit string `json:"weight_unit" validate:"required,oneof=lb kg"`
	WeeklyGoal int    `json:"weekly_goal" validate:"required,min=3,max=7"`
}

// DefaultUserSettings returns the default settings for new users.
func DefaultUserSettings() UserSettings {
	return UserSettings{
		WeightUnit: "lb",
		WeeklyGoal: 5,
	}
}

// ParseDate parses a "YYYY-MM-DD" string into time.Time.
func ParseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

// MondayOfWeek returns the Monday of the week containing t.
func MondayOfWeek(t time.Time) time.Time {
	weekday := t.Weekday()
	if weekday == 0 { // Sunday
		weekday = 7
	}
	return t.AddDate(0, 0, -int(weekday-1))
}
