package model

import (
	"os"
	"strings"
	"testing"
)

func TestSportActivityMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../../migrations/018_create_sport_activities.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration := string(body)
	for _, invariant := range []string{
		"CREATE TABLE sport_activities",
		"REFERENCES auth.users(id) ON DELETE CASCADE",
		"duration_minutes BETWEEN 1 AND 1440",
		"idx_sport_activities_user_date",
		"ALTER TABLE public.sport_activities ENABLE ROW LEVEL SECURITY",
		"REVOKE ALL PRIVILEGES ON TABLE public.sport_activities",
	} {
		if !strings.Contains(migration, invariant) {
			t.Errorf("sport activity migration missing %q", invariant)
		}
	}
	if strings.Contains(migration, "UNIQUE (user_id, date)") {
		t.Fatal("sport activities must allow multiple rows on one date")
	}
}

func TestSportActivityMigrationRollbackIsScoped(t *testing.T) {
	body, err := os.ReadFile("../../migrations/018_create_sport_activities.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	down := strings.TrimSpace(string(body))
	if down != "DROP TABLE IF EXISTS sport_activities;" {
		t.Fatalf("unexpected rollback scope: %s", down)
	}
}
