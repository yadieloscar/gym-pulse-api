package dao

import (
	"os"
	"strings"
	"testing"
)

func trainingBlockDAOSource(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile("training_block_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func trainingBlockMethod(t *testing.T, source, signature, next string) string {
	t.Helper()
	start := strings.Index(source, signature)
	if start < 0 {
		t.Fatalf("method %q not found", signature)
	}
	end := len(source)
	if next != "" {
		relative := strings.Index(source[start+len(signature):], next)
		if relative < 0 {
			t.Fatalf("next method %q not found", next)
		}
		end = start + len(signature) + relative
	}
	return source[start:end]
}

func TestTrainingBlockDAOScopesReadsAndBoundsLists(t *testing.T) {
	source := trainingBlockDAOSource(t)
	for _, invariant := range []string{
		"WHERE b.user_id=$1 AND ($2='all' OR b.status=$2)",
		"ORDER BY b.updated_at DESC, b.id DESC",
		"limit+1",
		"WHERE b.id=$1 AND b.user_id=$2",
		"SELECT true FROM programs WHERE id=$1 AND user_id=$2",
		"WHERE id=$1 AND block_id=$2",
	} {
		if !strings.Contains(source, invariant) {
			t.Errorf("training block DAO missing %q", invariant)
		}
	}
}

func TestTrainingBlockDAOMutationsLockReplayReviseAndCommitAtomically(t *testing.T) {
	source := trainingBlockDAOSource(t)
	methods := []struct{ signature, next, write string }{
		{"func (r *trainingBlockDAO) AddExposure", "func (r *trainingBlockDAO) RecordNextMorning", "INSERT INTO criteria_training_exposures"},
		{"func (r *trainingBlockDAO) RecordNextMorning", "func (r *trainingBlockDAO) Transition", "UPDATE criteria_training_exposures"},
		{"func (r *trainingBlockDAO) Transition", "const trainingBlockSummarySelect", "INSERT INTO criteria_training_transitions"},
	}
	for _, method := range methods {
		body := trainingBlockMethod(t, source, method.signature, method.next)
		markers := []string{
			"lockUserDomain(ctx, tx",
			"findIdempotency(ctx, tx",
			"loadTrainingBlock(ctx, tx, userID, blockID, true)",
			"requireBlockRevision(block, req.ExpectedRevision)",
			method.write,
			"storeTrainingBlockIdempotency(ctx, tx",
			"tx.Commit(ctx)",
		}
		last := -1
		for _, marker := range markers {
			position := strings.Index(body, marker)
			if position < 0 {
				t.Errorf("%s missing %q", method.signature, marker)
				continue
			}
			if position <= last {
				t.Errorf("%s has %q out of atomic order", method.signature, marker)
			}
			last = position
		}
	}
}

func TestTrainingBlockDAOQualificationIsServerDerivedAndCrossDomainIsolated(t *testing.T) {
	source := trainingBlockDAOSource(t)
	for _, qualification := range []string{
		"e.session_outcome='completed_as_planned'",
		"e.next_morning_response='baseline'",
		"TrainingExposureQualifies(exposure.SessionOutcome, exposure.NextMorningResponse)",
	} {
		if !strings.Contains(source, qualification) {
			t.Errorf("qualification missing %q", qualification)
		}
	}
	for _, prohibited := range []string{
		"UPDATE programs", "UPDATE scheduled_workouts", "INSERT INTO sport_activities",
		"INSERT INTO day_participation", "UPDATE day_participation",
	} {
		if strings.Contains(source, prohibited) {
			t.Errorf("training block DAO has prohibited cross-domain write %q", prohibited)
		}
	}
}
