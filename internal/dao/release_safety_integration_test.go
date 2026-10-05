//go:build integration

package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// These tests require a disposable, migrated database and DDL permissions for
// owner-scoped fault triggers. CI runs them against its PostgreSQL smoke stack.
type releaseFixture struct {
	ctx        context.Context
	cleanupCtx context.Context
	pool       *pgxpool.Pool
	userID     uuid.UUID
	program    *model.Program
	workout    model.ScheduledWorkout
	appName    string
	mutations  *trainingMutationDAO
	schedules  *scheduleDAO
}

func newReleaseFixture(t *testing.T) *releaseFixture {
	t.Helper()
	url := os.Getenv("GYMPULSE_TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("integration tests require GYMPULSE_TEST_DATABASE_URL pointing to a disposable migrated database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(t.Context()), 2*time.Minute)
	t.Cleanup(cleanupCancel)
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	userID := uuid.New()
	appName := "release-test-" + userID.String()
	config.ConnConfig.RuntimeParams["application_name"] = appName
	config.MaxConns = 8
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	f := &releaseFixture{ctx: ctx, cleanupCtx: cleanupCtx, pool: pool, userID: userID, appName: appName,
		mutations: &trainingMutationDAO{pool: pool}, schedules: &scheduleDAO{pool: pool}}
	f.exec(t, `INSERT INTO auth.users (id) VALUES ($1)`, userID)
	t.Cleanup(func() {
		if err := NewAccountDAO(pool).DeleteUserData(cleanupCtx, userID); err != nil {
			t.Errorf("cleaning fixture data: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM auth.users WHERE id=$1`, userID); err != nil {
			t.Errorf("cleaning fixture identity: %v", err)
		}
	})
	f.exec(t, `INSERT INTO training_profiles (user_id, primary_goal, available_days,
		usual_activity, experience, equipment, session_duration_minutes, timezone)
		VALUES ($1,'strength',ARRAY[1,3]::smallint[],'moderate','beginner','{}',45,'UTC')`, userID)
	f.program = releaseProgram()
	if err := (&programDAO{pool: pool}).Create(ctx, userID, f.program); err != nil {
		t.Fatal(err)
	}
	workouts := []model.ScheduledWorkout{releaseWorkout(f.program.ID, "2026-10-12")}
	if err := f.schedules.Create(ctx, userID, workouts); err != nil {
		t.Fatal(err)
	}
	f.workout = workouts[0]
	return f
}

func releaseProgram() *model.Program {
	reps := 8
	return &model.Program{Name: "Fixture", PrimaryGoal: "strength", Roadmap: map[string]any{}, Active: true,
		Workouts: []model.ProgramWorkout{{Name: "A", SequencePosition: 1,
			Exercises: []model.ProgramExercise{{Name: "Squat", Category: "legs", Modality: "strength",
				ExerciseOrder: 1, TargetSets: 1, TargetReps: &reps}}}}}
}

func releaseWorkout(programID uuid.UUID, date string) model.ScheduledWorkout {
	reps := 8
	return model.ScheduledWorkout{ProgramID: &programID, Date: date, Name: "A",
		Status: model.WorkoutStatusPlanned, RequiredSets: []model.ScheduledSet{{
			ExerciseName: "Squat", ExerciseCategory: "legs", ExerciseModality: "strength",
			ExerciseOrder: 1, SetIndex: 1, TargetReps: &reps}}}
}

func releaseRecord(scope string) model.IdempotencyRecord {
	return model.IdempotencyRecord{Scope: scope, OperationKey: uuid.NewString(), RequestHash: "original",
		ResponseStatus: 200, ResourceType: "integration_fixture"}
}

func (f *releaseFixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(f.ctx, sql, args...); err != nil {
		t.Fatal(err)
	}
}

// snapshot compares complete persisted rows, including child sets and replay
// records, so rollback checks detect partial updates rather than just counts.
func (f *releaseFixture) snapshot(t *testing.T) string {
	t.Helper()
	var snapshot string
	err := f.pool.QueryRow(f.ctx, `SELECT jsonb_build_object(
		'programs', (SELECT jsonb_agg(to_jsonb(p) ORDER BY p.id) FROM programs p WHERE p.user_id=$1),
		'workouts', (SELECT jsonb_agg(to_jsonb(w) ORDER BY w.id) FROM scheduled_workouts w WHERE w.user_id=$1),
		'sets', (SELECT jsonb_agg(to_jsonb(s) ORDER BY s.id) FROM scheduled_sets s JOIN scheduled_workouts w ON w.id=s.scheduled_workout_id WHERE w.user_id=$1),
		'sessions', (SELECT jsonb_agg(to_jsonb(s) ORDER BY s.id) FROM workout_sessions s WHERE s.user_id=$1),
		'logs', (SELECT jsonb_agg(to_jsonb(l) ORDER BY l.id) FROM set_logs l JOIN workout_sessions s ON s.id=l.workout_session_id WHERE s.user_id=$1),
		'participation', (SELECT jsonb_agg(to_jsonb(p) ORDER BY p.id) FROM day_participation p WHERE p.user_id=$1),
		'replay', (SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id) FROM idempotency_records r WHERE r.user_id=$1))::text`, f.userID).Scan(&snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func (f *releaseFixture) injectFailure(t *testing.T, table string) {
	t.Helper()
	name := "release_fault_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	owner := "NEW.user_id"
	if table == "scheduled_sets" {
		owner = "(SELECT user_id FROM scheduled_workouts WHERE id=NEW.scheduled_workout_id)"
	}
	// Names and owner UUID are generated here; table is one of the fixed test cases.
	f.exec(t, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF %s = '%s'::uuid THEN RAISE EXCEPTION 'release-test injected failure'; END IF;
		RETURN NEW; END $$`, name, owner, f.userID))
	t.Cleanup(func() {
		if _, err := f.pool.Exec(f.cleanupCtx, fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON %s; DROP FUNCTION IF EXISTS %s()", name, table, name)); err != nil {
			t.Errorf("removing fault trigger: %v", err)
		}
	})
	f.exec(t, fmt.Sprintf("CREATE TRIGGER %s BEFORE INSERT ON %s FOR EACH ROW EXECUTE FUNCTION %s()", name, table, name))
}

func assertInjectedRollback(t *testing.T, f *releaseFixture, mutate func() error) {
	t.Helper()
	before := f.snapshot(t)
	if err := mutate(); err == nil || !strings.Contains(err.Error(), "release-test injected failure") {
		t.Fatalf("mutation error = %v; want injected failure", err)
	}
	if after := f.snapshot(t); before != after {
		t.Fatalf("failed mutation changed persisted domain rows\nbefore: %s\nafter: %s", before, after)
	}
}

func TestReleaseCompletionRollbackIntegration(t *testing.T) {
	for _, operation := range []string{"scheduled", "session"} {
		for _, table := range []string{"day_participation", "idempotency_records"} {
			t.Run(operation+"/"+table, func(t *testing.T) {
				f := newReleaseFixture(t)
				var sessionID uuid.UUID
				if err := f.pool.QueryRow(f.ctx, `INSERT INTO workout_sessions (user_id,date,name,status,scheduled_workout_id)
					VALUES ($1,'2026-10-12','A','active',$2) RETURNING id`, f.userID, f.workout.ID).Scan(&sessionID); err != nil {
					t.Fatal(err)
				}
				f.injectFailure(t, table)
				assertInjectedRollback(t, f, func() error {
					if operation == "scheduled" {
						_, _, err := f.mutations.FinalizeScheduledWorkout(f.ctx, f.userID, f.workout.ID, 1, time.Now(), releaseRecord("scheduled-workouts/complete"))
						return err
					}
					session := &model.WorkoutSession{ID: sessionID, ScheduledWorkoutID: &f.workout.ID,
						Date: "2026-10-12", Name: "A", Status: "completed"}
					_, _, err := f.mutations.ReplaceWorkoutSession(f.ctx, f.userID, session, 1, time.Now(), releaseRecord("workout-sessions/update"))
					return err
				})
			})
		}
	}
}

func TestReleaseRegenerationRollbackIntegration(t *testing.T) {
	for _, table := range []string{"scheduled_sets", "idempotency_records"} {
		t.Run(table, func(t *testing.T) {
			f := newReleaseFixture(t)
			f.injectFailure(t, table)
			assertInjectedRollback(t, f, func() error {
				_, _, err := f.schedules.RegenerateIdempotent(f.ctx, f.userID, f.program.ID, 1,
					"2026-10-12", "2026-10-12", []model.ScheduledWorkout{releaseWorkout(f.program.ID, "2026-10-12")},
					&model.RegenerateScheduleResponse{}, releaseRecord("schedule/regenerate"))
				return err
			})
		})
	}
}

type releaseResult struct {
	value  any
	replay bool
	err    error
}

// waitForAdvisoryWaiters observes PostgreSQL lock waiters instead of using a
// sleep to guess whether both requests overlap.
func (f *releaseFixture) waitForAdvisoryWaiters(t *testing.T, want int) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var count int
		if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE application_name=$1 AND wait_event='advisory'`, f.appName).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count >= want {
			return
		}
		select {
		case <-f.ctx.Done():
			t.Fatalf("waiting for %d advisory waiters: %v", want, f.ctx.Err())
		case <-ticker.C:
		}
	}
}

func (f *releaseFixture) blockDomain(t *testing.T, namespace string) pgx.Tx {
	t.Helper()
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(f.cleanupCtx) })
	if err := lockUserDomain(f.ctx, tx, namespace, f.userID); err != nil {
		t.Fatal(err)
	}
	return tx
}

func assertSameResult(t *testing.T, a, b any) {
	t.Helper()
	aJSON, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	bJSON, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if string(aJSON) != string(bJSON) {
		t.Fatalf("replay differs\noriginal: %s\nreplay: %s", aJSON, bJSON)
	}
}

func TestReleaseDuplicateMutationsIntegration(t *testing.T) {
	for _, family := range []string{"clone", "materialize", "recovery", "finalize", "session-complete"} {
		t.Run(family, func(t *testing.T) {
			f := newReleaseFixture(t)
			record := releaseRecord(family)
			namespace := scheduleLockNamespace
			var starterID, sessionID uuid.UUID
			if family == "clone" {
				namespace = programLockNamespace
				if err := f.pool.QueryRow(f.ctx, `INSERT INTO starter_programs
					(slug,version,name,primary_goal,min_days,max_days,duration_minutes)
					VALUES ($1,1,'Fixture','strength',1,2,45) RETURNING id`, uuid.NewString()).Scan(&starterID); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _, _ = f.pool.Exec(f.cleanupCtx, `DELETE FROM starter_programs WHERE id=$1`, starterID) })
			}
			if family == "recovery" {
				f.exec(t, `UPDATE scheduled_workouts SET status='missed', finalized_at=now() WHERE id=$1`, f.workout.ID)
			}
			if family == "session-complete" {
				if err := f.pool.QueryRow(f.ctx, `INSERT INTO workout_sessions (user_id,date,name,status)
					VALUES ($1,'2026-10-12','Off plan','active') RETURNING id`, f.userID).Scan(&sessionID); err != nil {
					t.Fatal(err)
				}
			}
			mutate := func(rec model.IdempotencyRecord) releaseResult {
				switch family {
				case "clone":
					program := releaseProgram()
					version := 1
					program.StarterProgramID, program.StarterVersion = &starterID, &version
					v, replay, err := (&programDAO{pool: f.pool}).CreateIdempotent(f.ctx, f.userID, program, rec)
					return releaseResult{value: v, replay: replay, err: err}
				case "materialize":
					v, replay, err := f.schedules.MaterializeIdempotent(f.ctx, f.userID, f.program.ID, 1,
						[]model.ScheduledWorkout{releaseWorkout(f.program.ID, "2026-10-13")}, rec)
					return releaseResult{value: v, replay: replay, err: err}
				case "recovery":
					v, replay, err := f.schedules.RecoverIdempotent(f.ctx, f.userID, "2026-10-13", rec)
					return releaseResult{value: v, replay: replay, err: err}
				case "finalize":
					v, replay, err := f.mutations.FinalizeScheduledWorkout(f.ctx, f.userID, f.workout.ID, 1, time.Now(), rec)
					return releaseResult{value: v, replay: replay, err: err}
				default:
					session := &model.WorkoutSession{ID: sessionID, Date: "2026-10-12", Name: "Off plan", Status: "completed"}
					v, replay, err := f.mutations.ReplaceWorkoutSession(f.ctx, f.userID, session, 1, time.Now(), rec)
					return releaseResult{value: v, replay: replay, err: err}
				}
			}
			blocker := f.blockDomain(t, namespace)
			results := make(chan releaseResult, 2)
			for range 2 {
				go func() { results <- mutate(record) }()
			}
			f.waitForAdvisoryWaiters(t, 2)
			if err := blocker.Commit(f.ctx); err != nil {
				t.Fatal(err)
			}
			a, b := <-results, <-results
			if a.err != nil || b.err != nil || a.replay == b.replay {
				t.Fatalf("concurrent outcomes: first=%+v second=%+v", a, b)
			}
			assertSameResult(t, a.value, b.value)
			var count int
			if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM idempotency_records
				WHERE user_id=$1 AND scope=$2 AND operation_key=$3`, f.userID, record.Scope, record.OperationKey).Scan(&count); err != nil || count != 1 {
				t.Fatalf("replay record count=%d err=%v", count, err)
			}
			switch family {
			case "clone":
				if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM programs WHERE user_id=$1 AND starter_program_id=$2`, f.userID, starterID).Scan(&count); err != nil || count != 1 {
					t.Fatalf("cloned program count=%d err=%v", count, err)
				}
			case "materialize", "recovery":
				if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM scheduled_workouts WHERE user_id=$1 AND date='2026-10-13'`, f.userID).Scan(&count); err != nil || count != 1 {
					t.Fatalf("created occurrence count=%d err=%v", count, err)
				}
			default:
				if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM day_participation WHERE user_id=$1`, f.userID).Scan(&count); err != nil || count != 1 {
					t.Fatalf("participation count=%d err=%v", count, err)
				}
			}
			// Advance state independently: a replay must return the original body.
			f.exec(t, `UPDATE programs SET revision=revision+1, name='Changed' WHERE user_id=$1`, f.userID)
			f.exec(t, `UPDATE scheduled_workouts SET revision=revision+1 WHERE user_id=$1`, f.userID)
			f.exec(t, `UPDATE workout_sessions SET revision=revision+1 WHERE user_id=$1`, f.userID)
			replayed := mutate(record)
			if replayed.err != nil || !replayed.replay {
				t.Fatalf("later replay: %+v", replayed)
			}
			assertSameResult(t, a.value, replayed.value)
			before := f.snapshot(t)
			mismatch := record
			mismatch.RequestHash = "changed input"
			conflict := mutate(mismatch)
			var conflictErr *model.ConflictError
			if !errors.As(conflict.err, &conflictErr) {
				t.Fatalf("changed payload error=%v; want conflict", conflict.err)
			}
			if after := f.snapshot(t); before != after {
				t.Fatal("payload mismatch changed persisted state")
			}
		})
	}
}

func TestReleaseSessionStartRegenerationOrderingIntegration(t *testing.T) {
	for _, first := range []string{"start", "regenerate"} {
		t.Run(first+"-first", func(t *testing.T) {
			testSessionStartRegenerationOrder(t, first)
		})
	}
}

func testSessionStartRegenerationOrder(t *testing.T, first string) {
	t.Helper()
	f := newReleaseFixture(t)
	blocker := f.blockDomain(t, scheduleLockNamespace)
	started := make(chan releaseResult, 1)
	start := func() {
		v, replay, err := f.mutations.PutRequiredSet(f.ctx, f.userID, f.workout.ID, f.workout.RequiredSets[0].ID,
			model.SetMutationRequest{ExpectedRevision: 1, OperationKey: "start", Completed: true}, releaseRecord("required-set"))
		started <- releaseResult{value: v, replay: replay, err: err}
	}
	regenerated := make(chan error, 1)
	regenerate := func() {
		_, _, err := f.schedules.RegenerateIdempotent(f.ctx, f.userID, f.program.ID, 1,
			"2026-10-12", "2026-10-12", []model.ScheduledWorkout{releaseWorkout(f.program.ID, "2026-10-12")},
			&model.RegenerateScheduleResponse{}, releaseRecord("regenerate"))
		regenerated <- err
	}
	if first == "start" {
		go start()
		f.waitForAdvisoryWaiters(t, 1)
		go regenerate()
	} else {
		go regenerate()
		f.waitForAdvisoryWaiters(t, 1)
		go start()
	}
	f.waitForAdvisoryWaiters(t, 2)
	if err := blocker.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	startResult, regenerationErr := <-started, <-regenerated
	if first == "regenerate" {
		var missing *model.NotFoundError
		if regenerationErr != nil || !errors.As(startResult.err, &missing) {
			t.Fatalf("regeneration-first results: start=%v regenerate=%v", startResult.err, regenerationErr)
		}
		var sessions int
		if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM workout_sessions WHERE user_id=$1`, f.userID).Scan(&sessions); err != nil || sessions != 0 {
			t.Fatalf("regeneration-first left sessions=%d err=%v", sessions, err)
		}
		workouts, err := f.schedules.List(f.ctx, f.userID, "2026-10-12", "2026-10-12")
		if err != nil || len(workouts) != 1 || workouts[0].ID == f.workout.ID || workouts[0].RequiredSets[0].Checked {
			t.Fatalf("replacement schedule=%+v err=%v", workouts, err)
		}
		return
	}
	if startResult.err != nil {
		t.Fatalf("session start failed: %v", startResult.err)
	}
	var conflict *model.ConflictError
	if !errors.As(regenerationErr, &conflict) {
		t.Fatalf("regeneration error=%v; want active-session conflict", regenerationErr)
	}
	workout, err := f.schedules.Get(f.ctx, f.userID, f.workout.ID)
	if err != nil || !workout.RequiredSets[0].Checked || workout.Status != model.WorkoutStatusInProgress {
		t.Fatalf("started workout lost: workout=%+v err=%v", workout, err)
	}
}

func TestReleaseFreshAdoptionLoadsCurrentResourcesIntegration(t *testing.T) {
	f := newReleaseFixture(t)
	var templateID uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO workout_templates (user_id,name,type_id)
		VALUES ($1,'Legacy','strength') RETURNING id`, f.userID).Scan(&templateID); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO exercises (template_id,name,sets,reps) VALUES ($1,'Squat',1,8)`, templateID)
	f.exec(t, `INSERT INTO weekly_plans (user_id,weekday,template_id) VALUES ($1,1,$2)`, f.userID, templateID)
	record := releaseRecord("programs/adopt-legacy")
	adopted, _, err := f.mutations.AdoptLegacy(f.ctx, f.userID, time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC), record)
	if err != nil || !adopted.Adopted || len(adopted.Schedule) != 1 {
		t.Fatalf("adoption=%+v err=%v", adopted, err)
	}
	f.exec(t, `UPDATE programs SET name='Renamed',active=false,revision=revision+1 WHERE id=$1`, adopted.Program.ID)
	f.exec(t, `UPDATE scheduled_workouts SET name='Corrected',revision=revision+1 WHERE id=$1`, adopted.Schedule[0].ID)
	freshRecord := releaseRecord(record.Scope)
	fresh, _, err := f.mutations.AdoptLegacy(f.ctx, f.userID, time.Now(), freshRecord)
	if err != nil || fresh.Adopted || fresh.Program.Name != "Renamed" || fresh.Program.Active ||
		fresh.Program.Revision != adopted.Program.Revision+1 || len(fresh.Schedule) != 1 || fresh.Schedule[0].Name != "Corrected" {
		t.Fatalf("fresh adoption is stale: response=%+v err=%v", fresh, err)
	}
	originalReplay, replay, err := f.mutations.AdoptLegacy(f.ctx, f.userID, time.Now(), record)
	if err != nil || !replay {
		t.Fatalf("original operation replay=%t err=%v", replay, err)
	}
	assertSameResult(t, adopted, originalReplay)
	before := f.snapshot(t)
	for range 100 {
		response, replay, err := f.mutations.AdoptLegacy(f.ctx, f.userID, time.Now(), record)
		if err != nil || !replay {
			t.Fatalf("repeated adoption replay=%t err=%v", replay, err)
		}
		assertSameResult(t, adopted, response)
	}
	if after := f.snapshot(t); before != after {
		t.Fatal("100 adoption replays changed persisted state")
	}
	// A regenerated workout has a new UUID; a fresh operation must include it.
	f.exec(t, `DELETE FROM scheduled_workouts WHERE id=$1`, adopted.Schedule[0].ID)
	workouts := []model.ScheduledWorkout{releaseWorkout(adopted.Program.ID, "2026-10-13")}
	if err := f.schedules.Create(f.ctx, f.userID, workouts); err != nil {
		t.Fatal(err)
	}
	replaced, _, err := f.mutations.AdoptLegacy(f.ctx, f.userID, time.Now(), releaseRecord(record.Scope))
	if err != nil || len(replaced.Schedule) != 1 || replaced.Schedule[0].ID != workouts[0].ID {
		t.Fatalf("replacement missing from fresh adoption: response=%+v err=%v", replaced, err)
	}
	freshReplay, _, err := f.mutations.AdoptLegacy(f.ctx, f.userID, time.Now(), freshRecord)
	if err != nil {
		t.Fatal(err)
	}
	assertSameResult(t, fresh, freshReplay)
}
