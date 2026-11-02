package dao

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestProgramGenerationDAOReserveKeepsAtomicOrder(t *testing.T) {
	body, err := os.ReadFile("program_generation_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	start := strings.Index(source, "func (r *programGenerationDAO) Reserve(")
	if start < 0 {
		t.Fatal("Reserve not found")
	}
	reserve := source[start:]
	markers := []string{
		"lockUserDomain(ctx, tx, programGenerationLockNamespace",
		"liveGenerationCountQuery",
		"if used >= limit {",
		"&model.GenerationLimitError{Limit: limit}",
		"INSERT INTO program_generations",
		"tx.Commit(ctx)",
	}
	last := -1
	for _, marker := range markers {
		position := strings.Index(reserve, marker)
		if position < 0 {
			t.Fatalf("reservation missing %q", marker)
		}
		if position <= last {
			t.Fatalf("%q is out of transactional order", marker)
		}
		last = position
	}
	if strings.Count(source, "defer tx.Rollback(ctx)") != 1 {
		t.Fatal("early returns rely on one deferred rollback")
	}
}

func TestProgramGenerationDAOCountPredicateAndOwnership(t *testing.T) {
	body, err := os.ReadFile("program_generation_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, invariant := range []string{
		"WHERE user_id=$1 AND local_date=$2",
		"(status = 'succeeded' OR (status = 'pending' AND created_at > now() - $3::interval))",
		"WHERE id=$1 AND status = 'pending'",
		"VALUES ($1, $2, $3, 'pending', $4)",
	} {
		if !strings.Contains(source, invariant) {
			t.Errorf("program generation DAO missing %q", invariant)
		}
	}
	// Every statement that reads or writes the ledger is scoped by user or by
	// the reservation id the service obtained for that user.
	statements := regexp.MustCompile(`(?s)(SELECT|INSERT INTO|UPDATE)[^;]*?program_generations[^;]*?(WHERE|VALUES)[^`+"`"+`]*`).FindAllString(source, -1)
	if len(statements) < 3 {
		t.Fatalf("expected count, insert, and update statements, found %d", len(statements))
	}
	for _, statement := range statements {
		if !strings.Contains(statement, "user_id") && !strings.Contains(statement, "WHERE id=$1") {
			t.Errorf("statement not scoped: %s", statement)
		}
	}
}

func TestProgramGenerationDAOTouchesOnlyItsTable(t *testing.T) {
	body, err := os.ReadFile("program_generation_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	mutation := regexp.MustCompile(`(?i)(UPDATE|INSERT INTO|DELETE FROM)\s+(programs|program_workouts|program_exercises|scheduled_workouts|training_profiles)`)
	if mutation.MatchString(string(body)) {
		t.Fatal("generation must not mutate programs, schedules, or the profile")
	}
}

func TestAccountCleanupIncludesProgramGenerations(t *testing.T) {
	body, err := os.ReadFile("account_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "DELETE FROM program_generations WHERE user_id = $1") {
		t.Fatal("account cleanup does not delete owned program generations")
	}
}

func TestProgramGenerationDAOPendingWindowIsSeconds(t *testing.T) {
	dao, ok := NewProgramGenerationDAO(nil, 55*time.Second).(*programGenerationDAO)
	if !ok || dao.pendingWindow != "55 seconds" {
		t.Fatalf("pending window not derived from the duration: %+v", dao)
	}
}
