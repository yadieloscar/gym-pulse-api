-- Program generations remember how many program drafts an athlete received in
-- a local day so the provider is called at most that many times. No draft
-- content is stored; the row exists for the limit and for operational counts.
CREATE TABLE program_generations (
    id              UUID PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    local_date      DATE NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'succeeded', 'failed')),
    model           TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at    TIMESTAMPTZ
);

-- The daily count reads one athlete, one local date, by status.
CREATE INDEX idx_program_generations_user_date
    ON program_generations(user_id, local_date, status);

-- The Go API is the sole application-data path.
ALTER TABLE public.program_generations ENABLE ROW LEVEL SECURITY;

DO $$
DECLARE
    data_api_role TEXT;
BEGIN
    FOR data_api_role IN
        SELECT rolname FROM pg_roles
        WHERE rolname IN ('anon', 'authenticated', 'service_role')
    LOOP
        EXECUTE format(
            'REVOKE ALL PRIVILEGES ON TABLE public.program_generations FROM %I',
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
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.program_generations TO gym_pulse_api;
        DROP POLICY IF EXISTS gym_pulse_api_access ON public.program_generations;
        CREATE POLICY gym_pulse_api_access ON public.program_generations
            AS PERMISSIVE FOR ALL TO gym_pulse_api USING (true) WITH CHECK (true);
    END IF;
END
$$;
