package dao

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

type LogDAO interface {
	ListByWeek(ctx context.Context, userID uuid.UUID, weekStart time.Time) ([]model.DayLogSummary, error)
	GetByDate(ctx context.Context, userID uuid.UUID, date string) (*model.DayLog, error)
	Create(ctx context.Context, userID uuid.UUID, log *model.DayLog) error
	Update(ctx context.Context, userID uuid.UUID, date string, overrides []model.ExerciseOverride, setLogs []model.SetLog, sessionNotes *string, replace *model.LogReplacement) error
	Delete(ctx context.Context, userID uuid.UUID, date string) error
	ExerciseHistory(ctx context.Context, userID uuid.UUID, exerciseIDs []uuid.UUID) ([]model.ExerciseHistory, error)
}

type logDAO struct {
	pool *pgxpool.Pool
}

func NewLogDAO(pool *pgxpool.Pool) LogDAO {
	return &logDAO{pool: pool}
}

func (r *logDAO) ListByWeek(ctx context.Context, userID uuid.UUID, weekStart time.Time) ([]model.DayLogSummary, error) {
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

func (r *logDAO) GetByDate(ctx context.Context, userID uuid.UUID, date string) (*model.DayLog, error) {
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
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &model.NotFoundError{Message: "log not found"}
		}
		return nil, fmt.Errorf("querying day log: %w", err)
	}

	overrides, err := r.getOverrides(ctx, dl.ID)
	if err != nil {
		return nil, err
	}
	dl.Overrides = overrides

	setLogs, err := r.getSetLogs(ctx, dl.ID)
	if err != nil {
		return nil, err
	}
	dl.SetLogs = setLogs

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

func (r *logDAO) getOverrides(ctx context.Context, dayLogID uuid.UUID) ([]model.ExerciseOverride, error) {
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

func (r *logDAO) getSetLogs(ctx context.Context, dayLogID uuid.UUID) ([]model.SetLog, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, day_log_id, exercise_id, set_index,
		       target_reps, target_weight, actual_reps, actual_weight,
		       duration_seconds, completed
		FROM set_logs
		WHERE day_log_id = $1
		ORDER BY exercise_id, set_index`,
		dayLogID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying set logs: %w", err)
	}
	defer rows.Close()

	var sets []model.SetLog
	for rows.Next() {
		var s model.SetLog
		if err := rows.Scan(
			&s.ID, &s.DayLogID, &s.ExerciseID, &s.SetIndex,
			&s.TargetReps, &s.TargetWeight, &s.ActualReps, &s.ActualWeight,
			&s.DurationSeconds, &s.Completed,
		); err != nil {
			return nil, fmt.Errorf("scanning set log: %w", err)
		}
		sets = append(sets, s)
	}

	if sets == nil {
		sets = []model.SetLog{}
	}
	return sets, rows.Err()
}

// insertSetLogs writes the day's sets within the caller's transaction. Callers
// delete existing rows first (replace semantics), mirroring overrides.
func insertSetLogs(ctx context.Context, tx pgx.Tx, dayLogID uuid.UUID, setLogs []model.SetLog) error {
	for i := range setLogs {
		s := &setLogs[i]
		s.DayLogID = dayLogID
		err := tx.QueryRow(ctx, `
			INSERT INTO set_logs (day_log_id, exercise_id, set_index, target_reps, target_weight, actual_reps, actual_weight, duration_seconds, completed)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id`,
			s.DayLogID, s.ExerciseID, s.SetIndex,
			s.TargetReps, s.TargetWeight, s.ActualReps, s.ActualWeight,
			s.DurationSeconds, s.Completed,
		).Scan(&s.ID)
		if err != nil {
			return fmt.Errorf("inserting set log: %w", err)
		}
	}
	return nil
}

// ExerciseHistory returns, per requested exercise, the completed sets from the
// most recent day they were performed — the "last time you did X" data.
func (r *logDAO) ExerciseHistory(ctx context.Context, userID uuid.UUID, exerciseIDs []uuid.UUID) ([]model.ExerciseHistory, error) {
	if len(exerciseIDs) == 0 {
		return []model.ExerciseHistory{}, nil
	}

	rows, err := r.pool.Query(ctx, `
		WITH ranked AS (
			SELECT sl.exercise_id, to_char(dl.date, 'YYYY-MM-DD') AS date, sl.set_index,
			       sl.target_reps, sl.target_weight, sl.actual_reps, sl.actual_weight,
			       sl.duration_seconds, sl.completed,
			       DENSE_RANK() OVER (PARTITION BY sl.exercise_id ORDER BY dl.date DESC) AS rnk
			FROM set_logs sl
			JOIN day_logs dl ON dl.id = sl.day_log_id
			WHERE dl.user_id = $1 AND sl.exercise_id = ANY($2) AND sl.completed = true
		)
		SELECT exercise_id, date, set_index, target_reps, target_weight,
		       actual_reps, actual_weight, duration_seconds, completed
		FROM ranked
		WHERE rnk = 1
		ORDER BY exercise_id, set_index`,
		userID, exerciseIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("querying exercise history: %w", err)
	}
	defer rows.Close()

	byExercise := map[uuid.UUID]*model.ExerciseHistory{}
	order := []uuid.UUID{}
	for rows.Next() {
		var (
			exID uuid.UUID
			date string
			s    model.SetLog
		)
		if err := rows.Scan(
			&exID, &date, &s.SetIndex,
			&s.TargetReps, &s.TargetWeight, &s.ActualReps, &s.ActualWeight,
			&s.DurationSeconds, &s.Completed,
		); err != nil {
			return nil, fmt.Errorf("scanning exercise history: %w", err)
		}
		s.ExerciseID = exID
		h, ok := byExercise[exID]
		if !ok {
			h = &model.ExerciseHistory{ExerciseID: exID, Date: date, Sets: []model.SetLog{}}
			byExercise[exID] = h
			order = append(order, exID)
		}
		h.Sets = append(h.Sets, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating exercise history: %w", err)
	}

	history := make([]model.ExerciseHistory, 0, len(order))
	for _, id := range order {
		history = append(history, *byExercise[id])
	}
	return history, nil
}

func (r *logDAO) getTemplate(ctx context.Context, templateID uuid.UUID) (*model.WorkoutTemplate, error) {
	t := &model.WorkoutTemplate{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, type_id, subtype_id, created_at, updated_at
		FROM workout_templates
		WHERE id = $1`,
		templateID,
	).Scan(&t.ID, &t.Name, &t.TypeID, &t.SubtypeID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil //nolint:nilnil
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

func (r *logDAO) Create(ctx context.Context, userID uuid.UUID, dl *model.DayLog) error {
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

	if err := insertSetLogs(ctx, tx, dl.ID, dl.SetLogs); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *logDAO) Update(ctx context.Context, userID uuid.UUID, date string, overrides []model.ExerciseOverride, setLogs []model.SetLog, sessionNotes *string, replace *model.LogReplacement) error {
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
		if errors.Is(err, pgx.ErrNoRows) {
			return &model.NotFoundError{Message: "log not found"}
		}
		return fmt.Errorf("querying log for update: %w", err)
	}

	if replace != nil {
		_, err = tx.Exec(ctx, `
			UPDATE day_logs
			SET type_id = $1, subtype_id = $2, template_id = $3, session_notes = $4
			WHERE id = $5`,
			replace.TypeID, replace.SubtypeID, replace.TemplateID, sessionNotes, logID,
		)
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE day_logs SET session_notes = $1 WHERE id = $2`,
			sessionNotes, logID,
		)
	}
	if err != nil {
		return fmt.Errorf("updating day log: %w", err)
	}

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

	// Set logs follow the same replace semantics as overrides: a PUT rewrites
	// the day's whole set list.
	if _, err = tx.Exec(ctx, `DELETE FROM set_logs WHERE day_log_id = $1`, logID); err != nil {
		return fmt.Errorf("deleting set logs: %w", err)
	}
	if err := insertSetLogs(ctx, tx, logID, setLogs); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *logDAO) Delete(ctx context.Context, userID uuid.UUID, date string) error {
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
