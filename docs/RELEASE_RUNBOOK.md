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
- [ ] `DATABASE_MIGRATE_URL` is the schema-owner connection (`postgres` on Supabase),
      direct or session-mode, with TLS. `DATABASE_URL` and `DATABASE_LOCK_URL` connect
      as `gym_pulse_api`, never as the owner.
- [ ] `./scripts/validate-supabase-security.sh` passes against staging with the owner
      connection, proving `gym_pulse_api` holds only DML plus policies.
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
   (`DATABASE_URL` still the owner, `DATABASE_MIGRATE_URL` set to the same owner URL). The
   migration grants schema usage, table DML, default DML for future tables, and creates the
   `gym_pulse_api_access` policy on every application table. It also attempts
   `GRANT SELECT ON auth.users`; if the log shows the warning that this grant failed, run
   `GRANT USAGE ON SCHEMA auth TO gym_pulse_api; GRANT SELECT ON auth.users TO gym_pulse_api;`
   as a role that owns `auth.users` (`supabase_auth_admin` or the dashboard owner). If
   `auth.users` has row-level security enabled in the hosted project, add a read policy for
   `gym_pulse_api` or the identity check denies every request.
3. **Verify the grants** with `./scripts/validate-supabase-security.sh` using the owner URL.
4. **Switch the request connections**: set `DATABASE_URL` and `DATABASE_LOCK_URL` to
   `gym_pulse_api` credentials (same hosts and `sslmode`), keep `DATABASE_MIGRATE_URL` on the
   owner, and redeploy the same image.
5. **Verify**: `/health`, `/ready`, an authenticated read and write, account deletion, and no
   `permission denied` or `42501` in the logs. `SELECT usename, count(*) FROM pg_stat_activity
   GROUP BY 1` must show request traffic as `gym_pulse_api` and no idle owner connections after
   startup.

Rollback is a configuration change: point `DATABASE_URL` and `DATABASE_LOCK_URL` back at the
owner. Do not roll the migration down while any deployment still connects as `gym_pulse_api`.

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
7. Apply the same migrations to production.
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
