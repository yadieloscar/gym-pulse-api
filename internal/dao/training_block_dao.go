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

const trainingBlockLockNamespace = "criteria-training-block:"

type TrainingBlockDAO interface {
	List(ctx context.Context, userID uuid.UUID, status string, limit, offset int) (*model.TrainingBlockList, error)
	Get(ctx context.Context, userID, blockID uuid.UUID) (*model.TrainingBlock, error)
	Create(ctx context.Context, userID uuid.UUID, req model.CreateTrainingBlockRequest, requestHash string) (*model.TrainingBlock, bool, error)
	AddExposure(ctx context.Context, userID, blockID uuid.UUID, req model.CreateTrainingExposureRequest, requestHash string) (*model.TrainingBlock, bool, error)
	RecordNextMorning(ctx context.Context, userID, blockID, exposureID uuid.UUID, req model.RecordNextMorningRequest, requestHash string) (*model.TrainingBlock, bool, error)
	Transition(ctx context.Context, userID, blockID uuid.UUID, req model.CreateTrainingTransitionRequest, requestHash string) (*model.TrainingBlock, bool, error)
}

type trainingBlockDAO struct{ pool *pgxpool.Pool }

func NewTrainingBlockDAO(pool *pgxpool.Pool) TrainingBlockDAO {
	return &trainingBlockDAO{pool: pool}
}

func (r *trainingBlockDAO) List(ctx context.Context, userID uuid.UUID, status string, limit, offset int) (*model.TrainingBlockList, error) {
	rows, err := r.pool.Query(ctx, trainingBlockSummarySelect+`
		WHERE b.user_id=$1 AND ($2='all' OR b.status=$2)
		ORDER BY b.updated_at DESC, b.id DESC
		LIMIT $3 OFFSET $4`, userID, status, limit+1, offset)
	if err != nil {
		return nil, fmt.Errorf("querying training blocks: %w", err)
	}
	defer rows.Close()

	items := make([]model.TrainingBlockSummary, 0, limit)
	for rows.Next() {
		var block model.TrainingBlock
		if err := scanTrainingBlockSummary(rows, &block); err != nil {
			return nil, fmt.Errorf("scanning training block summary: %w", err)
		}
		items = append(items, block.TrainingBlockSummary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating training blocks: %w", err)
	}

	var next *int
	if len(items) > limit {
		items = items[:limit]
		value := offset + limit
		next = &value
	}
	return &model.TrainingBlockList{TrainingBlocks: items, NextOffset: next}, nil
}

func (r *trainingBlockDAO) Get(ctx context.Context, userID, blockID uuid.UUID) (*model.TrainingBlock, error) {
	return loadTrainingBlock(ctx, r.pool, userID, blockID, false)
}

func (r *trainingBlockDAO) Create(ctx context.Context, userID uuid.UUID, req model.CreateTrainingBlockRequest, requestHash string) (*model.TrainingBlock, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("beginning training block create: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, trainingBlockLockNamespace, userID); err != nil {
		return nil, false, err
	}
	var replay model.TrainingBlock
	if found, err := findIdempotency(ctx, tx, userID, "training-blocks/create", req.OperationKey, requestHash, &replay); err != nil {
		return nil, false, err
	} else if found {
		return &replay, true, nil
	}

	if req.ProgramID != nil {
		var owned bool
		if err := tx.QueryRow(ctx, `SELECT true FROM programs WHERE id=$1 AND user_id=$2`, *req.ProgramID, userID).Scan(&owned); errors.Is(err, pgx.ErrNoRows) {
			return nil, false, &model.NotFoundError{Message: "program not found"}
		} else if err != nil {
			return nil, false, fmt.Errorf("checking training block program: %w", err)
		}
	}

	blockID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO criteria_training_blocks (id,user_id,program_id,name,purpose)
		VALUES ($1,$2,$3,$4,$5)`, blockID, userID, req.ProgramID, req.Name, req.Purpose); err != nil {
		return nil, false, fmt.Errorf("inserting training block: %w", err)
	}
	for i, input := range req.Stages {
		if _, err := tx.Exec(ctx, `
			INSERT INTO criteria_training_stages (
				id,block_id,stage_order,name,instructions,load_level,target_count,
				target_duration_minutes,target_intensity_percent,required_qualifying_exposures)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, uuid.New(), blockID, i+1,
			input.Name, input.Instructions, input.LoadLevel, input.TargetCount,
			input.TargetDurationMinutes, input.TargetIntensityPercent,
			input.RequiredQualifyingExposures); err != nil {
			return nil, false, fmt.Errorf("inserting training stage: %w", err)
		}
	}
	block, err := loadTrainingBlock(ctx, tx, userID, blockID, false)
	if err != nil {
		return nil, false, err
	}
	if err := storeTrainingBlockIdempotency(ctx, tx, userID, "training-blocks/create", req.OperationKey, requestHash, 201, block); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing training block create: %w", err)
	}
	return block, false, nil
}

func (r *trainingBlockDAO) AddExposure(ctx context.Context, userID, blockID uuid.UUID, req model.CreateTrainingExposureRequest, requestHash string) (*model.TrainingBlock, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("beginning training exposure: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, trainingBlockLockNamespace, userID); err != nil {
		return nil, false, err
	}
	var replay model.TrainingBlock
	if found, err := findIdempotency(ctx, tx, userID, "training-blocks/exposures", req.OperationKey, requestHash, &replay); err != nil {
		return nil, false, err
	} else if found {
		return &replay, true, nil
	}
	block, err := loadTrainingBlock(ctx, tx, userID, blockID, true)
	if err != nil {
		return nil, false, err
	}
	if err := requireBlockRevision(block, req.ExpectedRevision); err != nil {
		return nil, false, err
	}
	if block.Status != model.TrainingBlockActive {
		return nil, false, &model.ConflictError{Message: "training block is read-only", Authoritative: block}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO criteria_training_exposures (
			id,block_id,stage_id,performed_on,activity_label,load_level,performed_count,
			duration_minutes,performed_intensity_percent,session_outcome,notes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, uuid.New(), blockID,
		block.CurrentStage.ID, req.PerformedOn, req.ActivityLabel, req.LoadLevel,
		req.PerformedCount, req.DurationMinutes, req.PerformedIntensityPercent,
		req.SessionOutcome, req.Notes); err != nil {
		return nil, false, fmt.Errorf("inserting training exposure: %w", err)
	}
	if err := bumpTrainingBlock(ctx, tx, blockID); err != nil {
		return nil, false, err
	}
	block, err = loadTrainingBlock(ctx, tx, userID, blockID, false)
	if err != nil {
		return nil, false, err
	}
	if err := storeTrainingBlockIdempotency(ctx, tx, userID, "training-blocks/exposures", req.OperationKey, requestHash, 200, block); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing training exposure: %w", err)
	}
	return block, false, nil
}

func (r *trainingBlockDAO) RecordNextMorning(ctx context.Context, userID, blockID, exposureID uuid.UUID, req model.RecordNextMorningRequest, requestHash string) (*model.TrainingBlock, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("beginning next-morning response: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, trainingBlockLockNamespace, userID); err != nil {
		return nil, false, err
	}
	var replay model.TrainingBlock
	if found, err := findIdempotency(ctx, tx, userID, "training-blocks/next-morning", req.OperationKey, requestHash, &replay); err != nil {
		return nil, false, err
	} else if found {
		return &replay, true, nil
	}
	block, err := loadTrainingBlock(ctx, tx, userID, blockID, true)
	if err != nil {
		return nil, false, err
	}
	if err := requireBlockRevision(block, req.ExpectedRevision); err != nil {
		return nil, false, err
	}
	if block.Status != model.TrainingBlockActive {
		return nil, false, &model.ConflictError{Message: "training block is read-only", Authoritative: block}
	}
	result, err := tx.Exec(ctx, `
		UPDATE criteria_training_exposures
		SET next_morning_response=$4, next_morning_recorded_at=now()
		WHERE id=$1 AND block_id=$2
		  AND EXISTS (SELECT 1 FROM criteria_training_blocks b WHERE b.id=$2 AND b.user_id=$3)
		  AND next_morning_response IS NULL`, exposureID, blockID, userID, req.Response)
	if err != nil {
		return nil, false, fmt.Errorf("recording next-morning response: %w", err)
	}
	if result.RowsAffected() == 0 {
		var exists bool
		err := tx.QueryRow(ctx, `SELECT true FROM criteria_training_exposures WHERE id=$1 AND block_id=$2`, exposureID, blockID).Scan(&exists)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, &model.NotFoundError{Message: "training exposure not found"}
		}
		if err != nil {
			return nil, false, fmt.Errorf("checking training exposure response: %w", err)
		}
		return nil, false, &model.ConflictError{Message: "next-morning response is already recorded", Authoritative: block}
	}
	if err := bumpTrainingBlock(ctx, tx, blockID); err != nil {
		return nil, false, err
	}
	block, err = loadTrainingBlock(ctx, tx, userID, blockID, false)
	if err != nil {
		return nil, false, err
	}
	if err := storeTrainingBlockIdempotency(ctx, tx, userID, "training-blocks/next-morning", req.OperationKey, requestHash, 200, block); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing next-morning response: %w", err)
	}
	return block, false, nil
}

func (r *trainingBlockDAO) Transition(ctx context.Context, userID, blockID uuid.UUID, req model.CreateTrainingTransitionRequest, requestHash string) (*model.TrainingBlock, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("beginning training transition: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, trainingBlockLockNamespace, userID); err != nil {
		return nil, false, err
	}
	var replay model.TrainingBlock
	if found, err := findIdempotency(ctx, tx, userID, "training-blocks/transitions", req.OperationKey, requestHash, &replay); err != nil {
		return nil, false, err
	} else if found {
		return &replay, true, nil
	}
	block, err := loadTrainingBlock(ctx, tx, userID, blockID, true)
	if err != nil {
		return nil, false, err
	}
	if err := requireBlockRevision(block, req.ExpectedRevision); err != nil {
		return nil, false, err
	}
	if block.Status == model.TrainingBlockArchived {
		return nil, false, &model.ConflictError{Message: "training block is archived", Authoritative: block}
	}

	fromID := block.CurrentStage.ID
	var toID *uuid.UUID
	newStatus := block.Status
	newOrder := block.CurrentStage.StageOrder
	switch req.Action {
	case model.TransitionAdvance:
		if block.Status != model.TrainingBlockActive {
			return nil, false, &model.ConflictError{Message: "training block is read-only", Authoritative: block}
		}
		if !block.CurrentStageProgress.CriteriaComplete {
			return nil, false, &model.ConflictError{Message: "current stage criteria are incomplete", Authoritative: block}
		}
		next := findTrainingStageByOrder(block.Stages, newOrder+1)
		if next == nil {
			return nil, false, &model.ConflictError{Message: "final stage must be completed explicitly", Authoritative: block}
		}
		if req.ToStageID != nil && *req.ToStageID != next.ID {
			return nil, false, &model.ConflictError{Message: "advance must target the next stage", Authoritative: block}
		}
		toID = &next.ID
		newOrder = next.StageOrder
	case model.TransitionRegress:
		if block.Status != model.TrainingBlockActive || req.ToStageID == nil {
			return nil, false, &model.ConflictError{Message: "regression requires an active block and earlier stage", Authoritative: block}
		}
		target := findTrainingStageByID(block.Stages, *req.ToStageID)
		if target == nil || target.StageOrder >= newOrder {
			return nil, false, &model.ConflictError{Message: "regression must target an earlier stage", Authoritative: block}
		}
		toID = &target.ID
		newOrder = target.StageOrder
	case model.TransitionComplete:
		if block.Status != model.TrainingBlockActive || newOrder != len(block.Stages) || !block.CurrentStageProgress.CriteriaComplete {
			return nil, false, &model.ConflictError{Message: "final stage criteria are incomplete", Authoritative: block}
		}
		newStatus = model.TrainingBlockCompleted
	case model.TransitionArchive:
		newStatus = model.TrainingBlockArchived
	default:
		return nil, false, &model.ValidationError{Message: "invalid transition action", Field: "action"}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE criteria_training_blocks
		SET status=$2,current_stage_order=$3,revision=revision+1,updated_at=now()
		WHERE id=$1`, blockID, newStatus, newOrder); err != nil {
		return nil, false, fmt.Errorf("updating training block transition: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO criteria_training_transitions (id,block_id,action,from_stage_id,to_stage_id,reason)
		VALUES ($1,$2,$3,$4,$5,$6)`, uuid.New(), blockID, req.Action, fromID, toID, req.Reason); err != nil {
		return nil, false, fmt.Errorf("inserting training transition: %w", err)
	}
	block, err = loadTrainingBlock(ctx, tx, userID, blockID, false)
	if err != nil {
		return nil, false, err
	}
	if err := storeTrainingBlockIdempotency(ctx, tx, userID, "training-blocks/transitions", req.OperationKey, requestHash, 200, block); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing training transition: %w", err)
	}
	return block, false, nil
}

const trainingBlockSummarySelect = `
	SELECT b.id,b.name,b.purpose,b.program_id,b.status,b.revision,b.updated_at,b.created_at,
	       s.id,s.stage_order,s.name,s.instructions,s.load_level,s.target_count,
	       s.target_duration_minutes,s.target_intensity_percent,s.required_qualifying_exposures,
	       (SELECT count(*) FROM criteria_training_exposures e
	        WHERE e.stage_id=s.id AND e.session_outcome='completed_as_planned'
	          AND e.next_morning_response='baseline') AS qualifying,
	       (SELECT count(*) FROM criteria_training_exposures e
	        WHERE e.block_id=b.id AND e.next_morning_response IS NULL) AS pending
	FROM criteria_training_blocks b
	JOIN criteria_training_stages s
	  ON s.block_id=b.id AND s.stage_order=b.current_stage_order`

type trainingBlockQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadTrainingBlock(ctx context.Context, q trainingBlockQuerier, userID, blockID uuid.UUID, lock bool) (*model.TrainingBlock, error) {
	query := trainingBlockSummarySelect + ` WHERE b.id=$1 AND b.user_id=$2`
	if lock {
		query += ` FOR UPDATE OF b`
	}
	block := &model.TrainingBlock{}
	if err := scanTrainingBlockSummary(q.QueryRow(ctx, query, blockID, userID), block); errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "training block not found"}
	} else if err != nil {
		return nil, fmt.Errorf("querying training block: %w", err)
	}

	stages, err := loadTrainingStages(ctx, q, blockID)
	if err != nil {
		return nil, err
	}
	exposures, err := loadTrainingExposures(ctx, q, blockID)
	if err != nil {
		return nil, err
	}
	transitions, err := loadTrainingTransitions(ctx, q, blockID)
	if err != nil {
		return nil, err
	}
	block.Stages, block.Exposures, block.Transitions = stages, exposures, transitions
	return block, nil
}

func scanTrainingBlockSummary(row rowScanner, block *model.TrainingBlock) error {
	var qualifying int
	if err := row.Scan(&block.ID, &block.Name, &block.Purpose, &block.ProgramID, &block.Status,
		&block.Revision, &block.UpdatedAt, &block.CreatedAt, &block.CurrentStage.ID,
		&block.CurrentStage.StageOrder, &block.CurrentStage.Name, &block.CurrentStage.Instructions,
		&block.CurrentStage.LoadLevel, &block.CurrentStage.TargetCount,
		&block.CurrentStage.TargetDurationMinutes, &block.CurrentStage.TargetIntensityPercent,
		&block.CurrentStage.RequiredQualifyingExposures, &qualifying,
		&block.PendingNextMorningCount); err != nil {
		return err
	}
	block.CurrentStageProgress = model.TrainingStageProgress{
		RequiredQualifyingExposures: block.CurrentStage.RequiredQualifyingExposures,
		QualifyingExposures:         qualifying,
		CriteriaComplete:            qualifying >= block.CurrentStage.RequiredQualifyingExposures,
	}
	return nil
}

func loadTrainingStages(ctx context.Context, q trainingBlockQuerier, blockID uuid.UUID) ([]model.TrainingStage, error) {
	rows, err := q.Query(ctx, `
		SELECT id,stage_order,name,instructions,load_level,target_count,
		       target_duration_minutes,target_intensity_percent,required_qualifying_exposures
		FROM criteria_training_stages WHERE block_id=$1 ORDER BY stage_order`, blockID)
	if err != nil {
		return nil, fmt.Errorf("querying training stages: %w", err)
	}
	defer rows.Close()
	items := []model.TrainingStage{}
	for rows.Next() {
		var stage model.TrainingStage
		if err := rows.Scan(&stage.ID, &stage.StageOrder, &stage.Name, &stage.Instructions,
			&stage.LoadLevel, &stage.TargetCount, &stage.TargetDurationMinutes,
			&stage.TargetIntensityPercent, &stage.RequiredQualifyingExposures); err != nil {
			return nil, fmt.Errorf("scanning training stage: %w", err)
		}
		items = append(items, stage)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating training stages: %w", err)
	}
	return items, nil
}

func loadTrainingExposures(ctx context.Context, q trainingBlockQuerier, blockID uuid.UUID) ([]model.TrainingExposure, error) {
	rows, err := q.Query(ctx, `
		SELECT id,stage_id,to_char(performed_on,'YYYY-MM-DD'),activity_label,load_level,
		       performed_count,duration_minutes,performed_intensity_percent,session_outcome,
		       next_morning_response,notes,created_at,next_morning_recorded_at
		FROM criteria_training_exposures WHERE block_id=$1
		ORDER BY created_at DESC,id DESC`, blockID)
	if err != nil {
		return nil, fmt.Errorf("querying training exposures: %w", err)
	}
	defer rows.Close()
	items := []model.TrainingExposure{}
	for rows.Next() {
		var exposure model.TrainingExposure
		if err := rows.Scan(&exposure.ID, &exposure.StageID, &exposure.PerformedOn,
			&exposure.ActivityLabel, &exposure.LoadLevel, &exposure.PerformedCount,
			&exposure.DurationMinutes, &exposure.PerformedIntensityPercent,
			&exposure.SessionOutcome, &exposure.NextMorningResponse, &exposure.Notes,
			&exposure.CreatedAt, &exposure.NextMorningRecordedAt); err != nil {
			return nil, fmt.Errorf("scanning training exposure: %w", err)
		}
		exposure.Qualifies = model.TrainingExposureQualifies(exposure.SessionOutcome, exposure.NextMorningResponse)
		items = append(items, exposure)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating training exposures: %w", err)
	}
	return items, nil
}

func loadTrainingTransitions(ctx context.Context, q trainingBlockQuerier, blockID uuid.UUID) ([]model.TrainingTransition, error) {
	rows, err := q.Query(ctx, `
		SELECT id,action,from_stage_id,to_stage_id,reason,created_at
		FROM criteria_training_transitions WHERE block_id=$1
		ORDER BY created_at,id`, blockID)
	if err != nil {
		return nil, fmt.Errorf("querying training transitions: %w", err)
	}
	defer rows.Close()
	items := []model.TrainingTransition{}
	for rows.Next() {
		var transition model.TrainingTransition
		if err := rows.Scan(&transition.ID, &transition.Action, &transition.FromStageID,
			&transition.ToStageID, &transition.Reason, &transition.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning training transition: %w", err)
		}
		items = append(items, transition)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating training transitions: %w", err)
	}
	return items, nil
}

func requireBlockRevision(block *model.TrainingBlock, expected int64) error {
	if block.Revision == expected {
		return nil
	}
	return &model.ConflictError{
		Message: "training block revision conflict", Expected: expected,
		Actual: block.Revision, Authoritative: block,
	}
}

func bumpTrainingBlock(ctx context.Context, tx pgx.Tx, blockID uuid.UUID) error {
	if _, err := tx.Exec(ctx, `UPDATE criteria_training_blocks SET revision=revision+1,updated_at=now() WHERE id=$1`, blockID); err != nil {
		return fmt.Errorf("updating training block revision: %w", err)
	}
	return nil
}

func storeTrainingBlockIdempotency(ctx context.Context, tx pgx.Tx, userID uuid.UUID, scope, operationKey, requestHash string, status int, block *model.TrainingBlock) error {
	revision := block.Revision
	record := model.IdempotencyRecord{
		Scope: scope, OperationKey: operationKey, RequestHash: requestHash,
		ResponseStatus: status, ResourceType: "criteria_training_block",
		ResourceID: &block.ID, ResourceRevision: &revision,
	}
	return insertIdempotency(ctx, tx, userID, record, block)
}

func findTrainingStageByOrder(stages []model.TrainingStage, order int) *model.TrainingStage {
	for i := range stages {
		if stages[i].StageOrder == order {
			return &stages[i]
		}
	}
	return nil
}

func findTrainingStageByID(stages []model.TrainingStage, id uuid.UUID) *model.TrainingStage {
	for i := range stages {
		if stages[i].ID == id {
			return &stages[i]
		}
	}
	return nil
}
