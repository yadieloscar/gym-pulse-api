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

// TrainingMutationDAO owns transactions that cross existing training
// aggregates. It keeps replay state and the observable mutation in one commit.
type TrainingMutationDAO interface {
	AdoptLegacy(ctx context.Context, userID uuid.UUID, now time.Time, record model.IdempotencyRecord) (*model.AdoptLegacyProgramResponse, bool, error)
	PutTrainingProfile(ctx context.Context, userID uuid.UUID, profile *model.TrainingProfile, expectedRevision int64, record model.IdempotencyRecord) (*model.TrainingProfile, bool, error)
	CreateProgram(ctx context.Context, userID uuid.UUID, program *model.Program, record model.IdempotencyRecord) (*model.Program, bool, error)
	ReplaceProgram(ctx context.Context, userID uuid.UUID, program *model.Program, expectedRevision int64, record model.IdempotencyRecord) (*model.Program, bool, error)
	ReplaceScheduledWorkout(ctx context.Context, userID uuid.UUID, workout *model.ScheduledWorkout, expectedRevision int64, record model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error)
	UpdateScheduledSetTarget(ctx context.Context, userID, workoutID, setID uuid.UUID, target model.PatchScheduledSetTargetRequest, record model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error)
	PutRequiredSet(ctx context.Context, userID, workoutID, setID uuid.UUID, request model.SetMutationRequest, record model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error)
	AddExtraSet(ctx context.Context, userID, workoutID uuid.UUID, request model.ExtraSetRequest, record model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error)
	FinalizeScheduledWorkout(ctx context.Context, userID, workoutID uuid.UUID, expectedRevision int64, now time.Time, record model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error)
	CreateWorkoutSession(ctx context.Context, userID uuid.UUID, session *model.WorkoutSession, record model.IdempotencyRecord) (*model.WorkoutSession, bool, error)
	ReplaceWorkoutSession(ctx context.Context, userID uuid.UUID, session *model.WorkoutSession, expectedRevision int64, now time.Time, record model.IdempotencyRecord) (*model.WorkoutSession, bool, error)
}

type trainingMutationDAO struct {
	pool *pgxpool.Pool
}

func NewTrainingMutationDAO(pool *pgxpool.Pool) TrainingMutationDAO {
	return &trainingMutationDAO{pool: pool}
}

func (r *trainingMutationDAO) PutTrainingProfile(ctx context.Context, userID uuid.UUID, profile *model.TrainingProfile, expectedRevision int64, record model.IdempotencyRecord) (*model.TrainingProfile, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("beginning training profile mutation: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, programLockNamespace, userID); err != nil {
		return nil, false, err
	}
	var replay model.TrainingProfile
	if found, err := findIdempotency(ctx, tx, userID, record.Scope, record.OperationKey, record.RequestHash, &replay); err != nil {
		return nil, false, err
	} else if found {
		return &replay, true, nil
	}

	if expectedRevision == 0 {
		err = tx.QueryRow(ctx, `
			INSERT INTO training_profiles (
				user_id, primary_goal, available_days, usual_activity, experience,
				equipment, session_duration_minutes, timezone, preferences)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			RETURNING revision, created_at, updated_at`,
			userID, profile.PrimaryGoal, profile.AvailableDays, profile.UsualActivity,
			profile.Experience, profile.Equipment, profile.SessionDurationMinutes,
			profile.Timezone, profile.Preferences,
		).Scan(&profile.Revision, &profile.CreatedAt, &profile.UpdatedAt)
	} else {
		err = tx.QueryRow(ctx, `
			UPDATE training_profiles
			SET primary_goal=$3, available_days=$4, usual_activity=$5, experience=$6,
			    equipment=$7, session_duration_minutes=$8, timezone=$9,
			    preferences=$10, revision=revision+1, updated_at=now()
			WHERE user_id=$1 AND revision=$2
			RETURNING revision, created_at, updated_at`,
			userID, expectedRevision, profile.PrimaryGoal, profile.AvailableDays,
			profile.UsualActivity, profile.Experience, profile.Equipment,
			profile.SessionDurationMinutes, profile.Timezone, profile.Preferences,
		).Scan(&profile.Revision, &profile.CreatedAt, &profile.UpdatedAt)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		current, getErr := loadTrainingProfile(ctx, tx, userID)
		if getErr != nil {
			return nil, false, getErr
		}
		return nil, false, &model.ConflictError{Message: "training profile revision conflict", Expected: expectedRevision, Actual: current.Revision, Authoritative: current}
	}
	if err != nil {
		return nil, false, fmt.Errorf("persisting training profile mutation: %w", err)
	}
	record.ResourceRevision = &profile.Revision
	if err := insertIdempotency(ctx, tx, userID, record, profile); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing training profile mutation: %w", err)
	}
	return profile, false, nil
}

func (r *trainingMutationDAO) CreateProgram(ctx context.Context, userID uuid.UUID, program *model.Program, record model.IdempotencyRecord) (*model.Program, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("beginning custom program create: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, programLockNamespace, userID); err != nil {
		return nil, false, err
	}
	var replay model.Program
	if found, err := findIdempotency(ctx, tx, userID, record.Scope, record.OperationKey, record.RequestHash, &replay); err != nil {
		return nil, false, err
	} else if found {
		return &replay, true, nil
	}
	if program.Active {
		if _, err := tx.Exec(ctx, `UPDATE programs SET active=false, revision=revision+1, updated_at=now() WHERE user_id=$1 AND active=true`, userID); err != nil {
			return nil, false, fmt.Errorf("deactivating prior program: %w", err)
		}
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO programs (user_id, starter_program_id, starter_version, name, primary_goal, roadmap, active)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, revision, created_at, updated_at`, userID, program.StarterProgramID,
		program.StarterVersion, program.Name, program.PrimaryGoal, program.Roadmap, program.Active,
	).Scan(&program.ID, &program.Revision, &program.CreatedAt, &program.UpdatedAt)
	if err != nil {
		return nil, false, fmt.Errorf("inserting custom program: %w", err)
	}
	if err := insertProgramWorkouts(ctx, tx, program.ID, program.Workouts); err != nil {
		return nil, false, err
	}
	record.ResourceID = &program.ID
	record.ResourceRevision = &program.Revision
	if err := insertIdempotency(ctx, tx, userID, record, program); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing custom program create: %w", err)
	}
	return program, false, nil
}

func (r *trainingMutationDAO) ReplaceProgram(ctx context.Context, userID uuid.UUID, program *model.Program, expectedRevision int64, record model.IdempotencyRecord) (*model.Program, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("beginning custom program update: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, programLockNamespace, userID); err != nil {
		return nil, false, err
	}
	var replay model.Program
	if found, err := findIdempotency(ctx, tx, userID, record.Scope, record.OperationKey, record.RequestHash, &replay); err != nil {
		return nil, false, err
	} else if found {
		return &replay, true, nil
	}
	if program.Active {
		if _, err := tx.Exec(ctx, `UPDATE programs SET active=false, revision=revision+1, updated_at=now() WHERE user_id=$1 AND active=true AND id<>$2`, userID, program.ID); err != nil {
			return nil, false, fmt.Errorf("deactivating prior program: %w", err)
		}
	}
	err = tx.QueryRow(ctx, `
		UPDATE programs SET name=$3, primary_goal=$4, roadmap=$5, active=$6,
		       revision=revision+1, updated_at=now()
		WHERE id=$1 AND user_id=$2 AND revision=$7
		RETURNING revision, created_at, updated_at`, program.ID, userID, program.Name,
		program.PrimaryGoal, program.Roadmap, program.Active, expectedRevision,
	).Scan(&program.Revision, &program.CreatedAt, &program.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		current, getErr := loadProgram(ctx, tx, userID, program.ID)
		if getErr != nil {
			return nil, false, getErr
		}
		return nil, false, &model.ConflictError{Message: "program revision conflict", Expected: expectedRevision, Actual: current.Revision, Authoritative: current}
	}
	if err != nil {
		return nil, false, fmt.Errorf("updating custom program: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM program_workouts WHERE program_id=$1`, program.ID); err != nil {
		return nil, false, fmt.Errorf("replacing custom program workouts: %w", err)
	}
	if err := insertProgramWorkouts(ctx, tx, program.ID, program.Workouts); err != nil {
		return nil, false, err
	}
	record.ResourceID = &program.ID
	record.ResourceRevision = &program.Revision
	if err := insertIdempotency(ctx, tx, userID, record, program); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing custom program update: %w", err)
	}
	return program, false, nil
}

func loadTrainingProfile(ctx context.Context, tx pgx.Tx, userID uuid.UUID) (*model.TrainingProfile, error) {
	profile := &model.TrainingProfile{}
	err := tx.QueryRow(ctx, `
		SELECT primary_goal, available_days, usual_activity, experience,
		       equipment, session_duration_minutes, timezone, preferences,
		       revision, created_at, updated_at
		FROM training_profiles WHERE user_id=$1`, userID).Scan(
		&profile.PrimaryGoal, &profile.AvailableDays, &profile.UsualActivity,
		&profile.Experience, &profile.Equipment, &profile.SessionDurationMinutes,
		&profile.Timezone, &profile.Preferences, &profile.Revision,
		&profile.CreatedAt, &profile.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "training profile not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("loading training profile: %w", err)
	}
	return profile, nil
}

func loadProgram(ctx context.Context, tx pgx.Tx, userID, programID uuid.UUID) (*model.Program, error) {
	program := &model.Program{}
	err := tx.QueryRow(ctx, `
		SELECT id, starter_program_id, starter_version, name, primary_goal,
		       roadmap, active, revision, created_at, updated_at
		FROM programs WHERE id=$1 AND user_id=$2`, programID, userID).Scan(
		&program.ID, &program.StarterProgramID, &program.StarterVersion, &program.Name,
		&program.PrimaryGoal, &program.Roadmap, &program.Active, &program.Revision,
		&program.CreatedAt, &program.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "program not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("loading program: %w", err)
	}
	workouts, err := loadProgramWorkouts(ctx, tx, userID, program.ID)
	if err != nil {
		return nil, err
	}
	program.Workouts = workouts
	return program, nil
}

func (r *trainingMutationDAO) AdoptLegacy(ctx context.Context, userID uuid.UUID, now time.Time, record model.IdempotencyRecord) (*model.AdoptLegacyProgramResponse, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("beginning legacy adoption: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockUserDomain(ctx, tx, programLockNamespace, userID); err != nil {
		return nil, false, err
	}
	if err := lockUserDomain(ctx, tx, scheduleLockNamespace, userID); err != nil {
		return nil, false, err
	}

	var replay model.AdoptLegacyProgramResponse
	if found, err := findIdempotency(ctx, tx, userID, record.Scope, record.OperationKey, record.RequestHash, &replay); err != nil {
		return nil, false, err
	} else if found {
		return &replay, true, nil
	}

	var originalOperationKey string
	err = tx.QueryRow(ctx, `
		SELECT operation_key
		FROM legacy_adoptions
		WHERE user_id=$1
		FOR UPDATE`, userID).Scan(&originalOperationKey)
	if err == nil {
		var body []byte
		if err := tx.QueryRow(ctx, `
			SELECT response_body
			FROM idempotency_records
			WHERE user_id=$1 AND scope=$2 AND operation_key=$3`,
			userID, record.Scope, originalOperationKey,
		).Scan(&body); err != nil {
			return nil, false, fmt.Errorf("loading prior legacy adoption: %w", err)
		}
		if err := json.Unmarshal(body, &replay); err != nil {
			return nil, false, fmt.Errorf("decoding prior legacy adoption: %w", err)
		}
		program, err := loadProgram(ctx, tx, userID, replay.Program.ID)
		if err != nil {
			return nil, false, err
		}
		schedule, err := loadAdoptedSchedule(ctx, tx, userID, program.ID, replay.Schedule)
		if err != nil {
			return nil, false, err
		}
		replay.Program = *program
		replay.Schedule = schedule
		replay.Adopted = false
		record.ResourceID = &replay.Program.ID
		record.ResourceRevision = &replay.Program.Revision
		if err := insertIdempotency(ctx, tx, userID, record, &replay); err != nil {
			return nil, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, false, fmt.Errorf("committing legacy adoption replay: %w", err)
		}
		return &replay, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("checking prior legacy adoption: %w", err)
	}

	var primaryGoal, timezone string
	if err := tx.QueryRow(ctx, `
		SELECT primary_goal, timezone
		FROM training_profiles
		WHERE user_id=$1`, userID).Scan(&primaryGoal, &timezone); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, &model.NotFoundError{Message: "training profile not found"}
		}
		return nil, false, fmt.Errorf("loading adoption training profile: %w", err)
	}

	assignments, err := loadLegacyWeeklyAssignments(ctx, tx, userID)
	if err != nil {
		return nil, false, err
	}
	if len(assignments) == 0 {
		return nil, false, &model.NotFoundError{Message: "no adoptable weekly plan"}
	}
	program, weekdays, err := model.ProgramFromLegacyWeeklyPlan(primaryGoal, assignments)
	if err != nil {
		return nil, false, err
	}
	from, to, err := model.NextFutureWeek(now, timezone)
	if err != nil {
		return nil, false, err
	}

	if _, err := tx.Exec(ctx, `UPDATE programs SET active=false, revision=revision+1, updated_at=now() WHERE user_id=$1 AND active=true`, userID); err != nil {
		return nil, false, fmt.Errorf("deactivating prior programs: %w", err)
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO programs (user_id, name, primary_goal, roadmap, active)
		VALUES ($1,$2,$3,$4,true)
		RETURNING id, revision, created_at, updated_at`,
		userID, program.Name, program.PrimaryGoal, program.Roadmap,
	).Scan(&program.ID, &program.Revision, &program.CreatedAt, &program.UpdatedAt)
	if err != nil {
		return nil, false, fmt.Errorf("inserting legacy adoption program: %w", err)
	}
	if err := insertProgramWorkouts(ctx, tx, program.ID, program.Workouts); err != nil {
		return nil, false, err
	}

	schedule, err := model.MaterializeProgramForWeekdays(program, weekdays, from, to)
	if err != nil {
		return nil, false, err
	}
	if err := insertScheduledWorkouts(ctx, tx, userID, schedule); err != nil {
		return nil, false, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO legacy_adoptions (user_id, program_id, operation_key)
		VALUES ($1,$2,$3)`, userID, program.ID, record.OperationKey); err != nil {
		return nil, false, fmt.Errorf("recording legacy adoption: %w", err)
	}
	response := &model.AdoptLegacyProgramResponse{Program: *program, Schedule: schedule, Adopted: true}
	record.ResourceID = &program.ID
	record.ResourceRevision = &program.Revision
	if err := insertIdempotency(ctx, tx, userID, record, response); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing legacy adoption: %w", err)
	}
	return response, false, nil
}

// loadAdoptedSchedule reloads the original adoption week, including replacements
// made by regeneration. Exact operation replays still use their stored body.
func loadAdoptedSchedule(ctx context.Context, tx pgx.Tx, userID, programID uuid.UUID, original []model.ScheduledWorkout) ([]model.ScheduledWorkout, error) {
	result := []model.ScheduledWorkout{}
	if len(original) == 0 {
		return result, nil
	}
	date, err := model.ParseDate(original[0].Date)
	if err != nil {
		return nil, fmt.Errorf("parsing original adoption date: %w", err)
	}
	daysSinceMonday := (int(date.Weekday()) + 6) % 7
	monday := date.AddDate(0, 0, -daysSinceMonday)
	from, to := monday.Format(time.DateOnly), monday.AddDate(0, 0, 6).Format(time.DateOnly)
	rows, err := tx.Query(ctx, `
		SELECT id FROM scheduled_workouts
		WHERE user_id=$1 AND program_id=$2 AND date BETWEEN $3 AND $4
		ORDER BY date, created_at, id`, userID, programID, from, to)
	if err != nil {
		return nil, fmt.Errorf("querying adopted schedule: %w", err)
	}
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scanning adopted schedule identity: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterating adopted schedule: %w", err)
	}
	rows.Close()
	for _, id := range ids {
		workout, err := loadScheduledWorkout(ctx, tx, userID, id)
		if err != nil {
			return nil, err
		}
		result = append(result, *workout)
	}
	return result, nil
}

func loadLegacyWeeklyAssignments(ctx context.Context, tx pgx.Tx, userID uuid.UUID) ([]model.LegacyWeeklyAssignment, error) {
	rows, err := tx.Query(ctx, `
		SELECT wp.weekday, wt.id, wt.name, wt.type_id, wt.subtype_id,
		       wt.created_at, wt.updated_at,
		       e.id, e.template_id, e.catalog_id, e.name, e.sort_order,
		       e.sets, e.reps, e.weight, e.rest_seconds, e.duration_minutes,
		       e.intensity, e.notes
		FROM weekly_plans wp
		JOIN workout_templates wt ON wt.id=wp.template_id AND wt.user_id=wp.user_id
		JOIN exercises e ON e.template_id=wt.id
		WHERE wp.user_id=$1 AND wp.rest=false AND wp.template_id IS NOT NULL
		ORDER BY wp.weekday, e.sort_order, e.id
		FOR SHARE OF wp, wt, e`, userID)
	if err != nil {
		return nil, fmt.Errorf("querying owned legacy weekly plan: %w", err)
	}
	defer rows.Close()

	assignments := []model.LegacyWeeklyAssignment{}
	weekdayIndex := map[int]int{}
	for rows.Next() {
		var weekday int
		var template model.WorkoutTemplate
		var exercise model.Exercise
		if err := rows.Scan(
			&weekday, &template.ID, &template.Name, &template.TypeID, &template.SubtypeID,
			&template.CreatedAt, &template.UpdatedAt,
			&exercise.ID, &exercise.TemplateID, &exercise.CatalogID, &exercise.Name,
			&exercise.SortOrder, &exercise.Sets, &exercise.Reps, &exercise.Weight,
			&exercise.RestSeconds, &exercise.DurationMinutes, &exercise.Intensity, &exercise.Notes,
		); err != nil {
			return nil, fmt.Errorf("scanning owned legacy weekly plan: %w", err)
		}
		index, ok := weekdayIndex[weekday]
		if !ok {
			template.UserID = userID
			template.Exercises = []model.Exercise{}
			assignments = append(assignments, model.LegacyWeeklyAssignment{Weekday: weekday, Template: template})
			index = len(assignments) - 1
			weekdayIndex[weekday] = index
		}
		assignments[index].Template.Exercises = append(assignments[index].Template.Exercises, exercise)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating owned legacy weekly plan: %w", err)
	}
	return assignments, nil
}

func (r *trainingMutationDAO) beginScheduleMutation(ctx context.Context, userID uuid.UUID) (pgx.Tx, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning schedule mutation: %w", err)
	}
	if err := lockUserDomain(ctx, tx, scheduleLockNamespace, userID); err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return nil, errors.Join(err, fmt.Errorf("rolling back schedule mutation: %w", rollbackErr))
		}
		return nil, err
	}
	return tx, nil
}

func loadScheduledWorkout(ctx context.Context, tx pgx.Tx, userID, workoutID uuid.UUID) (*model.ScheduledWorkout, error) {
	workout := &model.ScheduledWorkout{}
	err := tx.QueryRow(ctx, `
		SELECT id, program_id, program_workout_id, to_char(date, 'YYYY-MM-DD'),
		       name, sequence_position, status, finalized_at, revision, created_at, updated_at
		FROM scheduled_workouts WHERE id=$1 AND user_id=$2`, workoutID, userID).Scan(
		&workout.ID, &workout.ProgramID, &workout.ProgramWorkoutID, &workout.Date,
		&workout.Name, &workout.SequencePosition, &workout.Status, &workout.FinalizedAt,
		&workout.Revision, &workout.CreatedAt, &workout.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &model.NotFoundError{Message: "scheduled workout not found"}
	}
	if err != nil {
		return nil, fmt.Errorf("loading scheduled workout: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT ss.id, ss.program_exercise_id, ss.catalog_id, ss.exercise_name,
		       ss.exercise_category, ss.exercise_modality, ss.exercise_order,
		       ss.set_index, ss.target_reps, ss.target_weight,
		       ss.target_duration_seconds, ss.rest_seconds, ss.notes,
		       (sl.id IS NOT NULL) AS checked, sl.id,
		       sl.actual_reps, sl.actual_weight, sl.duration_seconds
		FROM scheduled_sets ss
		JOIN scheduled_workouts sw ON sw.id=ss.scheduled_workout_id AND sw.user_id=$1
		LEFT JOIN LATERAL (
			SELECT candidate.id, candidate.actual_reps, candidate.actual_weight, candidate.duration_seconds
			FROM set_logs candidate
			JOIN workout_sessions ws ON ws.id=candidate.workout_session_id AND ws.user_id=$1
			WHERE ws.scheduled_workout_id=sw.id AND candidate.scheduled_set_id=ss.id AND candidate.completed=true
			ORDER BY candidate.logged_at DESC, candidate.id DESC
			LIMIT 1
		) sl ON true
		WHERE ss.scheduled_workout_id=$2
		ORDER BY ss.exercise_order, ss.set_index`, userID, workout.ID)
	if err != nil {
		return nil, fmt.Errorf("loading scheduled sets: %w", err)
	}
	workout.RequiredSets = []model.ScheduledSet{}
	for rows.Next() {
		var set model.ScheduledSet
		if err := rows.Scan(
			&set.ID, &set.ProgramExerciseID, &set.CatalogID, &set.ExerciseName,
			&set.ExerciseCategory, &set.ExerciseModality, &set.ExerciseOrder,
			&set.SetIndex, &set.TargetReps, &set.TargetWeight,
			&set.TargetDurationSeconds, &set.RestSeconds, &set.Notes, &set.Checked,
			&set.PerformedSetID, &set.ActualReps, &set.ActualWeight,
			&set.ActualDurationSeconds,
		); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scanning scheduled set: %w", err)
		}
		workout.RequiredSets = append(workout.RequiredSets, set)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterating scheduled sets: %w", err)
	}
	rows.Close()

	extraRows, err := tx.Query(ctx, `
		SELECT sl.id, sl.scheduled_set_id, sl.exercise_id, sl.is_extra,
		       sl.exercise_name, sl.exercise_category, sl.exercise_modality,
		       sl.set_index, sl.target_reps, sl.target_weight, sl.actual_reps,
		       sl.actual_weight, sl.duration_seconds, sl.completed,
		       sl.operation_key, sl.revision
		FROM set_logs sl
		JOIN workout_sessions ws ON ws.id=sl.workout_session_id AND ws.user_id=$1
		WHERE ws.scheduled_workout_id=$2 AND sl.is_extra=true
		ORDER BY ws.created_at, sl.logged_at`, userID, workout.ID)
	if err != nil {
		return nil, fmt.Errorf("loading extra sets: %w", err)
	}
	defer extraRows.Close()
	workout.ExtraSets = []model.PerformedSet{}
	for extraRows.Next() {
		var set model.PerformedSet
		if err := scanPerformedSet(extraRows, &set); err != nil {
			return nil, fmt.Errorf("scanning extra set: %w", err)
		}
		workout.ExtraSets = append(workout.ExtraSets, set)
	}
	if err := extraRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating extra sets: %w", err)
	}
	return workout, nil
}

func loadWorkoutSession(ctx context.Context, tx pgx.Tx, userID, sessionID uuid.UUID) (*model.WorkoutSession, error) {
	session := &model.WorkoutSession{}
	if err := scanSession(tx.QueryRow(ctx, `
		SELECT id, scheduled_workout_id, to_char(date, 'YYYY-MM-DD'), name,
		       status, notes, started_at, completed_at, revision, created_at, updated_at
		FROM workout_sessions WHERE id=$1 AND user_id=$2`, sessionID, userID), session); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &model.NotFoundError{Message: "workout session not found"}
		}
		return nil, fmt.Errorf("loading workout session: %w", err)
	}
	sets, err := loadPerformedSets(ctx, tx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	session.Sets = sets
	return session, nil
}

func mutationRevisionConflict(message string, expected int64, authoritative any, actual int64) error {
	return &model.ConflictError{Message: message, Expected: expected, Actual: actual, Authoritative: authoritative}
}

func validateOwnedProgramExerciseProvenance(ctx context.Context, tx pgx.Tx, userID uuid.UUID, sets []model.ScheduledSet) error {
	unique := map[uuid.UUID]struct{}{}
	for _, set := range sets {
		if set.ProgramExerciseID != nil {
			unique[*set.ProgramExerciseID] = struct{}{}
		}
	}
	if len(unique) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	var owned int
	if err := tx.QueryRow(ctx, `
		SELECT count(DISTINCT pe.id)
		FROM program_exercises pe
		JOIN program_workouts pw ON pw.id=pe.program_workout_id
		JOIN programs p ON p.id=pw.program_id AND p.user_id=$2
		WHERE pe.id=ANY($1::uuid[])`, ids, userID).Scan(&owned); err != nil {
		return fmt.Errorf("validating scheduled set provenance: %w", err)
	}
	if owned != len(ids) {
		return &model.NotFoundError{Message: "program exercise not found"}
	}
	return nil
}

func normalizeExtraExerciseProvenance(ctx context.Context, tx pgx.Tx, userID uuid.UUID, exerciseID *uuid.UUID) (uuid.UUID, bool, error) {
	if exerciseID == nil {
		return uuid.Nil, false, nil
	}
	var owned, catalog bool
	if err := tx.QueryRow(ctx, `
		SELECT
			EXISTS (
				SELECT 1 FROM exercises e
				JOIN workout_templates wt ON wt.id=e.template_id AND wt.user_id=$2
				WHERE e.id=$1),
			EXISTS (SELECT 1 FROM exercise_catalog ec WHERE ec.id=$1)`,
		*exerciseID, userID).Scan(&owned, &catalog); err != nil {
		return uuid.Nil, false, fmt.Errorf("validating extra set provenance: %w", err)
	}
	if owned {
		return *exerciseID, true, nil
	}
	if catalog {
		// set_logs.exercise_id references legacy exercises. Catalog identity is
		// intentionally detached while immutable name/category/modality remain.
		return uuid.Nil, false, nil
	}
	return uuid.Nil, false, &model.NotFoundError{Message: "exercise not found"}
}

func (r *trainingMutationDAO) ReplaceScheduledWorkout(ctx context.Context, userID uuid.UUID, workout *model.ScheduledWorkout, expectedRevision int64, record model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error) {
	tx, err := r.beginScheduleMutation(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)
	var replay model.ScheduledWorkout
	if found, err := findIdempotency(ctx, tx, userID, record.Scope, record.OperationKey, record.RequestHash, &replay); err != nil {
		return nil, false, err
	} else if found {
		return &replay, true, nil
	}
	if err := validateOwnedProgramExerciseProvenance(ctx, tx, userID, workout.RequiredSets); err != nil {
		return nil, false, err
	}
	err = tx.QueryRow(ctx, `
		UPDATE scheduled_workouts SET name=$3, status=$4, finalized_at=$5,
		       revision=revision+1, updated_at=now()
		WHERE id=$1 AND user_id=$2 AND revision=$6 AND finalized_at IS NULL
		RETURNING revision, updated_at`, workout.ID, userID, workout.Name, workout.Status,
		workout.FinalizedAt, expectedRevision).Scan(&workout.Revision, &workout.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		current, getErr := loadScheduledWorkout(ctx, tx, userID, workout.ID)
		if getErr != nil {
			return nil, false, getErr
		}
		return nil, false, mutationRevisionConflict("scheduled workout revision conflict", expectedRevision, current, current.Revision)
	}
	if err != nil {
		return nil, false, fmt.Errorf("updating scheduled workout snapshot: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM scheduled_sets WHERE scheduled_workout_id=$1`, workout.ID); err != nil {
		return nil, false, fmt.Errorf("replacing scheduled sets: %w", err)
	}
	if err := insertScheduledSets(ctx, tx, workout.ID, workout.RequiredSets); err != nil {
		return nil, false, err
	}
	result, err := loadScheduledWorkout(ctx, tx, userID, workout.ID)
	if err != nil {
		return nil, false, err
	}
	record.ResourceID = &result.ID
	record.ResourceRevision = &result.Revision
	if err := insertIdempotency(ctx, tx, userID, record, result); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing scheduled workout snapshot: %w", err)
	}
	return result, false, nil
}

func (r *trainingMutationDAO) UpdateScheduledSetTarget(ctx context.Context, userID, workoutID, setID uuid.UUID, target model.PatchScheduledSetTargetRequest, record model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error) {
	tx, err := r.beginScheduleMutation(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)
	var replay model.ScheduledWorkout
	if found, err := findIdempotency(ctx, tx, userID, record.Scope, record.OperationKey, record.RequestHash, &replay); err != nil {
		return nil, false, err
	} else if found {
		return &replay, true, nil
	}
	var revision int64
	err = tx.QueryRow(ctx, `SELECT revision FROM scheduled_workouts WHERE id=$1 AND user_id=$2 FOR UPDATE`, workoutID, userID).Scan(&revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, &model.NotFoundError{Message: "scheduled workout not found"}
	}
	if err != nil {
		return nil, false, fmt.Errorf("locking scheduled workout: %w", err)
	}
	if revision != target.ExpectedRevision {
		current, getErr := loadScheduledWorkout(ctx, tx, userID, workoutID)
		if getErr != nil {
			return nil, false, getErr
		}
		return nil, false, mutationRevisionConflict("scheduled workout revision conflict", target.ExpectedRevision, current, revision)
	}
	command, err := tx.Exec(ctx, `
		UPDATE scheduled_sets ss SET target_reps=$4, target_weight=$5,
		       target_duration_seconds=$6, rest_seconds=$7, notes=$8
		FROM scheduled_workouts sw
		WHERE ss.id=$1 AND ss.scheduled_workout_id=$2 AND sw.id=$2 AND sw.user_id=$3`,
		setID, workoutID, userID, target.TargetReps, target.TargetWeight,
		target.TargetDurationSeconds, target.RestSeconds, target.Notes)
	if err != nil {
		return nil, false, fmt.Errorf("updating scheduled set target: %w", err)
	}
	if command.RowsAffected() == 0 {
		return nil, false, &model.NotFoundError{Message: "scheduled set not found"}
	}
	if _, err := tx.Exec(ctx, `UPDATE scheduled_workouts SET revision=revision+1, updated_at=now() WHERE id=$1 AND user_id=$2`, workoutID, userID); err != nil {
		return nil, false, fmt.Errorf("advancing scheduled workout revision: %w", err)
	}
	result, err := loadScheduledWorkout(ctx, tx, userID, workoutID)
	if err != nil {
		return nil, false, err
	}
	record.ResourceID = &result.ID
	record.ResourceRevision = &result.Revision
	if err := insertIdempotency(ctx, tx, userID, record, result); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing scheduled set target: %w", err)
	}
	return result, false, nil
}

func ensureScheduledWorkoutSession(ctx context.Context, tx pgx.Tx, userID uuid.UUID, workout *model.ScheduledWorkout) (uuid.UUID, error) {
	var sessionID uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT id FROM workout_sessions
		WHERE user_id=$1 AND scheduled_workout_id=$2 AND status<>'discarded'
		ORDER BY created_at LIMIT 1 FOR UPDATE`, userID, workout.ID).Scan(&sessionID)
	if err == nil {
		return sessionID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("locking scheduled workout session: %w", err)
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO workout_sessions (user_id, scheduled_workout_id, date, name, status)
		VALUES ($1,$2,$3,$4,'draft') RETURNING id`, userID, workout.ID, workout.Date, workout.Name).Scan(&sessionID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("creating scheduled workout session: %w", err)
	}
	return sessionID, nil
}

func updateScheduledStatus(ctx context.Context, tx pgx.Tx, userID, workoutID uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		WITH required_counts AS (
			SELECT count(*) AS total,
			       count(*) FILTER (WHERE EXISTS (
				   SELECT 1 FROM set_logs sl
				   JOIN workout_sessions ws ON ws.id=sl.workout_session_id AND ws.user_id=$2
				   WHERE sl.scheduled_set_id=ss.id AND sl.completed=true
				     AND ws.scheduled_workout_id=$1)) AS completed
			FROM scheduled_sets ss WHERE ss.scheduled_workout_id=$1
		)
		UPDATE scheduled_workouts sw
		SET status=CASE
			WHEN sw.finalized_at IS NOT NULL THEN CASE
				WHEN required_counts.total > 0 AND required_counts.completed=required_counts.total THEN 'completed'
				WHEN required_counts.completed > 0 THEN 'incomplete'
				ELSE 'missed' END
			WHEN required_counts.completed > 0 THEN 'in_progress'
			ELSE 'planned' END,
		revision=revision+1, updated_at=now()
		FROM required_counts
		WHERE sw.id=$1 AND sw.user_id=$2`, workoutID, userID)
	if err != nil {
		return fmt.Errorf("refreshing scheduled workout outcome: %w", err)
	}
	return nil
}

func (r *trainingMutationDAO) PutRequiredSet(ctx context.Context, userID, workoutID, setID uuid.UUID, request model.SetMutationRequest, record model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error) {
	tx, err := r.beginScheduleMutation(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)
	var replay model.ScheduledWorkout
	if found, err := findIdempotency(ctx, tx, userID, record.Scope, record.OperationKey, record.RequestHash, &replay); err != nil {
		return nil, false, err
	} else if found {
		return &replay, true, nil
	}
	var revision int64
	err = tx.QueryRow(ctx, `SELECT revision FROM scheduled_workouts WHERE id=$1 AND user_id=$2 FOR UPDATE`, workoutID, userID).Scan(&revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, &model.NotFoundError{Message: "scheduled workout not found"}
	}
	if err != nil {
		return nil, false, fmt.Errorf("locking scheduled workout: %w", err)
	}
	if revision != request.ExpectedRevision {
		current, getErr := loadScheduledWorkout(ctx, tx, userID, workoutID)
		if getErr != nil {
			return nil, false, getErr
		}
		return nil, false, mutationRevisionConflict("scheduled workout revision conflict", request.ExpectedRevision, current, revision)
	}
	workout, err := loadScheduledWorkout(ctx, tx, userID, workoutID)
	if err != nil {
		return nil, false, err
	}
	sessionID, err := ensureScheduledWorkoutSession(ctx, tx, userID, workout)
	if err != nil {
		return nil, false, err
	}
	var exerciseName, exerciseCategory, exerciseModality string
	var setIndex int
	var targetReps *int
	var targetWeight *float64
	err = tx.QueryRow(ctx, `
		SELECT ss.exercise_name, ss.exercise_category, ss.exercise_modality,
		       ss.set_index, ss.target_reps, ss.target_weight
		FROM scheduled_sets ss
		JOIN scheduled_workouts sw ON sw.id=ss.scheduled_workout_id AND sw.user_id=$3
		WHERE ss.id=$1 AND sw.id=$2`, setID, workoutID, userID).Scan(
		&exerciseName, &exerciseCategory, &exerciseModality, &setIndex, &targetReps, &targetWeight,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, &model.NotFoundError{Message: "scheduled set not found"}
	}
	if err != nil {
		return nil, false, fmt.Errorf("loading scheduled set snapshot: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO set_logs (
			workout_session_id, scheduled_set_id, is_extra, exercise_name,
			exercise_category, exercise_modality, set_index, target_reps,
			target_weight, actual_reps, actual_weight, duration_seconds,
			completed, operation_key)
		VALUES ($1,$2,false,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (workout_session_id, scheduled_set_id) WHERE scheduled_set_id IS NOT NULL
		DO UPDATE SET actual_reps=EXCLUDED.actual_reps, actual_weight=EXCLUDED.actual_weight,
		              duration_seconds=EXCLUDED.duration_seconds, completed=EXCLUDED.completed,
		              operation_key=EXCLUDED.operation_key, revision=set_logs.revision+1,
		              logged_at=now()`, sessionID, setID, exerciseName, exerciseCategory,
		exerciseModality, setIndex, targetReps, targetWeight, request.ActualReps,
		request.ActualWeight, request.DurationSeconds, request.Completed, request.OperationKey)
	if err != nil {
		return nil, false, fmt.Errorf("upserting required set result: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE workout_sessions SET revision=revision+1,
		       status=CASE WHEN $3 THEN 'completed' ELSE 'active' END,
		       started_at=COALESCE(started_at, now()),
		       completed_at=CASE WHEN $3 THEN COALESCE(completed_at, $4) ELSE completed_at END,
		       updated_at=now()
		WHERE id=$1 AND user_id=$2`, sessionID, userID, workout.FinalizedAt != nil, workout.FinalizedAt); err != nil {
		return nil, false, fmt.Errorf("advancing workout session revision: %w", err)
	}
	if err := updateScheduledStatus(ctx, tx, userID, workoutID); err != nil {
		return nil, false, err
	}
	result, err := loadScheduledWorkout(ctx, tx, userID, workoutID)
	if err != nil {
		return nil, false, err
	}
	record.ResourceID = &result.ID
	record.ResourceRevision = &result.Revision
	if err := insertIdempotency(ctx, tx, userID, record, result); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing required set mutation: %w", err)
	}
	return result, false, nil
}

func (r *trainingMutationDAO) AddExtraSet(ctx context.Context, userID, workoutID uuid.UUID, request model.ExtraSetRequest, record model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error) {
	tx, err := r.beginScheduleMutation(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)
	var replay model.ScheduledWorkout
	if found, err := findIdempotency(ctx, tx, userID, record.Scope, record.OperationKey, record.RequestHash, &replay); err != nil {
		return nil, false, err
	} else if found {
		return &replay, true, nil
	}
	var revision int64
	var finalizedAt *time.Time
	err = tx.QueryRow(ctx, `SELECT revision, finalized_at FROM scheduled_workouts WHERE id=$1 AND user_id=$2 FOR UPDATE`, workoutID, userID).Scan(&revision, &finalizedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, &model.NotFoundError{Message: "scheduled workout not found"}
	}
	if err != nil {
		return nil, false, fmt.Errorf("locking scheduled workout: %w", err)
	}
	if revision != request.ExpectedRevision {
		current, getErr := loadScheduledWorkout(ctx, tx, userID, workoutID)
		if getErr != nil {
			return nil, false, getErr
		}
		return nil, false, mutationRevisionConflict("scheduled workout revision conflict", request.ExpectedRevision, current, revision)
	}
	if finalizedAt != nil {
		current, getErr := loadScheduledWorkout(ctx, tx, userID, workoutID)
		if getErr != nil {
			return nil, false, getErr
		}
		return nil, false, &model.ConflictError{Message: "finalized scheduled workouts cannot be changed", Authoritative: current}
	}
	exerciseID, preserveExerciseID, err := normalizeExtraExerciseProvenance(ctx, tx, userID, request.ExerciseID)
	if err != nil {
		return nil, false, err
	}
	request.ExerciseID = nil
	if preserveExerciseID {
		request.ExerciseID = &exerciseID
	}
	workout, err := loadScheduledWorkout(ctx, tx, userID, workoutID)
	if err != nil {
		return nil, false, err
	}
	sessionID, err := ensureScheduledWorkoutSession(ctx, tx, userID, workout)
	if err != nil {
		return nil, false, err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO set_logs (
			workout_session_id, exercise_id, is_extra, exercise_name,
			exercise_category, exercise_modality, set_index, actual_reps,
			actual_weight, duration_seconds, completed, operation_key)
		VALUES ($1,$2,true,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, sessionID,
		request.ExerciseID, request.ExerciseName, request.ExerciseCategory,
		request.ExerciseModality, request.SetIndex, request.ActualReps,
		request.ActualWeight, request.DurationSeconds, request.Completed, request.OperationKey)
	if err != nil {
		return nil, false, fmt.Errorf("inserting extra set: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE workout_sessions SET revision=revision+1, status='active', started_at=COALESCE(started_at, now()), updated_at=now() WHERE id=$1 AND user_id=$2`, sessionID, userID); err != nil {
		return nil, false, fmt.Errorf("advancing workout session revision: %w", err)
	}
	if err := updateScheduledStatus(ctx, tx, userID, workoutID); err != nil {
		return nil, false, err
	}
	result, err := loadScheduledWorkout(ctx, tx, userID, workoutID)
	if err != nil {
		return nil, false, err
	}
	record.ResourceID = &result.ID
	record.ResourceRevision = &result.Revision
	if err := insertIdempotency(ctx, tx, userID, record, result); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing extra set mutation: %w", err)
	}
	return result, false, nil
}

func scheduledOutcome(workout *model.ScheduledWorkout) (string, bool) {
	checked := 0
	for _, set := range workout.RequiredSets {
		if set.Checked {
			checked++
		}
	}
	participated := checked > 0
	for _, set := range workout.ExtraSets {
		participated = participated || set.Completed
	}
	if len(workout.RequiredSets) > 0 && checked == len(workout.RequiredSets) {
		return model.WorkoutStatusCompleted, participated
	}
	if checked > 0 {
		return model.WorkoutStatusIncomplete, participated
	}
	return model.WorkoutStatusMissed, participated
}

func (r *trainingMutationDAO) FinalizeScheduledWorkout(ctx context.Context, userID, workoutID uuid.UUID, expectedRevision int64, now time.Time, record model.IdempotencyRecord) (*model.ScheduledWorkout, bool, error) {
	tx, err := r.beginScheduleMutation(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)
	var replay model.ScheduledWorkout
	if found, err := findIdempotency(ctx, tx, userID, record.Scope, record.OperationKey, record.RequestHash, &replay); err != nil {
		return nil, false, err
	} else if found {
		return &replay, true, nil
	}
	var revision int64
	err = tx.QueryRow(ctx, `SELECT revision FROM scheduled_workouts WHERE id=$1 AND user_id=$2 FOR UPDATE`, workoutID, userID).Scan(&revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, &model.NotFoundError{Message: "scheduled workout not found"}
	}
	if err != nil {
		return nil, false, fmt.Errorf("locking scheduled workout: %w", err)
	}
	if revision != expectedRevision {
		current, getErr := loadScheduledWorkout(ctx, tx, userID, workoutID)
		if getErr != nil {
			return nil, false, getErr
		}
		return nil, false, mutationRevisionConflict("scheduled workout revision conflict", expectedRevision, current, revision)
	}
	workout, err := loadScheduledWorkout(ctx, tx, userID, workoutID)
	if err != nil {
		return nil, false, err
	}
	if workout.FinalizedAt == nil {
		status, participated := scheduledOutcome(workout)
		finalizedAt := now.UTC()
		err = tx.QueryRow(ctx, `
			UPDATE scheduled_workouts SET status=$3, finalized_at=$4,
			       revision=revision+1, updated_at=now()
			WHERE id=$1 AND user_id=$2 AND revision=$5
			RETURNING revision`, workoutID, userID, status, finalizedAt, expectedRevision).Scan(&revision)
		if err != nil {
			return nil, false, fmt.Errorf("finalizing scheduled workout: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE workout_sessions SET status='completed', completed_at=COALESCE(completed_at,$3),
			       revision=revision+1, updated_at=now()
			WHERE user_id=$1 AND scheduled_workout_id=$2 AND status='active'`,
			userID, workoutID, finalizedAt); err != nil {
			return nil, false, fmt.Errorf("completing scheduled workout session: %w", err)
		}
		var timezone string
		if err := tx.QueryRow(ctx, `SELECT timezone FROM training_profiles WHERE user_id=$1`, userID).Scan(&timezone); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, false, &model.NotFoundError{Message: "training profile not found"}
			}
			return nil, false, fmt.Errorf("loading participation timezone: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO day_participation (
				user_id, date, scheduled_opportunity, participated,
				finalized_at, timezone, local_date)
			VALUES ($1,$2,true,$3,$4,$5,$2)
			ON CONFLICT (user_id, date) DO UPDATE SET
			  scheduled_opportunity=true,
			  participated=day_participation.participated OR EXCLUDED.participated,
			  revision=day_participation.revision+1`, userID, workout.Date,
			participated, finalizedAt, timezone); err != nil {
			return nil, false, fmt.Errorf("finalizing scheduled participation: %w", err)
		}
	}
	result, err := loadScheduledWorkout(ctx, tx, userID, workoutID)
	if err != nil {
		return nil, false, err
	}
	record.ResourceID = &result.ID
	record.ResourceRevision = &result.Revision
	if err := insertIdempotency(ctx, tx, userID, record, result); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing scheduled workout completion: %w", err)
	}
	return result, false, nil
}

func (r *trainingMutationDAO) CreateWorkoutSession(ctx context.Context, userID uuid.UUID, session *model.WorkoutSession, record model.IdempotencyRecord) (*model.WorkoutSession, bool, error) {
	tx, err := r.beginScheduleMutation(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)
	var replay model.WorkoutSession
	if found, err := findIdempotency(ctx, tx, userID, record.Scope, record.OperationKey, record.RequestHash, &replay); err != nil {
		return nil, false, err
	} else if found {
		return &replay, true, nil
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO workout_sessions (
			user_id, scheduled_workout_id, date, name, status, notes, started_at, completed_at)
		SELECT $1, sw.id, $3::date, $4, $5, $6, $7::timestamptz, $8::timestamptz
		FROM scheduled_workouts sw
		WHERE sw.id=$2 AND sw.user_id=$1
		UNION ALL
		SELECT $1, NULL, $3::date, $4, $5, $6, $7::timestamptz, $8::timestamptz WHERE $2::uuid IS NULL
		RETURNING id, revision, created_at, updated_at`, userID, session.ScheduledWorkoutID,
		session.Date, session.Name, session.Status, session.Notes, session.StartedAt,
		session.CompletedAt).Scan(&session.ID, &session.Revision, &session.CreatedAt, &session.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, &model.NotFoundError{Message: "scheduled workout not found"}
	}
	if err != nil {
		return nil, false, fmt.Errorf("creating workout session: %w", err)
	}
	session.Sets = []model.PerformedSet{}
	record.ResourceID = &session.ID
	record.ResourceRevision = &session.Revision
	if err := insertIdempotency(ctx, tx, userID, record, session); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing workout session create: %w", err)
	}
	return session, false, nil
}

func (r *trainingMutationDAO) ReplaceWorkoutSession(ctx context.Context, userID uuid.UUID, session *model.WorkoutSession, expectedRevision int64, now time.Time, record model.IdempotencyRecord) (*model.WorkoutSession, bool, error) {
	tx, err := r.beginScheduleMutation(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)
	var replay model.WorkoutSession
	if found, err := findIdempotency(ctx, tx, userID, record.Scope, record.OperationKey, record.RequestHash, &replay); err != nil {
		return nil, false, err
	} else if found {
		return &replay, true, nil
	}
	err = tx.QueryRow(ctx, `
		UPDATE workout_sessions SET name=$3, status=$4, notes=$5, started_at=$6,
		       completed_at=$7, revision=revision+1, updated_at=now()
		WHERE id=$1 AND user_id=$2 AND revision=$8
		RETURNING revision, updated_at`, session.ID, userID, session.Name, session.Status,
		session.Notes, session.StartedAt, session.CompletedAt, expectedRevision).Scan(
		&session.Revision, &session.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		current, getErr := loadWorkoutSession(ctx, tx, userID, session.ID)
		if getErr != nil {
			return nil, false, getErr
		}
		return nil, false, mutationRevisionConflict("workout session revision conflict", expectedRevision, current, current.Revision)
	}
	if err != nil {
		return nil, false, fmt.Errorf("updating workout session: %w", err)
	}
	if session.Status == "completed" {
		var timezone string
		if err := tx.QueryRow(ctx, `SELECT timezone FROM training_profiles WHERE user_id=$1`, userID).Scan(&timezone); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, false, &model.NotFoundError{Message: "training profile not found"}
			}
			return nil, false, fmt.Errorf("loading participation timezone: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO day_participation (
				user_id, date, scheduled_opportunity, participated,
				finalized_at, timezone, local_date)
			VALUES ($1,$2,$3,true,$4,$5,$2)
			ON CONFLICT (user_id, date) DO UPDATE SET
			  participated=true,
			  scheduled_opportunity=day_participation.scheduled_opportunity OR EXCLUDED.scheduled_opportunity,
			  revision=day_participation.revision+1`, userID, session.Date,
			session.ScheduledWorkoutID != nil, now.UTC(), timezone); err != nil {
			return nil, false, fmt.Errorf("preserving workout session participation: %w", err)
		}
	}
	result, err := loadWorkoutSession(ctx, tx, userID, session.ID)
	if err != nil {
		return nil, false, err
	}
	record.ResourceID = &result.ID
	record.ResourceRevision = &result.Revision
	if err := insertIdempotency(ctx, tx, userID, record, result); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("committing workout session update: %w", err)
	}
	return result, false, nil
}
