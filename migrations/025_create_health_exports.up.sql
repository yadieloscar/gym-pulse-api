-- Health exports remember the health-store identity the device received when
-- it wrote a finished workout to Apple Health, so the same workout is never
-- written twice. The referenced workout lives in scheduled_workouts or
-- day_logs; ownership and finished state are checked in the application
-- transaction, and rows are removed with the account.
CREATE TABLE health_exports (
    id              UUID PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    workout_kind    TEXT NOT NULL CHECK (workout_kind IN ('scheduled_workout', 'day_log')),
    workout_id      UUID NOT NULL,
    source          TEXT NOT NULL CHECK (source IN ('apple_health')),
    external_id     TEXT NOT NULL CHECK (char_length(external_id) BETWEEN 1 AND 128),
    exported_at     TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One export per workout and origin; the identity is write-once.
CREATE UNIQUE INDEX ux_health_exports_workout
    ON health_exports(user_id, workout_kind, workout_id, source);
CREATE INDEX idx_health_exports_user_exported
    ON health_exports(user_id, exported_at DESC, id DESC);

-- The Go API is the sole application-data path.
ALTER TABLE public.health_exports ENABLE ROW LEVEL SECURITY;

DO $$
DECLARE
    data_api_role TEXT;
BEGIN
    FOR data_api_role IN
        SELECT rolname FROM pg_roles
        WHERE rolname IN ('anon', 'authenticated', 'service_role')
    LOOP
        EXECUTE format(
            'REVOKE ALL PRIVILEGES ON TABLE public.health_exports FROM %I',
            data_api_role
        );
    END LOOP;
END
$$;
