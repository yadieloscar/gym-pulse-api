# Research: Least-privilege runtime database role

## Decisions

### Explicit permissive policies, not `BYPASSRLS`, not FORCE RLS changes

- **Decision**: Keep RLS enabled and not forced on all 30 application tables. Migration 026 adds
  `CREATE POLICY gym_pulse_api_access ON public.<table> AS PERMISSIVE FOR ALL TO gym_pulse_api
  USING (true) WITH CHECK (true)` on each. The API keeps enforcing ownership in DAO SQL.
- **Rationale**: `gym_pulse_api` does not own the tables, so owner bypass no longer applies; a
  non-owner with no policy sees zero rows. A policy is the one mechanism the project owner can grant
  on hosted Supabase, it is visible in `pg_policies`, and it is per table, so a forgotten table fails
  closed and is caught by the static test and by the migration's own final assertion.
- **Rejected: `BYPASSRLS` on the runtime role**. `ALTER ROLE ... BYPASSRLS` needs a superuser;
  Supabase's `postgres` is not one. It would also make RLS decorative for the request path and hide
  a table created without a policy. `docs/DATABASE_DESIGN.md` already excluded it for this role.
- **Rejected: keep requests on the owner role**. That is the current state and the gap: every
  request-path SQL statement carries DDL and ownership rights.
- **Rejected: FORCE RLS**. It only changes the owner's behavior; migrations and seed statements run
  as the owner and must keep working. The existing validator still asserts FORCE RLS is off.

### Role creation inside the migration, credential outside it

- **Decision**: `CREATE ROLE gym_pulse_api NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION
  NOBYPASSRLS NOLOGIN` only when absent. Production pre-creates the role with `LOGIN` and a
  credential; the migration then finds it and only grants. Existing roles are never altered.
- **Rationale**: Local and CI databases must work with no operator step, and a migration must never
  contain a credential (the static test forbids the word `PASSWORD` in 026). Hosted `postgres` holds
  `CREATEROLE`, so the create succeeds there too if an operator skipped it, leaving a `NOLOGIN` role
  the runbook tells them to finish with `ALTER ROLE`.
- **Rejected**: Operator-only creation (CI and local smoke would need a side channel) and
  altering a pre-existing role from the migration (stripping `BYPASSRLS` needs a superuser; clobbering
  `LOGIN` would break a running deployment).

### Grants: explicit per table plus default privileges, never `GRANT ALL`

- **Decision**: `GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.<table>` for each listed table,
  `GRANT USAGE ON SCHEMA public`, `REVOKE CREATE ON SCHEMA public`, and `ALTER DEFAULT PRIVILEGES FOR
  ROLE <owner> IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES` for the eligible
  owners (`current_user`, `postgres`), mirroring migration 016. `schema_migrations` is excluded.
- **Rationale**: Default privileges keep a future table reachable even if its migration forgets the
  grant; the policy still has to be written, which the test enforces. There are no sequences,
  functions, or views in `public` today, so no other grants are needed; the validator will surface
  any future one because the Data API checks already enumerate them.

### `auth.users` read access with a guarded grant

- **Decision**: `GRANT USAGE ON SCHEMA auth` and `GRANT SELECT ON TABLE auth.users` inside a
  `BEGIN ... EXCEPTION WHEN insufficient_privilege` block that raises a `WARNING`.
- **Rationale**: `AuthUserDAO` runs `SELECT EXISTS (... FROM auth.users WHERE id = $1)` on every
  authenticated request. Locally the migrator owns `auth.users` (migration 006 creates it). On hosted
  Supabase `supabase_auth_admin` owns it and `postgres` may lack grant option, so the migration must
  not fail; the runbook names the manual grant and the validator fails until it exists. FK checks to
  `auth.users` run as the table owner and need no runtime privilege.

### Separate migration connection: `DATABASE_MIGRATE_URL`

- **Decision**: New config field, required in staging/production, defaulting to `DATABASE_URL` in
  development/test, validated with the same TLS rule as the other URLs and the same session-mode
  rule as the lock URL. `cmd/server/main.go` uses it only for `runMigrations` and runs migrations
  before any request pool opens.
- **Rationale**: golang-migrate serializes with a session-level `pg_advisory_lock`, which the
  Supavisor transaction endpoint breaks, so the lock-URL rule applies. Running migrations first means
  the request pools never open against a half-migrated schema.
- **Rejected**: Deriving the owner URL from `DATABASE_URL` with a different user (credentials differ,
  and a derived string hides a misconfiguration).

### CI smoke as the restricted role

- **Decision**: The CI compose mounts `scripts/ci/runtime-role.sql` into the PostgreSQL init
  directory so the login exists before migration 026, then boots the API with `DATABASE_URL` and
  `DATABASE_LOCK_URL` as `gym_pulse_api` and `DATABASE_MIGRATE_URL` as the owner. The security gate
  and the integration tests keep the owner connection.
- **Rationale**: This is the production rollout order (create, grant, deploy, verify) executed on
  every pull request, so a regression in grants or policies fails smoke rather than production.
