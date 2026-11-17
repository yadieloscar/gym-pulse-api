# Feature Specification: Least-privilege runtime database role

**Feature Branch**: `security/least-privilege-runtime-role`

**Feature ID**: `gympulse-least-privilege-runtime-role`

**Created**: 2026-10-21

**Status**: Approved

**Input**: Engineering-lead follow-up to the 2026-10 security sweep (PR #32): "Design and implement a least-privilege runtime database role for the API so migrations run as the owner role and requests run as a role holding only DML on application tables, with RLS semantics that still let the API read every user's rows."

## Clarifications

### Session 2026-10-21

- Q: How does a non-owner role see rows when RLS is enabled with no policies? → A: Through an explicit permissive policy per table addressed to the runtime role. `BYPASSRLS` is rejected because granting it requires a superuser, which hosted Supabase does not provide, and because it would hide a table created without a policy. FORCE RLS stays off; it is irrelevant to a non-owner and would only change the owner's own behavior.
- Q: Does the policy restrict rows by user? → A: No. The API already scopes every owned query to the authenticated UUID in SQL and that remains the ownership boundary. The policy is unconditional so the API can read any user's rows the way it does today.
- Q: Who creates the role? → A: Production operators create it with a login credential before the release that grants it. Local and CI databases get it created automatically, without login, by the migration when it is absent. The migration never carries a credential and never alters an existing role.
- Q: What about the identity table that Supabase owns? → A: The API needs read access to it to confirm a token's subject still exists. The migration attempts the grant and warns if the owner cannot make it; the release runbook names the manual grant and the validator proves the result.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Requests run without schema power (Priority: P1)

An operator deploys the API so that request traffic connects as a database role that can only read and change rows in application tables. A bug or injection in request handling can no longer create, alter, or drop tables, change grants, or read the migration ledger.

**Why this priority**: This is the last structural gap from the security sweep: today the request path owns every table and has full DDL.

**Independent Test**: Start the API with the request connection pointing at the restricted role and the migration connection at the owner. Every smoke flow passes, and a query as the restricted role for a `CREATE TABLE`, `ALTER TABLE`, or `SELECT FROM schema_migrations` is denied.

**Acceptance Scenarios**:

1. **Given** a database migrated to the current version, **When** the API starts with the restricted request role and the owner migration role, **Then** it becomes ready and every existing smoke flow passes unchanged.
2. **Given** the restricted role, **When** it attempts any DDL, truncation, trigger creation, or a read of the migration ledger, **Then** the database denies it.
3. **Given** the restricted role, **When** the API reads or writes any user's rows in any application table, **Then** the rows are reachable exactly as before.

---

### User Story 2 - Provisioning is repeatable and verifiable (Priority: P1)

An operator can apply the release to a fresh local database, a CI database, staging, and production and get the same role, grants, and policies without hand-written SQL, then prove the result with the existing security validator.

**Why this priority**: Grants that drift between environments are the usual way a least-privilege design silently becomes an owner connection again.

**Independent Test**: Apply migrations to a database where the role does not exist and to one where an operator already created it; both end with identical grants and policies. The validator passes on both and fails when any one grant, policy, or attribute is wrong.

**Acceptance Scenarios**:

1. **Given** a database without the role, **When** migrations run, **Then** the role exists without login, holds DML on every application table, and has a policy on each.
2. **Given** a database where the operator pre-created the role with login, **When** migrations run, **Then** the role keeps its login and credential and gains the same grants and policies.
3. **Given** a future migration that creates a table without a policy for the role, **When** the repository tests run, **Then** they fail before the migration ships.
4. **Given** a provisioned database, **When** the security validator runs, **Then** it reports the role has no DDL, ownership, superuser, bypass, or role-creation power and has DML plus a policy on every application table.

---

### User Story 3 - Configuration refuses an unsafe release (Priority: P2)

A release deployment that forgets the separate owner migration connection, or points it at a transaction-mode pooler or a plaintext connection, fails at startup with a clear error instead of running migrations through the request pool.

**Independent Test**: Start the binary in staging or production without the migration connection, with the pooler's transaction port, and without TLS; each refuses to start with a distinct message. Development and test keep working with only the request connection.

**Acceptance Scenarios**:

1. **Given** staging or production, **When** the migration connection is missing, **Then** startup fails and names the missing setting.
2. **Given** staging or production, **When** the migration connection uses the transaction-mode pooler port or lacks TLS, **Then** startup fails like the existing lock connection rules.
3. **Given** development or test, **When** only the request connection is set, **Then** migrations use it and startup succeeds.

### Edge Cases

- The owner cannot re-grant read access to the identity table: the migration finishes with a warning, the runbook names the manual grant, and the validator fails until it is made.
- A table exists in the database but is missing from the migration's list: the migration fails loudly rather than leaving the table unreachable for the role.
- Rolling the migration back while a deployment still connects as the restricted role would break requests; the runbook requires switching the connection back first.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Request traffic MUST run as a database role with only `SELECT`, `INSERT`, `UPDATE`, and `DELETE` on the application tables, `USAGE` on the application and identity schemas, and read access to the identity table.
- **FR-002**: The runtime role MUST NOT be a superuser, bypass row-level security, create roles or databases, own any object, create objects in the application schema, hold `TRUNCATE`, `REFERENCES`, or `TRIGGER`, or reach the migration ledger.
- **FR-003**: Every application table MUST keep RLS enabled and not forced and MUST carry one permissive policy for all commands addressed to the runtime role.
- **FR-004**: Migrations MUST run through a separate owner connection. Staging and production MUST require it explicitly, with the same session-mode and TLS rules as the lock connection; development and test MUST default it to the request connection.
- **FR-005**: The migration MUST create the role without login when absent, MUST never alter an existing role or contain a credential, and MUST be safe to apply where the hosted identity roles do not exist.
- **FR-006**: The repository MUST fail its tests when a migration creates an application table without provisioning it for the runtime role.
- **FR-007**: The security validator MUST assert FR-001 through FR-003 against a live database and fail when the role is missing.
- **FR-008**: Documentation MUST record the chosen RLS strategy, the rejected alternatives, and the rollout order: create role, grant, deploy with the new configuration, verify.

### Key Entities

- **Runtime role** `gym_pulse_api`: the request identity; DML only.
- **Owner connection**: the existing migration identity (`postgres` on Supabase); DDL at startup only.
- **Access policy** `gym_pulse_api_access`: one per application table, unconditional, addressed to the runtime role.

## Success Criteria *(mandatory)*

- **SC-001**: The full smoke suite passes with the API connected as the runtime role on a database migrated by the owner.
- **SC-002**: The security validator passes on that database and lists no DDL, ownership, or privileged attribute for the runtime role.
- **SC-003**: `go test ./...`, `golangci-lint run`, and the database-design gate pass.
- **SC-004**: No public API contract changes; `docs/CONTRACTS.md` is unchanged.

## Amendment A1: migration job, local least privilege, table helper (2026-11-16)

**Why:** the owner asked for a cleaner database setup around roles and migrations. Three gaps were
left after the first rollout: the long-running API process still holds the owner credential because
it migrates at startup; local development and the DAO suite connect as a superuser, so a missing
grant or policy surfaces only in CI smoke; and every table-creating migration copies about 25 lines
of grant and policy SQL.

### Clarifications

- Q: Where do release migrations run? → A: In a separate one-shot `migrate` binary shipped in the
  same image and run as the platform's pre-deploy step (Railway `preDeployCommand: ./migrate`). A
  failed migration fails the deploy before any new server starts.
- Q: Does the server still migrate at startup? → A: In development and test, yes, using
  `DATABASE_MIGRATE_URL` (defaulting to `DATABASE_URL`). In staging and production never: no
  release deployment exists yet, so there is nothing to keep compatible, and Railway's pre-deploy
  step shares the service's variables, so "migrate when the URL is set" would re-run on every boot.
- Q: Can the server check the schema version itself? → A: No. The runtime role deliberately cannot
  read `schema_migrations` (FR-002); the deploy-blocking pre-deploy step is the guarantee.
  **Superseded by Amendment A2 (2026-11-17):** the server checks through an owner function.
- Q: Where does the table helper live? → A: In a private schema `gym_pulse_private` that no client
  role can use, so the Supabase Data API never exposes it. It runs as the caller (the migrator).
- Q: Does the whole DAO suite move to the runtime role? → A: No. Its fixtures insert into
  `auth.users` and some tests run migrations, which need the owner. A dedicated integration test
  connects as the runtime role and proves every application table is reachable and DDL is denied.

### Requirements

- **FR-A1-001:** `cmd/migrate` runs pending up migrations with `DATABASE_MIGRATE_URL` and exits
  non-zero on failure. In staging and production it applies the same session-capable and TLS
  rules as FR-004. It needs no other configuration.
- **FR-A1-002:** In staging and production the server never migrates, and `DATABASE_MIGRATE_URL`
  becomes optional for it. When set it is still validated and used only for the owner-connection
  check; when unset the server still refuses a request connection as `postgres` or `supabase_admin`
  (unless `ALLOW_OWNER_DATABASE_URL=true`).
- **FR-A1-003:** The local Docker stack creates the `gym_pulse_api` login at database init and runs
  the API's request and lock pools as that role, with migrations on the owner. `.env.example`
  shows the same split.
- **FR-A1-004:** An integration test connected as the runtime role reads every public application
  table and `auth.users.id`, and is denied `CREATE TABLE`, `TRUNCATE`, and `schema_migrations`.
  CI runs it.
- **FR-A1-005:** Migration 044 adds `gym_pulse_private.secure_table(regclass)`: enable RLS, revoke
  every Data API role, grant DML and the `gym_pulse_api_access` policy to the runtime role.
  `EXECUTE` is revoked from `PUBLIC`. New table migrations call it instead of copying the blocks,
  and the repository test accepts the call as provisioning.

- **FR-A1-006:** Migrations from 043 on are re-runnable and single-transaction: `INSERT` has
  `ON CONFLICT`; tables, indexes, schemas, and columns use `IF NOT EXISTS`; drops use `IF EXISTS`;
  functions and views use `CREATE OR REPLACE`; `CREATE POLICY` and `ADD CONSTRAINT` follow a
  `DROP ... IF EXISTS` of the same name; no `BEGIN`/`COMMIT` or `CONCURRENTLY`; a new table calls
  the FR-A1-005 helper. A static test enforces this in `go test ./...`. 001–042 stay as applied.
- **FR-A1-007:** Every version has exactly one up and one down file, and versions from 043 are
  contiguous.
- **FR-A1-008:** CI applies every migration to a scratch database, re-applies (no change), rolls all
  back to an empty schema, re-applies, and requires the catalog snapshot (columns, constraints,
  indexes, policies, RLS flags, grants, functions) to match the first apply. It also re-executes
  each strict migration against the migrated database in a rolled-back transaction.

### Acceptance

- `go test ./...` covers the config changes and the helper's static checks.
- The runtime-role integration test passes against a migrated database in CI.
- The CI smoke stack runs the image's `./migrate` as its own service, the API starts only after
  it exits 0, and `validate-supabase-security.sh` still passes.
- No public contract change; `docs/CONTRACTS.md` is unchanged.

## Amendment A2: the server refuses a schema that is behind (2026-11-17)

**Why:** A1 made the pre-deploy step the only guarantee that the schema matches the build, and
that step is a platform setting outside the repository (task T022). On 2026-11-17 it was not set
on Railway. API #67 deployed, the server logged `skipping startup migrations`, and every program
request returned 500 because a column from migration 045 did not exist. The owner then set the
pre-deploy command by hand and asked for the schema setup to be reassessed.

### Clarifications

- Q: How does a role that cannot read `schema_migrations` learn the version? → A: Through
  `gym_pulse_private.schema_version()` (migration 048), a `SECURITY DEFINER` function owned by the
  schema owner that returns only `version` and `dirty`. FR-002 holds: the role still has no
  privilege on the table.
- Q: What does the build compare against? → A: The highest version among the migration files in
  the image (`migrations/`, the same files `./migrate` applies).
- Q: Is a database newer than the build accepted? → A: Yes. `./migrate` runs before the new server
  starts while the previous build still serves, and a rollback runs an older build on the newer
  schema; migrations are required to stay compatible with the released build.
- Q: Does this replace the pre-deploy step? → A: No. The server still never migrates in staging
  and production. The check turns a missing or failed pre-deploy step into a failed deploy (the
  previous deployment keeps serving) instead of a running server that fails per request.
- Q: Is the pre-deploy command committed as `railway.json`? → A: No. Railway's documentation marks
  Config as Code as deprecated, with existing files honored only until 2026-12-01 (read
  2026-11-17). The setting stays a dashboard step in the runbook; this amendment is the guard.

### Requirements

- **FR-A2-001:** Before serving, in every environment, the server reads the applied schema version
  and exits non-zero with one clear log line when the version is lower than the newest migration
  the build ships, when the ledger is dirty, when no migration is applied, or when the version
  function does not exist yet.
- **FR-A2-002:** A schema newer than the build is accepted.
- **FR-A2-003:** The runtime role gains `EXECUTE` on `gym_pulse_private.schema_version()` only.
  `PUBLIC` and the Supabase Data API roles cannot call it, and the role's privileges on
  `public.schema_migrations` are unchanged (none).
- **FR-A2-004:** No public contract change; `docs/CONTRACTS.md` is unchanged.

### Acceptance

- Unit tests cover current, newer, behind, dirty, empty-ledger, missing-function, and unreadable
  migration-file cases.
- An integration test stages a database at version 47, at the newest version, one version short,
  and dirty, and gets refusal, acceptance, refusal, refusal.
- The runtime-role integration test verifies the schema as `gym_pulse_api` and still shows
  `SELECT version FROM public.schema_migrations` denied.
- `validate-supabase-security.sh` asserts the function's privileges.

## Assumptions

- Hosted Supabase's `postgres` role holds `CREATEROLE` and owns the application tables, so it can create the role locally when absent and grant on every table.
- The identity table in hosted projects may need a manual grant or policy by its owning role; this is an operator step, not an application behavior.
