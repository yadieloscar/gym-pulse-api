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
	// CreateOnce creates the log at most once per operation key. A replay of
	// the same key and request hash fills log from the stored response and
	// reports replayed; a different hash is an idempotency conflict.
	CreateOnce(ctx context.Context, userID uuid.UUID, log *model.DayLog, operationKey, requestHash string) (replayed bool, err error)
	Update(ctx context.Context, userID uuid.UUID, date string, update model.DayLogUpdate) error
	Delete(ctx context.Context, userID uuid.UUID, date string) error
	ExerciseHistory(ctx context.Context, userID uuid.UUID, exerciseIDs []uuid.UUID) ([]model.ExerciseHistory, error)
	RecordSets(ctx context.Context, userID uuid.UUID, exerciseIDs []uuid.UUID) ([]model.SetPerf, error)
	// WeeklyHeaviestSets returns, per requested exercise that has any, the
	// heaviest completed weighted set of each Monday-start week on or after
	// since, weeks oldest first.
	WeeklyHeaviestSets(ctx context.Context, userID uuid.UUID, exerciseIDs []uuid.UUID, since time.Time) ([]model.ExerciseProgression, error)
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
		SELECT dl.id, to_char(dl.date, 'YYYY-MM-DD'), dl.type_id, dl.subtype_id,
		       dl.template_id, wt.name AS template_name,
		       dl.session_notes, dl.logged_at, dl.import_id
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
			&s.SessionNotes, &s.LoggedAt, &s.ImportID,
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
		SELECT id, user_id, to_char(date, 'YYYY-MM-DD'),
		       type_id, subtype_id, template_id, session_notes, logged_at, import_id
		FROM day_logs
		WHERE user_id = $1 AND date = $2`,
		userID, date,
	).Scan(
		&dl.ID, &dl.UserID, &dl.Date, &dl.TypeID, &dl.SubtypeID,
		&dl.TemplateID, &dl.SessionNotes, &dl.LoggedAt, &dl.ImportID,
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
		tmpl, err := r.getTemplate(ctx, dl.UserID, *dl.TemplateID)
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
		SELECT id, day_log_id, exercise_id, exercise_name, exercise_category,
		       exercise_modality, set_index, target_reps, target_weight,
		       actual_reps, actual_weight, duration_seconds, completed
		FROM set_logs
		WHERE day_log_id = $1
		ORDER BY logged_at, exercise_id, set_index`,
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
			&s.ID, &s.DayLogID, &s.ExerciseID, &s.ExerciseName, &s.ExerciseCategory,
			&s.ExerciseModality, &s.SetIndex, &s.TargetReps, &s.TargetWeight,
			&s.ActualReps, &s.ActualWeight, &s.DurationSeconds, &s.Completed,
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

// insertOverrides writes only exercise references owned by the day-log user.
// A templated log also requires the exercise to belong to that template.
func insertOverrides(ctx context.Context, tx pgx.Tx, dayLogID uuid.UUID, overrides []model.ExerciseOverride) error {
	if len(overrides) == 0 {
		return nil
	}
	// One round trip for the whole day instead of one per exercise.
	batch := &pgx.Batch{}
	for i := range overrides {
		o := &overrides[i]
		o.DayLogID = dayLogID
		batch.Queue(`
			INSERT INTO exercise_overrides (
				day_log_id, exercise_id, actual_sets, actual_reps,
				actual_weight, notes, skipped
			)
			SELECT $1, e.id, $3, $4, $5, $6, $7
			FROM exercises e
			JOIN workout_templates wt ON wt.id = e.template_id
			JOIN day_logs dl ON dl.id = $1
				AND dl.user_id = wt.user_id
				AND (dl.template_id IS NULL OR dl.template_id = e.template_id)
			WHERE e.id = $2
			RETURNING id`,
			o.DayLogID, o.ExerciseID, o.ActualSets, o.ActualReps,
			o.ActualWeight, o.Notes, o.Skipped)
	}
	results := tx.SendBatch(ctx, batch)
	for i := range overrides {
		err := results.QueryRow().Scan(&overrides[i].ID)
		if err != nil {
			_ = results.Close()
			if errors.Is(err, pgx.ErrNoRows) {
				return &model.ValidationError{Message: "exercise does not belong to this workout", Field: "overrides"}
			}
			return fmt.Errorf("inserting override: %w", err)
		}
	}
	if err := results.Close(); err != nil {
		return fmt.Errorf("inserting overrides: %w", err)
	}
	return nil
}

// insertSetLogs writes the day's sets within the caller's transaction. Callers
// delete existing rows first (replace semantics), mirroring overrides. The
// immutable exercise snapshot is derived server-side from the owned exercise;
// released clients do not send those training-domain columns. The rows go in
// one round trip: a finished workout is a few dozen sets.
func insertSetLogs(ctx context.Context, tx pgx.Tx, dayLogID uuid.UUID, setLogs []model.SetLog) error {
	if len(setLogs) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for i := range setLogs {
		s := &setLogs[i]
		s.DayLogID = dayLogID
		batch.Queue(`
			INSERT INTO set_logs (
				day_log_id, exercise_id, is_extra,
				exercise_name, exercise_category, exercise_modality,
				set_index, target_reps, target_weight, actual_reps,
				actual_weight, duration_seconds, completed
			)
			SELECT
				$1, e.id, true,
				e.name, wt.type_id,
				CASE WHEN e.duration_minutes IS NULL THEN 'strength' ELSE 'cardio' END,
				$3, $4, $5, $6, $7, $8, $9
			FROM exercises e
			JOIN workout_templates wt ON wt.id = e.template_id
			JOIN day_logs dl ON dl.id = $1
				AND dl.user_id = wt.user_id
				AND (dl.template_id IS NULL OR dl.template_id = e.template_id)
			WHERE e.id = $2
			RETURNING id`,
			s.DayLogID, s.ExerciseID, s.SetIndex,
			s.TargetReps, s.TargetWeight, s.ActualReps, s.ActualWeight,
			s.DurationSeconds, s.Completed)
	}
	results := tx.SendBatch(ctx, batch)
	for i := range setLogs {
		err := results.QueryRow().Scan(&setLogs[i].ID)
		if err != nil {
			_ = results.Close()
			if errors.Is(err, pgx.ErrNoRows) {
				return &model.ValidationError{Message: "exercise does not belong to this workout", Field: "set_logs"}
			}
			return fmt.Errorf("inserting set log: %w", err)
		}
	}
	if err := results.Close(); err != nil {
		return fmt.Errorf("inserting set logs: %w", err)
	}
	return nil
}

// RecordSets returns every completed weighted set (weight + reps) for the
// requested exercises, for the user — the raw data the service reduces into
// per-exercise records.
func (r *logDAO) RecordSets(ctx context.Context, userID uuid.UUID, exerciseIDs []uuid.UUID) ([]model.SetPerf, error) {
	if len(exerciseIDs) == 0 {
		return []model.SetPerf{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT exercise_id, actual_weight, actual_reps, to_char(date, 'YYYY-MM-DD') FROM (
		  SELECT sl.exercise_id, sl.actual_weight, sl.actual_reps, dl.date FROM set_logs sl JOIN day_logs dl ON dl.id=sl.day_log_id WHERE dl.user_id=$1 AND sl.completed=true
		  UNION ALL
		  SELECT sl.exercise_id, sl.actual_weight, sl.actual_reps, ws.date FROM set_logs sl JOIN workout_sessions ws ON ws.id=sl.workout_session_id WHERE ws.user_id=$1 AND ws.status <> 'discarded' AND sl.completed=true
		) performed
		WHERE exercise_id = ANY($2) AND actual_weight IS NOT NULL AND actual_reps IS NOT NULL AND actual_reps > 0`,
		userID, exerciseIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("querying record sets: %w", err)
	}
	defer rows.Close()

	perfs := []model.SetPerf{}
	for rows.Next() {
		var p model.SetPerf
		if err := rows.Scan(&p.ExerciseID, &p.Weight, &p.Reps, &p.Date); err != nil {
			return nil, fmt.Errorf("scanning record set: %w", err)
		}
		perfs = append(perfs, p)
	}
	return perfs, rows.Err()
}

// WeeklyHeaviestSets reads the same performed sets as RecordSets, limited to
// dates on or after since, and keeps one set per exercise and week: the
// heaviest, then the one with more reps, then the earlier date. Weeks are
// keyed by their Monday, the bucketing GetWeeklyVolume uses. Rows arrive
// grouped by exercise with weeks oldest first.
func (r *logDAO) WeeklyHeaviestSets(ctx context.Context, userID uuid.UUID, exerciseIDs []uuid.UUID, since time.Time) ([]model.ExerciseProgression, error) {
	if len(exerciseIDs) == 0 {
		return []model.ExerciseProgression{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT ON (exercise_id, week_start)
		       exercise_id, to_char(week_start, 'YYYY-MM-DD'), actual_weight, actual_reps, to_char(date, 'YYYY-MM-DD')
		FROM (
		  SELECT exercise_id, actual_weight, actual_reps, date, date_trunc('week', date) AS week_start FROM (
		    SELECT sl.exercise_id, sl.actual_weight, sl.actual_reps, dl.date FROM set_logs sl JOIN day_logs dl ON dl.id=sl.day_log_id
		    WHERE dl.user_id=$1 AND dl.date >= $3 AND sl.completed=true
		    UNION ALL
		    SELECT sl.exercise_id, sl.actual_weight, sl.actual_reps, ws.date FROM set_logs sl JOIN workout_sessions ws ON ws.id=sl.workout_session_id
		    WHERE ws.user_id=$1 AND ws.date >= $3 AND ws.status <> 'discarded' AND sl.completed=true
		  ) performed
		  WHERE exercise_id = ANY($2) AND actual_weight IS NOT NULL AND actual_reps IS NOT NULL AND actual_reps > 0
		) weighted
		ORDER BY exercise_id, week_start, actual_weight DESC, actual_reps DESC, date ASC`,
		userID, exerciseIDs, since,
	)
	if err != nil {
		return nil, fmt.Errorf("querying weekly heaviest sets: %w", err)
	}
	defer rows.Close()

	progressions := []model.ExerciseProgression{}
	for rows.Next() {
		var (
			exerciseID uuid.UUID
			week       model.ProgressionWeek
		)
		if err := rows.Scan(&exerciseID, &week.WeekStart, &week.MaxWeight, &week.MaxWeightReps, &week.Date); err != nil {
			return nil, fmt.Errorf("scanning weekly heaviest set: %w", err)
		}
		if n := len(progressions); n == 0 || progressions[n-1].ExerciseID != exerciseID {
			progressions = append(progressions, model.ExerciseProgression{ExerciseID: exerciseID})
		}
		last := &progressions[len(progressions)-1]
		last.Weeks = append(last.Weeks, week)
	}
	return progressions, rows.Err()
}

// ExerciseHistory returns, per requested exercise, the completed sets from the
// most recent day they were performed — the "last time you did X" data.
func (r *logDAO) ExerciseHistory(ctx context.Context, userID uuid.UUID, exerciseIDs []uuid.UUID) ([]model.ExerciseHistory, error) {
	if len(exerciseIDs) == 0 {
		return []model.ExerciseHistory{}, nil
	}

	rows, err := r.pool.Query(ctx, `
		WITH performed AS (
			SELECT sl.exercise_id, dl.date, sl.set_index, sl.target_reps, sl.target_weight, sl.actual_reps, sl.actual_weight, sl.duration_seconds, sl.completed FROM set_logs sl JOIN day_logs dl ON dl.id=sl.day_log_id WHERE dl.user_id=$1
			UNION ALL
			SELECT sl.exercise_id, ws.date, sl.set_index, sl.target_reps, sl.target_weight, sl.actual_reps, sl.actual_weight, sl.duration_seconds, sl.completed FROM set_logs sl JOIN workout_sessions ws ON ws.id=sl.workout_session_id WHERE ws.user_id=$1 AND ws.status <> 'discarded'
		), ranked AS (
			SELECT exercise_id, to_char(date, 'YYYY-MM-DD') AS date, set_index,
			       sl.target_reps, sl.target_weight, sl.actual_reps, sl.actual_weight,
			       sl.duration_seconds, sl.completed,
			       DENSE_RANK() OVER (PARTITION BY exercise_id ORDER BY date DESC) AS rnk
			FROM performed sl
			WHERE exercise_id = ANY($2) AND completed = true
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
		s.ExerciseID = &exID
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

// getTemplate loads the day log's template only when the log's owner also
// owns it, so a template id never exposes another user's template.
func (r *logDAO) getTemplate(ctx context.Context, userID, templateID uuid.UUID) (*model.WorkoutTemplate, error) {
	t := &model.WorkoutTemplate{}
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, type_id, subtype_id, warmup_routine_id, cooldown_routine_id, created_at, updated_at
		FROM workout_templates
		WHERE id = $1 AND user_id = $2`,
		templateID, userID,
	).Scan(&t.ID, &t.Name, &t.TypeID, &t.SubtypeID, &t.WarmupRoutineID, &t.CooldownRoutineID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil //nolint:nilnil
		}
		return nil, fmt.Errorf("querying template for log: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, template_id, name, sort_order, sets, reps, weight, rest_seconds, duration_seconds, notes
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
			&e.Sets, &e.Reps, &e.Weight, &e.RestSeconds, &e.DurationSeconds, &e.Notes,
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

	if err := insertDayLog(ctx, tx, userID, dl); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const (
	dayLogLockNamespace = "day-log:"
	dayLogCreateScope   = "logs/create"
)

func (r *logDAO) CreateOnce(ctx context.Context, userID uuid.UUID, dl *model.DayLog, operationKey, requestHash string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Serialize this user's keyed creates so two concurrent deliveries of one
	// offline finish cannot both miss the record and insert twice.
	if err := lockUserDomain(ctx, tx, dayLogLockNamespace, userID); err != nil {
		return false, err
	}
	var replay model.DayLog
	if found, err := findIdempotency(ctx, tx, userID, dayLogCreateScope, operationKey, requestHash, &replay); err != nil {
		return false, err
	} else if found {
		*dl = replay
		dl.UserID = userID
		return true, nil
	}

	if err := insertDayLog(ctx, tx, userID, dl); err != nil {
		return false, err
	}
	record := model.IdempotencyRecord{
		Scope:          dayLogCreateScope,
		OperationKey:   operationKey,
		RequestHash:    requestHash,
		ResponseStatus: 201,
		ResourceType:   "day_log",
		ResourceID:     &dl.ID,
	}
	if err := insertIdempotency(ctx, tx, userID, record, dl); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("committing day log create: %w", err)
	}
	return false, nil
}

func insertDayLog(ctx context.Context, tx pgx.Tx, userID uuid.UUID, dl *model.DayLog) error {
	err := tx.QueryRow(ctx, `
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

	if err := insertOverrides(ctx, tx, dl.ID, dl.Overrides); err != nil {
		return err
	}

	return insertSetLogs(ctx, tx, dl.ID, dl.SetLogs)
}

func (r *logDAO) Update(ctx context.Context, userID uuid.UUID, date string, update model.DayLogUpdate) error {
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

	if update.Replacement != nil {
		_, err = tx.Exec(ctx, `
			UPDATE day_logs
			SET type_id = $1, subtype_id = $2, template_id = $3
			WHERE id = $4`,
			update.Replacement.TypeID, update.Replacement.SubtypeID, update.Replacement.TemplateID, logID,
		)
	}
	if err == nil && update.ReplaceNotes {
		_, err = tx.Exec(ctx, `
			UPDATE day_logs SET session_notes = $1 WHERE id = $2`,
			update.SessionNotes, logID,
		)
	}
	if err != nil {
		return fmt.Errorf("updating day log: %w", err)
	}

	if update.ReplaceOverrides {
		_, err = tx.Exec(ctx, `DELETE FROM exercise_overrides WHERE day_log_id = $1`, logID)
		if err != nil {
			return fmt.Errorf("deleting overrides: %w", err)
		}
	}

	if err := insertOverrides(ctx, tx, logID, update.Overrides); err != nil {
		return err
	}

	if update.ReplaceSetLogs {
		if _, err = tx.Exec(ctx, `DELETE FROM set_logs WHERE day_log_id = $1`, logID); err != nil {
			return fmt.Errorf("deleting set logs: %w", err)
		}
		if err := insertSetLogs(ctx, tx, logID, update.SetLogs); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *logDAO) Delete(ctx context.Context, userID uuid.UUID, date string) error {
	// Set logs and overrides cascade; the Apple Health export record has no
	// foreign key, so the same statement removes it with the log.
	result, err := r.pool.Exec(ctx, `
		WITH deleted AS (
			DELETE FROM day_logs WHERE user_id = $1 AND date = $2 RETURNING id
		), exports AS (
			DELETE FROM health_exports he USING deleted
			WHERE he.user_id = $1 AND he.workout_kind = 'day_log' AND he.workout_id = deleted.id
		)
		SELECT 1 FROM deleted`,
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
