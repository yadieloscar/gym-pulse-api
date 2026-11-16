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

// PlanTransitionApply is one start of a program, optionally together with a
// training-profile change.
type PlanTransitionApply struct {
	// Profile is saved when non-nil; a nil profile leaves the stored one alone.
	Profile                 *model.TrainingProfile
	ExpectedProfileRevision int64
	// Target is an owned program (ID set) or a starter copy to create (ID zero,
	// units already placed).
	Target *model.Program
	// AvailableDays places an owned target that has no weekdays yet.
	AvailableDays []int
	// Start is the first date that may change. Through is the last date to
	// fill; From and To bound the sessions returned to the caller.
	Start, Through string
	From, To       string
	OperationKey   string
	RequestHash    string
}

// PlanTransitionResult reports what a start did.
type PlanTransitionResult struct {
	ProgramID uuid.UUID
	// Workouts are the target program's sessions between From and To after
	// the start. It is nil on a replay.
	Workouts []model.ScheduledWorkout
	Replay   bool
}

type PlanTransitionDAO interface {
	Apply(ctx context.Context, userID uuid.UUID, apply PlanTransitionApply) (*PlanTransitionResult, error)
}

type planTransitionDAO struct{ pool *pgxpool.Pool }

func NewPlanTransitionDAO(pool *pgxpool.Pool) PlanTransitionDAO {
	return &planTransitionDAO{pool: pool}
}

// Apply is the single way a program starts. In one transaction it saves the
// profile when one is proposed, makes the target the only active program,
// replaces every unstarted planned session dated on or after Start, and fills
// the target's sessions. A target that is already the active, placed program
// changes no session: a profile edit keeps the program the athlete follows.
func (r *planTransitionDAO) Apply(ctx context.Context, userID uuid.UUID, apply PlanTransitionApply) (*PlanTransitionResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning plan transition: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, programLockNamespace, userID); err != nil {
		return nil, err
	}
	if err := lockUserDomain(ctx, tx, scheduleLockNamespace, userID); err != nil {
		return nil, err
	}
	var priorHash string
	var priorID *uuid.UUID
	err = tx.QueryRow(ctx, `SELECT request_hash, resource_id FROM idempotency_records WHERE user_id=$1 AND scope='plan-transitions/apply' AND operation_key=$2`, userID, apply.OperationKey).Scan(&priorHash, &priorID)
	if err == nil {
		if priorHash != apply.RequestHash {
			return nil, &model.ConflictError{Message: "idempotency key was already used with a different payload"}
		}
		if priorID == nil {
			return nil, &model.ConflictError{Message: "idempotency record has no program"}
		}
		return &PlanTransitionResult{ProgramID: *priorID, Replay: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("checking transition idempotency: %w", err)
	}

	if apply.Profile != nil {
		if err := saveTransitionProfile(ctx, tx, userID, apply.Profile, apply.ExpectedProfileRevision); err != nil {
			return nil, err
		}
	}

	target, unchanged, err := activateTransitionTarget(ctx, tx, userID, apply)
	if err != nil {
		return nil, err
	}
	if !unchanged {
		if _, err := tx.Exec(ctx, `DELETE FROM scheduled_workouts sw WHERE sw.user_id=$1 AND sw.date >= $2 AND `+unstartedPlanned, userID, apply.Start); err != nil {
			return nil, fmt.Errorf("replacing unstarted future workouts: %w", err)
		}
		if err := fillProgram(ctx, tx, userID, target, apply.Start, apply.Through); err != nil {
			return nil, err
		}
	}
	body, _ := json.Marshal(map[string]any{"program_id": target.ID})
	if _, err := tx.Exec(ctx, `INSERT INTO idempotency_records (user_id, scope, operation_key, request_hash, response_status, response_body, resource_type, resource_id, resource_revision) VALUES ($1,'plan-transitions/apply',$2,$3,200,$4,'program',$5,$6)`, userID, apply.OperationKey, apply.RequestHash, body, target.ID, target.Revision); err != nil {
		return nil, fmt.Errorf("recording transition idempotency: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing plan transition: %w", err)
	}

	scheduled, err := (&scheduleDAO{pool: r.pool}).List(ctx, userID, apply.From, apply.To)
	if err != nil {
		return nil, err
	}
	workouts := []model.ScheduledWorkout{}
	for i := range scheduled {
		if scheduled[i].ProgramID != nil && *scheduled[i].ProgramID == target.ID {
			workouts = append(workouts, scheduled[i])
		}
	}
	return &PlanTransitionResult{ProgramID: target.ID, Workouts: workouts}, nil
}

func saveTransitionProfile(ctx context.Context, tx pgx.Tx, userID uuid.UUID, profile *model.TrainingProfile, expected int64) error {
	var err error
	if expected == 0 {
		err = tx.QueryRow(ctx, `INSERT INTO training_profiles (user_id, primary_goal, available_days, usual_activity, experience, equipment, session_duration_minutes, timezone, preferences) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING revision, created_at, updated_at`, userID, profile.PrimaryGoal, profile.AvailableDays, profile.UsualActivity, profile.Experience, profile.Equipment, profile.SessionDurationMinutes, profile.Timezone, profile.Preferences).Scan(&profile.Revision, &profile.CreatedAt, &profile.UpdatedAt)
	} else {
		err = tx.QueryRow(ctx, `UPDATE training_profiles SET primary_goal=$2, available_days=$3, usual_activity=$4, experience=$5, equipment=$6, session_duration_minutes=$7, timezone=$8, preferences=$9, revision=revision+1, updated_at=now() WHERE user_id=$1 AND revision=$10 RETURNING revision, created_at, updated_at`, userID, profile.PrimaryGoal, profile.AvailableDays, profile.UsualActivity, profile.Experience, profile.Equipment, profile.SessionDurationMinutes, profile.Timezone, profile.Preferences, expected).Scan(&profile.Revision, &profile.CreatedAt, &profile.UpdatedAt)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return &model.ConflictError{Message: "training profile revision conflict", Expected: expected}
	}
	if err != nil {
		return fmt.Errorf("persisting transition profile: %w", err)
	}
	return nil
}

// activateTransitionTarget makes the target the only active program and
// returns it as stored. unchanged is true when the target was already the
// active program and already placed, in which case nothing was written.
func activateTransitionTarget(ctx context.Context, tx pgx.Tx, userID uuid.UUID, apply PlanTransitionApply) (target *model.Program, unchanged bool, err error) {
	if apply.Target.ID == uuid.Nil {
		target = apply.Target
		if _, err := tx.Exec(ctx, `UPDATE programs SET active=false, revision=revision+1, updated_at=now() WHERE user_id=$1 AND active=true`, userID); err != nil {
			return nil, false, fmt.Errorf("deactivating prior programs: %w", err)
		}
		err = tx.QueryRow(ctx, `INSERT INTO programs (user_id, starter_program_id, starter_version, name, primary_goal, roadmap, active) VALUES ($1,$2,$3,$4,$5,$6,true) RETURNING id, revision, created_at, updated_at`, userID, target.StarterProgramID, target.StarterVersion, target.Name, target.PrimaryGoal, target.Roadmap).Scan(&target.ID, &target.Revision, &target.CreatedAt, &target.UpdatedAt)
		if err != nil {
			return nil, false, fmt.Errorf("creating transition program: %w", err)
		}
		if err := insertProgramWorkouts(ctx, tx, target.ID, target.Workouts); err != nil {
			return nil, false, err
		}
		return target, false, nil
	}

	target, err = loadProgram(ctx, tx, userID, apply.Target.ID)
	if err != nil {
		var notFound *model.NotFoundError
		if errors.As(err, &notFound) {
			return nil, false, &model.NotFoundError{Message: "target program not found"}
		}
		return nil, false, err
	}
	if target.Active && target.Placed() {
		return target, true, nil
	}
	if !target.Placed() {
		if err := model.PlaceProgram(target, apply.AvailableDays, apply.Start); err != nil {
			return nil, false, err
		}
		if err := storePlacement(ctx, tx, target.ID, target.Workouts); err != nil {
			return nil, false, err
		}
	}
	if !target.Active {
		if _, err := tx.Exec(ctx, `UPDATE programs SET active=false, revision=revision+1, updated_at=now() WHERE user_id=$1 AND active=true`, userID); err != nil {
			return nil, false, fmt.Errorf("deactivating prior programs: %w", err)
		}
		err = tx.QueryRow(ctx, `UPDATE programs SET active=true, revision=revision+1, updated_at=now() WHERE id=$1 AND user_id=$2 RETURNING revision, updated_at`, target.ID, userID).Scan(&target.Revision, &target.UpdatedAt)
		if err != nil {
			return nil, false, fmt.Errorf("activating target program: %w", err)
		}
		target.Active = true
	}
	return target, false, nil
}
