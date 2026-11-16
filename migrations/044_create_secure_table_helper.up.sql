-- One call provisions a new application table for the GymPulse security model.
--
-- Before this migration every table-creating migration copied two DO blocks:
-- revoke the Supabase Data API roles, then grant DML and the
-- gym_pulse_api_access policy to the runtime role (see 026 and 033). New
-- migrations now end with:
--
--     SELECT gym_pulse_private.secure_table('public.<table>');
--
-- The function lives in gym_pulse_private (migration 043), which the Supabase
-- Data API does not expose. gym_pulse_api may use that schema but gets no
-- EXECUTE here, and the function runs as the caller (the schema owner running
-- migrations). It is idempotent and safe where the hosted
-- Supabase roles do not exist. internal/dao/supabase_security_migration_test.go
-- accepts the call as provisioning for both RLS and the runtime role.
CREATE SCHEMA IF NOT EXISTS gym_pulse_private;
REVOKE ALL ON SCHEMA gym_pulse_private FROM PUBLIC;

CREATE OR REPLACE FUNCTION gym_pulse_private.secure_table(target regclass)
RETURNS void
LANGUAGE plpgsql
SECURITY INVOKER
-- With only pg_catalog on the path, regclass prints schema-qualified names.
SET search_path = pg_catalog
AS $$
DECLARE
    runtime_role CONSTANT TEXT := 'gym_pulse_api';
    data_api_role TEXT;
BEGIN
    -- Enabled, never forced: the owner keeps its bypass for migrations and the
    -- runtime role reaches rows only through the policy below.
    EXECUTE format('ALTER TABLE %s ENABLE ROW LEVEL SECURITY', target);

    FOR data_api_role IN
        SELECT rolname FROM pg_roles
        WHERE rolname IN ('anon', 'authenticated', 'service_role')
    LOOP
        EXECUTE format('REVOKE ALL PRIVILEGES ON TABLE %s FROM %I', target, data_api_role);
    END LOOP;

    -- Migration 026 always creates the role; the guard keeps the helper usable
    -- on a database restored from before it.
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = runtime_role) THEN
        EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE %s TO %I', target, runtime_role);
        EXECUTE format('DROP POLICY IF EXISTS gym_pulse_api_access ON %s', target);
        EXECUTE format(
            'CREATE POLICY gym_pulse_api_access ON %s AS PERMISSIVE FOR ALL TO %I USING (true) WITH CHECK (true)',
            target, runtime_role
        );
    END IF;
END
$$;

REVOKE ALL ON FUNCTION gym_pulse_private.secure_table(regclass) FROM PUBLIC;
