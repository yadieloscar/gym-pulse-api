DROP TABLE IF EXISTS legacy_adoptions;
DROP TABLE IF EXISTS idempotency_records;
DROP TABLE IF EXISTS day_participation;

DROP INDEX IF EXISTS idx_setlogs_required_result;
DROP INDEX IF EXISTS idx_setlogs_session_operation;
ALTER TABLE set_logs
    DROP CONSTRAINT IF EXISTS set_logs_modality_check,
    DROP CONSTRAINT IF EXISTS set_logs_required_extra_check,
    DROP CONSTRAINT IF EXISTS set_logs_parent_check,
    DROP CONSTRAINT IF EXISTS set_logs_exercise_id_fkey,
    ADD CONSTRAINT set_logs_exercise_id_fkey FOREIGN KEY (exercise_id) REFERENCES exercises(id) ON DELETE CASCADE,
    ALTER COLUMN day_log_id SET NOT NULL,
    ALTER COLUMN exercise_id SET NOT NULL,
    DROP COLUMN IF EXISTS revision,
    DROP COLUMN IF EXISTS operation_key,
    DROP COLUMN IF EXISTS exercise_modality,
    DROP COLUMN IF EXISTS exercise_category,
    DROP COLUMN IF EXISTS exercise_name,
    DROP COLUMN IF EXISTS is_extra,
    DROP COLUMN IF EXISTS scheduled_set_id,
    DROP COLUMN IF EXISTS workout_session_id;

ALTER TABLE day_logs
    ADD CONSTRAINT day_logs_user_id_date_key UNIQUE (user_id, date);

DROP TABLE IF EXISTS workout_sessions;
DROP TABLE IF EXISTS scheduled_sets;
DROP TABLE IF EXISTS scheduled_workouts;
DROP TABLE IF EXISTS program_exercises;
DROP TABLE IF EXISTS program_workouts;
DROP TABLE IF EXISTS programs;
DROP TABLE IF EXISTS starter_exercises;
DROP TABLE IF EXISTS starter_workouts;
DROP TABLE IF EXISTS starter_programs;
DROP TABLE IF EXISTS training_profiles;
