# Implementation Plan

## Constitution check

- Contract First: deletion and readiness behavior update `docs/CONTRACTS.md` and executable tests.
- Owned Authenticated Data: Supabase roles lose application-table access; Go ownership checks remain.
- Idempotent Evolution: deletion and migrations are retry-safe and forward-compatible.
- Executable Evidence: security, partial-failure, contract, and coverage assertions precede completion.
- Simple Go Boundaries: handlers own transport, services coordinate deletion policy, DAOs own SQL,
  and storage/auth clients remain narrow interfaces.

## Phase 1 — Supabase application-data boundary

Add a forward migration that enables RLS and revokes existing/default application-object privileges
from Data API roles. Add a validation script or PostgreSQL-backed test proving the resulting grants
and RLS state. Do not use `FORCE ROW LEVEL SECURITY`, because the direct API database role must
continue to operate.

## Phase 2 — Complete account deletion

Extend avatar storage with idempotent removal. Serialize account deletion and avatar upload with a
cross-replica per-user lock held on a separate bounded database pool, recheck identity inside that
boundary, and coordinate application-data, avatar, and final auth-user deletion without returning
`204` after a failed required step. Auth stays last so an ambiguous provider response cannot strand
application-held personal data after the identity is gone. Keeping lock sessions outside the main
query pool prevents pool-saturation deadlocks. Preserve stable public errors and make the entire
operation safely repeatable.

## Phase 3 — HTTP and operational readiness

Fix CORS method/header drift, add database readiness, configure bounded pgx pool behavior, and include
request correlation without logging secrets or user data. Keep `/health` backward-compatible.

## Phase 4 — Runtime and CI

Move to supported patched Go and Alpine images, update affected dependencies, add vulnerability and
container gates, enforce accurate statement coverage, and run CI on the protected production branch.

## Phase 5 — Deployment pipeline (amended 2026-10-18)

- Move migration execution into `internal/migration` with `Up` (golang-migrate over a session-mode
  URL) and `RequireCurrent` (reads `schema_migrations` through the query pool without advisory locks,
  so it is safe over the transaction pooler). Add `cmd/migrate`, which loads the full config and
  migrates over `DATABASE_LOCK_URL`; the config already rejects port 6543 for that URL in release
  environments, and golang-migrate needs the same session semantics for its advisory lock.
- `cmd/server` migrates on boot only in development/test; staging/production call `RequireCurrent`
  and exit if the schema is behind or dirty. Accepting a newer schema keeps rollback images usable.
- Inject the commit into `internal/buildinfo.Version` with `-ldflags -X` (Docker build arg
  `VERSION`, default `dev`) and add it additively to `/health`. Build both binaries in the image.
- `.railway/railway.ts` declares the `api` service: image `ghcr.io/<owner>/gym-pulse-api:<env>` with
  auto-updates off, pre-deploy `./migrate`, `/ready` healthcheck, on-failure restarts, and draining
  longer than the server's 10s shutdown. Secrets stay `preserve()`. CI typechecks the file.
- `.github/workflows/deploy.yml` runs after a successful `CI` run on `main` (or manual dispatch):
  build and push once, scan the pushed digest, retag `:staging`, `railway redeploy`, wait for
  `/health` to report the commit and `/ready` to pass, then repeat for `:production` behind the
  `production` GitHub environment approval. Deploys are serialized with a concurrency group.
- Rejected: Railway building from the repo per environment (production would not be the accepted
  image); `railway.json` (Railway stops reading Config as Code on 2026-12-01); applying a digest
  through `railway config apply` from CI (undocumented for image sources).

## Verification

- Focused service, middleware, handler, migration, and client tests.
- `gofmt` on changed Go files.
- `go test ./...`
- `go test -race ./...`
- `golangci-lint run`
- `govulncheck ./...`
- `./scripts/smoke-toggle.sh`
- Container build and vulnerability scan.
- Direct Supabase-role denial checks in staging before production migration.
- Phase 5: `internal/migration` tests for version parsing, behind, dirty, and newer schemas
  (PostgreSQL-backed under the `integration` tag); health version test; `actionlint` on workflows;
  `tsc --noEmit` on `.railway/railway.ts`; container build with both binaries. A live Railway deploy
  is unverified until the owner provisions Railway, GHCR credentials, and GitHub environments.

## Rollout

1. Back up and validate staging.
2. Apply the privilege/RLS migration and run direct denial plus API smoke tests.
3. Deploy the backward-compatible API.
4. Observe readiness and errors.
5. Release the linked app only after API acceptance.
