CREATE TABLE day_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL,
    date            DATE NOT NULL,
    type_id         TEXT NOT NULL,
    subtype_id      TEXT NOT NULL DEFAULT 'general',
    template_id     UUID REFERENCES workout_templates(id) ON DELETE SET NULL,
    session_notes   TEXT,
    logged_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE(user_id, date)
);

CREATE INDEX idx_logs_user_date ON day_logs(user_id, date);
