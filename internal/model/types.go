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
	Message       string
	Expected      int64
	Actual        int64
	Authoritative any
}

func (e *ConflictError) Error() string { return e.Message }

// ResolutionConflictError indicates a skip or unskip that does not apply to the
// workout's current outcome or resolution.
type ResolutionConflictError struct {
	Message       string
	Status        string
	Resolution    *string
	Authoritative any
}

func (e *ResolutionConflictError) Error() string { return e.Message }

// SportSessionConflictError reports a sport activity recorded against a
// scheduled sport session that is already completed. Authoritative is the
// current ScheduledWorkout.
type SportSessionConflictError struct {
	Message string
	Status  string
	// Reason distinguishes link and unlink conflicts: activity_linked,
	// session_completed, or not_linked. Empty for a create against a completed session.
	Reason        string
	Authoritative any
}

func (e *SportSessionConflictError) Error() string { return e.Message }

// UserSettings holds per-user preferences.
type UserSettings struct {
	WeightUnit string `json:"weight_unit" validate:"required,oneof=lb kg"`
	WeeklyGoal int    `json:"weekly_goal" validate:"required,min=1,max=7"`
	Palette    string `json:"palette" validate:"required,oneof=obsidianEmber abyssCerulean mossChartreuse voltLime oliveCitrine slateMint"`
}

type UpdateUserSettingsRequest struct {
	WeightUnit *string `json:"weight_unit,omitempty" validate:"omitempty,oneof=lb kg"`
	WeeklyGoal *int    `json:"weekly_goal,omitempty" validate:"omitempty,min=1,max=7"`
	Palette    *string `json:"palette,omitempty" validate:"omitempty,oneof=obsidianEmber abyssCerulean mossChartreuse voltLime oliveCitrine slateMint"`
}

// DefaultUserSettings returns the default settings for new users.
func DefaultUserSettings() UserSettings {
	return UserSettings{
		WeightUnit: "lb",
		WeeklyGoal: 5,
		Palette:    "obsidianEmber",
	}
}

// ParseDate parses a "YYYY-MM-DD" string into time.Time.
func ParseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

// UTCToday returns midnight of the current UTC calendar day. Date validation
// compares against this so it matches ParseDate (UTC) and the client's
// UTC `toISOString` dates, rather than skewing by a day at local midnight.
func UTCToday() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

// MondayOfWeek returns the Monday of the week containing t.
func MondayOfWeek(t time.Time) time.Time {
	weekday := t.Weekday()
	if weekday == 0 { // Sunday
		weekday = 7
	}
	return t.AddDate(0, 0, -int(weekday-1))
}
