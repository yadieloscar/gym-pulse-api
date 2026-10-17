package model

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestHealthExportMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../../migrations/025_create_health_exports.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration := string(body)
	for _, invariant := range []string{
		"CREATE TABLE health_exports",
		"REFERENCES auth.users(id) ON DELETE CASCADE",
		"CHECK (workout_kind IN ('scheduled_workout', 'day_log'))",
		"CHECK (source IN ('apple_health'))",
		"char_length(external_id) BETWEEN 1 AND 128",
		"CREATE UNIQUE INDEX ux_health_exports_workout",
		"ON health_exports(user_id, workout_kind, workout_id, source)",
		"idx_health_exports_user_exported",
		"ALTER TABLE public.health_exports ENABLE ROW LEVEL SECURITY",
		"REVOKE ALL PRIVILEGES ON TABLE public.health_exports",
	} {
		if !strings.Contains(migration, invariant) {
			t.Errorf("health export migration missing %q", invariant)
		}
	}
	if strings.Contains(migration, "REFERENCES scheduled_workouts") || strings.Contains(migration, "REFERENCES day_logs") {
		t.Fatal("exports must not foreign-key one workout table; the kind column spans two")
	}
}

func TestHealthExportMigrationRollbackIsScoped(t *testing.T) {
	body, err := os.ReadFile("../../migrations/025_create_health_exports.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != "DROP TABLE IF EXISTS health_exports;" {
		t.Fatalf("unexpected rollback scope: %s", body)
	}
}

func TestHealthExportJSONShape(t *testing.T) {
	var req RecordHealthExportRequest
	if err := json.Unmarshal([]byte(`{"source":"apple_health","external_id":"B5D7","operation_key":"op"}`), &req); err != nil {
		t.Fatal(err)
	}
	if req.Source != HealthExportSourceAppleHealth || req.ExternalID != "B5D7" || req.ExportedAt != nil {
		t.Fatalf("request decoded wrong: %+v", req)
	}
	body, err := json.Marshal(HealthExport{WorkoutKind: HealthExportKindDayLog, Source: HealthExportSourceAppleHealth, ExternalID: "B5D7", Date: "2026-10-13"})
	if err != nil {
		t.Fatal(err)
	}
	for _, present := range []string{`"workout_kind":"day_log"`, `"source":"apple_health"`, `"external_id":"B5D7"`, `"date":"2026-10-13"`, `"exported_at"`} {
		if !strings.Contains(string(body), present) {
			t.Errorf("export JSON missing %s: %s", present, body)
		}
	}
	if strings.Contains(string(body), "user_id") {
		t.Fatal("export JSON must not expose user_id")
	}
}
