-- Completed sports are independent of structured workout sessions. Existing
-- clients and rows require no backfill.
CREATE TABLE sport_activities (
    id                  UUID PRIMARY KEY,
    user_id             UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    date                DATE NOT NULL,
    sport_id            TEXT NOT NULL CHECK (
                            char_length(sport_id) BETWEEN 1 AND 64
                            AND sport_id ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    sport_name          TEXT NOT NULL CHECK (char_length(sport_name) BETWEEN 1 AND 80),
    duration_minutes    INTEGER NOT NULL CHECK (duration_minutes BETWEEN 1 AND 1440),
    notes               TEXT CHECK (notes IS NULL OR char_length(notes) BETWEEN 1 AND 2000),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sport_activities_user_date
    ON sport_activities(user_id, date DESC, created_at DESC, id DESC);

-- The Go API is the sole application-data path. Owner bypass is required by
-- the direct API database role, so RLS is enabled but not forced.
ALTER TABLE public.sport_activities ENABLE ROW LEVEL SECURITY;

DO $$
DECLARE
    data_api_role TEXT;
BEGIN
    FOR data_api_role IN
        SELECT rolname FROM pg_roles
        WHERE rolname IN ('anon', 'authenticated', 'service_role')
    LOOP
        EXECUTE format(
            'REVOKE ALL PRIVILEGES ON TABLE public.sport_activities FROM %I',
            data_api_role
        );
    END LOOP;
END
$$;
