package dao

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Stretching must never move consistency (FR-005): no other DAO may read or
// write mobility_sessions, apart from account deletion.
func TestNoReadModelReadsMobilitySessions(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") || file == "mobility_session_dao.go" {
			continue
		}
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		source := string(body)
		if !strings.Contains(source, "mobility_sessions") {
			continue
		}
		if file != "account_dao.go" {
			t.Errorf("%s references mobility_sessions; no read model may consume it", file)
			continue
		}
		if strings.Count(source, "mobility_sessions") != 1 || !strings.Contains(source, "`DELETE FROM mobility_sessions WHERE user_id = $1`") {
			t.Error("account deletion may only delete the athlete's mobility sessions")
		}
	}
}

func TestMobilitySessionDAOTouchesOnlyItsTable(t *testing.T) {
	body, err := os.ReadFile("mobility_session_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	tables := regexp.MustCompile(`(?i)\b(?:FROM|INTO|UPDATE|JOIN)\s+([a-z_]+)`).FindAllStringSubmatch(source, -1)
	if len(tables) == 0 {
		t.Fatal("no statements found")
	}
	for _, match := range tables {
		if match[1] != "mobility_sessions" {
			t.Errorf("mobility DAO touches %s", match[1])
		}
	}
	for _, invariant := range []string{
		"WHERE user_id=$1 AND date BETWEEN $2 AND $3",
		"ORDER BY date DESC, created_at DESC, id DESC",
		"FROM mobility_sessions WHERE id=$1 AND user_id=$2",
		"DELETE FROM mobility_sessions WHERE id=$1 AND user_id=$2",
		"ON CONFLICT (user_id, operation_key) DO NOTHING",
		"if storedHash != requestHash",
	} {
		if !strings.Contains(source, invariant) {
			t.Errorf("mobility DAO missing %q", invariant)
		}
	}
}
