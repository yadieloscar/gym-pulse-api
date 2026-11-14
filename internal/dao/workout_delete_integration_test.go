//go:build integration

package dao

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// performWorkout records a finished scheduled workout the way completion
// does: a completed linked session with one performed set, a health export,
// and the day's participation.
func (f *releaseFixture) performWorkout(t *testing.T, workout model.ScheduledWorkout, status string, participated bool) uuid.UUID {
	t.Helper()
	f.exec(t, `UPDATE scheduled_workouts SET status=$3, finalized_at=now() WHERE id=$1 AND user_id=$2`, workout.ID, f.userID, status)
	var sessionID uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO workout_sessions (user_id,date,name,status,scheduled_workout_id,completed_at)
		VALUES ($1,$2,'A','completed',$3,now()) RETURNING id`, f.userID, workout.Date, workout.ID).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO set_logs (workout_session_id, scheduled_set_id, is_extra, exercise_name, exercise_category,
		exercise_modality, set_index, actual_reps, actual_weight, completed)
		VALUES ($1,$2,false,'Squat','legs','strength',1,8,100,true)`, sessionID, workout.RequiredSets[0].ID)
	f.exec(t, `INSERT INTO health_exports (id,user_id,workout_kind,workout_id,source,external_id,exported_at)
		VALUES ($1,$2,'scheduled_workout',$3,'apple_health',$4,now())`, uuid.New(), f.userID, workout.ID, uuid.NewString())
	f.exec(t, `INSERT INTO day_participation (user_id,date,scheduled_opportunity,participated,finalized_at,timezone,local_date)
		VALUES ($1,$2,true,$3,now(),'UTC',$2)
		ON CONFLICT (user_id,date) DO UPDATE SET participated=day_participation.participated OR EXCLUDED.participated`,
		f.userID, workout.Date, participated)
	return sessionID
}

func (f *releaseFixture) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(f.ctx, sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestScheduledWorkoutDeleteRemovesPerformanceAndParticipation(t *testing.T) {
	f := newReleaseFixture(t)
	sessionID := f.performWorkout(t, f.workout, model.WorkoutStatusCompleted, true)
	total, err := NewStatsDAO(f.pool).GetTotalWorkouts(f.ctx, f.userID)
	if err != nil || total != 1 {
		t.Fatalf("fixture total workouts = %d, %v; want 1", total, err)
	}

	if err := f.schedules.Delete(f.ctx, f.userID, f.workout.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	for name, n := range map[string]int{
		"scheduled_workouts": f.count(t, `SELECT count(*) FROM scheduled_workouts WHERE id=$1`, f.workout.ID),
		"scheduled_sets":     f.count(t, `SELECT count(*) FROM scheduled_sets WHERE scheduled_workout_id=$1`, f.workout.ID),
		"workout_sessions":   f.count(t, `SELECT count(*) FROM workout_sessions WHERE id=$1`, sessionID),
		"set_logs":           f.count(t, `SELECT count(*) FROM set_logs WHERE workout_session_id=$1`, sessionID),
		"health_exports":     f.count(t, `SELECT count(*) FROM health_exports WHERE user_id=$1`, f.userID),
		"day_participation":  f.count(t, `SELECT count(*) FROM day_participation WHERE user_id=$1`, f.userID),
	} {
		if n != 0 {
			t.Errorf("%s rows after delete = %d, want 0", name, n)
		}
	}
	if total, err := NewStatsDAO(f.pool).GetTotalWorkouts(f.ctx, f.userID); err != nil || total != 0 {
		t.Errorf("total workouts after delete = %d, %v; want 0", total, err)
	}
	var notFound *model.NotFoundError
	if err := f.schedules.Delete(f.ctx, f.userID, f.workout.ID); !errors.As(err, &notFound) {
		t.Errorf("second Delete() error = %v, want NotFoundError", err)
	}
}

func TestScheduledWorkoutDeleteKeepsSameDayEvidence(t *testing.T) {
	f := newReleaseFixture(t)
	others := []model.ScheduledWorkout{releaseWorkout(f.program.ID, f.workout.Date)}
	if err := f.schedules.Create(f.ctx, f.userID, others); err != nil {
		t.Fatal(err)
	}
	f.performWorkout(t, f.workout, model.WorkoutStatusCompleted, true)
	f.exec(t, `UPDATE scheduled_workouts SET status='missed', finalized_at=now() WHERE id=$1`, others[0].ID)

	if err := f.schedules.Delete(f.ctx, f.userID, f.workout.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	var opportunity, participated bool
	if err := f.pool.QueryRow(f.ctx, `SELECT scheduled_opportunity, participated FROM day_participation WHERE user_id=$1 AND date=$2`,
		f.userID, f.workout.Date).Scan(&opportunity, &participated); err != nil {
		t.Fatal(err)
	}
	if !opportunity || participated {
		t.Errorf("participation = opportunity %v participated %v; want the remaining missed workout (true, false)", opportunity, participated)
	}
}

func TestScheduledWorkoutDeleteRejectsActiveSessionAndForeignOwner(t *testing.T) {
	f := newReleaseFixture(t)
	f.exec(t, `INSERT INTO workout_sessions (user_id,date,name,status,scheduled_workout_id) VALUES ($1,$2,'A','active',$3)`,
		f.userID, f.workout.Date, f.workout.ID)
	var conflict *model.ConflictError
	if err := f.schedules.Delete(f.ctx, f.userID, f.workout.ID); !errors.As(err, &conflict) {
		t.Fatalf("Delete() with active session error = %v, want ConflictError", err)
	}
	var notFound *model.NotFoundError
	if err := f.schedules.Delete(f.ctx, uuid.New(), f.workout.ID); !errors.As(err, &notFound) {
		t.Fatalf("foreign Delete() error = %v, want NotFoundError", err)
	}
	if n := f.count(t, `SELECT count(*) FROM scheduled_workouts WHERE id=$1`, f.workout.ID); n != 1 {
		t.Fatalf("rejected deletes removed the workout")
	}
}

func TestDayLogDeleteRemovesHealthExport(t *testing.T) {
	f := newReleaseFixture(t)
	var logID uuid.UUID
	if err := f.pool.QueryRow(f.ctx, `INSERT INTO day_logs (user_id,date,type_id,subtype_id) VALUES ($1,'2026-09-01','push','general') RETURNING id`,
		f.userID).Scan(&logID); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO health_exports (id,user_id,workout_kind,workout_id,source,external_id,exported_at)
		VALUES ($1,$2,'day_log',$3,'apple_health',$4,now())`, uuid.New(), f.userID, logID, uuid.NewString())

	if err := NewLogDAO(f.pool).Delete(f.ctx, f.userID, "2026-09-01"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM health_exports WHERE user_id=$1`, f.userID); n != 0 {
		t.Errorf("health exports after day log delete = %d, want 0", n)
	}
	var notFound *model.NotFoundError
	if err := NewLogDAO(f.pool).Delete(f.ctx, f.userID, "2026-09-01"); !errors.As(err, &notFound) {
		t.Errorf("second Delete() error = %v, want NotFoundError", err)
	}
}
