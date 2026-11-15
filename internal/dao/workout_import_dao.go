package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gym-pulse/gym-pulse-api/internal/model"
)

const importLockNamespace = "import:"

const workoutImportColumns = `id, source, source_weight_unit, weight_unit, unmatched,
		       to_char(first_date, 'YYYY-MM-DD'), to_char(last_date, 'YYYY-MM-DD'),
		       workout_count, day_count, set_count, created_at`

// WorkoutImportDAO persists workout-history imports: the import row and the
// day logs and set logs it creates.
type WorkoutImportDAO interface {
	// Context reads what an import resolves against: the weight unit,
	// timezone, saved-workout exercises, and catalog.
	Context(ctx context.Context, userID uuid.UUID) (*model.ImportContext, error)
	// ExistingLogDates returns the dates in [from, to] that hold any day log.
	ExistingLogDates(ctx context.Context, userID uuid.UUID, from, to string) (map[string]bool, error)
	// Commit writes the import and its days in one transaction unless the
	// athlete already used operationKey. The same key and hash replays the
	// stored import; a different hash is an idempotency conflict. A day that
	// gained a log since the preview fails the whole commit with a conflict.
	Commit(ctx context.Context, userID uuid.UUID, imp *model.WorkoutImport, days []model.ImportDay, operationKey, requestHash string) (*model.WorkoutImport, model.ImportCommitOutcome, error)
	// Replay returns the import stored under operationKey when its hash
	// matches, or nil when the key is unused; a different hash is an
	// idempotency conflict. Commit repeats the check under its lock.
	Replay(ctx context.Context, userID uuid.UUID, operationKey, requestHash string) (*model.WorkoutImport, error)
	List(ctx context.Context, userID uuid.UUID) ([]model.WorkoutImport, error)
	// Delete removes an owned import and, by cascade, every day log it made.
	Delete(ctx context.Context, userID, importID uuid.UUID) error
}

type workoutImportDAO struct{ pool *pgxpool.Pool }

// NewWorkoutImportDAO returns a WorkoutImportDAO backed by pool.
func NewWorkoutImportDAO(pool *pgxpool.Pool) WorkoutImportDAO {
	return &workoutImportDAO{pool: pool}
}

func (r *workoutImportDAO) Context(ctx context.Context, userID uuid.UUID) (*model.ImportContext, error) {
	out := &model.ImportContext{WeightUnit: model.DefaultUserSettings().WeightUnit, Timezone: "UTC", SavedExercises: map[uuid.UUID]string{}}
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE((SELECT weight_unit FROM user_settings WHERE user_id=$1), $2),
		       COALESCE((SELECT timezone FROM training_profiles WHERE user_id=$1), 'UTC')`,
		userID, out.WeightUnit).Scan(&out.WeightUnit, &out.Timezone)
	if err != nil {
		return nil, fmt.Errorf("querying import settings: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT e.id, e.name FROM exercises e
		JOIN workout_templates wt ON wt.id = e.template_id
		WHERE wt.user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying saved exercises: %w", err)
	}
	for rows.Next() {
		var id uuid.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scanning saved exercise: %w", err)
		}
		out.SavedExercises[id] = name
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading saved exercises: %w", err)
	}
	rows, err = r.pool.Query(ctx, `SELECT id, name, category, modality FROM exercise_catalog`)
	if err != nil {
		return nil, fmt.Errorf("querying exercise catalog: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item model.CatalogExercise
		if err := rows.Scan(&item.ID, &item.Name, &item.Category, &item.Modality); err != nil {
			return nil, fmt.Errorf("scanning catalog exercise: %w", err)
		}
		out.Catalog = append(out.Catalog, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading exercise catalog: %w", err)
	}
	return out, nil
}

func (r *workoutImportDAO) ExistingLogDates(ctx context.Context, userID uuid.UUID, from, to string) (map[string]bool, error) {
	return existingLogDates(ctx, r.pool, userID, from, to)
}

func existingLogDates(ctx context.Context, q queryer, userID uuid.UUID, from, to string) (map[string]bool, error) {
	rows, err := q.Query(ctx, `
		SELECT DISTINCT to_char(date, 'YYYY-MM-DD') FROM day_logs
		WHERE user_id = $1 AND date BETWEEN $2 AND $3`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying existing log dates: %w", err)
	}
	defer rows.Close()
	dates := map[string]bool{}
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err != nil {
			return nil, fmt.Errorf("scanning existing log date: %w", err)
		}
		dates[date] = true
	}
	return dates, rows.Err()
}

func (r *workoutImportDAO) Commit(ctx context.Context, userID uuid.UUID, imp *model.WorkoutImport, days []model.ImportDay, operationKey, requestHash string) (*model.WorkoutImport, model.ImportCommitOutcome, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, model.ImportCreated, fmt.Errorf("beginning workout import: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := lockUserDomain(ctx, tx, importLockNamespace, userID); err != nil {
		return nil, model.ImportCreated, err
	}
	if replay, err := findImportReplay(ctx, tx, userID, operationKey, requestHash); err != nil || replay != nil {
		return replay, model.ImportReplayed, err
	}

	if len(days) > 0 {
		existing, err := existingLogDates(ctx, tx, userID, days[0].Date, days[len(days)-1].Date)
		if err != nil {
			return nil, model.ImportCreated, err
		}
		for _, day := range days {
			if existing[day.Date] {
				return nil, model.ImportCreated, &model.ConflictError{Message: "a workout was logged on an imported day since the preview"}
			}
		}
	}

	imp.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)
	body, err := json.Marshal(imp)
	if err != nil {
		return nil, model.ImportCreated, fmt.Errorf("encoding import: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO workout_imports (
			id, user_id, source, source_weight_unit, weight_unit, unmatched, first_date, last_date,
			workout_count, day_count, set_count, operation_key, request_hash, response_body, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		imp.ID, userID, imp.Source, imp.SourceWeightUnit, imp.WeightUnit, imp.Unmatched, imp.FirstDate, imp.LastDate,
		imp.WorkoutCount, imp.DayCount, imp.SetCount, operationKey, requestHash, body, imp.CreatedAt); err != nil {
		return nil, model.ImportCreated, fmt.Errorf("inserting workout import: %w", err)
	}

	if err := insertImportedDays(ctx, tx, userID, imp.ID, days); err != nil {
		return nil, model.ImportCreated, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, model.ImportCreated, fmt.Errorf("committing workout import: %w", err)
	}
	return imp, model.ImportCreated, nil
}

func (r *workoutImportDAO) Replay(ctx context.Context, userID uuid.UUID, operationKey, requestHash string) (*model.WorkoutImport, error) {
	return findImportReplay(ctx, r.pool, userID, operationKey, requestHash)
}

type rowQueryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func findImportReplay(ctx context.Context, q rowQueryer, userID uuid.UUID, operationKey, requestHash string) (*model.WorkoutImport, error) {
	var storedHash string
	var body []byte
	err := q.QueryRow(ctx, `SELECT request_hash, response_body FROM workout_imports WHERE user_id=$1 AND operation_key=$2`,
		userID, operationKey).Scan(&storedHash, &body)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // an unused key has nothing to replay
	}
	if err != nil {
		return nil, fmt.Errorf("checking import operation key: %w", err)
	}
	if storedHash != requestHash {
		return nil, &model.ConflictError{Message: "idempotency key was already used with a different payload"}
	}
	var replay model.WorkoutImport
	if err := json.Unmarshal(body, &replay); err != nil {
		return nil, fmt.Errorf("decoding stored import: %w", err)
	}
	return &replay, nil
}

func (r *workoutImportDAO) List(ctx context.Context, userID uuid.UUID) ([]model.WorkoutImport, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+workoutImportColumns+`
		FROM workout_imports WHERE user_id = $1
		ORDER BY created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying workout imports: %w", err)
	}
	defer rows.Close()
	imports := []model.WorkoutImport{}
	for rows.Next() {
		var imp model.WorkoutImport
		if err := rows.Scan(&imp.ID, &imp.Source, &imp.SourceWeightUnit, &imp.WeightUnit, &imp.Unmatched,
			&imp.FirstDate, &imp.LastDate, &imp.WorkoutCount, &imp.DayCount, &imp.SetCount, &imp.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning workout import: %w", err)
		}
		imports = append(imports, imp)
	}
	return imports, rows.Err()
}

func (r *workoutImportDAO) Delete(ctx context.Context, userID, importID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM workout_imports WHERE id = $1 AND user_id = $2`, importID, userID)
	if err != nil {
		return fmt.Errorf("deleting workout import: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &model.NotFoundError{Message: "import not found"}
	}
	return nil
}

// importInsertChunk bounds the rows sent in one statement. COPY is not
// allowed under row-level security for the runtime role, so rows go in as
// arrays expanded by unnest.
const importInsertChunk = 2000

func insertImportedDays(ctx context.Context, tx pgx.Tx, userID, importID uuid.UUID, days []model.ImportDay) error {
	type setRow struct {
		logID uuid.UUID
		set   model.ImportSet
	}
	var sets []setRow
	for start := 0; start < len(days); start += importInsertChunk {
		chunk := days[start:min(start+importInsertChunk, len(days))]
		ids := make([]uuid.UUID, len(chunk))
		dates := make([]string, len(chunk))
		notes := make([]*string, len(chunk))
		loggedAt := make([]time.Time, len(chunk))
		for i, day := range chunk {
			ids[i], dates[i], notes[i], loggedAt[i] = uuid.New(), day.Date, day.SessionNotes, day.LoggedAt
			for _, set := range day.Sets {
				sets = append(sets, setRow{logID: ids[i], set: set})
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO day_logs (id, user_id, date, type_id, subtype_id, session_notes, logged_at, import_id)
			SELECT id, $1, date, 'other', 'general', notes, logged_at, $2
			FROM unnest($3::uuid[], $4::date[], $5::text[], $6::timestamptz[]) AS d(id, date, notes, logged_at)`,
			userID, importID, ids, dates, notes, loggedAt); err != nil {
			return fmt.Errorf("inserting imported day logs: %w", err)
		}
	}
	for start := 0; start < len(sets); start += importInsertChunk {
		chunk := sets[start:min(start+importInsertChunk, len(sets))]
		n := len(chunk)
		logIDs := make([]uuid.UUID, n)
		exerciseIDs := make([]*string, n)
		names, categories, modalities := make([]string, n), make([]string, n), make([]string, n)
		indexes := make([]int32, n)
		reps, seconds := make([]*int32, n), make([]*int32, n)
		weights := make([]*float64, n)
		loggedAt := make([]time.Time, n)
		for i, row := range chunk {
			set := row.set
			logIDs[i] = row.logID
			if set.ExerciseID != nil {
				id := set.ExerciseID.String()
				exerciseIDs[i] = &id
			}
			names[i], categories[i], modalities[i] = set.ExerciseName, set.ExerciseCategory, set.ExerciseModality
			indexes[i], reps[i], seconds[i] = int32(set.SetIndex), optionalInt32(set.ActualReps), optionalInt32(set.DurationSeconds)
			weights[i], loggedAt[i] = set.ActualWeight, set.LoggedAt
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO set_logs (
				day_log_id, exercise_id, is_extra, exercise_name, exercise_category, exercise_modality,
				set_index, actual_reps, actual_weight, duration_seconds, completed, logged_at)
			SELECT log_id, exercise_id, true, name, category, modality, set_index, reps, weight, seconds, true, logged_at
			FROM unnest($1::uuid[], $2::uuid[], $3::text[], $4::text[], $5::text[], $6::int4[], $7::int4[],
			            $8::numeric[], $9::int4[], $10::timestamptz[])
			     AS s(log_id, exercise_id, name, category, modality, set_index, reps, weight, seconds, logged_at)`,
			logIDs, exerciseIDs, names, categories, modalities, indexes, reps, weights, seconds, loggedAt); err != nil {
			return fmt.Errorf("inserting imported set logs: %w", err)
		}
	}
	return nil
}

func optionalInt32(value *int) *int32 {
	if value == nil {
		return nil
	}
	v := int32(*value)
	return &v
}
