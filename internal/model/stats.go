package model

type StatsSummary struct {
	ThisWeek      WeekProgress `json:"this_week"`
	StreakWeeks   int          `json:"streak_weeks"`
	StreakDays    int          `json:"streak_days"`
	TotalWorkouts int          `json:"total_workouts"`
}

type WeekProgress struct {
	Completed int `json:"completed"`
	Goal      int `json:"goal"`
}

type TypeDistribution struct {
	TypeID   string                `json:"type_id"`
	Count    int                   `json:"count"`
	Subtypes []SubtypeDistribution `json:"subtypes"`
}

type SubtypeDistribution struct {
	SubtypeID string `json:"subtype_id"`
	Count     int    `json:"count"`
}

// WeeklyVolume is total lifted volume (Σ weight × reps over completed sets) for
// one week, keyed by its Monday. Drives the Profile volume chart.
type WeeklyVolume struct {
	WeekStart string  `json:"week_start"`
	Volume    float64 `json:"volume"`
}
