package dao

import (
	"os"
	"strings"
	"testing"
)

func TestLegacyUserTablesCascadeFromAuthoritativeAuthIdentity(t *testing.T) {
	body, err := os.ReadFile("../../migrations/017_link_legacy_user_data_to_auth.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	contents := string(body)

	for _, table := range []string{
		"workout_templates",
		"day_logs",
		"user_settings",
		"body_weights",
		"weekly_plans",
		"plan_overrides",
	} {
		constraint := table + "_user_auth_fk"
		if !strings.Contains(contents, "ALTER TABLE public."+table) {
			t.Errorf("%s has no auth foreign-key migration", table)
		}
		if !strings.Contains(contents, "CONSTRAINT "+constraint) {
			t.Errorf("%s constraint is missing", constraint)
		}
		if !strings.Contains(contents, "VALIDATE CONSTRAINT "+constraint) {
			t.Errorf("%s is never validated", constraint)
		}
	}

	if got := strings.Count(contents, "FOREIGN KEY (user_id) REFERENCES auth.users(id) ON DELETE CASCADE NOT VALID;"); got != 6 {
		t.Fatalf("found %d authoritative cascade foreign keys, want 6", got)
	}
}
