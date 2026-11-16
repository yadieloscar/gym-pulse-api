-- Let the runtime role confirm that a JWT subject still exists without any
-- access to the auth schema.
--
-- Hosted Supabase owns the auth schema through supabase_admin and gives the
-- dashboard and migration role (postgres) no grant option on it, so
-- `GRANT USAGE ON SCHEMA auth TO gym_pulse_api` silently grants nothing and
-- every identity check fails. postgres itself can read auth.users and bypasses
-- RLS there, so a SECURITY DEFINER function it owns answers the one question
-- the API asks: does this id exist. It returns only a boolean.
--
-- The function lives in its own schema, which the Supabase Data API does not
-- expose, and only gym_pulse_api may call it.
CREATE SCHEMA IF NOT EXISTS gym_pulse_private;
REVOKE ALL ON SCHEMA gym_pulse_private FROM PUBLIC;

CREATE OR REPLACE FUNCTION gym_pulse_private.auth_user_exists(subject UUID)
RETURNS BOOLEAN
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = ''
AS $$
    SELECT EXISTS (SELECT 1 FROM auth.users WHERE id = subject)
$$;

REVOKE ALL ON FUNCTION gym_pulse_private.auth_user_exists(UUID) FROM PUBLIC;

DO $$
DECLARE
    runtime_role CONSTANT TEXT := 'gym_pulse_api';
    data_api_role TEXT;
BEGIN
    EXECUTE format('GRANT USAGE ON SCHEMA gym_pulse_private TO %I', runtime_role);
    EXECUTE format('GRANT EXECUTE ON FUNCTION gym_pulse_private.auth_user_exists(UUID) TO %I', runtime_role);
    -- Supabase may add default function privileges for its Data API roles.
    FOREACH data_api_role IN ARRAY ARRAY['anon', 'authenticated', 'service_role'] LOOP
        IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = data_api_role) THEN
            EXECUTE format('REVOKE ALL ON FUNCTION gym_pulse_private.auth_user_exists(UUID) FROM %I', data_api_role);
            EXECUTE format('REVOKE ALL ON SCHEMA gym_pulse_private FROM %I', data_api_role);
        END IF;
    END LOOP;
END
$$;
