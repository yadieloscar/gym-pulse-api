-- Restore the table-wide read that migration 026 granted.
DO $$
DECLARE
    runtime_role CONSTANT TEXT := 'gym_pulse_api';
BEGIN
    EXECUTE format('REVOKE SELECT (id) ON TABLE auth.users FROM %I', runtime_role);
    EXECUTE format('GRANT SELECT ON TABLE auth.users TO %I', runtime_role);
EXCEPTION
    WHEN insufficient_privilege THEN
        RAISE WARNING 'could not restore auth.users access for %; grant it as a privileged role', runtime_role;
END
$$;
