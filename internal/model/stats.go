package model

type StatsSummary struct {
	ThisWeek      WeekProgress `json:"this_week"`
	StreakWeeks   int          `json:"streak_weeks"`
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
