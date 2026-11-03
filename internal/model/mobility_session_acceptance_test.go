package model

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMobilitySessionMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../../migrations/033_create_mobility_sessions.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration := string(body)
	for _, invariant := range []string{
		"CREATE TABLE mobility_sessions",
		"REFERENCES auth.users(id) ON DELETE CASCADE",
		"routine_id ~ '^[a-z0-9]+(-[a-z0-9]+)*$'",
		"char_length(routine_name) BETWEEN 1 AND 80",
		"context IN ('warmup', 'cooldown', 'rest_day', 'anytime')",
		"duration_seconds BETWEEN 1 AND 7200",
		"moves_completed BETWEEN 1 AND 100",
		"moves_total BETWEEN 1 AND 100",
		"CHECK (moves_completed <= moves_total)",
		"char_length(content_version) BETWEEN 1 AND 32",
		"char_length(operation_key) BETWEEN 1 AND 128",
		"CREATE UNIQUE INDEX ux_mobility_sessions_user_operation",
		"ON mobility_sessions(user_id, operation_key)",
		"idx_mobility_sessions_user_date",
		"ALTER TABLE public.mobility_sessions ENABLE ROW LEVEL SECURITY",
		"REVOKE ALL PRIVILEGES ON TABLE public.mobility_sessions",
		"CREATE POLICY gym_pulse_api_access ON public.mobility_sessions",
	} {
		if !strings.Contains(migration, invariant) {
			t.Errorf("mobility migration missing %q", invariant)
		}
	}
	for _, forbidden := range []string{"FORCE ROW LEVEL SECURITY", "ALTER TABLE sport_activities", "ALTER TABLE day_participation"} {
		if strings.Contains(migration, forbidden) {
			t.Errorf("mobility migration must not contain %q", forbidden)
		}
	}
	down, err := os.ReadFile("../../migrations/033_create_mobility_sessions.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(down)) != "DROP TABLE IF EXISTS mobility_sessions;" {
		t.Fatalf("unexpected rollback scope: %s", down)
	}
}

func TestMobilitySessionJSONShape(t *testing.T) {
	body, err := json.Marshal(MobilitySession{ID: uuid.New(), Date: "2026-11-03", RoutineID: "hips", RoutineName: "Hips",
		Context: MobilityContextRestDay, DurationSeconds: 300, MovesCompleted: 4, MovesTotal: 5, ContentVersion: "1",
		OperationKey: "op", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	want := []string{"id", "date", "routine_id", "routine_name", "context", "duration_seconds", "moves_completed", "moves_total", "content_version", "operation_key", "created_at"}
	if len(fields) != len(want) {
		t.Fatalf("unexpected fields: %s", body)
	}
	for _, key := range want {
		if _, ok := fields[key]; !ok {
			t.Errorf("missing %q in %s", key, body)
		}
	}
	if fields["context"] != "rest_day" {
		t.Fatalf("context wire value: %v", fields["context"])
	}
}

func TestMobilityContextValid(t *testing.T) {
	for _, c := range []MobilityContext{"warmup", "cooldown", "rest_day", "anytime"} {
		if !c.Valid() {
			t.Errorf("%s must be valid", c)
		}
	}
	for _, c := range []MobilityContext{"", "Warmup", "evening"} {
		if c.Valid() {
			t.Errorf("%q must be invalid", c)
		}
	}
}
