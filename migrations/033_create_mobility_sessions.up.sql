-- Mobility sessions record finished stretching routines as facts. They are
-- deliberately separate from workout sessions and sport activities: nothing
-- that derives stats, participation, records, volume, plan, or schedule reads
-- this table. Retries are identified on the row by (user_id, operation_key).
-- New table only, so existing data and clients are unaffected.
CREATE TABLE mobility_sessions (
    id                  UUID PRIMARY KEY,
    user_id             UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    date                DATE NOT NULL,
    routine_id          TEXT NOT NULL CHECK (
                            char_length(routine_id) BETWEEN 1 AND 64
                            AND routine_id ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    routine_name        TEXT NOT NULL CHECK (char_length(routine_name) BETWEEN 1 AND 80),
    context             TEXT NOT NULL CHECK (context IN ('warmup', 'cooldown', 'rest_day', 'anytime')),
    duration_seconds    INTEGER NOT NULL CHECK (duration_seconds BETWEEN 1 AND 7200),
    moves_completed     SMALLINT NOT NULL CHECK (moves_completed BETWEEN 1 AND 100),
    moves_total         SMALLINT NOT NULL CHECK (moves_total BETWEEN 1 AND 100),
    content_version     TEXT NOT NULL CHECK (char_length(content_version) BETWEEN 1 AND 32),
    operation_key       TEXT NOT NULL CHECK (char_length(operation_key) BETWEEN 1 AND 128),
    request_hash        TEXT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT mobility_sessions_moves_check CHECK (moves_completed <= moves_total)
);

-- One entry per athlete and operation key; the create retry identity.
CREATE UNIQUE INDEX ux_mobility_sessions_user_operation
    ON mobility_sessions(user_id, operation_key);

-- Owner range reads, newest first.
CREATE INDEX idx_mobility_sessions_user_date
    ON mobility_sessions(user_id, date DESC, created_at DESC, id DESC);

-- The Go API is the sole application-data path. Owner bypass is required by
-- the direct API database role, so RLS is enabled but not forced.
ALTER TABLE public.mobility_sessions ENABLE ROW LEVEL SECURITY;

DO $$
DECLARE
    data_api_role TEXT;
BEGIN
    FOR data_api_role IN
        SELECT rolname FROM pg_roles
        WHERE rolname IN ('anon', 'authenticated', 'service_role')
    LOOP
        EXECUTE format(
            'REVOKE ALL PRIVILEGES ON TABLE public.mobility_sessions FROM %I',
            data_api_role
        );
    END LOOP;
END
$$;

-- Migration 026 grants the least-privilege runtime role default DML on future
-- tables, but each new table still needs its own permissive policy so the
-- non-owner role can see rows. Skipped where the role does not exist yet.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'gym_pulse_api') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.mobility_sessions TO gym_pulse_api;
        DROP POLICY IF EXISTS gym_pulse_api_access ON public.mobility_sessions;
        CREATE POLICY gym_pulse_api_access ON public.mobility_sessions
            AS PERMISSIVE FOR ALL TO gym_pulse_api USING (true) WITH CHECK (true);
    END IF;
END
$$;
