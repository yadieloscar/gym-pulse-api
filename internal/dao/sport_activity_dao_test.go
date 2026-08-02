package dao

import (
	"os"
	"strings"
	"testing"
)

func TestSportActivityDAOCreateKeepsAtomicWriteOrder(t *testing.T) {
	body, err := os.ReadFile("sport_activity_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	markers := []string{
		"findIdempotency(ctx, tx",
		"INSERT INTO sport_activities",
		"INSERT INTO day_participation",
		"insertIdempotency(ctx, tx",
		"tx.Commit(ctx)",
	}
	last := -1
	for _, marker := range markers {
		position := strings.Index(source, marker)
		if position < 0 {
			t.Fatalf("atomic create missing %q", marker)
		}
		if position <= last {
			t.Fatalf("%q is out of transactional order", marker)
		}
		last = position
	}
	for _, invariant := range []string{
		"WHERE user_id=$1 AND date BETWEEN $2 AND $3",
		"WHERE id=$1 AND user_id=$2",
		"scheduled_opportunity=day_participation.scheduled_opportunity OR EXCLUDED.scheduled_opportunity",
		"ORDER BY date DESC, created_at DESC, id DESC",
	} {
		if !strings.Contains(source, invariant) {
			t.Errorf("sport DAO missing %q", invariant)
		}
	}
}

func TestWeeklyStatsUnionsSportDatesWithoutChangingWorkoutTotals(t *testing.T) {
	body, err := os.ReadFile("stats_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	weeklyStart := strings.Index(source, "func (r *statsDAO) GetWeeklyCount")
	totalStart := strings.Index(source, "func (r *statsDAO) GetTotalWorkouts")
	if weeklyStart < 0 || totalStart < 0 {
		t.Fatal("stats methods not found")
	}
	if !strings.Contains(source[weeklyStart:totalStart], "SELECT date FROM sport_activities") {
		t.Fatal("weekly count does not include sport dates")
	}
	nextMethod := strings.Index(source[totalStart+1:], "func (r *statsDAO) GetDistribution")
	if nextMethod < 0 {
		t.Fatal("distribution method not found")
	}
	if strings.Contains(source[totalStart:totalStart+1+nextMethod], "sport_activities") {
		t.Fatal("sport activities unexpectedly changed total_workouts")
	}
}

func TestAccountCleanupIncludesSportActivities(t *testing.T) {
	body, err := os.ReadFile("account_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "DELETE FROM sport_activities WHERE user_id = $1") {
		t.Fatal("account cleanup does not delete owned sport activities")
	}
}
