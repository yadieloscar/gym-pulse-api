package dao

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func kneeKitDAOSource(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile("knee_kit_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestKneeKitDAOSaveDayKeepsAtomicOrder(t *testing.T) {
	source := kneeKitDAOSource(t)
	start := strings.Index(source, "func (r *kneeKitDAO) SaveDay(")
	end := strings.Index(source, "func checkKneeKitRevision(")
	if start < 0 || end < start {
		t.Fatal("SaveDay not found")
	}
	assertMarkersInOrder(t, source[start:end],
		"lockUserDomain(ctx, tx, kneeKitLockNamespace",
		"findIdempotency(ctx, tx, userID, kneeKitDayScope",
		"WHERE user_id=$1 AND date=$2\n\t\tFOR UPDATE",
		"checkKneeKitRevision(expectedRevision, existing)",
		"INSERT INTO knee_kit_days",
		"UPDATE knee_kit_days",
		"insertIdempotency(ctx, tx, userID, record, kneeKitDayLedger",
		"tx.Commit(ctx)",
	)
	if strings.Count(source[start:end], "defer tx.Rollback(ctx)") != 1 {
		t.Fatal("early returns rely on one deferred rollback")
	}
}

func TestKneeKitDAOOwnershipAndRevisionRules(t *testing.T) {
	source := kneeKitDAOSource(t)
	for _, invariant := range []string{
		// Every statement is owner-scoped.
		"WHERE user_id=$1 AND date=$2",
		"WHERE user_id=$1 AND date BETWEEN $2 AND $3",
		// Oldest first, never a null collection.
		"ORDER BY date ASC",
		"days := []model.KneeKitDay{}",
		"day.CompletedItems = []string{}",
		// Replacement is whole-record and bumps the revision.
		"SET completed_items=$3, morning_pain=$4, activity_pain=$5, sharp_pain=$6, limping=$7,",
		"revision=revision+1, updated_at=now()",
		`Message: "resource revision conflict", Expected: expected, Actual: actual, Authoritative: authoritative`,
	} {
		if !strings.Contains(source, invariant) {
			t.Errorf("knee kit DAO missing %q", invariant)
		}
	}
	// There is no delete-by-empty: only account deletion removes a day.
	if strings.Contains(source, "DELETE FROM") {
		t.Fatal("the knee kit DAO never deletes a day")
	}
}

func TestKneeKitDAOTouchesOnlyItsTable(t *testing.T) {
	tables := regexp.MustCompile(`(?i)\b(?:FROM|INTO|UPDATE|JOIN)\s+([a-z_]+)`).FindAllStringSubmatch(kneeKitDAOSource(t), -1)
	if len(tables) == 0 {
		t.Fatal("no statements found")
	}
	for _, match := range tables {
		if match[1] != "knee_kit_days" {
			t.Errorf("knee kit DAO touches %s", match[1])
		}
	}
}

// The knee kit must never move participation, stats, streaks, records,
// volume, or day status (FR-005): apart from account deletion and the data
// export, no other DAO may name knee_kit_days.
func TestNoReadModelReadsKneeKitDays(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") || file == "knee_kit_dao.go" {
			continue
		}
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		source := string(body)
		if !strings.Contains(source, "knee_kit_days") {
			continue
		}
		switch file {
		case "account_dao.go":
			if strings.Count(source, "knee_kit_days") != 1 || !strings.Contains(source, "`DELETE FROM knee_kit_days WHERE user_id = $1`") {
				t.Error("account deletion may only delete the athlete's knee kit days")
			}
		case "export_dao.go":
			if strings.Count(source, "knee_kit_days") != 1 || !strings.Contains(source, "FROM knee_kit_days WHERE user_id=$1 ORDER BY date") {
				t.Error("the export may only read the athlete's own knee kit days")
			}
		default:
			t.Errorf("%s references knee_kit_days; no read model may consume it", file)
		}
	}
}

func TestCheckKneeKitRevision(t *testing.T) {
	if err := checkKneeKitRevision(0, nil); err != nil {
		t.Fatalf("0 against no row must pass: %v", err)
	}
	stored := &model.KneeKitDay{Revision: 3}
	if err := checkKneeKitRevision(3, stored); err != nil {
		t.Fatalf("the stored revision must pass: %v", err)
	}
	var conflict *model.ConflictError
	err := checkKneeKitRevision(0, stored)
	if !errors.As(err, &conflict) || conflict.Expected != 0 || conflict.Actual != 3 || conflict.Authoritative != stored || conflict.Message != "resource revision conflict" {
		t.Fatalf("a stale revision must conflict with the stored row: %v", err)
	}
	err = checkKneeKitRevision(2, nil)
	if !errors.As(err, &conflict) || conflict.Actual != 0 || conflict.Authoritative != nil {
		t.Fatalf("a nonzero revision against a missing row must conflict without a resource: %v", err)
	}
}
