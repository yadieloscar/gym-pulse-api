# Data Model: Least-privilege runtime database role

No table or column changes. Migration 026 changes who can reach the existing schema.

## Roles

| Role | Connection | Attributes | Privileges |
|---|---|---|---|
| Owner (`gympulse` locally, `postgres` on Supabase) | `DATABASE_MIGRATE_URL` | Unchanged | Owns all tables; runs migrations only |
| `gym_pulse_api` | `DATABASE_URL`, `DATABASE_LOCK_URL` | `NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS`; `LOGIN` only when an operator or `scripts/ci/runtime-role.sql` adds it | `USAGE` on `public`, `auth`; `SELECT, INSERT, UPDATE, DELETE` on the 30 application tables; `SELECT` on `auth.users`; default DML on future owner-created tables in `public` |

Explicitly absent: `CREATE` on `public`, any privilege on `schema_migrations`, `TRUNCATE`,
`REFERENCES`, `TRIGGER`, ownership of any object, membership in any other role.

## Policies

One per application table, created by 026 after `DROP POLICY IF EXISTS`:

```sql
CREATE POLICY gym_pulse_api_access ON public.<table>
    AS PERMISSIVE FOR ALL TO gym_pulse_api USING (true) WITH CHECK (true);
```

Tables (30): `workout_templates`, `exercises`, `day_logs`, `exercise_overrides`, `user_settings`,
`user_profiles`, `body_weights`, `exercise_catalog`, `weekly_plans`, `plan_overrides`, `set_logs`,
`training_profiles`, `starter_programs`, `starter_workouts`, `starter_exercises`, `programs`,
`program_workouts`, `program_exercises`, `scheduled_workouts`, `scheduled_sets`,
`workout_sessions`, `day_participation`, `idempotency_records`, `legacy_adoptions`,
`sport_activities`, `criteria_training_blocks`, `criteria_training_stages`,
`criteria_training_exposures`, `criteria_training_transitions`, `health_exports`.

RLS flags are unchanged: enabled, not forced, no policy for `anon`, `authenticated`, or
`service_role`.

## Migration behavior

- **Up (026)**: idempotent; creates the role when absent, grants, recreates policies, sets default
  privileges, attempts the `auth` grants with a warning on `insufficient_privilege`, then raises if
  any public table other than `schema_migrations` lacks the policy or any of the four DML
  privileges.
- **Down**: drops the policies, revokes table and schema privileges and default privileges, attempts
  the `auth` revokes with a warning, and leaves the role in place.
- **Existing data**: untouched. **Mixed versions**: an API image before this change still connects
  as the owner and is unaffected by 026 being applied.
