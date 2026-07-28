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

SELECT 'supabase security validation passed: RLS enabled, FORCE RLS disabled, and present Data API roles have no public object privileges' AS result;
SQL
