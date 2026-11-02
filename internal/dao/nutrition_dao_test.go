package dao

import (
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

func nutritionDAOSource(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile("nutrition_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestNutritionDAOSaveDayKeepsAtomicOrder(t *testing.T) {
	source := nutritionDAOSource(t)
	start := strings.Index(source, "func (r *nutritionDAO) SaveDay(")
	end := strings.Index(source, "func existingRevision(")
	if start < 0 || end < start {
		t.Fatal("SaveDay not found")
	}
	assertMarkersInOrder(t, source[start:end],
		"lockUserDomain(ctx, tx, nutritionLockNamespace",
		"findIdempotency(ctx, tx, userID, nutritionDayScope",
		"WHERE user_id=$1 AND date=$2\n\t\tFOR UPDATE",
		"case remove:",
		"DELETE FROM nutrition_days WHERE user_id=$1 AND date=$2",
		"case day.Source == model.NutritionSourceAppleHealth:",
		"case existing.Source == model.NutritionSourceAppleHealth:",
		"result, outcome = existing, model.NutritionDayIgnored",
		"day.Source = model.NutritionSourceManual",
		"insertIdempotency(ctx, tx, userID, record, nutritionDayLedger",
		"tx.Commit(ctx)",
	)
	if strings.Count(source[start:end], "defer tx.Rollback(ctx)") != 1 {
		t.Fatal("early returns rely on one deferred rollback")
	}
}

func TestNutritionDAOSaveTargetKeepsAtomicOrder(t *testing.T) {
	source := nutritionDAOSource(t)
	start := strings.Index(source, "func (r *nutritionDAO) SaveTarget(")
	end := strings.Index(source, "func sameNutritionTargetValues(")
	if start < 0 || end < start {
		t.Fatal("SaveTarget not found")
	}
	assertMarkersInOrder(t, source[start:end],
		"lockUserDomain(ctx, tx, nutritionLockNamespace",
		"findIdempotency(ctx, tx, userID, nutritionTargetScope",
		"currentNutritionTargetQuery+` FOR UPDATE`",
		"if expectedRevision != actual {",
		"model.NutritionTargetUnchanged",
		"SELECT id FROM nutrition_targets WHERE user_id=$1 AND effective_from=$2 FOR UPDATE",
		"INSERT INTO nutrition_targets",
		"UPDATE nutrition_targets",
		"insertIdempotency(ctx, tx, userID, record, nutritionTargetLedger",
		"tx.Commit(ctx)",
	)
}

func TestNutritionDAOOwnershipAndRevisionRules(t *testing.T) {
	source := nutritionDAOSource(t)
	for _, invariant := range []string{
		// Every statement is owner-scoped.
		"WHERE user_id=$1 AND date=$2",
		"WHERE user_id=$1 AND date BETWEEN $2 AND $3",
		"WHERE user_id=$1 AND effective_from <= $2",
		"WHERE user_id=$1 AND effective_from=$2",
		"SELECT max(effective_from) FROM nutrition_targets WHERE user_id=$1 AND effective_from <= $2",
		// Oldest first, never null collections.
		"ORDER BY date ASC",
		"ORDER BY effective_from ASC",
		"Days: []model.NutritionDay{}, Targets: []model.NutritionTarget{}",
		// Revision bumps on replace; the Health branch never checks a revision.
		"revision=revision+1, logged_at=now(), updated_at=now()",
		"revision=revision+1, updated_at=now()",
		`&model.NotFoundError{Message: "nutrition target not found"}`,
		`Message: "resource revision conflict", Expected: *expected, Actual: actual, Authoritative: authoritative`,
	} {
		if !strings.Contains(source, invariant) {
			t.Errorf("nutrition DAO missing %q", invariant)
		}
	}
	healthBranch := source[strings.Index(source, "case day.Source == model.NutritionSourceAppleHealth:"):strings.Index(source, "\tdefault:\n\t\tif err := checkNutritionRevision")]
	if strings.Contains(healthBranch, "checkNutritionRevision") {
		t.Fatal("a Health save must not require a revision")
	}
	defaultArm := healthBranch[strings.LastIndex(healthBranch, "default:"):]
	if !strings.Contains(defaultArm, "result, outcome = existing, model.NutritionDayIgnored") || strings.Contains(defaultArm, "updateNutritionDay") || strings.Contains(defaultArm, "insertNutritionDay") {
		t.Fatal("a Health save over a manual row must return the stored row without writing")
	}
}

func TestNutritionDAOOnlyTouchesNutritionTables(t *testing.T) {
	source := nutritionDAOSource(t)
	mutation := regexp.MustCompile(`(?i)(UPDATE|INSERT INTO|DELETE FROM)\s+(body_weights|day_participation|scheduled_workouts|workout_sessions|sport_activities|training_profiles)`)
	if mutation.MatchString(source) {
		t.Fatal("nutrition saves must not mutate other domains")
	}
	for _, statement := range []string{"INSERT INTO nutrition_days", "UPDATE nutrition_days", "DELETE FROM nutrition_days", "INSERT INTO nutrition_targets", "UPDATE nutrition_targets"} {
		if !strings.Contains(source, statement) {
			t.Errorf("missing %q", statement)
		}
	}
	if strings.Contains(source, "DELETE FROM nutrition_targets") {
		t.Fatal("targets are never deleted outside account deletion")
	}
}

func TestCheckNutritionRevision(t *testing.T) {
	zero, two := int64(0), int64(2)
	if err := checkNutritionRevision(nil, nil, 0); err == nil {
		t.Fatal("missing expected_revision must be a validation error")
	}
	if err := checkNutritionRevision(&zero, nil, 0); err != nil {
		t.Fatalf("0 against no row must pass: %v", err)
	}
	stored := &model.NutritionDay{Revision: 1}
	var conflict *model.ConflictError
	err := checkNutritionRevision(&two, stored, 1)
	if !errors.As(err, &conflict) || conflict.Expected != 2 || conflict.Actual != 1 || conflict.Authoritative != stored {
		t.Fatalf("stale revision must conflict with the stored row: %v", err)
	}
	err = checkNutritionRevision(&two, authoritativeDay(nil), 0)
	if !errors.As(err, &conflict) || conflict.Actual != 0 || conflict.Message != "resource revision conflict" || conflict.Authoritative != nil {
		t.Fatalf("nonzero revision against a missing row must be a revision conflict without a resource: %v", err)
	}
	if authoritativeDay(stored) != stored {
		t.Fatal("authoritativeDay must pass a real row through")
	}
}

func TestNutritionDayStatusByOutcome(t *testing.T) {
	for outcome, want := range map[model.NutritionDayOutcome]int{
		model.NutritionDayCreated: 201, model.NutritionDayReplaced: 200, model.NutritionDayIgnored: 200, model.NutritionDayRemoved: 204,
	} {
		if got := nutritionDayStatus(outcome); got != want {
			t.Errorf("outcome %d status=%d want %d", outcome, got, want)
		}
	}
}

func TestSameNutritionTargetValues(t *testing.T) {
	kcal, otherKcal, fat := 2200, 2100, 70.0
	a := &model.NutritionTarget{ProteinG: 150, EnergyKcal: &kcal, FatG: &fat}
	b := &model.NutritionTarget{ProteinG: 150, EnergyKcal: &kcal, FatG: &fat}
	if !sameNutritionTargetValues(a, b) {
		t.Fatal("equal values must compare equal")
	}
	b.EnergyKcal = &otherKcal
	if sameNutritionTargetValues(a, b) {
		t.Fatal("different energy must differ")
	}
	b.EnergyKcal = nil
	if sameNutritionTargetValues(a, b) {
		t.Fatal("nil versus set must differ")
	}
}

func TestAccountCleanupIncludesNutrition(t *testing.T) {
	body, err := os.ReadFile("account_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{"DELETE FROM nutrition_days WHERE user_id = $1", "DELETE FROM nutrition_targets WHERE user_id = $1"} {
		if !strings.Contains(string(body), statement) {
			t.Errorf("account cleanup missing %q", statement)
		}
	}
}
