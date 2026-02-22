CREATE TABLE exercises (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id     UUID NOT NULL REFERENCES workout_templates(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    sort_order      INTEGER NOT NULL DEFAULT 0,
    sets            INTEGER,
    reps            INTEGER,
    weight          NUMERIC(7,2),
    rest_seconds    INTEGER,
    notes           TEXT
);

CREATE INDEX idx_exercises_template ON exercises(template_id);
