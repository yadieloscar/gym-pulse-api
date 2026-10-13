package model

import (
	"encoding/json"
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

func TestSportActivitySourceMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../../migrations/022_add_sport_activity_source.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration := string(body)
	for _, invariant := range []string{
		"ADD COLUMN source TEXT NOT NULL DEFAULT 'manual'",
		"CHECK (source IN ('manual', 'apple_health'))",
		"ADD COLUMN external_id TEXT",
		"char_length(external_id) BETWEEN 1 AND 128",
		"ADD COLUMN distance_meters INTEGER",
		"distance_meters BETWEEN 1 AND 1000000",
		"ADD COLUMN energy_kcal INTEGER",
		"energy_kcal BETWEEN 1 AND 20000",
		"ADD COLUMN avg_heart_rate_bpm SMALLINT",
		"avg_heart_rate_bpm BETWEEN 30 AND 250",
		"CHECK ((source = 'manual') = (external_id IS NULL))",
		"CREATE UNIQUE INDEX ux_sport_activities_user_source_external",
		"WHERE external_id IS NOT NULL",
	} {
		if !strings.Contains(migration, invariant) {
			t.Errorf("sport activity source migration missing %q", invariant)
		}
	}
	if strings.Contains(migration, "UPDATE sport_activities") {
		t.Fatal("source migration must not backfill; the column default covers existing rows")
	}
}

func TestSportActivitySourceMigrationRollbackIsScoped(t *testing.T) {
	body, err := os.ReadFile("../../migrations/022_add_sport_activity_source.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	down := string(body)
	for _, statement := range []string{
		"DROP INDEX IF EXISTS ux_sport_activities_user_source_external",
		"DROP CONSTRAINT IF EXISTS sport_activities_source_identity_check",
		"DROP COLUMN IF EXISTS source",
		"DROP COLUMN IF EXISTS external_id",
		"DROP COLUMN IF EXISTS distance_meters",
		"DROP COLUMN IF EXISTS energy_kcal",
		"DROP COLUMN IF EXISTS avg_heart_rate_bpm",
	} {
		if !strings.Contains(down, statement) {
			t.Errorf("rollback missing %q", statement)
		}
	}
	if strings.Contains(down, "DROP TABLE") {
		t.Fatal("rollback must not drop the sport_activities table")
	}
}

func TestSportActivityJSONKeepsLegacyShapeAndAddsSource(t *testing.T) {
	var legacy CreateSportActivityRequest
	if err := json.Unmarshal([]byte(`{"sport_id":"tennis","sport_name":"Tennis","duration_minutes":30,"operation_key":"op"}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Source != "" || legacy.ExternalID != nil || legacy.DistanceMeters != nil {
		t.Fatalf("legacy request must leave new fields empty: %+v", legacy)
	}

	manual, err := json.Marshal(SportActivity{Source: ActivitySourceManual})
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"external_id", "distance_meters", "energy_kcal", "avg_heart_rate_bpm"} {
		if strings.Contains(string(manual), absent) {
			t.Errorf("manual activity must omit %q: %s", absent, manual)
		}
	}
	if !strings.Contains(string(manual), `"source":"manual"`) {
		t.Fatalf("source must always be present: %s", manual)
	}

	external, meters := "9B3C2E1A", 8120
	imported, err := json.Marshal(SportActivity{Source: ActivitySourceAppleHealth, ExternalID: &external, DistanceMeters: &meters})
	if err != nil {
		t.Fatal(err)
	}
	for _, present := range []string{`"source":"apple_health"`, `"external_id":"9B3C2E1A"`, `"distance_meters":8120`} {
		if !strings.Contains(string(imported), present) {
			t.Errorf("imported activity missing %s: %s", present, imported)
		}
	}
}
