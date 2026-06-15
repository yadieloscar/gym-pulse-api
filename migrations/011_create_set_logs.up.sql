-- Per-set performance history. exercise_overrides keeps per-exercise aggregates
-- (notes/skipped); set_logs is the per-set source of truth that powers the
-- active workout player and "last time you did X" progressive-overload prompts.
CREATE TABLE set_logs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    day_log_id       UUID NOT NULL REFERENCES day_logs(id) ON DELETE CASCADE,
    exercise_id      UUID NOT NULL REFERENCES exercises(id) ON DELETE CASCADE,
    set_index        INTEGER NOT NULL,            -- 1-based order within the exercise
    target_reps      INTEGER,                     -- plan snapshot captured at session start
    target_weight    NUMERIC(7,2),
    actual_reps      INTEGER,
    actual_weight    NUMERIC(7,2),
    duration_seconds INTEGER,                      -- cardio sets
    completed        BOOLEAN NOT NULL DEFAULT false,
    logged_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (day_log_id, exercise_id, set_index)
);

CREATE INDEX idx_setlogs_log ON set_logs(day_log_id);
CREATE INDEX idx_setlogs_exercise ON set_logs(exercise_id);
