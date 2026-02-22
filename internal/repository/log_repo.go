package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

type LogRepository interface {
	ListByWeek(ctx context.Context, userID uuid.UUID, weekStart time.Time) ([]model.DayLogSummary, error)
	GetByDate(ctx context.Context, userID uuid.UUID, date string) (*model.DayLog, error)
	Create(ctx context.Context, userID uuid.UUID, log *model.DayLog) error
	Update(ctx context.Context, userID uuid.UUID, date string, overrides []model.ExerciseOverride, sessionNotes *string) error
	Delete(ctx context.Context, userID uuid.UUID, date string) error
}

type logRepo struct {
	pool *pgxpool.Pool
}

func NewLogRepo(pool *pgxpool.Pool) LogRepository {
	return &logRepo{pool: pool}
}

func (r *logRepo) ListByWeek(ctx context.Context, userID uuid.UUID, weekStart time.Time) ([]model.DayLogSummary, error) {
	weekEnd := weekStart.AddDate(0, 0, 6)

	rows, err := r.pool.Query(ctx, `
		SELECT dl.id, dl.date, dl.type_id, dl.subtype_id,
		       dl.template_id, wt.name AS template_name,
		       dl.session_notes, dl.logged_at
		FROM day_logs dl
		LEFT JOIN workout_templates wt ON wt.id = dl.template_id
		WHERE dl.user_id = $1 AND dl.date BETWEEN $2 AND $3
		ORDER BY dl.date`,
		userID, weekStart, weekEnd,
	)
	if err != nil {
		return nil, fmt.Errorf("querying weekly logs: %w", err)
	}
	defer rows.Close()

	var summaries []model.DayLogSummary
	for rows.Next() {
		var s model.DayLogSummary
		if err := rows.Scan(
			&s.ID, &s.Date, &s.TypeID, &s.SubtypeID,
			&s.TemplateID, &s.TemplateName,
			&s.SessionNotes, &s.LoggedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning day log summary: %w", err)
		}
		summaries = append(summaries, s)
	}

	if summaries == nil {
		summaries = []model.DayLogSummary{}
	}
	return summaries, rows.Err()
}

func (r *logRepo) GetByDate(ctx context.Context, userID uuid.UUID, date string) (*model.DayLog, error) {
	dl := &model.DayLog{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, date, type_id, subtype_id, template_id, session_notes, logged_at
		FROM day_logs
		WHERE user_id = $1 AND date = $2`,
		userID, date,
	).Scan(
		&dl.ID, &dl.UserID, &dl.Date, &dl.TypeID, &dl.SubtypeID,
		&dl.TemplateID, &dl.SessionNotes, &dl.LoggedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, &model.NotFoundError{Message: "log not found"}
		}
		return nil, fmt.Errorf("querying day log: %w", err)
	}

	// Load overrides.
	overrides, err := r.getOverrides(ctx, dl.ID)
	if err != nil {
		return nil, err
	}
	dl.Overrides = overrides

	// If linked to a template, load it.
	if dl.TemplateID != nil {
		tmpl, err := r.getTemplate(ctx, *dl.TemplateID)
		if err != nil {
			return nil, err
		}
		dl.Template = tmpl
		if tmpl != nil {
			dl.TemplateName = &tmpl.Name
		}
	}

	return dl, nil
}

func (r *logRepo) getOverrides(ctx context.Context, dayLogID uuid.UUID) ([]model.ExerciseOverride, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, day_log_id, exercise_id, actual_sets, actual_reps, actual_weight, notes, skipped
		FROM exercise_overrides
		WHERE day_log_id = $1`,
		dayLogID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying overrides: %w", err)
	}
	defer rows.Close()

	var overrides []model.ExerciseOverride
	for rows.Next() {
		var o model.ExerciseOverride
		if err := rows.Scan(
			&o.ID, &o.DayLogID, &o.ExerciseID,
			&o.ActualSets, &o.ActualReps, &o.ActualWeight,
			&o.Notes, &o.Skipped,
		); err != nil {
			return nil, fmt.Errorf("scanning override: %w", err)
		}
		overrides = append(overrides, o)
	}

	if overrides == nil {
		overrides = []model.ExerciseOverride{}
	}
	return overrides, rows.Err()
}

func (r *logRepo) getTemplate(ctx context.Context, templateID uuid.UUID) (*model.WorkoutTemplate, error) {
	t := &model.WorkoutTemplate{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, type_id, subtype_id, created_at, updated_at
		FROM workout_templates
		WHERE id = $1`,
		templateID,
	).Scan(&t.ID, &t.Name, &t.TypeID, &t.SubtypeID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("querying template for log: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, template_id, name, sort_order, sets, reps, weight, rest_seconds, notes
		FROM exercises
		WHERE template_id = $1
		ORDER BY sort_order`,
		templateID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying exercises for log template: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var e model.Exercise
		if err := rows.Scan(
			&e.ID, &e.TemplateID, &e.Name, &e.SortOrder,
			&e.Sets, &e.Reps, &e.Weight, &e.RestSeconds, &e.Notes,
		); err != nil {
			return nil, fmt.Errorf("scanning exercise: %w", err)
		}
		t.Exercises = append(t.Exercises, e)
	}

	if t.Exercises == nil {
		t.Exercises = []model.Exercise{}
	}
	return t, rows.Err()
}

func (r *logRepo) Create(ctx context.Context, userID uuid.UUID, dl *model.DayLog) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		INSERT INTO day_logs (user_id, date, type_id, subtype_id, template_id, session_notes)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, logged_at`,
		userID, dl.Date, dl.TypeID, dl.SubtypeID, dl.TemplateID, dl.SessionNotes,
	).Scan(&dl.ID, &dl.LoggedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return &model.ConflictError{Message: "a log already exists for this date"}
		}
		return fmt.Errorf("inserting day log: %w", err)
	}
	dl.UserID = userID

	for i := range dl.Overrides {
		o := &dl.Overrides[i]
		o.DayLogID = dl.ID
		err := tx.QueryRow(ctx, `
			INSERT INTO exercise_overrides (day_log_id, exercise_id, actual_sets, actual_reps, actual_weight, notes, skipped)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id`,
			o.DayLogID, o.ExerciseID,
			o.ActualSets, o.ActualReps, o.ActualWeight,
			o.Notes, o.Skipped,
		).Scan(&o.ID)
		if err != nil {
			return fmt.Errorf("inserting override: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (r *logRepo) Update(ctx context.Context, userID uuid.UUID, date string, overrides []model.ExerciseOverride, sessionNotes *string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var logID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT id FROM day_logs WHERE user_id = $1 AND date = $2`,
		userID, date,
	).Scan(&logID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &model.NotFoundError{Message: "log not found"}
		}
		return fmt.Errorf("querying log for update: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE day_logs SET session_notes = $1 WHERE id = $2`,
		sessionNotes, logID,
	)
	if err != nil {
		return fmt.Errorf("updating session notes: %w", err)
	}

	// Replace overrides: delete existing, insert new.
	_, err = tx.Exec(ctx, `DELETE FROM exercise_overrides WHERE day_log_id = $1`, logID)
	if err != nil {
		return fmt.Errorf("deleting overrides: %w", err)
	}

	for i := range overrides {
		o := &overrides[i]
		o.DayLogID = logID
		err := tx.QueryRow(ctx, `
			INSERT INTO exercise_overrides (day_log_id, exercise_id, actual_sets, actual_reps, actual_weight, notes, skipped)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id`,
			o.DayLogID, o.ExerciseID,
			o.ActualSets, o.ActualReps, o.ActualWeight,
			o.Notes, o.Skipped,
		).Scan(&o.ID)
		if err != nil {
			return fmt.Errorf("inserting override: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (r *logRepo) Delete(ctx context.Context, userID uuid.UUID, date string) error {
	result, err := r.pool.Exec(ctx, `
		DELETE FROM day_logs WHERE user_id = $1 AND date = $2`,
		userID, date,
	)
	if err != nil {
		return fmt.Errorf("deleting day log: %w", err)
	}
	if result.RowsAffected() == 0 {
		return &model.NotFoundError{Message: "log not found"}
	}
	return nil
}
