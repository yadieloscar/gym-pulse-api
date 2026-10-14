package dao

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestHealthExportDAORecordKeepsAtomicOrder(t *testing.T) {
	body, err := os.ReadFile("health_export_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	markers := []string{
		"lockUserDomain(ctx, tx, healthExportLockNamespace",
		"findIdempotency(ctx, tx",
		"tx.QueryRow(ctx, workoutDateQuery",
		"WHERE user_id=$1 AND workout_kind=$2 AND workout_id=$3 AND source=$4",
		"return existing, model.HealthExportExisting, nil",
		"health export identity already recorded",
		"INSERT INTO health_exports",
		"insertIdempotency(ctx, tx",
		"tx.Commit(ctx)",
	}
	last := -1
	for _, marker := range markers {
		position := strings.Index(source, marker)
		if position < 0 {
			t.Fatalf("record transaction missing %q", marker)
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

func TestHealthExportDAOFinishedPredicatesAndOwnership(t *testing.T) {
	body, err := os.ReadFile("health_export_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, invariant := range []string{
		"finalized_at IS NOT NULL AND status IN ('completed', 'incomplete')",
		"AND type_id <> 'rest'",
		"WHERE $1 = 'scheduled_workout' AND id=$2 AND user_id=$3",
		"WHERE $1 = 'day_log' AND id=$2 AND user_id=$3",
		`&model.NotFoundError{Message: "finished workout not found"}`,
		"WHERE e.user_id=$1 AND w.date BETWEEN $2 AND $3",
		"ORDER BY w.date DESC, e.exported_at DESC, e.id DESC",
		"exports := []model.HealthExport{}",
	} {
		if !strings.Contains(source, invariant) {
			t.Errorf("health export DAO missing %q", invariant)
		}
	}
}

func TestHealthExportDAOOnlyReadsWorkoutTables(t *testing.T) {
	body, err := os.ReadFile("health_export_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	mutation := regexp.MustCompile(`(?i)(UPDATE|INSERT INTO|DELETE FROM)\s+(scheduled_workouts|day_logs|day_participation|workout_sessions)`)
	if mutation.MatchString(string(body)) {
		t.Fatal("recording an export must not mutate workouts, participation, or sessions")
	}
	if !strings.Contains(string(body), "INSERT INTO health_exports") {
		t.Fatal("record must insert into health_exports")
	}
}

func TestAccountCleanupIncludesHealthExports(t *testing.T) {
	body, err := os.ReadFile("account_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "DELETE FROM health_exports WHERE user_id = $1") {
		t.Fatal("account cleanup does not delete owned health exports")
	}
}
