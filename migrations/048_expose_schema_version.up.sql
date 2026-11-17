-- Let the server confirm at startup that the schema is at least as new as the
-- build, without giving the runtime role the migration ledger.
--
-- A staging or production server never migrates; ./migrate runs as the
-- pre-deploy step. When that step is missing, new code starts on an old schema
-- and fails request by request (production, 2026-11-17: a column added by 045
-- did not exist). The server now calls this function before it serves and
-- refuses to start when the schema is behind or dirty.
--
-- gym_pulse_api still has no privilege on public.schema_migrations (026). The
-- function runs as its owner, returns only the version and the dirty flag, and
-- lives in gym_pulse_private, which the Supabase Data API does not expose.
CREATE OR REPLACE FUNCTION gym_pulse_private.schema_version()
RETURNS TABLE (version BIGINT, dirty BOOLEAN)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = ''
AS $$
    SELECT m.version, m.dirty FROM public.schema_migrations m
$$;

REVOKE ALL ON FUNCTION gym_pulse_private.schema_version() FROM PUBLIC;

DO $$
DECLARE
    runtime_role CONSTANT TEXT := 'gym_pulse_api';
    data_api_role TEXT;
BEGIN
    EXECUTE format('GRANT EXECUTE ON FUNCTION gym_pulse_private.schema_version() TO %I', runtime_role);
    -- Supabase may add default function privileges for its Data API roles.
    FOREACH data_api_role IN ARRAY ARRAY['anon', 'authenticated', 'service_role'] LOOP
        IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = data_api_role) THEN
            EXECUTE format('REVOKE ALL ON FUNCTION gym_pulse_private.schema_version() FROM %I', data_api_role);
        END IF;
    END LOOP;
END
$$;
