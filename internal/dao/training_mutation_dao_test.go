package dao

import (
	"os"
	"strings"
	"testing"
)

func TestLegacyAdoptionTransactionContract(t *testing.T) {
	body, err := os.ReadFile("training_mutation_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	allSource := string(body)
	start := strings.Index(allSource, "func (r *trainingMutationDAO) AdoptLegacy")
	end := strings.Index(allSource, "func loadLegacyWeeklyAssignments")
	if start < 0 || end <= start {
		t.Fatal("could not isolate legacy adoption transaction")
	}
	source := allSource[start:end]
	ordered := []string{
		"r.pool.Begin(ctx)",
		"lockUserDomain(ctx, tx, programLockNamespace, userID)",
		"lockUserDomain(ctx, tx, scheduleLockNamespace, userID)",
		"findIdempotency(ctx, tx, userID",
		"FROM legacy_adoptions",
		"FROM training_profiles",
		"loadLegacyWeeklyAssignments(ctx, tx, userID)",
	}
	last := -1
	for _, marker := range ordered {
		index := strings.Index(source, marker)
		if index < 0 {
			t.Fatalf("adoption transaction missing %q", marker)
		}
		if index <= last {
			t.Fatalf("adoption transaction marker %q is out of order", marker)
		}
		last = index
	}
	mutationOrder := []string{
		"UPDATE programs SET active=false",
		"insertProgramWorkouts(ctx, tx",
		"insertScheduledWorkouts(ctx, tx",
		"INSERT INTO legacy_adoptions",
	}
	last = -1
	for _, marker := range mutationOrder {
		index := strings.LastIndex(source, marker)
		if index < 0 {
			t.Fatalf("adoption transaction missing %q", marker)
		}
		if index <= last {
			t.Fatalf("adoption mutation marker %q is out of order", marker)
		}
		last = index
	}
	idempotencyIndex := strings.LastIndex(source, "insertIdempotency(ctx, tx")
	commitIndex := strings.LastIndex(source, "tx.Commit(ctx)")
	if idempotencyIndex <= last || commitIndex <= idempotencyIndex {
		t.Fatal("adoption response must be stored before the transaction commits")
	}
	for _, ownership := range []string{
		"FROM weekly_plans wp",
		"wp.user_id=$1",
		"wt.user_id=wp.user_id",
		"WHERE user_id=$1",
	} {
		if !strings.Contains(allSource, ownership) {
			t.Errorf("adoption query missing ownership guard %q", ownership)
		}
	}
}

func mutationSource(t *testing.T, allSource, name string) string {
	t.Helper()
	start := strings.Index(allSource, "func (r *trainingMutationDAO) "+name)
	if start < 0 {
		t.Fatalf("could not find transaction method %s", name)
	}
	rest := allSource[start+1:]
	end := strings.Index(rest, "\nfunc ")
	if end < 0 {
		return allSource[start:]
	}
	return allSource[start : start+1+end]
}

func assertMarkersInOrder(t *testing.T, source string, markers ...string) {
	t.Helper()
	last := -1
	for _, marker := range markers {
		index := strings.Index(source, marker)
		if index < 0 {
			t.Fatalf("transaction missing %q", marker)
		}
		if index <= last {
			t.Fatalf("transaction marker %q is out of order", marker)
		}
		last = index
	}
}

func TestTrainingMutationsStoreReplayInsideObservableTransaction(t *testing.T) {
	body, err := os.ReadFile("training_mutation_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	allSource := string(body)
	cases := []struct {
		name     string
		mutation string
	}{
		{"ReplaceScheduledWorkout", "UPDATE scheduled_workouts SET name"},
		{"UpdateScheduledSetTarget", "UPDATE scheduled_sets ss SET"},
		{"PutRequiredSet", "INSERT INTO set_logs"},
		{"AddExtraSet", "INSERT INTO set_logs"},
		{"FinalizeScheduledWorkout", "UPDATE scheduled_workouts SET status"},
		{"CreateWorkoutSession", "INSERT INTO workout_sessions"},
		{"ReplaceWorkoutSession", "UPDATE workout_sessions SET name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := mutationSource(t, allSource, tc.name)
			assertMarkersInOrder(t, source,
				"beginScheduleMutation(ctx, userID)",
				"findIdempotency(ctx, tx, userID",
				tc.mutation,
				"insertIdempotency(ctx, tx, userID",
				"tx.Commit(ctx)",
			)
		})
	}
}

func TestCompletionAndParticipationShareOneCommit(t *testing.T) {
	body, err := os.ReadFile("training_mutation_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	allSource := string(body)
	assertMarkersInOrder(t, mutationSource(t, allSource, "FinalizeScheduledWorkout"),
		"findIdempotency(ctx, tx, userID",
		"UPDATE scheduled_workouts SET status",
		"UPDATE workout_sessions SET status='completed'",
		"INSERT INTO day_participation",
		"insertIdempotency(ctx, tx, userID",
		"tx.Commit(ctx)",
	)
	assertMarkersInOrder(t, mutationSource(t, allSource, "ReplaceWorkoutSession"),
		"findIdempotency(ctx, tx, userID",
		"UPDATE workout_sessions SET name",
		"INSERT INTO day_participation",
		"insertIdempotency(ctx, tx, userID",
		"tx.Commit(ctx)",
	)
	if !strings.Contains(mutationSource(t, allSource, "AddExtraSet"), "finalized scheduled workouts cannot be changed") {
		t.Error("AddExtraSet does not protect finalized outcomes")
	}
	for _, name := range []string{"UpdateScheduledSetTarget", "PutRequiredSet"} {
		source := mutationSource(t, allSource, name)
		if strings.Contains(source, "finalized scheduled workouts cannot be changed") {
			t.Errorf("%s blocks durable history corrections", name)
		}
	}
	if source := mutationSource(t, allSource, "PutRequiredSet"); !strings.Contains(source, "updateScheduledStatus(ctx, tx, userID") || strings.Contains(source, "INSERT INTO day_participation") {
		t.Error("required-set corrections must re-derive the scheduled result without rewriting participation")
	}
}

func TestNestedMutationProvenanceIsOwnerScoped(t *testing.T) {
	body, err := os.ReadFile("training_mutation_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	allSource := string(body)
	for _, marker := range []string{
		"JOIN programs p ON p.id=pw.program_id AND p.user_id=$2",
		"JOIN workout_templates wt ON wt.id=e.template_id AND wt.user_id=$2",
		"EXISTS (SELECT 1 FROM exercise_catalog ec WHERE ec.id=$1)",
	} {
		if !strings.Contains(allSource, marker) {
			t.Errorf("nested ownership guard missing %q", marker)
		}
	}
	assertMarkersInOrder(t, mutationSource(t, allSource, "ReplaceScheduledWorkout"),
		"findIdempotency(ctx, tx, userID",
		"validateOwnedProgramExerciseProvenance(ctx, tx, userID",
		"UPDATE scheduled_workouts SET name",
	)
	assertMarkersInOrder(t, mutationSource(t, allSource, "AddExtraSet"),
		"findIdempotency(ctx, tx, userID",
		"normalizeExtraExerciseProvenance(ctx, tx, userID",
		"INSERT INTO set_logs",
	)
}
