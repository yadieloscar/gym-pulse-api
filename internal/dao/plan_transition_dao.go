package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type PlanTransitionDAO interface {
	Apply(ctx context.Context, userID uuid.UUID, expectedProfileRevision int64, profile *model.TrainingProfile, target *model.Program, from, to, operationKey, requestHash string) (uuid.UUID, []model.ScheduledWorkout, bool, error)
}

type planTransitionDAO struct{ pool *pgxpool.Pool }

func NewPlanTransitionDAO(pool *pgxpool.Pool) PlanTransitionDAO {
	return &planTransitionDAO{pool: pool}
}

func (r *planTransitionDAO) Apply(ctx context.Context, userID uuid.UUID, expected int64, profile *model.TrainingProfile, target *model.Program, from, to, operationKey, requestHash string) (uuid.UUID, []model.ScheduledWorkout, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, nil, false, fmt.Errorf("beginning plan transition: %w", err)
	}
	defer tx.Rollback(ctx)
	var priorHash string
	var priorID *uuid.UUID
	err = tx.QueryRow(ctx, `SELECT request_hash, resource_id FROM idempotency_records WHERE user_id=$1 AND scope='plan-transitions/apply' AND operation_key=$2`, userID, operationKey).Scan(&priorHash, &priorID)
	if err == nil {
		if priorHash != requestHash {
			return uuid.Nil, nil, false, &model.ConflictError{Message: "idempotency key was already used with a different payload"}
		}
		if priorID == nil {
			return uuid.Nil, nil, false, &model.ConflictError{Message: "idempotency record has no program"}
		}
		return *priorID, nil, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil, false, fmt.Errorf("checking transition idempotency: %w", err)
	}

	if expected == 0 {
		err = tx.QueryRow(ctx, `INSERT INTO training_profiles (user_id, primary_goal, available_days, usual_activity, experience, equipment, session_duration_minutes, timezone, preferences) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING revision, created_at, updated_at`, userID, profile.PrimaryGoal, profile.AvailableDays, profile.UsualActivity, profile.Experience, profile.Equipment, profile.SessionDurationMinutes, profile.Timezone, profile.Preferences).Scan(&profile.Revision, &profile.CreatedAt, &profile.UpdatedAt)
	} else {
		err = tx.QueryRow(ctx, `UPDATE training_profiles SET primary_goal=$2, available_days=$3, usual_activity=$4, experience=$5, equipment=$6, session_duration_minutes=$7, timezone=$8, preferences=$9, revision=revision+1, updated_at=now() WHERE user_id=$1 AND revision=$10 RETURNING revision, created_at, updated_at`, userID, profile.PrimaryGoal, profile.AvailableDays, profile.UsualActivity, profile.Experience, profile.Equipment, profile.SessionDurationMinutes, profile.Timezone, profile.Preferences, expected).Scan(&profile.Revision, &profile.CreatedAt, &profile.UpdatedAt)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil, false, &model.ConflictError{Message: "training profile revision conflict", Expected: expected}
	}
	if err != nil {
		return uuid.Nil, nil, false, fmt.Errorf("persisting transition profile: %w", err)
	}

	if _, err := tx.Exec(ctx, `UPDATE programs SET active=false, revision=revision+1, updated_at=now() WHERE user_id=$1 AND active=true`, userID); err != nil {
		return uuid.Nil, nil, false, fmt.Errorf("deactivating prior programs: %w", err)
	}
	if target.ID == uuid.Nil {
		err = tx.QueryRow(ctx, `INSERT INTO programs (user_id, starter_program_id, starter_version, name, primary_goal, roadmap, active) VALUES ($1,$2,$3,$4,$5,$6,true) RETURNING id, revision, created_at, updated_at`, userID, target.StarterProgramID, target.StarterVersion, target.Name, target.PrimaryGoal, target.Roadmap).Scan(&target.ID, &target.Revision, &target.CreatedAt, &target.UpdatedAt)
		if err != nil {
			return uuid.Nil, nil, false, fmt.Errorf("creating transition program: %w", err)
		}
		if err := insertProgramWorkouts(ctx, tx, target.ID, target.Workouts); err != nil {
			return uuid.Nil, nil, false, err
		}
	} else {
		err = tx.QueryRow(ctx, `UPDATE programs SET active=true, revision=revision+1, updated_at=now() WHERE id=$1 AND user_id=$2 RETURNING revision, created_at, updated_at`, target.ID, userID).Scan(&target.Revision, &target.CreatedAt, &target.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, nil, false, &model.NotFoundError{Message: "target program not found"}
		}
		if err != nil {
			return uuid.Nil, nil, false, fmt.Errorf("activating target program: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM scheduled_workouts sw WHERE sw.user_id=$1 AND sw.date BETWEEN $2 AND $3 AND sw.status='planned' AND sw.finalized_at IS NULL AND NOT EXISTS (SELECT 1 FROM workout_sessions ws WHERE ws.scheduled_workout_id=sw.id AND ws.status IN ('active','completed'))`, userID, from, to); err != nil {
		return uuid.Nil, nil, false, fmt.Errorf("replacing unstarted future workouts: %w", err)
	}
	workouts, err := model.MaterializeProgramForWeekdays(target, profile.AvailableDays, from, to)
	if err != nil {
		return uuid.Nil, nil, false, err
	}
	for i := range workouts {
		w := &workouts[i]
		err := tx.QueryRow(ctx, `INSERT INTO scheduled_workouts (user_id, program_id, program_workout_id, date, name, sequence_position, status) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id, revision, created_at, updated_at`, userID, w.ProgramID, w.ProgramWorkoutID, w.Date, w.Name, w.SequencePosition, w.Status).Scan(&w.ID, &w.Revision, &w.CreatedAt, &w.UpdatedAt)
		if err != nil {
			return uuid.Nil, nil, false, fmt.Errorf("inserting transition workout: %w", err)
		}
		if err := insertScheduledSets(ctx, tx, w.ID, w.RequiredSets); err != nil {
			return uuid.Nil, nil, false, err
		}
	}
	body, _ := json.Marshal(map[string]any{"program_id": target.ID})
	if _, err := tx.Exec(ctx, `INSERT INTO idempotency_records (user_id, scope, operation_key, request_hash, response_status, response_body, resource_type, resource_id, resource_revision) VALUES ($1,'plan-transitions/apply',$2,$3,200,$4,'program',$5,$6)`, userID, operationKey, requestHash, body, target.ID, target.Revision); err != nil {
		return uuid.Nil, nil, false, fmt.Errorf("recording transition idempotency: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, nil, false, fmt.Errorf("committing plan transition: %w", err)
	}
	return target.ID, workouts, false, nil
}
