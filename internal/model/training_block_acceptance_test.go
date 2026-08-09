package model

import (
	"os"
	"strings"
	"testing"
)

func TestCriteriaTrainingBlockMigrationSecurityAndInvariants(t *testing.T) {
	body, err := os.ReadFile("../../migrations/019_create_criteria_training_blocks.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	contents := string(body)
	for _, required := range []string{
		"CREATE TABLE criteria_training_blocks",
		"CREATE TABLE criteria_training_stages",
		"CREATE TABLE criteria_training_exposures",
		"CREATE TABLE criteria_training_transitions",
		"ON DELETE CASCADE",
		"ON DELETE SET NULL",
		"required_qualifying_exposures BETWEEN 1 AND 20",
		"session_outcome IN ('completed_as_planned', 'modified', 'stopped')",
		"next_morning_response IN ('baseline', 'above_baseline')",
		"ALTER TABLE public.criteria_training_blocks ENABLE ROW LEVEL SECURITY",
		"REVOKE ALL PRIVILEGES ON TABLE public.%I",
	} {
		if !strings.Contains(contents, required) {
			t.Errorf("migration missing %q", required)
		}
	}
}

func TestCriteriaTrainingBlockRollbackIsScoped(t *testing.T) {
	body, err := os.ReadFile("../../migrations/019_create_criteria_training_blocks.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	contents := string(body)
	for _, table := range []string{"criteria_training_transitions", "criteria_training_exposures", "criteria_training_stages", "criteria_training_blocks"} {
		if !strings.Contains(contents, "DROP TABLE IF EXISTS "+table) {
			t.Errorf("rollback does not drop %s", table)
		}
	}
	if strings.Contains(contents, "programs;") || strings.Contains(contents, "sport_activities") {
		t.Fatal("rollback changes an existing training domain")
	}
}

func TestAccountDeletionIncludesCriteriaTrainingBlocks(t *testing.T) {
	body, err := os.ReadFile("../dao/account_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "DELETE FROM criteria_training_blocks WHERE user_id = $1") {
		t.Fatal("account deletion omits criteria-based training blocks")
	}
}
