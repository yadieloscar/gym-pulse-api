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

type TemplateDAO interface {
	List(ctx context.Context, userID uuid.UUID, typeFilter, subtypeFilter string) ([]model.TemplateSummary, error)
	GetByID(ctx context.Context, userID, templateID uuid.UUID) (*model.WorkoutTemplate, error)
	Create(ctx context.Context, userID uuid.UUID, t *model.WorkoutTemplate) error
	Update(ctx context.Context, userID uuid.UUID, t *model.WorkoutTemplate) error
	Delete(ctx context.Context, userID, templateID uuid.UUID) error
}

type templateDAO struct {
	pool *pgxpool.Pool
}

func NewTemplateDAO(pool *pgxpool.Pool) TemplateDAO {
	return &templateDAO{pool: pool}
}

func (r *templateDAO) List(ctx context.Context, userID uuid.UUID, typeFilter, subtypeFilter string) ([]model.TemplateSummary, error) {
	query := `
		SELECT t.id, t.name, t.type_id, t.subtype_id, t.created_at, t.updated_at,
		       COUNT(e.id) AS exercise_count,
		       COALESCE(
		           ARRAY_AGG(e.name ORDER BY e.sort_order) FILTER (WHERE e.sort_order < 3),
		           '{}'
		       ) AS exercises_preview
		FROM workout_templates t
		LEFT JOIN exercises e ON e.template_id = t.id
		WHERE t.user_id = $1`

	args := []any{userID}
	argIdx := 2

	if typeFilter != "" {
		query += fmt.Sprintf(" AND t.type_id = $%d", argIdx)
		args = append(args, typeFilter)
		argIdx++
	}
	if subtypeFilter != "" {
		query += fmt.Sprintf(" AND t.subtype_id = $%d", argIdx)
		args = append(args, subtypeFilter)
	}

	query += ` GROUP BY t.id ORDER BY t.updated_at DESC`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying templates: %w", err)
	}
	defer rows.Close()

	var summaries []model.TemplateSummary
	for rows.Next() {
		var s model.TemplateSummary
		if err := rows.Scan(
			&s.ID, &s.Name, &s.TypeID, &s.SubtypeID,
			&s.CreatedAt, &s.UpdatedAt,
			&s.ExerciseCount, &s.ExercisesPreview,
		); err != nil {
			return nil, fmt.Errorf("scanning template summary: %w", err)
		}
		summaries = append(summaries, s)
	}

	if summaries == nil {
		summaries = []model.TemplateSummary{}
	}
	return summaries, rows.Err()
}

func (r *templateDAO) GetByID(ctx context.Context, userID, templateID uuid.UUID) (*model.WorkoutTemplate, error) {
	t := &model.WorkoutTemplate{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, name, type_id, subtype_id, created_at, updated_at
		FROM workout_templates
		WHERE id = $1 AND user_id = $2`,
		templateID, userID,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.TypeID, &t.SubtypeID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &model.NotFoundError{Message: "template not found"}
		}
		return nil, fmt.Errorf("querying template: %w", err)
	}

	exercises, err := r.getExercises(ctx, templateID)
	if err != nil {
		return nil, err
	}
	t.Exercises = exercises
	return t, nil
}

func (r *templateDAO) getExercises(ctx context.Context, templateID uuid.UUID) ([]model.Exercise, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, template_id, name, sort_order, sets, reps, weight, rest_seconds, notes
		FROM exercises
		WHERE template_id = $1
		ORDER BY sort_order`,
		templateID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying exercises: %w", err)
	}
	defer rows.Close()

	var exercises []model.Exercise
	for rows.Next() {
		var e model.Exercise
		if err := rows.Scan(
			&e.ID, &e.TemplateID, &e.Name, &e.SortOrder,
			&e.Sets, &e.Reps, &e.Weight, &e.RestSeconds, &e.Notes,
		); err != nil {
			return nil, fmt.Errorf("scanning exercise: %w", err)
		}
		exercises = append(exercises, e)
	}

	if exercises == nil {
		exercises = []model.Exercise{}
	}
	return exercises, rows.Err()
}

func (r *templateDAO) Create(ctx context.Context, userID uuid.UUID, t *model.WorkoutTemplate) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		INSERT INTO workout_templates (user_id, name, type_id, subtype_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`,
		userID, t.Name, t.TypeID, t.SubtypeID,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return fmt.Errorf("inserting template: %w", err)
	}
	t.UserID = userID

	for i := range t.Exercises {
		e := &t.Exercises[i]
		e.TemplateID = t.ID
		e.SortOrder = i
		err := tx.QueryRow(ctx, `
			INSERT INTO exercises (template_id, name, sort_order, sets, reps, weight, rest_seconds, notes)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id`,
			e.TemplateID, e.Name, e.SortOrder,
			e.Sets, e.Reps, e.Weight, e.RestSeconds, e.Notes,
		).Scan(&e.ID)
		if err != nil {
			return fmt.Errorf("inserting exercise: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (r *templateDAO) Update(ctx context.Context, userID uuid.UUID, t *model.WorkoutTemplate) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var exists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM workout_templates WHERE id = $1 AND user_id = $2)`,
		t.ID, userID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("checking template ownership: %w", err)
	}
	if !exists {
		return &model.NotFoundError{Message: "template not found"}
	}

	err = tx.QueryRow(ctx, `
		UPDATE workout_templates
		SET name = $1, type_id = $2, subtype_id = $3, updated_at = now()
		WHERE id = $4 AND user_id = $5
		RETURNING updated_at`,
		t.Name, t.TypeID, t.SubtypeID, t.ID, userID,
	).Scan(&t.UpdatedAt)
	if err != nil {
		return fmt.Errorf("updating template: %w", err)
	}

	_, err = tx.Exec(ctx, `DELETE FROM exercises WHERE template_id = $1`, t.ID)
	if err != nil {
		return fmt.Errorf("deleting exercises: %w", err)
	}

	for i := range t.Exercises {
		e := &t.Exercises[i]
		e.TemplateID = t.ID
		e.SortOrder = i
		err := tx.QueryRow(ctx, `
			INSERT INTO exercises (template_id, name, sort_order, sets, reps, weight, rest_seconds, notes)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id`,
			e.TemplateID, e.Name, e.SortOrder,
			e.Sets, e.Reps, e.Weight, e.RestSeconds, e.Notes,
		).Scan(&e.ID)
		if err != nil {
			return fmt.Errorf("inserting exercise: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (r *templateDAO) Delete(ctx context.Context, userID, templateID uuid.UUID) error {
	result, err := r.pool.Exec(ctx, `
		DELETE FROM workout_templates WHERE id = $1 AND user_id = $2`,
		templateID, userID,
	)
	if err != nil {
		return fmt.Errorf("deleting template: %w", err)
	}
	if result.RowsAffected() == 0 {
		return &model.NotFoundError{Message: "template not found"}
	}
	return nil
}
