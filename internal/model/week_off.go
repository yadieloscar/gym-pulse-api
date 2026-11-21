package model

import "time"

// MaxWeekOffAheadWeeks is how many weeks after the current one a week off may
// start. It keeps a week off inside the schedule the API has already filled.
const MaxWeekOffAheadWeeks = 8

// WeekOff is one Monday-to-Sunday week the athlete took off. WeekStart is
// the Monday as YYYY-MM-DD. See docs/CONTRACTS.md "Weeks off".
type WeekOff struct {
	WeekStart string    `json:"week_start"`
	CreatedAt time.Time `json:"created_at"`
}

// WeeksOffResponse is the body of GET /weeks-off. WeeksOff is never nil.
type WeeksOffResponse struct {
	WeeksOff []WeekOff `json:"weeks_off"`
}
