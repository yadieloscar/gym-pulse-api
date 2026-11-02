DROP INDEX IF EXISTS ux_sport_activities_scheduled_workout;
ALTER TABLE sport_activities
    DROP COLUMN IF EXISTS scheduled_workout_id;
ALTER TABLE scheduled_workouts
    DROP CONSTRAINT IF EXISTS scheduled_workouts_sport_kind_check,
    DROP COLUMN IF EXISTS sport_notes,
    DROP COLUMN IF EXISTS target_distance_meters,
    DROP COLUMN IF EXISTS target_duration_minutes,
    DROP COLUMN IF EXISTS sport_name,
    DROP COLUMN IF EXISTS sport_id,
    DROP COLUMN IF EXISTS kind;
ALTER TABLE program_workouts
    DROP CONSTRAINT IF EXISTS program_workouts_sport_kind_check,
    DROP COLUMN IF EXISTS sport_notes,
    DROP COLUMN IF EXISTS target_distance_meters,
    DROP COLUMN IF EXISTS target_duration_minutes,
    DROP COLUMN IF EXISTS sport_name,
    DROP COLUMN IF EXISTS sport_id,
    DROP COLUMN IF EXISTS kind;
