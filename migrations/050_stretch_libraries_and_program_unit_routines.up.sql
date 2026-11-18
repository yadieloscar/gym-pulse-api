-- The athlete's own stretches, and stretching routines on program workouts
-- (spec 036, gympulse-own-stretches).
--
-- Additive only. No row is rewritten: every existing program workout keeps
-- the automatic warm-up and cool-down (both columns NULL), and no athlete
-- has a library until they save one.

-- One document per athlete: the stretch moves and routines they wrote. It is
-- read and written whole and never queried by field, so it is one JSONB
-- value with an optimistic revision. The service validates its shape; the
-- checks here only bound what one row can hold.
CREATE TABLE IF NOT EXISTS stretch_libraries (
    user_id     UUID PRIMARY KEY REFERENCES auth.users(id) ON DELETE CASCADE,
    revision    BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    content     JSONB NOT NULL
                CHECK (jsonb_typeof(content) = 'object' AND octet_length(content::text) <= 262144),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The stretching routine the athlete chose to open a program workout with,
-- and the one to close it with. Same rule as workout_templates (migration
-- 049): slugs the API stores and does not look up. NULL means the app picks
-- by body area, as before.
ALTER TABLE program_workouts ADD COLUMN IF NOT EXISTS warmup_routine_id TEXT;
ALTER TABLE program_workouts ADD COLUMN IF NOT EXISTS cooldown_routine_id TEXT;

ALTER TABLE program_workouts DROP CONSTRAINT IF EXISTS program_workouts_warmup_routine_id_check;
ALTER TABLE program_workouts ADD CONSTRAINT program_workouts_warmup_routine_id_check CHECK (
    warmup_routine_id IS NULL
    OR (char_length(warmup_routine_id) BETWEEN 1 AND 64 AND warmup_routine_id ~ '^[a-z0-9]+(-[a-z0-9]+)*$')
);

ALTER TABLE program_workouts DROP CONSTRAINT IF EXISTS program_workouts_cooldown_routine_id_check;
ALTER TABLE program_workouts ADD CONSTRAINT program_workouts_cooldown_routine_id_check CHECK (
    cooldown_routine_id IS NULL
    OR (char_length(cooldown_routine_id) BETWEEN 1 AND 64 AND cooldown_routine_id ~ '^[a-z0-9]+(-[a-z0-9]+)*$')
);

SELECT gym_pulse_private.secure_table('public.stretch_libraries');
