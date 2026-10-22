-- Remove the runtime role's grants and policies. The role itself is left in
-- place: roles are cluster-level, production creates it out of band, and a
-- deployment still pointing DATABASE_URL at it must be switched back to the
-- owner connection before this rollback is useful.
DO $$
DECLARE
    runtime_role CONSTANT TEXT := 'gym_pulse_api';
    table_name TEXT;
    default_owner TEXT;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = runtime_role) THEN
        RETURN;
    END IF;

    FOR table_name IN
        SELECT c.relname
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p')
    LOOP
        EXECUTE format('DROP POLICY IF EXISTS gym_pulse_api_access ON public.%I', table_name);
    END LOOP;

    EXECUTE format('REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM %I', runtime_role);
    EXECUTE format('REVOKE ALL PRIVILEGES ON SCHEMA public FROM %I', runtime_role);

    FOR default_owner IN
        SELECT rolname
        FROM pg_roles
        WHERE rolname IN (current_user, 'postgres')
          AND pg_has_role(current_user, oid, 'USAGE')
    LOOP
        EXECUTE format(
            'ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public REVOKE ALL PRIVILEGES ON TABLES FROM %I',
            default_owner, runtime_role
        );
    END LOOP;

    BEGIN
        EXECUTE format('REVOKE ALL PRIVILEGES ON TABLE auth.users FROM %I', runtime_role);
        EXECUTE format('REVOKE ALL PRIVILEGES ON SCHEMA auth FROM %I', runtime_role);
    EXCEPTION
        WHEN insufficient_privilege THEN
            RAISE WARNING 'could not revoke auth.users access from %; revoke it as a privileged role', runtime_role;
    END;
END
$$;
