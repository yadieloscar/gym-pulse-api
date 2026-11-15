-- Narrow the runtime role's view of auth.users to the id column.
--
-- Migration 026 granted gym_pulse_api SELECT on the whole table so the API can
-- confirm that a JWT subject still exists. On hosted Supabase that table also
-- holds emails, phone numbers, and password hashes, none of which a request
-- ever needs. The identity check reads `id` only, so a column grant is enough.
--
-- Hosted Supabase owns auth.users through supabase_auth_admin, so the migrator
-- may be unable to change the grant; docs/RELEASE_RUNBOOK.md covers that case
-- and scripts/validate-supabase-security.sh asserts the result.
DO $$
DECLARE
    runtime_role CONSTANT TEXT := 'gym_pulse_api';
BEGIN
    EXECUTE format('REVOKE SELECT ON TABLE auth.users FROM %I', runtime_role);
    EXECUTE format('GRANT SELECT (id) ON TABLE auth.users TO %I', runtime_role);
EXCEPTION
    WHEN insufficient_privilege THEN
        RAISE WARNING 'could not narrow auth.users access for % to the id column; run the runbook statements as a privileged role', runtime_role;
END
$$;
