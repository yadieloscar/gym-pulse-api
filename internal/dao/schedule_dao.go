package dao

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type ScheduleDAO interface {
	List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.ScheduledWorkout, error)
	Get(ctx context.Context, userID, workoutID uuid.UUID) (*model.ScheduledWorkout, error)
	Create(ctx context.Context, userID uuid.UUID, workouts []model.ScheduledWorkout) error
	ReplaceSnapshot(ctx context.Context, userID uuid.UUID, workout *model.ScheduledWorkout, expectedRevision int64) error
	UpdateOutcome(ctx context.Context, userID uuid.UUID, workout *model.ScheduledWorkout, expectedRevision int64) error
	UpdateSetTarget(ctx context.Context, userID, workoutID, setID uuid.UUID, target model.PatchScheduledSetTargetRequest) error
	DeleteUnstartedRange(ctx context.Context, userID uuid.UUID, from, to string) ([]uuid.UUID, error)
}

func (r *scheduleDAO) UpdateOutcome(ctx context.Context, userID uuid.UUID, w *model.ScheduledWorkout, expectedRevision int64) error {
	err := r.pool.QueryRow(ctx, `
		UPDATE scheduled_workouts SET status=$3, finalized_at=$4,
		       revision=revision+1, updated_at=now()
		WHERE id=$1 AND user_id=$2 AND revision=$5
		RETURNING revision, updated_at`, w.ID, userID, w.Status, w.FinalizedAt,
		expectedRevision).Scan(&w.Revision, &w.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		current, getErr := r.Get(ctx, userID, w.ID)
		if getErr != nil {
			return getErr
		}
		return &model.ConflictError{Message: "scheduled workout revision conflict", Expected: expectedRevision, Actual: current.Revision, Authoritative: current}
	}
	if err != nil {
		return fmt.Errorf("updating scheduled workout outcome: %w", err)
	}
	return nil
}

type scheduleDAO struct {
	pool *pgxpool.Pool
}

func NewScheduleDAO(pool *pgxpool.Pool) ScheduleDAO {
	return &scheduleDAO{pool: pool}
}

func (r *scheduleDAO) List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.ScheduledWorkout, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, program_id, program_workout_id, to_char(date, 'YYYY-MM-DD'),
		       name, sequence_position, status, finalized_at, revision, created_at, updated_at
		FROM scheduled_workouts
		WHERE user_id=$1 AND date BETWEEN $2 AND $3
		ORDER BY date, created_at`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying scheduled workouts: %w", err)
	}
	defer rows.Close()

	workouts := []model.ScheduledWorkout{}
	for rows.Next() {
		var w model.ScheduledWorkout
		if err := rows.Scan(&w.ID, &w.ProgramID, &w.ProgramWorkoutID, &w.Date,
			&w.Name, &w.SequencePosition, &w.Status, &w.FinalizedAt, &w.Revision,
			&w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning scheduled workout: %w", err)
		}
		workouts = append(workouts, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating scheduled workouts: %w", err)
	}
	for i := range workouts {
		if err := r.loadSets(ctx, userID, &workouts[i]); err != nil {
			return nil, err
		}
	}
	return workouts, nil
}

func (r *scheduleDAO) Get(ctx context.Context, userID, workoutID uuid.UUID) (*model.ScheduledWorkout, error) {
	w := &model.ScheduledWorkout{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, program_id, program_workout_id, to_char(date, 'YYYY-MM-DD'),
		       name, sequence_position, status, finalized_at, revision, created_at, updated_at
		FROM scheduled_workouts WHERE id=$1 AND user_id=$2`, workoutID, userID).Scan(
		&w.ID, &w.ProgramID, &w.ProgramWorkoutID, &w.Date, &w.Name,
		&w.SequencePosition, &w.Status, &w.FinalizedAt, &w.Revision,
		&w.CreatedAt, &w.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "scheduled workout not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("querying scheduled workout: %w", err)
	}
	if err := r.loadSets(ctx, userID, w); err != nil {
		return nil, err
	}
	return w, nil
}

func (r *scheduleDAO) loadSets(ctx context.Context, userID uuid.UUID, w *model.ScheduledWorkout) error {
	rows, err := r.pool.Query(ctx, `
		SELECT ss.id, ss.program_exercise_id, ss.catalog_id, ss.exercise_name,
		       ss.exercise_category, ss.exercise_modality, ss.exercise_order,
		       ss.set_index, ss.target_reps, ss.target_weight,
		       ss.target_duration_seconds, ss.rest_seconds, ss.notes,
		       (sl.id IS NOT NULL) AS checked, sl.id,
		       sl.actual_reps, sl.actual_weight, sl.duration_seconds
		FROM scheduled_sets ss
		JOIN scheduled_workouts sw ON sw.id=ss.scheduled_workout_id AND sw.user_id=$1
		LEFT JOIN workout_sessions ws ON ws.scheduled_workout_id=sw.id AND ws.user_id=$1
		LEFT JOIN set_logs sl ON sl.workout_session_id=ws.id
		                     AND sl.scheduled_set_id=ss.id AND sl.completed=true
		WHERE ss.scheduled_workout_id=$2
		ORDER BY ss.exercise_order, ss.set_index`, userID, w.ID)
	if err != nil {
		return fmt.Errorf("querying scheduled sets: %w", err)
	}
	defer rows.Close()
	w.RequiredSets = []model.ScheduledSet{}
	for rows.Next() {
		var s model.ScheduledSet
		if err := rows.Scan(
			&s.ID, &s.ProgramExerciseID, &s.CatalogID, &s.ExerciseName,
			&s.ExerciseCategory, &s.ExerciseModality, &s.ExerciseOrder, &s.SetIndex,
			&s.TargetReps, &s.TargetWeight, &s.TargetDurationSeconds, &s.RestSeconds,
			&s.Notes, &s.Checked, &s.PerformedSetID, &s.ActualReps,
			&s.ActualWeight, &s.ActualDurationSeconds,
		); err != nil {
			return fmt.Errorf("scanning scheduled set: %w", err)
		}
		w.RequiredSets = append(w.RequiredSets, s)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating scheduled sets: %w", err)
	}

	extraRows, err := r.pool.Query(ctx, `
		SELECT sl.id, sl.scheduled_set_id, sl.exercise_id, sl.is_extra,
		       sl.exercise_name, sl.exercise_category, sl.exercise_modality,
		       sl.set_index, sl.target_reps, sl.target_weight, sl.actual_reps,
		       sl.actual_weight, sl.duration_seconds, sl.completed,
		       sl.operation_key, sl.revision
		FROM set_logs sl
		JOIN workout_sessions ws ON ws.id=sl.workout_session_id AND ws.user_id=$1
		WHERE ws.scheduled_workout_id=$2 AND sl.is_extra=true
		ORDER BY ws.created_at, sl.logged_at`, userID, w.ID)
	if err != nil {
		return fmt.Errorf("querying extra sets: %w", err)
	}
	defer extraRows.Close()
	w.ExtraSets = []model.PerformedSet{}
	for extraRows.Next() {
		var s model.PerformedSet
		if err := scanPerformedSet(extraRows, &s); err != nil {
			return fmt.Errorf("scanning extra set: %w", err)
		}
		w.ExtraSets = append(w.ExtraSets, s)
	}
	return extraRows.Err()
}

func (r *scheduleDAO) UpdateSetTarget(ctx context.Context, userID, workoutID, setID uuid.UUID, target model.PatchScheduledSetTargetRequest) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning scheduled set target update: %w", err)
	}
	defer tx.Rollback(ctx)
	var revision int64
	err = tx.QueryRow(ctx, `SELECT revision FROM scheduled_workouts WHERE id=$1 AND user_id=$2 FOR UPDATE`, workoutID, userID).Scan(&revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return &model.NotFoundError{Message: "scheduled workout not found"}
	}
	if err != nil {
		return fmt.Errorf("locking scheduled workout: %w", err)
	}
	if revision != target.ExpectedRevision {
		return &model.ConflictError{Message: "scheduled workout revision conflict", Expected: target.ExpectedRevision, Actual: revision}
	}
	result, err := tx.Exec(ctx, `
		UPDATE scheduled_sets ss SET target_reps=$4, target_weight=$5,
		       target_duration_seconds=$6, rest_seconds=$7, notes=$8
		FROM scheduled_workouts sw
		WHERE ss.id=$1 AND ss.scheduled_workout_id=$2 AND sw.id=$2 AND sw.user_id=$3`,
		setID, workoutID, userID, target.TargetReps, target.TargetWeight,
		target.TargetDurationSeconds, target.RestSeconds, target.Notes)
	if err != nil {
		return fmt.Errorf("updating scheduled set target: %w", err)
	}
	if result.RowsAffected() == 0 {
		return &model.NotFoundError{Message: "scheduled set not found"}
	}
	if _, err := tx.Exec(ctx, `UPDATE scheduled_workouts SET revision=revision+1, updated_at=now() WHERE id=$1 AND user_id=$2`, workoutID, userID); err != nil {
		return fmt.Errorf("advancing scheduled workout revision: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing scheduled set target update: %w", err)
	}
	return nil
}

func (r *scheduleDAO) Create(ctx context.Context, userID uuid.UUID, workouts []model.ScheduledWorkout) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning schedule create: %w", err)
	}
	defer tx.Rollback(ctx)
	for i := range workouts {
		w := &workouts[i]
		err := tx.QueryRow(ctx, `
			INSERT INTO scheduled_workouts (
				user_id, program_id, program_workout_id, date, name,
				sequence_position, status, finalized_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			RETURNING id, revision, created_at, updated_at`,
			userID, w.ProgramID, w.ProgramWorkoutID, w.Date, w.Name,
			w.SequencePosition, w.Status, w.FinalizedAt,
		).Scan(&w.ID, &w.Revision, &w.CreatedAt, &w.UpdatedAt)
		if err != nil {
			return fmt.Errorf("inserting scheduled workout: %w", err)
		}
		if err := insertScheduledSets(ctx, tx, w.ID, w.RequiredSets); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing schedule create: %w", err)
	}
	return nil
}

func (r *scheduleDAO) ReplaceSnapshot(ctx context.Context, userID uuid.UUID, w *model.ScheduledWorkout, expectedRevision int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning scheduled workout update: %w", err)
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `
		UPDATE scheduled_workouts SET name=$3, status=$4, finalized_at=$5,
		       revision=revision+1, updated_at=now()
		WHERE id=$1 AND user_id=$2 AND revision=$6
		RETURNING revision, updated_at`, w.ID, userID, w.Name, w.Status,
		w.FinalizedAt, expectedRevision).Scan(&w.Revision, &w.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		current, getErr := r.Get(ctx, userID, w.ID)
		if getErr != nil {
			return getErr
		}
		return &model.ConflictError{Message: "scheduled workout revision conflict", Expected: expectedRevision, Actual: current.Revision, Authoritative: current}
	}
	if err != nil {
		return fmt.Errorf("updating scheduled workout: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM scheduled_sets WHERE scheduled_workout_id=$1`, w.ID); err != nil {
		return fmt.Errorf("replacing scheduled sets: %w", err)
	}
	if err := insertScheduledSets(ctx, tx, w.ID, w.RequiredSets); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing scheduled workout update: %w", err)
	}
	return nil
}

func insertScheduledSets(ctx context.Context, tx pgx.Tx, workoutID uuid.UUID, sets []model.ScheduledSet) error {
	for i := range sets {
		s := &sets[i]
		err := tx.QueryRow(ctx, `
			INSERT INTO scheduled_sets (
				scheduled_workout_id, program_exercise_id, catalog_id, exercise_name,
				exercise_category, exercise_modality, exercise_order, set_index,
				target_reps, target_weight, target_duration_seconds, rest_seconds, notes)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`,
			workoutID, s.ProgramExerciseID, s.CatalogID, s.ExerciseName,
			s.ExerciseCategory, s.ExerciseModality, s.ExerciseOrder, s.SetIndex,
			s.TargetReps, s.TargetWeight, s.TargetDurationSeconds, s.RestSeconds, s.Notes,
		).Scan(&s.ID)
		if err != nil {
			return fmt.Errorf("inserting scheduled set: %w", err)
		}
	}
	return nil
}

func (r *scheduleDAO) DeleteUnstartedRange(ctx context.Context, userID uuid.UUID, from, to string) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `
		DELETE FROM scheduled_workouts sw
		WHERE sw.user_id=$1 AND sw.date BETWEEN $2 AND $3
		  AND sw.status='planned' AND sw.finalized_at IS NULL
		  AND NOT EXISTS (
			SELECT 1 FROM workout_sessions ws
			WHERE ws.scheduled_workout_id=sw.id AND ws.status IN ('active','completed'))
		RETURNING sw.id`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("deleting unstarted scheduled workouts: %w", err)
	}
	defer rows.Close()
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning deleted scheduled workout: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
