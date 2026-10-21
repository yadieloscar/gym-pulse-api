#!/usr/bin/env bash

set -euo pipefail

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "supabase security validation failed: DATABASE_URL is required" >&2
  exit 1
fi
if ! command -v psql >/dev/null 2>&1; then
  echo "supabase security validation failed: psql is required" >&2
  exit 1
fi

psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 <<'SQL'
DO $$
DECLARE
    unsecured_tables TEXT;
    forced_tables TEXT;
    data_api_role TEXT;
    exposed_objects TEXT;
    exposed_defaults TEXT;
BEGIN
    SELECT string_agg(format('%I.%I', n.nspname, c.relname), ', ' ORDER BY c.relname)
    INTO unsecured_tables
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public'
      AND c.relkind IN ('r', 'p')
      AND c.relname <> 'schema_migrations'
      AND NOT c.relrowsecurity;

    IF unsecured_tables IS NOT NULL THEN
        RAISE EXCEPTION 'public application tables without RLS: %', unsecured_tables;
    END IF;

    SELECT string_agg(format('%I.%I', n.nspname, c.relname), ', ' ORDER BY c.relname)
    INTO forced_tables
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = 'public'
      AND c.relkind IN ('r', 'p')
      AND c.relforcerowsecurity;

    IF forced_tables IS NOT NULL THEN
        RAISE EXCEPTION 'public tables unexpectedly FORCE RLS: %', forced_tables;
    END IF;

    FOR data_api_role IN
        SELECT rolname
        FROM pg_roles
        WHERE rolname IN ('anon', 'authenticated', 'service_role')
    LOOP
        SELECT string_agg(object_name, ', ' ORDER BY object_name)
        INTO exposed_objects
        FROM (
            SELECT format('table %I.%I', n.nspname, c.relname) AS object_name
            FROM pg_class c
            JOIN pg_namespace n ON n.oid = c.relnamespace
            WHERE n.nspname = 'public'
              AND c.relkind IN ('r', 'p')
              AND (
                  has_table_privilege(data_api_role, c.oid, 'SELECT')
                  OR has_table_privilege(data_api_role, c.oid, 'INSERT')
                  OR has_table_privilege(data_api_role, c.oid, 'UPDATE')
                  OR has_table_privilege(data_api_role, c.oid, 'DELETE')
                  OR has_table_privilege(data_api_role, c.oid, 'TRUNCATE')
                  OR has_table_privilege(data_api_role, c.oid, 'REFERENCES')
                  OR has_table_privilege(data_api_role, c.oid, 'TRIGGER')
              )
            UNION ALL
            SELECT format('sequence %I.%I', n.nspname, c.relname)
            FROM pg_class c
            JOIN pg_namespace n ON n.oid = c.relnamespace
            WHERE n.nspname = 'public'
              AND c.relkind = 'S'
              AND (
                  has_sequence_privilege(data_api_role, c.oid, 'USAGE')
                  OR has_sequence_privilege(data_api_role, c.oid, 'SELECT')
                  OR has_sequence_privilege(data_api_role, c.oid, 'UPDATE')
              )
            UNION ALL
            SELECT format('function %I.%I(%s)', n.nspname, p.proname, pg_get_function_identity_arguments(p.oid))
            FROM pg_proc p
            JOIN pg_namespace n ON n.oid = p.pronamespace
            WHERE n.nspname = 'public'
              AND has_function_privilege(data_api_role, p.oid, 'EXECUTE')
        ) exposed;

        IF exposed_objects IS NOT NULL THEN
            RAISE EXCEPTION 'Data API role % retains access to: %', data_api_role, exposed_objects;
        END IF;

        SELECT string_agg(
            format(
                '%I default %s privilege by %s',
                owner_role.rolname,
                privilege.privilege_type,
                CASE defaults.defaclobjtype
                    WHEN 'r' THEN 'table'
                    WHEN 'S' THEN 'sequence'
                    WHEN 'f' THEN 'function'
                    ELSE defaults.defaclobjtype::TEXT
                END
            ),
            ', '
        )
        INTO exposed_defaults
        FROM pg_default_acl defaults
        JOIN pg_roles owner_role ON owner_role.oid = defaults.defaclrole
        LEFT JOIN pg_namespace namespace ON namespace.oid = defaults.defaclnamespace
        CROSS JOIN LATERAL aclexplode(defaults.defaclacl) privilege
        JOIN pg_roles grantee_role ON grantee_role.oid = privilege.grantee
        WHERE (defaults.defaclnamespace = 0 OR namespace.nspname = 'public')
          AND owner_role.rolname IN (current_user, 'postgres')
          AND grantee_role.rolname = data_api_role;

        IF exposed_defaults IS NOT NULL THEN
            RAISE EXCEPTION 'Data API role % retains default access: %', data_api_role, exposed_defaults;
        END IF;
    END LOOP;

    SELECT string_agg(format('%I default function EXECUTE to PUBLIC', owner_role.rolname), ', ')
    INTO exposed_defaults
    FROM pg_roles owner_role
    CROSS JOIN LATERAL aclexplode(
        COALESCE(
            (
                SELECT defaults.defaclacl
                FROM pg_default_acl defaults
                WHERE defaults.defaclrole = owner_role.oid
                  AND defaults.defaclnamespace = 0
                  AND defaults.defaclobjtype = 'f'
            ),
            acldefault('f', owner_role.oid)
        )
    ) privilege
    WHERE owner_role.rolname IN (current_user, 'postgres')
      AND privilege.grantee = 0
      AND privilege.privilege_type = 'EXECUTE';

    IF exposed_defaults IS NOT NULL THEN
        RAISE EXCEPTION 'PUBLIC retains default function execution: %', exposed_defaults;
    END IF;
END
$$;

-- Least-privilege runtime role (migration 026). The API connects as
-- gym_pulse_api for requests, so it must hold DML plus a permissive policy on
-- every application table and nothing that amounts to DDL or ownership.
DO $$
DECLARE
    runtime_role CONSTANT TEXT := 'gym_pulse_api';
    role_row pg_roles%ROWTYPE;
    findings TEXT;
BEGIN
    SELECT * INTO role_row FROM pg_roles WHERE rolname = runtime_role;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'runtime role % does not exist; migration 026 has not been applied', runtime_role;
    END IF;

    IF role_row.rolsuper OR role_row.rolbypassrls OR role_row.rolcreaterole
        OR role_row.rolcreatedb OR role_row.rolreplication THEN
        RAISE EXCEPTION 'runtime role % holds a privileged attribute (super=% bypassrls=% createrole=% createdb=% replication=%)',
            runtime_role, role_row.rolsuper, role_row.rolbypassrls, role_row.rolcreaterole,
            role_row.rolcreatedb, role_row.rolreplication;
    END IF;

    -- Membership would inherit the owner's or a superuser's rights.
    SELECT string_agg(r.rolname, ', ' ORDER BY r.rolname)
    INTO findings
    FROM pg_roles r
    WHERE r.rolname <> runtime_role
      AND r.rolname NOT LIKE 'pg\_%'
      AND pg_has_role(runtime_role, r.oid, 'MEMBER');
    IF findings IS NOT NULL THEN
        RAISE EXCEPTION 'runtime role % is a member of: %', runtime_role, findings;
    END IF;

    -- Ownership of any relation, schema, or function is DDL by another name.
    SELECT string_agg(owned, ', ' ORDER BY owned)
    INTO findings
    FROM (
        SELECT format('%I.%I', n.nspname, c.relname) AS owned
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE c.relowner = role_row.oid
        UNION ALL
        SELECT format('schema %I', n.nspname)
        FROM pg_namespace n
        WHERE n.nspowner = role_row.oid
        UNION ALL
        SELECT format('function %I.%I', n.nspname, p.proname)
        FROM pg_proc p
        JOIN pg_namespace n ON n.oid = p.pronamespace
        WHERE p.proowner = role_row.oid
    ) owned_objects;
    IF findings IS NOT NULL THEN
        RAISE EXCEPTION 'runtime role % owns: %', runtime_role, findings;
    END IF;

    IF has_schema_privilege(runtime_role, 'public', 'CREATE') THEN
        RAISE EXCEPTION 'runtime role % can CREATE in schema public', runtime_role;
    END IF;
    IF NOT has_schema_privilege(runtime_role, 'public', 'USAGE') THEN
        RAISE EXCEPTION 'runtime role % lacks USAGE on schema public', runtime_role;
    END IF;

    -- Every application table: exactly DML, nothing more, and a permissive
    -- policy addressed to the role so it can see rows without owning them.
    SELECT string_agg(problem, '; ' ORDER BY problem)
    INTO findings
    FROM (
        SELECT format('%I.%I lacks DML', n.nspname, c.relname) AS problem
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public'
          AND c.relkind IN ('r', 'p')
          AND c.relname <> 'schema_migrations'
          AND NOT (
              has_table_privilege(runtime_role, c.oid, 'SELECT')
              AND has_table_privilege(runtime_role, c.oid, 'INSERT')
              AND has_table_privilege(runtime_role, c.oid, 'UPDATE')
              AND has_table_privilege(runtime_role, c.oid, 'DELETE')
          )
        UNION ALL
        SELECT format('%I.%I grants more than DML', n.nspname, c.relname)
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public'
          AND c.relkind IN ('r', 'p')
          AND (
              has_table_privilege(runtime_role, c.oid, 'TRUNCATE')
              OR has_table_privilege(runtime_role, c.oid, 'REFERENCES')
              OR has_table_privilege(runtime_role, c.oid, 'TRIGGER')
          )
        UNION ALL
        SELECT format('%I.%I is reachable by the runtime role', n.nspname, c.relname)
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public'
          AND c.relkind IN ('r', 'p')
          AND c.relname = 'schema_migrations'
          AND (
              has_table_privilege(runtime_role, c.oid, 'SELECT')
              OR has_table_privilege(runtime_role, c.oid, 'INSERT')
              OR has_table_privilege(runtime_role, c.oid, 'UPDATE')
              OR has_table_privilege(runtime_role, c.oid, 'DELETE')
          )
        UNION ALL
        SELECT format('%I.%I has no permissive ALL policy for the runtime role', n.nspname, c.relname)
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public'
          AND c.relkind IN ('r', 'p')
          AND c.relname <> 'schema_migrations'
          AND NOT EXISTS (
              SELECT 1
              FROM pg_policy p
              WHERE p.polrelid = c.oid
                AND p.polpermissive
                AND p.polcmd = '*'
                AND role_row.oid = ANY (p.polroles)
          )
    ) problems;
    IF findings IS NOT NULL THEN
        RAISE EXCEPTION 'runtime role % table provisioning: %', runtime_role, findings;
    END IF;

    -- Identity verification reads auth.users and nothing else in auth.
    IF NOT has_table_privilege(runtime_role, 'auth.users', 'SELECT') THEN
        RAISE EXCEPTION 'runtime role % cannot read auth.users', runtime_role;
    END IF;
    IF has_table_privilege(runtime_role, 'auth.users', 'INSERT')
        OR has_table_privilege(runtime_role, 'auth.users', 'UPDATE')
        OR has_table_privilege(runtime_role, 'auth.users', 'DELETE') THEN
        RAISE EXCEPTION 'runtime role % can write auth.users', runtime_role;
    END IF;
END
$$;

SELECT 'supabase security validation passed: RLS enabled, FORCE RLS disabled, present Data API roles have no public object privileges, and gym_pulse_api holds DML plus policies without DDL or ownership' AS result;
SQL
