package model

import "github.com/google/uuid"

// WeeklyPlanDay assigns a workout template (or rest) to an ISO weekday
// (1 = Monday … 7 = Sunday). A weekday with no row is intentionally unplanned.
type WeeklyPlanDay struct {
	Weekday    int        `json:"weekday" validate:"required,min=1,max=7"`
	TemplateID *uuid.UUID `json:"template_id"`
	Rest       bool       `json:"rest"`
}

// PlanOverride replaces the weekly plan for one specific date.
type PlanOverride struct {
	Date       string     `json:"date" validate:"required"`
	TemplateID *uuid.UUID `json:"template_id"`
	Rest       bool       `json:"rest"`
}

// PlanResponse is the body of GET /api/v1/plan. Effective-day resolution
// (override ?? weekly[weekday]) is the client's job — the API only stores.
type PlanResponse struct {
	Weekly    []WeeklyPlanDay `json:"weekly"`
	Overrides []PlanOverride  `json:"overrides"`
}

// PutWeeklyPlanRequest fully replaces the recurring plan. Sparse is allowed:
// weekdays missing from Days become unplanned.
type PutWeeklyPlanRequest struct {
	Days []WeeklyPlanDay `json:"days" validate:"dive"`
}

// PutPlanOverrideRequest upserts a one-day override.
type PutPlanOverrideRequest struct {
	TemplateID *uuid.UUID `json:"template_id"`
	Rest       bool       `json:"rest"`
}
