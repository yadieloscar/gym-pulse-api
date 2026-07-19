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

type WorkoutSessionDAO interface {
	List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.WorkoutSession, error)
	Get(ctx context.Context, userID, sessionID uuid.UUID) (*model.WorkoutSession, error)
	Create(ctx context.Context, userID uuid.UUID, session *model.WorkoutSession) error
	Update(ctx context.Context, userID uuid.UUID, session *model.WorkoutSession, expectedRevision int64) error
}

type PerformedSetDAO interface {
	PutRequired(ctx context.Context, userID, sessionID, scheduledSetID uuid.UUID, set *model.PerformedSet, expectedRevision int64) (*model.WorkoutSession, error)
	AddExtra(ctx context.Context, userID, sessionID uuid.UUID, set *model.PerformedSet, expectedRevision int64) (*model.WorkoutSession, error)
}

type ParticipationDAO interface {
	List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.DayParticipation, error)
	Finalize(ctx context.Context, userID uuid.UUID, outcome *model.DayParticipation) error
	Preserve(ctx context.Context, userID uuid.UUID, outcome *model.DayParticipation) error
}

type IdempotencyDAO interface {
	Get(ctx context.Context, userID uuid.UUID, scope, operationKey string) (*model.IdempotencyRecord, error)
	Create(ctx context.Context, userID uuid.UUID, record *model.IdempotencyRecord) error
}

type workoutSessionDAO struct{ pool *pgxpool.Pool }
type performedSetDAO struct{ pool *pgxpool.Pool }
type participationDAO struct{ pool *pgxpool.Pool }
type idempotencyDAO struct{ pool *pgxpool.Pool }

func NewWorkoutSessionDAO(pool *pgxpool.Pool) WorkoutSessionDAO {
	return &workoutSessionDAO{pool: pool}
}

func NewPerformedSetDAO(pool *pgxpool.Pool) PerformedSetDAO {
	return &performedSetDAO{pool: pool}
}

func NewParticipationDAO(pool *pgxpool.Pool) ParticipationDAO {
	return &participationDAO{pool: pool}
}

func NewIdempotencyDAO(pool *pgxpool.Pool) IdempotencyDAO {
	return &idempotencyDAO{pool: pool}
}

func (r *workoutSessionDAO) List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.WorkoutSession, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, scheduled_workout_id, to_char(date, 'YYYY-MM-DD'), name,
		       status, notes, started_at, completed_at, revision, created_at, updated_at
		FROM workout_sessions
		WHERE user_id=$1 AND date BETWEEN $2 AND $3
		ORDER BY date, created_at`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying workout sessions: %w", err)
	}
	defer rows.Close()
	sessions := []model.WorkoutSession{}
	for rows.Next() {
		var s model.WorkoutSession
		if err := scanSession(rows, &s); err != nil {
			return nil, fmt.Errorf("scanning workout session: %w", err)
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating workout sessions: %w", err)
	}
	for i := range sessions {
		sets, err := loadPerformedSets(ctx, r.pool, userID, sessions[i].ID)
		if err != nil {
			return nil, err
		}
		sessions[i].Sets = sets
	}
	return sessions, nil
}

func (r *workoutSessionDAO) Get(ctx context.Context, userID, sessionID uuid.UUID) (*model.WorkoutSession, error) {
	s := &model.WorkoutSession{}
	if err := scanSession(r.pool.QueryRow(ctx, `
		SELECT id, scheduled_workout_id, to_char(date, 'YYYY-MM-DD'), name,
		       status, notes, started_at, completed_at, revision, created_at, updated_at
		FROM workout_sessions WHERE id=$1 AND user_id=$2`, sessionID, userID), s); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &model.NotFoundError{Message: "workout session not found"}
		}
		return nil, fmt.Errorf("querying workout session: %w", err)
	}
	sets, err := loadPerformedSets(ctx, r.pool, userID, sessionID)
	if err != nil {
		return nil, err
	}
	s.Sets = sets
	return s, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSession(row rowScanner, s *model.WorkoutSession) error {
	return row.Scan(&s.ID, &s.ScheduledWorkoutID, &s.Date, &s.Name, &s.Status,
		&s.Notes, &s.StartedAt, &s.CompletedAt, &s.Revision, &s.CreatedAt, &s.UpdatedAt)
}

func (r *workoutSessionDAO) Create(ctx context.Context, userID uuid.UUID, s *model.WorkoutSession) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning workout session create: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, scheduleLockNamespace, userID); err != nil {
		return err
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO workout_sessions (
			user_id, scheduled_workout_id, date, name, status, notes, started_at, completed_at)
		SELECT $1, sw.id, $3::date, $4, $5, $6, $7::timestamptz, $8::timestamptz
		FROM scheduled_workouts sw
		WHERE sw.id=$2 AND sw.user_id=$1
		UNION ALL
		SELECT $1, NULL, $3::date, $4, $5, $6, $7::timestamptz, $8::timestamptz WHERE $2::uuid IS NULL
		RETURNING id, revision, created_at, updated_at`,
		userID, s.ScheduledWorkoutID, s.Date, s.Name, s.Status, s.Notes, s.StartedAt, s.CompletedAt,
	).Scan(&s.ID, &s.Revision, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return &model.NotFoundError{Message: "scheduled workout not found"}
	}
	if err != nil {
		return fmt.Errorf("creating workout session: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing workout session create: %w", err)
	}
	return nil
}

func (r *workoutSessionDAO) Update(ctx context.Context, userID uuid.UUID, s *model.WorkoutSession, expectedRevision int64) error {
	err := r.pool.QueryRow(ctx, `
		UPDATE workout_sessions SET name=$3, status=$4, notes=$5, started_at=$6,
		       completed_at=$7, revision=revision+1, updated_at=now()
		WHERE id=$1 AND user_id=$2 AND revision=$8
		RETURNING revision, updated_at`, s.ID, userID, s.Name, s.Status, s.Notes,
		s.StartedAt, s.CompletedAt, expectedRevision).Scan(&s.Revision, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		current, getErr := r.Get(ctx, userID, s.ID)
		if getErr != nil {
			return getErr
		}
		return &model.ConflictError{Message: "workout session revision conflict", Expected: expectedRevision, Actual: current.Revision, Authoritative: current}
	}
	if err != nil {
		return fmt.Errorf("updating workout session: %w", err)
	}
	return nil
}

func loadPerformedSets(ctx context.Context, q queryer, userID, sessionID uuid.UUID) ([]model.PerformedSet, error) {
	rows, err := q.Query(ctx, `
		SELECT sl.id, sl.scheduled_set_id, sl.exercise_id, sl.is_extra,
		       sl.exercise_name, sl.exercise_category, sl.exercise_modality,
		       sl.set_index, sl.target_reps, sl.target_weight, sl.actual_reps,
		       sl.actual_weight, sl.duration_seconds, sl.completed,
		       sl.operation_key, sl.revision
		FROM set_logs sl
		JOIN workout_sessions ws ON ws.id=sl.workout_session_id AND ws.user_id=$1
		WHERE sl.workout_session_id=$2 ORDER BY sl.logged_at`, userID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("querying performed sets: %w", err)
	}
	defer rows.Close()
	sets := []model.PerformedSet{}
	for rows.Next() {
		var s model.PerformedSet
		if err := scanPerformedSet(rows, &s); err != nil {
			return nil, fmt.Errorf("scanning performed set: %w", err)
		}
		sets = append(sets, s)
	}
	return sets, rows.Err()
}

func scanPerformedSet(row rowScanner, s *model.PerformedSet) error {
	return row.Scan(&s.ID, &s.ScheduledSetID, &s.ExerciseID, &s.IsExtra,
		&s.ExerciseName, &s.ExerciseCategory, &s.ExerciseModality, &s.SetIndex,
		&s.TargetReps, &s.TargetWeight, &s.ActualReps, &s.ActualWeight,
		&s.DurationSeconds, &s.Completed, &s.OperationKey, &s.Revision)
}

func (r *performedSetDAO) PutRequired(ctx context.Context, userID, sessionID, scheduledSetID uuid.UUID, s *model.PerformedSet, expectedRevision int64) (*model.WorkoutSession, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning required set update: %w", err)
	}
	defer tx.Rollback(ctx)

	var currentRevision int64
	err = tx.QueryRow(ctx, `
		SELECT revision FROM workout_sessions WHERE id=$1 AND user_id=$2 FOR UPDATE`, sessionID, userID).Scan(&currentRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "workout session not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("locking workout session: %w", err)
	}
	if currentRevision != expectedRevision {
		return nil, &model.ConflictError{Message: "workout session revision conflict", Expected: expectedRevision, Actual: currentRevision}
	}

	err = tx.QueryRow(ctx, `
		SELECT NULL::uuid, ss.exercise_name, ss.exercise_category, ss.exercise_modality,
		       ss.set_index, ss.target_reps, ss.target_weight
		FROM scheduled_sets ss
		JOIN scheduled_workouts sw ON sw.id=ss.scheduled_workout_id
		JOIN workout_sessions ws ON ws.scheduled_workout_id=sw.id
		WHERE ss.id=$1 AND ws.id=$2 AND ws.user_id=$3`, scheduledSetID, sessionID, userID).Scan(
		&s.ExerciseID, &s.ExerciseName, &s.ExerciseCategory, &s.ExerciseModality, &s.SetIndex,
		&s.TargetReps, &s.TargetWeight,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "scheduled set not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("querying scheduled set snapshot: %w", err)
	}
	s.ScheduledSetID = &scheduledSetID
	s.IsExtra = false
	err = tx.QueryRow(ctx, `
		INSERT INTO set_logs (
			workout_session_id, scheduled_set_id, exercise_id, is_extra,
			exercise_name, exercise_category, exercise_modality, set_index,
			target_reps, target_weight, actual_reps, actual_weight,
			duration_seconds, completed, operation_key)
		VALUES ($1,$2,$3,false,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (workout_session_id, scheduled_set_id) WHERE scheduled_set_id IS NOT NULL
		DO UPDATE SET actual_reps=EXCLUDED.actual_reps, actual_weight=EXCLUDED.actual_weight,
		              duration_seconds=EXCLUDED.duration_seconds, completed=EXCLUDED.completed,
		              operation_key=EXCLUDED.operation_key, revision=set_logs.revision+1,
		              logged_at=now()
		RETURNING id, revision`, sessionID, scheduledSetID, s.ExerciseID,
		s.ExerciseName, s.ExerciseCategory, s.ExerciseModality, s.SetIndex,
		s.TargetReps, s.TargetWeight, s.ActualReps, s.ActualWeight,
		s.DurationSeconds, s.Completed, s.OperationKey).Scan(&s.ID, &s.Revision)
	if err != nil {
		return nil, fmt.Errorf("upserting required set result: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE workout_sessions SET revision=revision+1, status='active',
		       started_at=COALESCE(started_at, now()), updated_at=now()
		WHERE id=$1 AND user_id=$2`, sessionID, userID); err != nil {
		return nil, fmt.Errorf("advancing workout session revision: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing required set update: %w", err)
	}
	return (&workoutSessionDAO{pool: r.pool}).Get(ctx, userID, sessionID)
}

func (r *performedSetDAO) AddExtra(ctx context.Context, userID, sessionID uuid.UUID, s *model.PerformedSet, expectedRevision int64) (*model.WorkoutSession, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning extra set create: %w", err)
	}
	defer tx.Rollback(ctx)
	var currentRevision int64
	err = tx.QueryRow(ctx, `SELECT revision FROM workout_sessions WHERE id=$1 AND user_id=$2 FOR UPDATE`, sessionID, userID).Scan(&currentRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "workout session not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("locking workout session: %w", err)
	}
	if currentRevision != expectedRevision {
		return nil, &model.ConflictError{Message: "workout session revision conflict", Expected: expectedRevision, Actual: currentRevision}
	}
	s.IsExtra = true
	s.ScheduledSetID = nil
	err = tx.QueryRow(ctx, `
		INSERT INTO set_logs (
			workout_session_id, exercise_id, is_extra, exercise_name,
			exercise_category, exercise_modality, set_index, target_reps,
			target_weight, actual_reps, actual_weight, duration_seconds,
			completed, operation_key)
		VALUES ($1,$2,true,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id, revision`, sessionID, s.ExerciseID, s.ExerciseName,
		s.ExerciseCategory, s.ExerciseModality, s.SetIndex, s.TargetReps,
		s.TargetWeight, s.ActualReps, s.ActualWeight, s.DurationSeconds,
		s.Completed, s.OperationKey).Scan(&s.ID, &s.Revision)
	if err != nil {
		return nil, fmt.Errorf("inserting extra set: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE workout_sessions SET revision=revision+1, status='active',
		       started_at=COALESCE(started_at, now()), updated_at=now()
		WHERE id=$1 AND user_id=$2`, sessionID, userID); err != nil {
		return nil, fmt.Errorf("advancing workout session revision: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing extra set create: %w", err)
	}
	return (&workoutSessionDAO{pool: r.pool}).Get(ctx, userID, sessionID)
}

func (r *participationDAO) List(ctx context.Context, userID uuid.UUID, from, to string) ([]model.DayParticipation, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, to_char(date, 'YYYY-MM-DD'), scheduled_opportunity, participated,
		       finalized_at, timezone, to_char(local_date, 'YYYY-MM-DD'), revision
		FROM day_participation WHERE user_id=$1 AND date BETWEEN $2 AND $3
		ORDER BY date`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying participation: %w", err)
	}
	defer rows.Close()
	outcomes := []model.DayParticipation{}
	for rows.Next() {
		var outcome model.DayParticipation
		if err := rows.Scan(&outcome.ID, &outcome.Date, &outcome.ScheduledOpportunity,
			&outcome.Participated, &outcome.FinalizedAt, &outcome.Timezone,
			&outcome.LocalDate, &outcome.Revision); err != nil {
			return nil, fmt.Errorf("scanning participation: %w", err)
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, rows.Err()
}

func (r *participationDAO) Finalize(ctx context.Context, userID uuid.UUID, o *model.DayParticipation) error {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO day_participation (
			user_id, date, scheduled_opportunity, participated, finalized_at, timezone, local_date)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (user_id, date) DO NOTHING
		RETURNING id, revision`, userID, o.Date, o.ScheduledOpportunity,
		o.Participated, o.FinalizedAt, o.Timezone, o.LocalDate).Scan(&o.ID, &o.Revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return &model.ConflictError{Message: "participation is already finalized"}
	}
	if err != nil {
		return fmt.Errorf("finalizing participation: %w", err)
	}
	return nil
}

func (r *participationDAO) Preserve(ctx context.Context, userID uuid.UUID, o *model.DayParticipation) error {
	return r.pool.QueryRow(ctx, `
		INSERT INTO day_participation (user_id, date, scheduled_opportunity, participated, finalized_at, timezone, local_date)
		VALUES ($1,$2,$3,true,$4,$5,$6)
		ON CONFLICT (user_id, date) DO UPDATE SET
		  participated=true,
		  scheduled_opportunity=day_participation.scheduled_opportunity OR EXCLUDED.scheduled_opportunity,
		  revision=day_participation.revision+1
		RETURNING id, revision`, userID, o.Date, o.ScheduledOpportunity, o.FinalizedAt, o.Timezone, o.LocalDate).Scan(&o.ID, &o.Revision)
}

func (r *idempotencyDAO) Get(ctx context.Context, userID uuid.UUID, scope, operationKey string) (*model.IdempotencyRecord, error) {
	record := &model.IdempotencyRecord{}
	err := r.pool.QueryRow(ctx, `
		SELECT scope, operation_key, request_hash, response_status, response_body,
		       resource_type, resource_id, resource_revision, created_at, expires_at
		FROM idempotency_records
		WHERE user_id=$1 AND scope=$2 AND operation_key=$3
		  AND (expires_at IS NULL OR expires_at > now())`, userID, scope, operationKey).Scan(
		&record.Scope, &record.OperationKey, &record.RequestHash, &record.ResponseStatus,
		&record.ResponseBody, &record.ResourceType, &record.ResourceID,
		&record.ResourceRevision, &record.CreatedAt, &record.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "idempotency record not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("querying idempotency record: %w", err)
	}
	return record, nil
}

func (r *idempotencyDAO) Create(ctx context.Context, userID uuid.UUID, record *model.IdempotencyRecord) error {
	var body any
	if err := json.Unmarshal(record.ResponseBody, &body); err != nil {
		return fmt.Errorf("decoding idempotency response: %w", err)
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO idempotency_records (
			user_id, scope, operation_key, request_hash, response_status,
			response_body, resource_type, resource_id, resource_revision, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, userID, record.Scope,
		record.OperationKey, record.RequestHash, record.ResponseStatus, body,
		record.ResourceType, record.ResourceID, record.ResourceRevision, record.ExpiresAt)
	if err != nil {
		return fmt.Errorf("creating idempotency record: %w", err)
	}
	return nil
}
