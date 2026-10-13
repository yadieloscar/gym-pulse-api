ALTER TABLE scheduled_workouts
    DROP CONSTRAINT IF EXISTS scheduled_workouts_resolution_pair,
    DROP CONSTRAINT IF EXISTS scheduled_workouts_resolution_check,
    DROP COLUMN IF EXISTS resolved_at,
    DROP COLUMN IF EXISTS resolution;
