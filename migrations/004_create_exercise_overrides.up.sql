CREATE TABLE exercise_overrides (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    day_log_id      UUID NOT NULL REFERENCES day_logs(id) ON DELETE CASCADE,
    exercise_id     UUID NOT NULL REFERENCES exercises(id) ON DELETE CASCADE,
    actual_sets     INTEGER,
    actual_reps     INTEGER,
    actual_weight   NUMERIC(7,2),
    notes           TEXT,
    skipped         BOOLEAN DEFAULT false
);

CREATE INDEX idx_overrides_log ON exercise_overrides(day_log_id);
