//go:build integration

package dao

import (
	"testing"
	"time"
)

// TestPerformedDatesAgreesWithTheWeeklyCount holds the one-query streak read
// to the rule the per-week count applies: distinct dates, discarded sessions
// left out, and nothing from outside the range.
func TestPerformedDatesAgreesWithTheWeeklyCount(t *testing.T) {
	f := newReleaseFixture(t)
	stats := &statsDAO{pool: f.pool}
	monday := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	session := func(offset int, status string) {
		t.Helper()
		f.exec(t, `INSERT INTO workout_sessions (user_id, date, name, status) VALUES ($1,$2,'session',$3)`,
			f.userID, monday.AddDate(0, 0, offset), status)
	}
	session(0, "completed")
	session(0, "completed") // same day twice counts once
	session(2, "completed")
	session(4, "discarded")
	session(9, "completed") // the week after

	sunday := monday.AddDate(0, 0, 6)
	dates, err := stats.PerformedDates(f.ctx, f.userID, monday, sunday)
	if err != nil {
		t.Fatal(err)
	}
	count, err := stats.GetWeeklyCount(f.ctx, f.userID, monday, sunday)
	if err != nil {
		t.Fatal(err)
	}
	if len(dates) != 2 || count != 2 {
		t.Fatalf("PerformedDates returned %d dates and GetWeeklyCount %d, want 2 and 2", len(dates), count)
	}
	got := map[string]bool{}
	for _, date := range dates {
		got[date.Format("2006-01-02")] = true
	}
	if !got["2026-06-01"] || !got["2026-06-03"] {
		t.Errorf("PerformedDates = %v, want June 1 and June 3", got)
	}
}
