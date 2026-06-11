CREATE TABLE body_weights (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    date DATE NOT NULL,
    weight NUMERIC(5, 2) NOT NULL,
    unit TEXT NOT NULL CHECK (unit IN ('lb', 'kg')),
    logged_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, date)
);
CREATE INDEX idx_body_weights_user ON body_weights(user_id);
