CREATE TABLE workout_templates (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL,
    name        TEXT NOT NULL,
    type_id     TEXT NOT NULL,
    subtype_id  TEXT NOT NULL DEFAULT 'general',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_templates_user ON workout_templates(user_id);
CREATE INDEX idx_templates_user_type ON workout_templates(user_id, type_id);
