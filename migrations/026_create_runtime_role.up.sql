-- Least-privilege runtime role for the Go API.
--
-- Migrations keep running as the schema owner (DATABASE_MIGRATE_URL). Requests
-- run as gym_pulse_api (DATABASE_URL), which holds only DML on application
-- tables. RLS stays enabled and not forced; because gym_pulse_api is not the
-- owner, it needs an explicit permissive policy on every table to see any row.
-- The API enforces ownership in SQL, so the policy is intentionally
-- unconditional. BYPASSRLS was rejected: it needs a superuser to grant, which
-- hosted Supabase does not provide, and it would hide a missing policy.
--
-- The role is created NOLOGIN when absent so local and CI databases work
-- without an operator step. Production creates it ahead of this migration with
-- LOGIN and its own credential (see docs/RELEASE_RUNBOOK.md); this migration then only
-- grants. A pre-existing role is never altered.
DO $$
DECLARE
    runtime_role CONSTANT TEXT := 'gym_pulse_api';
    application_tables CONSTANT TEXT[] := ARRAY[
        'workout_templates',
        'exercises',
        'day_logs',
        'exercise_overrides',
        'user_settings',
        'user_profiles',
        'body_weights',
        'exercise_catalog',
        'weekly_plans',
        'plan_overrides',
        'set_logs',
        'training_profiles',
        'starter_programs',
        'starter_workouts',
        'starter_exercises',
        'programs',
        'program_workouts',
        'program_exercises',
        'scheduled_workouts',
        'scheduled_sets',
        'workout_sessions',
        'day_participation',
        'idempotency_records',
        'legacy_adoptions',
        'sport_activities',
        'criteria_training_blocks',
        'criteria_training_stages',
        'criteria_training_exposures',
        'criteria_training_transitions',
        'health_exports'
    ];
    table_name TEXT;
    default_owner TEXT;
    uncovered TEXT;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = runtime_role) THEN
        EXECUTE format(
            'CREATE ROLE %I NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS NOLOGIN',
            runtime_role
        );
    END IF;

    -- Object reachability: schema usage plus DML on every application table.
    -- schema_migrations is deliberately excluded; only the migrator touches it.
    EXECUTE format('GRANT USAGE ON SCHEMA public TO %I', runtime_role);
    EXECUTE format('REVOKE CREATE ON SCHEMA public FROM %I', runtime_role);

    FOREACH table_name IN ARRAY application_tables LOOP
        EXECUTE format(
            'GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.%I TO %I',
            table_name, runtime_role
        );
        -- Row reachability: one unconditional permissive policy per table. The
        -- Go API scopes every query to the authenticated user in SQL.
        EXECUTE format('DROP POLICY IF EXISTS gym_pulse_api_access ON public.%I', table_name);
        EXECUTE format(
            'CREATE POLICY gym_pulse_api_access ON public.%I AS PERMISSIVE FOR ALL TO %I USING (true) WITH CHECK (true)',
            table_name, runtime_role
        );
    END LOOP;

    -- Future tables created by the migrator get the same DML automatically.
    -- Their RLS policy still has to be written in the creating migration;
    -- internal/dao/supabase_security_migration_test.go enforces that.
    FOR default_owner IN
        SELECT rolname
        FROM pg_roles
        WHERE rolname IN (current_user, 'postgres')
          AND pg_has_role(current_user, oid, 'USAGE')
    LOOP
        EXECUTE format(
            'ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %I',
            default_owner, runtime_role
        );
    END LOOP;

    -- The API verifies that a JWT subject still exists in auth.users. Hosted
    -- Supabase owns that table through supabase_auth_admin, so the migrator
    -- may be unable to re-grant it; the release runbook covers that case and
    -- scripts/validate-supabase-security.sh asserts the result.
    BEGIN
        EXECUTE format('GRANT USAGE ON SCHEMA auth TO %I', runtime_role);
        EXECUTE format('GRANT SELECT ON TABLE auth.users TO %I', runtime_role);
    EXCEPTION
        WHEN insufficient_privilege THEN
            RAISE WARNING 'could not grant auth.users read access to %; grant it as a privileged role before pointing DATABASE_URL at the runtime role', runtime_role;
    END;

    -- Fail loudly if any application table was created without being listed
    -- above, so the runtime role never silently loses a table.
    SELECT string_agg(c.relname, ', ' ORDER BY c.relname)
    INTO uncovered
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public'
      AND c.relkind IN ('r', 'p')
      AND c.relname <> 'schema_migrations'
      AND (
          NOT EXISTS (
              SELECT 1 FROM pg_policy p
              WHERE p.polrelid = c.oid AND p.polname = 'gym_pulse_api_access'
          )
          OR NOT has_table_privilege(runtime_role, c.oid, 'SELECT')
          OR NOT has_table_privilege(runtime_role, c.oid, 'INSERT')
          OR NOT has_table_privilege(runtime_role, c.oid, 'UPDATE')
          OR NOT has_table_privilege(runtime_role, c.oid, 'DELETE')
      );

    IF uncovered IS NOT NULL THEN
        RAISE EXCEPTION 'runtime role % is not fully provisioned on: %', runtime_role, uncovered;
    END IF;
END
$$;
