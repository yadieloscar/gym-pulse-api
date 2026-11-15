package model

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestKneeKitMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../../migrations/037_create_knee_kit_days.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration := string(body)
	for _, invariant := range []string{
		"CREATE TABLE knee_kit_days",
		"REFERENCES auth.users(id) ON DELETE CASCADE",
		"completed_items TEXT[] NOT NULL DEFAULT '{}'",
		"CHECK (morning_pain BETWEEN 0 AND 10)",
		"CHECK (activity_pain BETWEEN 0 AND 10)",
		"sharp_pain      BOOLEAN NOT NULL DEFAULT false",
		"limping         BOOLEAN NOT NULL DEFAULT false",
		"revision        BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0)",
		"CONSTRAINT ck_knee_kit_days_completed_items CHECK",
		"cardinality(completed_items) <= 12",
		"array_position(completed_items, NULL) IS NULL",
		"array_position(completed_items, '') IS NULL",
		"'^([a-z0-9]+(-[a-z0-9]+)*(,[a-z0-9]+(-[a-z0-9]+)*)*)?$'",
		"!~ '[^,]{65}'",
		"CREATE UNIQUE INDEX ux_knee_kit_days_user_date",
		"ON knee_kit_days(user_id, date)",
		"ALTER TABLE public.knee_kit_days ENABLE ROW LEVEL SECURITY",
		"REVOKE ALL PRIVILEGES ON TABLE public.knee_kit_days",
		"GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.knee_kit_days TO gym_pulse_api",
		"CREATE POLICY gym_pulse_api_access ON public.knee_kit_days",
	} {
		if !strings.Contains(migration, invariant) {
			t.Errorf("knee kit migration missing %q", invariant)
		}
	}
	if strings.Contains(strings.ToUpper(migration), "FORCE ROW LEVEL SECURITY") {
		t.Fatal("RLS must be enabled, not forced")
	}
	// Additive only: the migration creates its table and touches no other.
	for _, forbidden := range []string{"ALTER TABLE exercise_catalog", "INSERT INTO", "UPDATE ", "DROP "} {
		if strings.Contains(strings.ReplaceAll(migration, "DROP POLICY IF EXISTS gym_pulse_api_access ON public.knee_kit_days", ""), forbidden) {
			t.Errorf("knee kit migration must not contain %q", forbidden)
		}
	}
}

func TestKneeKitMigrationRollbackIsScoped(t *testing.T) {
	body, err := os.ReadFile("../../migrations/037_create_knee_kit_days.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != "DROP TABLE IF EXISTS knee_kit_days;" {
		t.Fatalf("unexpected rollback scope: %s", body)
	}
}

func TestKneeKitJSONShape(t *testing.T) {
	var full SaveKneeKitDayRequest
	if err := json.Unmarshal([]byte(`{"completed_items":["morning-check","isometric-hold"],"morning_pain":3,"activity_pain":null,"sharp_pain":false,"limping":true,"expected_revision":0,"operation_key":"knee-day-1"}`), &full); err != nil {
		t.Fatal(err)
	}
	if len(full.CompletedItems) != 2 || full.MorningPain == nil || *full.MorningPain != 3 || full.ActivityPain != nil ||
		full.SharpPain || !full.Limping || full.ExpectedRevision == nil || *full.ExpectedRevision != 0 || full.OperationKey != "knee-day-1" {
		t.Fatalf("request decoded wrong: %+v", full)
	}

	// An empty array is present; an absent or null one is not. Booleans
	// default to false and an absent revision stays distinguishable from 0.
	var empty, absent, null SaveKneeKitDayRequest
	for body, target := range map[string]*SaveKneeKitDayRequest{
		`{"completed_items":[]}`:   &empty,
		`{}`:                       &absent,
		`{"completed_items":null}`: &null,
	} {
		if err := json.Unmarshal([]byte(body), target); err != nil {
			t.Fatal(err)
		}
	}
	if empty.CompletedItems == nil || len(empty.CompletedItems) != 0 {
		t.Fatalf("an empty array must decode as present: %#v", empty.CompletedItems)
	}
	if absent.CompletedItems != nil || null.CompletedItems != nil || absent.SharpPain || absent.Limping || absent.ExpectedRevision != nil {
		t.Fatalf("absent fields decoded wrong: %+v %+v", absent, null)
	}

	pain := 4
	day, err := json.Marshal(KneeKitDay{Date: "2026-11-14", CompletedItems: []string{}, MorningPain: &pain, Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, present := range []string{`"id":`, `"date":"2026-11-14"`, `"completed_items":[]`, `"morning_pain":4`, `"activity_pain":null`,
		`"sharp_pain":false`, `"limping":false`, `"revision":1`, `"created_at":`, `"updated_at":`} {
		if !strings.Contains(string(day), present) {
			t.Errorf("day JSON missing %s: %s", present, day)
		}
	}
	if strings.Contains(string(day), "user_id") || strings.Contains(string(day), "operation_key") {
		t.Errorf("day JSON must not expose ownership or retry identity: %s", day)
	}

	rng, err := json.Marshal(KneeKitRange{Days: []KneeKitDay{}})
	if err != nil || string(rng) != `{"days":[]}` {
		t.Fatalf("empty range JSON = %s, err=%v", rng, err)
	}
}
