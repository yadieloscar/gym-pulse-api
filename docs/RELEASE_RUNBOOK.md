# API Release Runbook

This runbook governs production releases of the GymPulse API. A release is not complete until the
mobile release checklist has verified the deployed API.

## Required access

- Railway production and staging projects
- Supabase production and staging projects
- PostgreSQL backup/restore access
- Production DNS
- GitHub Actions and repository secrets

Never paste credentials, JWTs, database URLs, service-role keys, or production user data into issues,
logs, commits, or release notes.

## Pre-deployment gate

- [ ] The release commit is identified and reviewed.
- [ ] `docs/CONTRACTS.md` matches observable behavior.
- [ ] All migrations are forward-compatible with the currently released app.
- [ ] Migration 017 preflight finds no legacy rows whose `user_id` is missing
      from `auth.users`; reconcile any orphan in staging before deployment.
- [ ] A current database backup exists and restore access is verified.
- [ ] The deployed runtime resolves `ENVIRONMENT=production`; the image defaults
      to production, and any platform override must preserve that value.
- [ ] Staging uses production-equivalent JWKS, issuer, audience, CORS, and storage configuration.
- [ ] `SUPABASE_URL` is the intended project; issuer and JWKS are derived from it
      (any legacy explicit overrides must match it exactly).
- [ ] `DATABASE_LOCK_URL` uses a direct PostgreSQL or Supavisor session-mode
      connection (normally port 5432), never transaction mode on port 6543.
- [ ] The platform's pre-deploy command is `./migrate` (Railway: `preDeployCommand`), and only
      that step has `DATABASE_MIGRATE_URL`: the schema-owner connection (`postgres` on Supabase),
      direct or session-mode, with TLS. A staging or production server never migrates and never
      opens that connection; see "Migration job" below. `DATABASE_URL` and
      `DATABASE_LOCK_URL` connect as `gym_pulse_api`, never as the owner. Staging and production
      refuse to start when `DATABASE_URL` uses the same role as `DATABASE_MIGRATE_URL`, `postgres`,
      or `supabase_admin`, unless `ALLOW_OWNER_DATABASE_URL=true`. That override exists only
      for the role bootstrap and rollback below; `ALLOW_OWNER_DATABASE_URL` is unset in
      steady state.
- [ ] `ALLOWED_ORIGINS` lists exact HTTPS origins. Wildcards such as `https://*.example.com`
      are refused at startup.
- [ ] `REQUIRE_SUPABASE_DATA_API_ROLES=true ./scripts/validate-supabase-security.sh` passes
      against staging with the owner connection, proving `gym_pulse_api` holds only DML plus
      policies and that `anon`, `authenticated`, and `service_role` exist and hold nothing.
      Without the variable the Data API checks silently cover only roles that exist.
- [ ] Database capacity accounts for both the main query pool and the separate
      `DATABASE_LOCK_MAX_CONNS` advisory-lock pool.
- [ ] `golangci-lint run` passes.
- [ ] `go test ./...` passes.
- [ ] `go test -race ./...` passes.
- [ ] `govulncheck ./...` has no unaccepted reachable finding.
- [ ] `./scripts/smoke-toggle.sh` passes.
- [ ] The production container scan has no unaccepted critical or high finding.
- [ ] Direct Supabase Data API access to application tables is denied for `anon` and
      `authenticated`.
- [ ] The Supabase Auth dashboard settings below match this runbook (they are not
      enforced by code or migrations).

## Supabase Auth dashboard settings

The API verifies tokens; Supabase Auth issues them and owns sign-up, password, and email policy.
Review these in **Authentication → Settings** (and **Rate Limits**) for staging and production on
every release, and record the values in the release notes:

- [ ] **Password minimum length** is 8 or more and the required-characters rule is at least
      "letters and digits". The app enforces 8 on sign-up and reset; the project must not
      accept shorter passwords through any other client.
- [ ] **Leaked password protection** (HaveIBeenPwned check) is enabled.
- [ ] **Confirm email** is enabled, so an unverified address never receives a session.
- [ ] **Email OTP / link expiry** is 3600 seconds or less.
- [ ] **JWT expiry** is 3600 seconds or less. A shorter value bounds how long a signed-out
      or compromised access token stays usable; the app refreshes silently. The API checks
      only that the token's user still exists in `auth.users`, not `banned_until`, so a
      **banned** user keeps API access until their current access token expires. To cut a
      user off immediately, delete them in Supabase Auth; the next request is `401`.
- [ ] **Refresh token rotation** is on with **reuse detection**; reuse interval at the default
      10 seconds.
- [ ] **Auth rate limits** for sign-in, sign-up, password reset, and token refresh are at or
      below the defaults, and **CAPTCHA** is considered if sign-up abuse is observed.
- [ ] **Redirect URLs** contain only the app's registered links (see the mobile release
      checklist) and no wildcard.
- [ ] **JWT signing keys**: the project uses an asymmetric key (ES256 or RS256) served from
      JWKS; the legacy HMAC secret is not used by any release environment.

## Private pre-launch access

Until the app opens to other people, keep each hosted environment private to its owner:

- [ ] In Supabase **Authentication → Settings**, turn off **Allow new users to sign up** after
      creating the owner's account.
- [ ] Set `ALLOWED_USER_IDS` on the Railway service to the owner's Supabase user id (from
      **Authentication → Users**). A wrong or malformed id stops startup; an unlisted account
      gets `403 ACCOUNT_NOT_ALLOWED`.
- [ ] After deploy, confirm the owner's app loads data, and that a token for any other test
      user gets 403 on `GET /api/v1/settings`.

Clearing `ALLOWED_USER_IDS` opens the API to every verified Supabase user; do that only together
with re-enabling sign-up at launch.

## Runtime role rollout (migration 026)

The API runs migrations as the schema owner and serves requests as `gym_pulse_api`, a role with
`SELECT`, `INSERT`, `UPDATE`, `DELETE` on application tables and a permissive RLS policy on each.
It has no DDL, owns nothing, and cannot bypass RLS. Roll it out in this order, in staging first:

1. **Create the role** in the SQL editor as the owner, with its own credential. Never put the
   credential in a migration, issue, or log:

   ```sql
   CREATE ROLE gym_pulse_api LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
   \password gym_pulse_api
   ```

   (Or set the password with `ALTER ROLE gym_pulse_api PASSWORD '...'` entered interactively.)
   If the role already exists because migration 026 created it `NOLOGIN`, add `LOGIN` and a
   credential with `ALTER ROLE`.
2. **Grant** by deploying the release that contains migration 026 with the current configuration
   (`DATABASE_URL` still the owner, `DATABASE_MIGRATE_URL` set to the same owner URL, and
   `ALLOW_OWNER_DATABASE_URL=true` because release environments otherwise refuse an owner
   request connection). The
   migration grants schema usage, table DML, default DML for future tables, and creates the
   `gym_pulse_api_access` policy on every application table. Migrations 026 and 039 together
   leave the role with `SELECT` on `auth.users.id` only, never the emails or password hashes
   in the other columns. If the log shows a warning that either statement failed, run
   `GRANT USAGE ON SCHEMA auth TO gym_pulse_api; REVOKE SELECT ON auth.users FROM gym_pulse_api;
   GRANT SELECT (id) ON auth.users TO gym_pulse_api;`
   as a role that owns `auth.users` (`supabase_auth_admin` or the dashboard owner). If
   `auth.users` has row-level security enabled in the hosted project, add a read policy for
   `gym_pulse_api` or the identity check denies every request.
3. **Verify the grants** with `REQUIRE_SUPABASE_DATA_API_ROLES=true
   ./scripts/validate-supabase-security.sh` using the owner URL.
4. **Switch the request connections**: set `DATABASE_URL` and `DATABASE_LOCK_URL` to
   `gym_pulse_api` credentials (same hosts and `sslmode`), keep `DATABASE_MIGRATE_URL` on the
   owner, remove `ALLOW_OWNER_DATABASE_URL`, and redeploy the same image.
5. **Verify**: `/health`, `/ready`, an authenticated read and write, account deletion, and no
   `permission denied` or `42501` in the logs. `SELECT usename, count(*) FROM pg_stat_activity
   GROUP BY 1` must show request traffic as `gym_pulse_api` and no idle owner connections after
   startup.

Rollback is a configuration change: point `DATABASE_URL` and `DATABASE_LOCK_URL` back at the
owner and set `ALLOW_OWNER_DATABASE_URL=true`. Do not roll the migration down while any deployment still connects as `gym_pulse_api`.

## Migration job (`cmd/migrate`)

The image ships two binaries: `./server` and `./migrate`. `./migrate` reads only `ENVIRONMENT`,
`LOG_LEVEL`, and `DATABASE_MIGRATE_URL`, applies pending migrations, logs the resulting version,
and exits non-zero on any failure, so the platform aborts the deploy before a new server starts.

- **Railway**: set the service's pre-deploy command to `./migrate` and give the service
  `DATABASE_MIGRATE_URL`. Pre-deploy shares the service's variables, so the owner URL is visible to
  the server container, but a staging or production server never opens it: it does not migrate and
  uses the value only to refuse an owner request connection. To keep the credential out of the
  server container entirely, run `./migrate` from a separate one-off service instead.
- **Startup log**: a release server logs `skipping startup migrations`; development and test
  servers still migrate at startup with `DATABASE_MIGRATE_URL` (default `DATABASE_URL`).
- **Failure**: a failed migration leaves `schema_migrations` dirty because each file runs as one
  transaction and rolls back, but golang-migrate records the attempt. Fix the cause, then clear the
  flag as the owner with `UPDATE schema_migrations SET dirty = false, version = <last good>` and
  rerun `./migrate`. Never edit an applied migration file; add a new one.

## Writing a migration

`internal/dbmigrate/lint_test.go` (in `go test ./...`) and the CI round-trip job enforce these for
migration 043 and later; 001–042 predate them and are never edited.

- Fetch `origin/main` first and take the next number; versions are contiguous from 043.
- Every up has a down that fully reverses it. CI applies all migrations to a scratch database,
  rolls them all back, re-applies them, and requires an identical schema.
- Each file runs as one transaction: no `BEGIN`/`COMMIT`, no `CREATE INDEX CONCURRENTLY`.
- Re-runnable: `INSERT ... ON CONFLICT`, `IF NOT EXISTS` on tables, indexes, schemas, and
  columns, `IF EXISTS` on drops, `CREATE OR REPLACE` for functions and views, and a
  `DROP ... IF EXISTS` of the same name before `CREATE POLICY` or `ADD CONSTRAINT`. Renames and
  types go inside a `DO` block that checks the catalog. CI re-executes each strict migration on the
  migrated database inside a rolled-back transaction.
- A new table ends with `SELECT gym_pulse_private.secure_table('public.<table>');` (migration 043),
  which enables RLS, revokes the Supabase Data API roles, and provisions `gym_pulse_api`.
- Assert what the migration promises: when it backfills or transforms data, end with a `DO` block
  that `RAISE EXCEPTION`s if the postcondition does not hold (see 026), so a bad migration rolls
  back instead of shipping.

## Deployment order

1. Record the release commit and current production image.
2. Confirm the latest backup completed successfully.
3. Apply additive migrations 016 and 017 in staging. Migration 017 deliberately
   fails on orphaned legacy ownership rows; investigate and reconcile them
   rather than bypassing constraint validation.
4. Verify direct Supabase access is denied and the Go API smoke flow still succeeds.
5. Deploy the release image to staging.
6. Verify `/health`, `/ready`, authentication, avatar upload/deletion, account
   deletion, and that a deleted identity's old JWT is rejected on every
   non-deletion route while an idempotent deletion-cleanup retry remains allowed.
   For avatar replacement, verify the database URL alternates between
   `<uuid>/avatar-0` and `<uuid>/avatar-1`, a simulated profile-write failure
   leaves the prior URL and bytes active while retaining the bounded inactive
   upload, and a delayed commit after a negative confirmation still points to
   existing bytes. Verify normal and positively confirmed replacements issue no
   Storage DELETE and both slots remain readable; generic profile updates can
   legally restore a previously issued `avatar_url`. Only account deletion
   removes the legacy path plus both slots. Supabase Storage deletion must
   receive explicit full object paths; do not rely on recursive folder
   deletion.
7. Apply the same migrations to production (the `./migrate` pre-deploy step does this).
8. Deploy the exact staging-accepted image to production, following the runtime role rollout
   order above the first time migration 026 ships.
9. Verify `/health` and `/ready` before enabling dependent app rollout.
10. Run a bounded production smoke with a dedicated test account and remove it afterward.

Do not deploy the mobile app before the API acceptance checks pass.

## Observation gate

Observe the production service before app promotion:

- readiness and restart count;
- HTTP 5xx and 429 rate;
- authentication failures;
- database-pool utilization and acquisition latency;
- avatar storage failures;
- account-deletion failures;
- p95 request latency.

Logs must include a request identifier but not request bodies, tokens, email addresses, workout
notes, or service credentials.

## Recovering a failed migration

A failed migration leaves `schema_migrations` marked dirty, and every later start logs
`Dirty database version N. Fix and force version.` Each migration file runs as one implicit
transaction, so a failure applies nothing from that file. To recover:

1. Read the first error in the log; it names the statement and `SQLSTATE`. Fix its cause in a new
   release (or in the database, for a missing privilege).
2. In the SQL editor, as the owner, confirm the state and step back one version:

   ```sql
   SELECT version, dirty FROM schema_migrations;   -- expect N, true
   UPDATE schema_migrations SET version = N - 1, dirty = false;   -- write the number, e.g. 5
   ```

3. Redeploy the fixed release; it re-runs migration N and everything after it.

Before 2026-11-16, migration 006 failed on hosted Supabase with `42501 permission denied for schema
auth` (dirty version 6), because it created a local `auth.users` stub. It now creates the stub only
when `auth.users` is missing.

## Rollback and roll-forward

- Prefer roll-forward for database security and contract fixes.
- Never roll back to a version or grant state that re-exposes application tables.
- A previous API image may be restored only when it remains compatible with applied migrations.
- Restore a database backup only for confirmed data corruption and only after stopping writes.
- If deletion is partially complete, preserve evidence, use the idempotent cleanup retry, and
  complete deletion operationally before closing the incident.

## Release evidence

Record:

- API commit and image digest;
- migrations applied;
- `./scripts/validate-supabase-security.sh` output showing `gym_pulse_api` provisioned with DML
  and policies only;
- validation that all six legacy ownership foreign keys are validated
  `ON DELETE CASCADE` constraints;
- staging and production smoke results;
- vulnerability and container-scan results;
- start/end timestamps;
- approver and rollback owner;
- linked mobile release commit and build identifiers.
