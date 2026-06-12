CREATE TABLE weekly_plans (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL,
    weekday     INTEGER NOT NULL CHECK (weekday BETWEEN 1 AND 7),
    template_id UUID REFERENCES workout_templates(id) ON DELETE SET NULL,
    rest        BOOLEAN NOT NULL DEFAULT false,
    UNIQUE (user_id, weekday),
    -- rest days carry no template; both null/false = weekday intentionally unplanned
    CHECK (NOT (rest AND template_id IS NOT NULL))
);

CREATE TABLE plan_overrides (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL,
    date        DATE NOT NULL,
    template_id UUID REFERENCES workout_templates(id) ON DELETE SET NULL,
    rest        BOOLEAN NOT NULL DEFAULT false,
    UNIQUE (user_id, date),
    CHECK (NOT (rest AND template_id IS NOT NULL))
);

CREATE INDEX idx_weekly_plans_user ON weekly_plans(user_id);
CREATE INDEX idx_plan_overrides_user_date ON plan_overrides(user_id, date);
