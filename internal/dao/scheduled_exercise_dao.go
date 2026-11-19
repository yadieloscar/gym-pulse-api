package dao

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

// scheduledExerciseSets is one exercise of a dated workout split by whether a
// set has a performed result. A performed set keeps its identity and its
// exercise: only open sets can be swapped or skipped. A set that was ticked
// and then unticked has no performed result and is open.
type scheduledExerciseSets struct {
	open   []uuid.UUID
	logged int
	others int
}

// beginExerciseChange opens the mutation, answers a replay, and locks the
// unfinished workout at the expected revision.
func (r *trainingMutationDAO) beginExerciseChange(ctx context.Context, userID, workoutID uuid.UUID, exerciseOrder int, expectedRevision int64, record model.IdempotencyRecord) (pgx.Tx, *model.ScheduledWorkout, *scheduledExerciseSets, error) {
	tx, err := r.beginScheduleMutation(ctx, userID)
	if err != nil {
		return nil, nil, nil, err
	}
	fail := func(err error) (pgx.Tx, *model.ScheduledWorkout, *scheduledExerciseSets, error) {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			return nil, nil, nil, errors.Join(err, rollbackErr)
		}
		return nil, nil, nil, err
	}
	var replay model.ScheduledWorkout
	if found, err := findIdempotency(ctx, tx, userID, record.Scope, record.OperationKey, record.RequestHash, &replay); err != nil {
		return fail(err)
	} else if found {
		if err := tx.Rollback(ctx); err != nil {
			return nil, nil, nil, fmt.Errorf("closing replayed exercise change: %w", err)
		}
		return nil, &replay, nil, nil
	}
	var revision int64
	var finalized bool
	err = tx.QueryRow(ctx, `SELECT revision, finalized_at IS NOT NULL FROM scheduled_workouts WHERE id=$1 AND user_id=$2 FOR UPDATE`, workoutID, userID).Scan(&revision, &finalized)
	if errors.Is(err, pgx.ErrNoRows) {
		return fail(&model.NotFoundError{Message: "scheduled workout not found"})
	}
	if err != nil {
		return fail(fmt.Errorf("locking scheduled workout: %w", err))
	}
	if finalized || revision != expectedRevision {
		current, getErr := loadScheduledWorkout(ctx, tx, userID, workoutID)
		if getErr != nil {
			return fail(getErr)
		}
		if finalized {
			return fail(&model.ConflictError{Message: "a finished workout cannot change its exercises", Authoritative: current})
		}
		return fail(mutationRevisionConflict("scheduled workout revision conflict", expectedRevision, current, revision))
	}
	rows, err := tx.Query(ctx, `
		SELECT ss.id, ss.exercise_order,
		       EXISTS (SELECT 1 FROM set_logs sl WHERE sl.scheduled_set_id=ss.id AND sl.completed=true)
		FROM scheduled_sets ss WHERE ss.scheduled_workout_id=$1
		ORDER BY ss.exercise_order, ss.set_index`, workoutID)
	if err != nil {
		return fail(fmt.Errorf("reading scheduled sets: %w", err))
	}
	sets := &scheduledExerciseSets{}
	found := false
	for rows.Next() {
		var id uuid.UUID
		var order int
		var logged bool
		if err := rows.Scan(&id, &order, &logged); err != nil {
			rows.Close()
			return fail(fmt.Errorf("scanning scheduled set: %w", err))
		}
		switch {
		case order != exerciseOrder:
			sets.others++
		case logged:
			found = true
			sets.logged++
		default:
			found = true
			sets.open = append(sets.open, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fail(fmt.Errorf("reading scheduled sets: %w", err))
	}
	if !found {
		return fail(&model.NotFoundError{Message: "exercise not found in this workout"})
	}
	if len(sets.open) == 0 {
		return fail(&model.ValidationError{Message: "every set of this exercise is already logged", Field: "exercise_order"})
	}
	// A set the athlete ticked and unticked is open again, as the workout
	// reports it. Its emptied result goes, so the set can leave or be logged
	// later under its new exercise.
	if _, err := tx.Exec(ctx, `DELETE FROM set_logs WHERE scheduled_set_id=ANY($1) AND completed=false`, sets.open); err != nil {
		return fail(fmt.Errorf("clearing unticked set results: %w", err))
	}
	return tx, nil, sets, nil
}

// finishExerciseChange advances the revision and stores the replay record
// with the authoritative workout, in the same commit as the change.
func finishExerciseChange(ctx context.Context, tx pgx.Tx, userID, workoutID uuid.UUID, record model.IdempotencyRecord) (*model.ScheduledWorkout, error) {
	if _, err := tx.Exec(ctx, `UPDATE scheduled_workouts SET revision=revision+1, updated_at=now() WHERE id=$1 AND user_id=$2`, workoutID, userID); err != nil {
		return nil, fmt.Errorf("advancing scheduled workout revision: %w", err)
	}
	result, err := loadScheduledWorkout(ctx, tx, userID, workoutID)
	if err != nil {
		return nil, err
	}
	record.ResourceID = &result.ID
	record.ResourceRevision = &result.Revision
	if err := insertIdempotency(ctx, tx, userID, record, result); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing exercise change: %w", err)
	}
	return result, nil
}

// shiftExerciseOrders moves every exercise after `after` by delta positions.
// Rows pass through a parked range so the unique (workout, order, index)
// never collides part-way through the statement.
func shiftExerciseOrders(ctx context.Context, tx pgx.Tx, workoutID uuid.UUID, after, delta int) error {
	if _, err := tx.Exec(ctx, `UPDATE scheduled_sets SET exercise_order=exercise_order+$3 WHERE scheduled_workout_id=$1 AND exercise_order>$2`, workoutID, after, startedSetParking); err != nil {
		return fmt.Errorf("parking later exercises: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE scheduled_sets SET exercise_order=exercise_order-$2+$3 WHERE scheduled_workout_id=$1 AND exercise_order>$2`, workoutID, startedSetParking, delta); err != nil {
		return fmt.Errorf("renumbering later exercises: %w", err)
	}
	return nil
}

// SwapScheduledExercise gives the open sets of one exercise a new exercise
// for this date only. They keep their reps, duration, rest, and notes; the
// target weight is cleared because it belonged to the other exercise. When
// some sets were already performed, those stay where they are and the open
// sets become a new exercise directly after them.
func (r *trainingMutationDAO) SwapScheduledExercise(ctx context.Context, userID, workoutID uuid.UUID, exerciseOrder int, request model.SwapScheduledExerciseRequest, record model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error) {
	tx, replay, sets, err := r.beginExerciseChange(ctx, userID, workoutID, exerciseOrder, request.ExpectedRevision, record)
	if err != nil || replay != nil {
		return replay, replay != nil, err
	}
	defer tx.Rollback(ctx)
	if sets.logged > 0 {
		if err := shiftExerciseOrders(ctx, tx, workoutID, exerciseOrder, 1); err != nil {
			return nil, false, err
		}
		// Park the set index as well, then number the moved sets from one.
		if _, err := tx.Exec(ctx, `UPDATE scheduled_sets SET exercise_order=$2, set_index=set_index+$3 WHERE id=ANY($1)`, sets.open, exerciseOrder+1, startedSetParking); err != nil {
			return nil, false, fmt.Errorf("moving swapped sets: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE scheduled_sets ss SET set_index=numbered.position
			FROM (SELECT id, row_number() OVER (ORDER BY set_index) AS position FROM scheduled_sets WHERE id=ANY($1)) numbered
			WHERE ss.id=numbered.id`, sets.open); err != nil {
			return nil, false, fmt.Errorf("numbering swapped sets: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE scheduled_sets SET exercise_name=$2, exercise_category=$3,
		       program_exercise_id=NULL, catalog_id=NULL, target_weight=NULL
		WHERE id=ANY($1)`, sets.open, request.ExerciseName, request.ExerciseCategory); err != nil {
		return nil, false, fmt.Errorf("swapping scheduled exercise: %w", err)
	}
	result, err := finishExerciseChange(ctx, tx, userID, workoutID, record)
	return result, false, err
}

// SkipScheduledExercise removes the open sets of one exercise from this date
// only. Performed sets stay. A workout keeps at least one set: skipping its
// last remaining work is refused so the athlete skips the workout instead.
func (r *trainingMutationDAO) SkipScheduledExercise(ctx context.Context, userID, workoutID uuid.UUID, exerciseOrder int, expectedRevision int64, record model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error) {
	tx, replay, sets, err := r.beginExerciseChange(ctx, userID, workoutID, exerciseOrder, expectedRevision, record)
	if err != nil || replay != nil {
		return replay, replay != nil, err
	}
	defer tx.Rollback(ctx)
	if sets.logged+sets.others == 0 {
		return nil, false, &model.ValidationError{Message: "a workout needs at least one set; skip the workout instead", Field: "exercise_order"}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM scheduled_sets WHERE id=ANY($1)`, sets.open); err != nil {
		return nil, false, fmt.Errorf("skipping scheduled exercise: %w", err)
	}
	if sets.logged == 0 {
		if err := shiftExerciseOrders(ctx, tx, workoutID, exerciseOrder, -1); err != nil {
			return nil, false, err
		}
	}
	result, err := finishExerciseChange(ctx, tx, userID, workoutID, record)
	return result, false, err
}
