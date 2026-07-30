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
8. Deploy the exact staging-accepted image to production.
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
- validation that all six legacy ownership foreign keys are validated
  `ON DELETE CASCADE` constraints;
- staging and production smoke results;
- vulnerability and container-scan results;
- start/end timestamps;
- approver and rollback owner;
- linked mobile release commit and build identifiers.
