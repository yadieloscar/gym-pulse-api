package dao

import (
	"os"
	"strings"
	"testing"
)

func TestSportActivityDAOCreateKeepsAtomicWriteOrder(t *testing.T) {
	body, err := os.ReadFile("sport_activity_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	markers := []string{
		"findIdempotency(ctx, tx",
		"WHERE user_id=$1 AND source=$2 AND external_id=$3",
		"INSERT INTO sport_activities",
		"INSERT INTO day_participation",
		"insertIdempotency(ctx, tx",
		"tx.Commit(ctx)",
	}
	last := -1
	for _, marker := range markers {
		position := strings.Index(source, marker)
		if position < 0 {
			t.Fatalf("atomic create missing %q", marker)
		}
		if position <= last {
			t.Fatalf("%q is out of transactional order", marker)
		}
		last = position
	}
	for _, invariant := range []string{
		"WHERE user_id=$1 AND date BETWEEN $2 AND $3",
		"WHERE id=$1 AND user_id=$2",
		"scheduled_opportunity=day_participation.scheduled_opportunity OR EXCLUDED.scheduled_opportunity",
		"ORDER BY date DESC, created_at DESC, id DESC",
		"source, external_id, distance_meters, energy_kcal, avg_heart_rate_bpm",
		"return existing, model.SportActivityExisting, nil",
	} {
		if !strings.Contains(source, invariant) {
			t.Errorf("sport DAO missing %q", invariant)
		}
	}
}

func TestWeeklyStatsUnionsSportDatesWithoutChangingWorkoutTotals(t *testing.T) {
	body, err := os.ReadFile("stats_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	weeklyStart := strings.Index(source, "func (r *statsDAO) GetWeeklyCount")
	totalStart := strings.Index(source, "func (r *statsDAO) GetTotalWorkouts")
	if weeklyStart < 0 || totalStart < 0 {
		t.Fatal("stats methods not found")
	}
	if !strings.Contains(source[weeklyStart:totalStart], "SELECT date FROM sport_activities") {
		t.Fatal("weekly count does not include sport dates")
	}
	nextMethod := strings.Index(source[totalStart+1:], "func (r *statsDAO) GetDistribution")
	if nextMethod < 0 {
		t.Fatal("distribution method not found")
	}
	if strings.Contains(source[totalStart:totalStart+1+nextMethod], "sport_activities") {
		t.Fatal("sport activities unexpectedly changed total_workouts")
	}
}

func TestAccountCleanupIncludesSportActivities(t *testing.T) {
	body, err := os.ReadFile("account_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "DELETE FROM sport_activities WHERE user_id = $1") {
		t.Fatal("account cleanup does not delete owned sport activities")
	}
}

func TestSportActivityDAODuplicateImportWritesNothing(t *testing.T) {
	body, err := os.ReadFile("sport_activity_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	lookup := strings.Index(source, "WHERE user_id=$1 AND source=$2 AND external_id=$3")
	existing := strings.Index(source, "return existing, model.SportActivityExisting, nil")
	insert := strings.Index(source, "INSERT INTO sport_activities")
	participation := strings.Index(source, "INSERT INTO day_participation")
	if lookup < 0 || existing < 0 || insert < 0 || participation < 0 {
		t.Fatal("duplicate import guard not found")
	}
	if lookup >= existing || existing >= insert || insert >= participation {
		t.Fatal("an already imported activity must return before the insert and participation upsert")
	}
	if !strings.Contains(source, "if activity.ExternalID != nil {") {
		t.Fatal("duplicate lookup must only run for imported activities")
	}
	create := source[strings.Index(source, "func (r *sportActivityDAO) Create"):strings.Index(source, "func scanSportActivity")]
	if strings.Count(create, "defer tx.Rollback(ctx)") != 1 {
		t.Fatal("the early return relies on the deferred rollback discarding the open transaction")
	}
}

func TestSportActivityDAOCreateLinksScheduledSportSessionInOrder(t *testing.T) {
	body, err := os.ReadFile("sport_activity_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	createStart, createEnd := strings.Index(source, "func (r *sportActivityDAO) Create"), strings.Index(source, "func scanSportActivity")
	linkStart := strings.Index(source, "func lockSportSession")
	if createStart < 0 || createEnd <= createStart || linkStart < 0 {
		t.Fatal("could not isolate the create transaction and the session linker")
	}
	create := source[createStart:createEnd]
	ordered := []string{
		"lockUserDomain(ctx, tx, scheduleLockNamespace, userID)",
		"lockUserDomain(ctx, tx, sportActivityLockNamespace, userID)",
		"findIdempotency(ctx, tx",
		"WHERE user_id=$1 AND source=$2 AND external_id=$3",
		"lockSportSession(ctx, tx, userID, activity)",
		"INSERT INTO sport_activities",
		"scheduled_workout_id, perceived_effort)",
		"completeSportSession(ctx, tx, userID, session.ID)",
		"INSERT INTO day_participation",
		"insertIdempotency(ctx, tx",
		"tx.Commit(ctx)",
	}
	last := -1
	for _, marker := range ordered {
		position := strings.Index(create, marker)
		if position < 0 {
			t.Fatalf("linked create missing %q", marker)
		}
		if position <= last {
			t.Fatalf("%q is out of transactional order", marker)
		}
		last = position
	}
	linker := source[linkStart:]
	for _, invariant := range []string{
		"WHERE id=$1 AND user_id=$2 FOR UPDATE",
		`&model.NotFoundError{Message: "scheduled workout not found"}`,
		`Field: "scheduled_workout_id"`,
		`Field: "sport_id"`,
		"row.Status == model.WorkoutStatusCompleted",
		"&model.SportSessionConflictError{",
		"case activity.Source != model.ActivitySourceManual:",
		"sw.status IN ('planned', 'missed')",
		"NOT EXISTS (SELECT 1 FROM sport_activities sa WHERE sa.scheduled_workout_id = sw.id)",
		"AND sw.target_duration_minutes <= 3 * $4",
		"ORDER BY abs(sw.target_duration_minutes - $4), sw.sequence_position NULLS LAST, sw.created_at, sw.id",
		"LIMIT 1 FOR UPDATE",
	} {
		if !strings.Contains(linker, invariant) {
			t.Errorf("sport session linker missing %q", invariant)
		}
	}
	if strings.Index(create, "WHERE user_id=$1 AND source=$2 AND external_id=$3") > strings.Index(create, "lockSportSession(") {
		t.Fatal("import duplicate check must run before auto-match so re-imports never link")
	}
	helperStart := strings.Index(source, "func completeSportSession")
	if helperStart < 0 {
		t.Fatal("completeSportSession not found")
	}
	helper := source[helperStart:]
	for _, marker := range []string{"UPDATE scheduled_workouts SET status='completed', finalized_at=COALESCE(finalized_at, now())", "resolution=NULL, resolved_at=NULL, revision=revision+1"} {
		if !strings.Contains(helper, marker) {
			t.Fatalf("completeSportSession missing %q", marker)
		}
	}
	if !strings.Contains(source, "scheduled_workout_id, created_at, updated_at") {
		t.Fatal("reads must scan scheduled_workout_id")
	}
}

func TestAccountCleanupOrderSurvivesSportSessionLink(t *testing.T) {
	body, err := os.ReadFile("account_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	activities := strings.Index(source, "DELETE FROM sport_activities WHERE user_id = $1")
	scheduled := strings.Index(source, "DELETE FROM scheduled_workouts")
	if activities < 0 || scheduled < 0 || activities > scheduled {
		t.Fatal("sport activities must be deleted before scheduled workouts; the link FK is SET NULL for that order")
	}
	migration, err := os.ReadFile("../../migrations/027_add_planned_sport_sessions.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(migration), "REFERENCES scheduled_workouts(id) ON DELETE SET NULL") {
		t.Fatal("link FK must be ON DELETE SET NULL")
	}
}

func TestSportActivityDAOLinkAndUnlinkKeepTransactionalOrder(t *testing.T) {
	body, err := os.ReadFile("sport_activity_dao.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for name, end := range map[string]string{
		"func (r *sportActivityDAO) Link(":   "func (r *sportActivityDAO) Unlink(",
		"func (r *sportActivityDAO) Unlink(": "",
	} {
		start := strings.Index(source, name)
		if start < 0 {
			t.Fatalf("missing %s", name)
		}
		method := source[start:]
		if end != "" {
			stop := strings.Index(source, end)
			if stop <= start {
				t.Fatalf("could not isolate %s", name)
			}
			method = source[start:stop]
		}
		scope := "sport-activities/link"
		if strings.Contains(name, "Unlink") {
			scope = "sport-activities/unlink"
		}
		ordered := []string{
			"lockUserDomain(ctx, tx, scheduleLockNamespace, userID)",
			"lockUserDomain(ctx, tx, sportActivityLockNamespace, userID)",
			`findIdempotency(ctx, tx, userID, "` + scope + `"`,
			"lockOwnedActivity(ctx, tx, userID, activityID)",
			"lockSessionForLinkChange(ctx, tx, userID,",
			"UPDATE sport_activities SET scheduled_workout_id=",
			"insertIdempotency(ctx, tx, userID, record, result)",
			"tx.Commit(ctx)",
		}
		last := -1
		for _, marker := range ordered {
			position := strings.Index(method, marker)
			if position < 0 {
				t.Fatalf("%s missing %q", name, marker)
			}
			if position <= last {
				t.Fatalf("%s: %q is out of order", name, marker)
			}
			last = position
		}
		if strings.Contains(method, "INSERT INTO sport_activities") {
			t.Fatalf("%s must never create an activity", name)
		}
	}
	linkStart, unlinkStart := strings.Index(source, "func (r *sportActivityDAO) Link("), strings.Index(source, "func (r *sportActivityDAO) Unlink(")
	if linkStart < 0 || unlinkStart <= linkStart {
		t.Fatal("could not isolate link and unlink")
	}
	link := source[linkStart:unlinkStart]
	for _, marker := range []string{`Field: "kind"`, `Field: "sport_id"`, `Reason: "activity_linked"`, `Reason: "session_completed"`, "completeSportSession(ctx, tx, userID, session.ID)"} {
		if !strings.Contains(link, marker) {
			t.Errorf("link missing %q", marker)
		}
	}
	if strings.Contains(link, "INSERT INTO day_participation") {
		t.Error("link must not credit participation again")
	}
	unlink := source[unlinkStart:]
	for _, marker := range []string{`Reason: "not_linked"`, "WHEN date >= $3::date THEN 'planned' ELSE 'missed' END", "WHEN date >= $3::date THEN NULL ELSE COALESCE(finalized_at, now()) END", "scheduled_opportunity=true"} {
		if !strings.Contains(unlink, marker) {
			t.Errorf("unlink missing %q", marker)
		}
	}
}
