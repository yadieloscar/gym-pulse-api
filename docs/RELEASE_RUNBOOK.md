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

## One-time setup

Do these once, in order. The pipeline never creates accounts, environments, or secrets.

1. **Railway project.** Create project `gym-pulse` with environments `staging` and `production`.
   In each, create service `api` from the Docker image
   `ghcr.io/yadieloscar/gym-pulse-api:<environment>` (`:staging` or `:production`). The first
   deploy fails until the pipeline pushes an image; that is expected.
2. **Service variables.** In each environment, set the variables `.railway/railway.ts` marks
   `preserve()`: `DATABASE_URL`, `DATABASE_LOCK_URL` (direct or session mode, port 5432),
   `SUPABASE_URL`, `SUPABASE_SERVICE_ROLE_KEY`, `SUPABASE_JWT_AUDIENCE` (normally
   `authenticated`), and `ALLOWED_ORIGINS` (see `README.md`). Generate a Railway domain or attach
   the custom domain.
3. **Apply the infrastructure file** for each environment, with that environment linked:
   ```bash
   cd .railway && npm ci && railway link
   railway config plan
   railway config apply
   ```
   This sets the image tag, the `./migrate` pre-deploy command, the `/ready` healthcheck,
   on-failure restarts, and 15s draining. Railway's Config as Code (`railway.json`) stops being
   read on 2026-12-01, so do not add one.
4. **Railway tokens.** Create a project token scoped to each environment.
5. **GitHub environments** (repository Settings → Environments):
   - `staging`: secret `RAILWAY_TOKEN` (staging token) and variable `API_URL` (staging base URL).
   - `production`: secret `RAILWAY_TOKEN` (production token), variable `API_URL`, **required
     reviewers** (the release approver), and deployment branches limited to `main`.
6. **First image.** Run the **Deploy** workflow from `main` (Actions → Deploy → Run workflow).
   It pushes `ghcr.io/yadieloscar/gym-pulse-api`. Then make Railway able to pull it, either:
   - set the GHCR package visibility to public (the repository is public, so the image reveals
     nothing new), or
   - keep it private and add a read-only `read:packages` token under the service's Registry
     Credentials (requires Railway Pro).

   Re-run the failed staging job if it could not pull the image.

## Pre-deployment gate

- [ ] The release commit is identified and reviewed.
- [ ] `docs/CONTRACTS.md` matches observable behavior.
- [ ] All migrations are forward-compatible with the currently released app and with the
      previous API image, which keeps serving while `./migrate` runs.
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

The **Deploy** workflow (`.github/workflows/deploy.yml`) runs these steps after CI passes on
`main`; manual steps are marked.

1. **Build.** If the commit is still the tip of `main` (CI runs can finish out of order; an older
   commit never deploys over a newer one), the workflow builds it once, pushes
   `ghcr.io/yadieloscar/gym-pulse-api:sha-<commit>`, scans the pushed digest, and records the
   digest in the run summary.
2. **Staging deploy.** It moves `:staging` to that digest and runs `railway redeploy`. Railway runs
   `./migrate` as the pre-deploy step over `DATABASE_LOCK_URL`; a failed migration stops the
   deployment and leaves the previous build serving. Migration 017 deliberately fails on orphaned
   legacy ownership rows; investigate and reconcile them rather than bypassing constraint
   validation. Traffic moves once `/ready` passes, and the workflow waits until `/health` reports
   the release commit.
3. **Manual: verify staging.** Confirm direct Supabase access is denied and the Go API smoke flow
   succeeds. Verify `/health`, `/ready`, authentication, avatar upload/deletion, account
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
4. **Manual: confirm the production backup**, then approve the `production` job in GitHub.
5. **Production deploy.** The workflow moves `:production` to the **same digest** and repeats
   step 2 against production, including the migration release step.
6. **Manual:** run a bounded production smoke with a dedicated test account and remove it
   afterward.

A server in `staging` or `production` never migrates on boot. It exits at startup if the schema is
behind its image or dirty. A dirty schema needs operator repair: inspect the failed migration,
fix the database, then mark the last good version with the golang-migrate CLI from a trusted
machine (`migrate -database "$DATABASE_LOCK_URL" -path migrations force <version>`). The image's
`./migrate` only applies pending migrations; it has no force or down command.

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
  The server accepts a schema newer than its image, so a compatible image starts. To restore it,
  take the earlier digest from a previous Deploy run summary and, with that environment's
  `RAILWAY_TOKEN`:
  ```bash
  skopeo copy --all --preserve-digests \
    docker://ghcr.io/yadieloscar/gym-pulse-api@sha256:<previous-digest> \
    docker://ghcr.io/yadieloscar/gym-pulse-api:production
  railway redeploy --service api --yes
  scripts/wait-for-deploy.sh "$API_URL" <previous-commit>
  ```
  The pre-deploy `./migrate` finds nothing to apply; it never runs down-migrations. Rollback is
  done only when the wait script confirms the previous commit is serving and ready.
- A failed release migration leaves the schema dirty at a version newer than the serving build.
  That build keeps serving and restarting (it logs a repair warning), but the next release is
  blocked until the schema is repaired as described under Deployment order.
- Restore a database backup only for confirmed data corruption and only after stopping writes.
- If deletion is partially complete, preserve evidence, use the idempotent cleanup retry, and
  complete deletion operationally before closing the incident.

## Release evidence

Record:

- API commit and image digest (from the Deploy run summary);
- migrations applied;
- validation that all six legacy ownership foreign keys are validated
  `ON DELETE CASCADE` constraints;
- staging and production smoke results;
- vulnerability and container-scan results;
- start/end timestamps;
- approver and rollback owner;
- linked mobile release commit and build identifiers.
