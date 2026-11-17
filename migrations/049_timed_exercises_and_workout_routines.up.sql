-- Timed exercises and chosen stretching routines for saved workouts
-- (spec 033, gympulse-workout-stretches).
--
-- Additive only. No row is rewritten: every existing exercise keeps its
-- reps or minutes, and every existing workout keeps the automatic warm-up
-- and cool-down (both columns NULL).

-- Seconds per set for a hold or stretch ("2 sets of 30 s"). NULL for an
-- exercise counted in reps or in cardio minutes.
ALTER TABLE exercises ADD COLUMN IF NOT EXISTS duration_seconds INTEGER;

ALTER TABLE exercises DROP CONSTRAINT IF EXISTS exercises_duration_seconds_check;
ALTER TABLE exercises ADD CONSTRAINT exercises_duration_seconds_check CHECK (
    duration_seconds IS NULL OR duration_seconds BETWEEN 1 AND 3600
);

-- The stretching routine the athlete chose to open this workout with, and
-- the one to close it with. Routines are bundled in the app, so these are
-- slugs the API stores and does not look up. NULL means the app picks by
-- body area, as before.
ALTER TABLE workout_templates ADD COLUMN IF NOT EXISTS warmup_routine_id TEXT;
ALTER TABLE workout_templates ADD COLUMN IF NOT EXISTS cooldown_routine_id TEXT;

ALTER TABLE workout_templates DROP CONSTRAINT IF EXISTS workout_templates_warmup_routine_id_check;
ALTER TABLE workout_templates ADD CONSTRAINT workout_templates_warmup_routine_id_check CHECK (
    warmup_routine_id IS NULL
    OR (char_length(warmup_routine_id) BETWEEN 1 AND 64 AND warmup_routine_id ~ '^[a-z0-9]+(-[a-z0-9]+)*$')
);

ALTER TABLE workout_templates DROP CONSTRAINT IF EXISTS workout_templates_cooldown_routine_id_check;
ALTER TABLE workout_templates ADD CONSTRAINT workout_templates_cooldown_routine_id_check CHECK (
    cooldown_routine_id IS NULL
    OR (char_length(cooldown_routine_id) BETWEEN 1 AND 64 AND cooldown_routine_id ~ '^[a-z0-9]+(-[a-z0-9]+)*$')
);
