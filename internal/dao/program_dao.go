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

type StarterProgramDAO interface {
	List(ctx context.Context, filter model.StarterProgramFilter) ([]model.StarterProgram, error)
	Get(ctx context.Context, starterID uuid.UUID, version int) (*model.StarterProgram, error)
}

type ProgramDAO interface {
	List(ctx context.Context, userID uuid.UUID) ([]model.Program, error)
	Get(ctx context.Context, userID, programID uuid.UUID) (*model.Program, error)
	Create(ctx context.Context, userID uuid.UUID, program *model.Program) error
	Replace(ctx context.Context, userID uuid.UUID, program *model.Program, expectedRevision int64) error
	RecordLegacyAdoption(ctx context.Context, userID, programID uuid.UUID, operationKey string) error
}

type starterProgramDAO struct {
	pool *pgxpool.Pool
}

type programDAO struct {
	pool *pgxpool.Pool
}

func NewStarterProgramDAO(pool *pgxpool.Pool) StarterProgramDAO {
	return &starterProgramDAO{pool: pool}
}

func NewProgramDAO(pool *pgxpool.Pool) ProgramDAO {
	return &programDAO{pool: pool}
}

func (r *starterProgramDAO) List(ctx context.Context, f model.StarterProgramFilter) ([]model.StarterProgram, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, slug, version, name, description, primary_goal, min_days,
		       max_days, experience, equipment, duration_minutes, rationale, roadmap
		FROM starter_programs
		WHERE active = true
		  AND ($1 = '' OR primary_goal = $1)
		  AND ($2 = 0 OR $2 BETWEEN min_days AND max_days)
		  AND ($3 = '' OR $3 = ANY(experience))
		  AND ($4::text[] = '{}' OR equipment = '{}' OR equipment && $4)
		  AND ($5 = 0 OR duration_minutes <= $5)
		ORDER BY primary_goal, slug, version DESC`,
		f.PrimaryGoal, f.AvailableDays, f.Experience, f.Equipment, f.SessionDurationMinutes,
	)
	if err != nil {
		return nil, fmt.Errorf("querying starter programs: %w", err)
	}
	defer rows.Close()

	programs := []model.StarterProgram{}
	for rows.Next() {
		var p model.StarterProgram
		if err := rows.Scan(
			&p.ID, &p.Slug, &p.Version, &p.Name, &p.Description, &p.PrimaryGoal,
			&p.MinDays, &p.MaxDays, &p.Experience, &p.Equipment, &p.DurationMinutes,
			&p.Rationale, &p.Roadmap,
		); err != nil {
			return nil, fmt.Errorf("scanning starter program: %w", err)
		}
		programs = append(programs, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating starter programs: %w", err)
	}
	return programs, nil
}

func (r *starterProgramDAO) Get(ctx context.Context, starterID uuid.UUID, version int) (*model.StarterProgram, error) {
	p := &model.StarterProgram{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, slug, version, name, description, primary_goal, min_days,
		       max_days, experience, equipment, duration_minutes, rationale, roadmap
		FROM starter_programs WHERE id=$1 AND version=$2`, starterID, version).Scan(
		&p.ID, &p.Slug, &p.Version, &p.Name, &p.Description, &p.PrimaryGoal,
		&p.MinDays, &p.MaxDays, &p.Experience, &p.Equipment, &p.DurationMinutes,
		&p.Rationale, &p.Roadmap,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "starter program not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("querying starter program: %w", err)
	}

	workouts, err := loadStarterWorkouts(ctx, r.pool, p.ID)
	if err != nil {
		return nil, err
	}
	p.Workouts = workouts
	return p, nil
}

type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func loadStarterWorkouts(ctx context.Context, q queryer, starterID uuid.UUID) ([]model.ProgramWorkout, error) {
	rows, err := q.Query(ctx, `
		SELECT sw.id, sw.name, sw.weekday, sw.sequence_position,
		       se.id, se.catalog_id, se.name, se.category, se.modality,
		       se.exercise_order, se.target_sets, se.target_reps, se.target_weight,
		       se.target_duration_seconds, se.rest_seconds, se.notes
		FROM starter_workouts sw
		LEFT JOIN starter_exercises se ON se.starter_workout_id = sw.id
		WHERE sw.starter_program_id=$1
		ORDER BY sw.sequence_position, se.exercise_order`, starterID)
	if err != nil {
		return nil, fmt.Errorf("querying starter workouts: %w", err)
	}
	defer rows.Close()
	return scanProgramWorkouts(rows, true)
}

func (r *programDAO) List(ctx context.Context, userID uuid.UUID) ([]model.Program, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, starter_program_id, starter_version, name, primary_goal, roadmap,
		       active, revision, created_at, updated_at
		FROM programs WHERE user_id=$1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying programs: %w", err)
	}
	defer rows.Close()
	programs := []model.Program{}
	for rows.Next() {
		var p model.Program
		if err := rows.Scan(&p.ID, &p.StarterProgramID, &p.StarterVersion, &p.Name,
			&p.PrimaryGoal, &p.Roadmap, &p.Active, &p.Revision, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning program: %w", err)
		}
		programs = append(programs, p)
	}
	return programs, rows.Err()
}

func (r *programDAO) Get(ctx context.Context, userID, programID uuid.UUID) (*model.Program, error) {
	p := &model.Program{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, starter_program_id, starter_version, name, primary_goal, roadmap,
		       active, revision, created_at, updated_at
		FROM programs WHERE id=$1 AND user_id=$2`, programID, userID).Scan(
		&p.ID, &p.StarterProgramID, &p.StarterVersion, &p.Name, &p.PrimaryGoal,
		&p.Roadmap, &p.Active, &p.Revision, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "program not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("querying program: %w", err)
	}
	workouts, err := loadProgramWorkouts(ctx, r.pool, userID, programID)
	if err != nil {
		return nil, err
	}
	p.Workouts = workouts
	return p, nil
}

func loadProgramWorkouts(ctx context.Context, q queryer, userID, programID uuid.UUID) ([]model.ProgramWorkout, error) {
	rows, err := q.Query(ctx, `
		SELECT pw.id, pw.name, pw.preferred_weekday, pw.sequence_position,
		       pe.id, pe.catalog_id, pe.source_starter_exercise_id, pe.name,
		       pe.category, pe.modality, pe.exercise_order, pe.target_sets,
		       pe.target_reps, pe.target_weight, pe.target_duration_seconds,
		       pe.rest_seconds, pe.notes
		FROM program_workouts pw
		JOIN programs p ON p.id = pw.program_id AND p.user_id=$1
		LEFT JOIN program_exercises pe ON pe.program_workout_id = pw.id
		WHERE pw.program_id=$2
		ORDER BY pw.sequence_position, pe.exercise_order`, userID, programID)
	if err != nil {
		return nil, fmt.Errorf("querying program workouts: %w", err)
	}
	defer rows.Close()
	return scanProgramWorkouts(rows, false)
}

func scanProgramWorkouts(rows pgx.Rows, starter bool) ([]model.ProgramWorkout, error) {
	workouts := []model.ProgramWorkout{}
	byID := make(map[uuid.UUID]int)
	for rows.Next() {
		var (
			workout    model.ProgramWorkout
			exerciseID *uuid.UUID
			exercise   model.ProgramExercise
		)
		if starter {
			err := rows.Scan(
				&workout.ID, &workout.Name, &workout.PreferredWeekday, &workout.SequencePosition,
				&exerciseID, &exercise.CatalogID, &exercise.Name, &exercise.Category,
				&exercise.Modality, &exercise.ExerciseOrder, &exercise.TargetSets,
				&exercise.TargetReps, &exercise.TargetWeight, &exercise.TargetDurationSeconds,
				&exercise.RestSeconds, &exercise.Notes,
			)
			if err != nil {
				return nil, fmt.Errorf("scanning starter workout: %w", err)
			}
		} else {
			err := rows.Scan(
				&workout.ID, &workout.Name, &workout.PreferredWeekday, &workout.SequencePosition,
				&exerciseID, &exercise.CatalogID, &exercise.SourceStarterExerciseID,
				&exercise.Name, &exercise.Category, &exercise.Modality, &exercise.ExerciseOrder,
				&exercise.TargetSets, &exercise.TargetReps, &exercise.TargetWeight,
				&exercise.TargetDurationSeconds, &exercise.RestSeconds, &exercise.Notes,
			)
			if err != nil {
				return nil, fmt.Errorf("scanning program workout: %w", err)
			}
		}
		idx, ok := byID[workout.ID]
		if !ok {
			workout.Exercises = []model.ProgramExercise{}
			workouts = append(workouts, workout)
			idx = len(workouts) - 1
			byID[workout.ID] = idx
		}
		if exerciseID != nil {
			exercise.ID = *exerciseID
			workouts[idx].Exercises = append(workouts[idx].Exercises, exercise)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating program workouts: %w", err)
	}
	return workouts, nil
}

func (r *programDAO) Create(ctx context.Context, userID uuid.UUID, p *model.Program) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning program create: %w", err)
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		INSERT INTO programs (user_id, starter_program_id, starter_version, name, primary_goal, roadmap, active)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, revision, created_at, updated_at`,
		userID, p.StarterProgramID, p.StarterVersion, p.Name, p.PrimaryGoal, p.Roadmap, p.Active,
	).Scan(&p.ID, &p.Revision, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("inserting program: %w", err)
	}
	if err := insertProgramWorkouts(ctx, tx, p.ID, p.Workouts); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing program create: %w", err)
	}
	return nil
}

func (r *programDAO) Replace(ctx context.Context, userID uuid.UUID, p *model.Program, expectedRevision int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning program update: %w", err)
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		UPDATE programs SET name=$3, primary_goal=$4, roadmap=$5, active=$6,
		       revision=revision+1, updated_at=now()
		WHERE id=$1 AND user_id=$2 AND revision=$7
		RETURNING revision, created_at, updated_at`, p.ID, userID, p.Name, p.PrimaryGoal,
		p.Roadmap, p.Active, expectedRevision).Scan(&p.Revision, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		current, getErr := r.Get(ctx, userID, p.ID)
		if getErr != nil {
			return getErr
		}
		return &model.ConflictError{Message: "program revision conflict", Expected: expectedRevision, Actual: current.Revision, Authoritative: current}
	}
	if err != nil {
		return fmt.Errorf("updating program: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM program_workouts WHERE program_id=$1`, p.ID); err != nil {
		return fmt.Errorf("replacing program workouts: %w", err)
	}
	if err := insertProgramWorkouts(ctx, tx, p.ID, p.Workouts); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing program update: %w", err)
	}
	return nil
}

func insertProgramWorkouts(ctx context.Context, tx pgx.Tx, programID uuid.UUID, workouts []model.ProgramWorkout) error {
	for i := range workouts {
		w := &workouts[i]
		if err := tx.QueryRow(ctx, `
			INSERT INTO program_workouts (program_id, name, preferred_weekday, sequence_position)
			VALUES ($1,$2,$3,$4) RETURNING id`,
			programID, w.Name, w.PreferredWeekday, w.SequencePosition).Scan(&w.ID); err != nil {
			return fmt.Errorf("inserting program workout: %w", err)
		}
		for j := range w.Exercises {
			e := &w.Exercises[j]
			if err := tx.QueryRow(ctx, `
				INSERT INTO program_exercises (
					program_workout_id, catalog_id, source_starter_exercise_id, name,
					category, modality, exercise_order, target_sets, target_reps,
					target_weight, target_duration_seconds, rest_seconds, notes)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`,
				w.ID, e.CatalogID, e.SourceStarterExerciseID, e.Name, e.Category,
				e.Modality, e.ExerciseOrder, e.TargetSets, e.TargetReps, e.TargetWeight,
				e.TargetDurationSeconds, e.RestSeconds, e.Notes).Scan(&e.ID); err != nil {
				return fmt.Errorf("inserting program exercise: %w", err)
			}
		}
	}
	return nil
}

func (r *programDAO) RecordLegacyAdoption(ctx context.Context, userID, programID uuid.UUID, operationKey string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO legacy_adoptions (user_id, program_id, operation_key)
		SELECT $1, p.id, $3 FROM programs p WHERE p.id=$2 AND p.user_id=$1
		ON CONFLICT (user_id) DO NOTHING`, userID, programID, operationKey)
	if err != nil {
		return fmt.Errorf("recording legacy adoption: %w", err)
	}
	return nil
}
