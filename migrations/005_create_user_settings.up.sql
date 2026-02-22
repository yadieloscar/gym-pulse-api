CREATE TABLE user_settings (
    user_id         UUID PRIMARY KEY,
    weight_unit     TEXT NOT NULL DEFAULT 'lb',
    weekly_goal     INTEGER NOT NULL DEFAULT 5,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
