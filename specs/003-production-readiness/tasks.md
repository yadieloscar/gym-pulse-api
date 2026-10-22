# Tasks

## Data boundary

- [x] Add failing privilege/RLS assertions for every application table.
- [x] Add the forward RLS and privilege-revocation migration.
- [x] Prevent automatic grants for future public objects.
- [x] Prove direct API database access still passes smoke.

## Deletion

- [x] Add avatar deletion and missing-object tests.
- [x] Add account deletion partial-failure and retry tests.
- [x] Implement complete idempotent deletion.
- [x] Update contract and smoke evidence.

## Operations and security

- [x] Fix CORS method and header coverage.
- [x] Add database readiness and bounded pool configuration.
- [x] Patch Go dependencies and runtime images.
- [x] Add accurate coverage, vulnerability, container, and production-branch CI gates.
- [x] Run all automated API verification.
- [ ] Complete independent review.

### CI toolchain repair — 2026-10-04

- [x] Align the minimum Go version and digest-pinned container builder on Go 1.26.8;
  let CI read `go.mod` so its compiler cannot silently lag the declared minimum.
- [x] Go build, full atomic-coverage tests, golangci-lint 2.11.4, Swagger drift,
  and govulncheck 1.6.0 pass. Package coverage: service 91.0%, middleware 90.4%,
  handler 91.5%, router/config 100%. No reachable vulnerabilities reported.
- Container scan and database smoke require the repair PR's GitHub Actions run;
  local Docker daemon was stopped. No API, schema, or contract behavior changes.

- Container CI exposed additional fixed advisories: update `golang.org/x/crypto`
  to 0.55.0 and apply Alpine security updates when building the runtime image.
  The initial PR smoke passed; rerun the complete pipeline after these fixes.

### Deployment pipeline — 2026-10-18

- [x] Add `internal/migration` (`Up`, `RequireCurrent`) with unit and integration tests.
- [x] Add `cmd/migrate`; stop boot migrations in staging/production and gate startup on schema state.
- [x] Add `internal/buildinfo.Version` to `/health`; update `docs/CONTRACTS.md` and Swagger.
- [x] Build `server` and `migrate` with the version ldflag in the Dockerfile.
- [x] Add `.railway/railway.ts` and typecheck it in CI.
- [x] Add `.github/workflows/deploy.yml` (build once, staging, approved production).
- [x] Update `docs/RELEASE_RUNBOOK.md` and `README.md` with setup and the release step.
- [x] Run API gates, actionlint, container build, and an independent review.
- [ ] Owner: provision Railway, GitHub `staging`/`production` environments, and GHCR pull access
  (public package, or a read token with Railway Pro), then run the first deploy. Unverified until then.

Evidence (2026-10-18, local):
- `golangci-lint run` 0 issues; `go test ./...` passes; coverage service 90.5%, middleware 90.4%,
  handler 92.1%, router/config 100%; `govulncheck` no reachable findings; `actionlint` clean.
- `-tags=integration` `TestRelease*` in `internal/dao` and `internal/migration` pass on PostgreSQL 16.
- Container, `ENVIRONMENT=staging`: server exits on an unmigrated schema ("behind … want 25");
  `./migrate` reaches 25 and repeats as a no-op; server then serves `/health` with the build
  version and `/ready` 200. `scripts/wait-for-deploy.sh` passes on a match and fails on a stale build.
- `scripts/smoke.sh` 23/23 against the new image in `ENVIRONMENT=test` (boot migration path),
  reproducing `smoke-toggle.sh`'s steps (JWKS off, seeded `auth.users` row, `smoke.sh`) on an
  isolated container. `smoke-toggle.sh` itself was not run: it recreates the shared workspace
  container from the main checkout's image (`--no-build`), so it cannot exercise this branch and
  would disturb other sessions. CI's `smoke` job runs the same steps on the PR image.
- `go test -race ./...` passes (no concurrency changes; run because the plan lists it).
- `.railway/railway.ts` typechecks against `railway@3.12.0` and evaluates for staging/production;
  other environments throw. `railway config plan`, GHCR push, and Railway redeploy are unverified
  until the owner setup in `docs/RELEASE_RUNBOOK.md` exists.
- Independent review (no blockers) fixed: per-environment job concurrency instead of a workflow
  group; only the tip of `main` deploys; a dirty schema newer than the build warns instead of
  blocking the serving build's restarts; the version table is pinned to `public.schema_migrations`;
  digest from `buildx --metadata-file`; inputs passed via `env`; rollback waits for the old commit.
- Found while verifying: `docker buildx imagetools create` re-wraps a single manifest in a new
  index (digest changed in a local registry test), so retagging uses
  `skopeo copy --preserve-digests` and asserts the tag's digest; verified against a local registry.

### Security sweep — 2026-10-20

Cross-repository sweep (sibling: `gym-pulse-app/specs/002-production-readiness/tasks.md`).
Three independent read-only review lanes (auth/ownership, persistence/infra, app client)
found no critical or high defects; every DAO query on a user-owned table is scoped by
`user_id` or reached through a user-scoped parent in the same transaction.

- [x] Go 1.26.10 toolchain and digest-pinned builder/runtime images (GO-2026-6809);
  chi v5.5.0 (GO-2026-5774/5775/5777, `RealIP` was never used). govulncheck: 0 reachable.
- [x] Staging/production refuse `DATABASE_URL` / `DATABASE_LOCK_URL` without
  `sslmode=require|verify-ca|verify-full` (pgx otherwise defaults to `prefer`).
- [x] Security response headers on every route; `Cache-Control: no-store` on `/api/*`;
  Swagger UI no longer mounted in production.
- [x] Per-user rate limits (`429 RATE_LIMITED`) on avatar upload, account deletion,
  schedule rewrites, and plan transitions. Keyed by JWT subject, never by IP.
- [x] Training-block idempotency hashes now bind the target block/exposure id.
- [x] Starter-program filters reject malformed or oversized values (422); exercise
  history/records cap `ids` at 100; `avatar_url` on `PUT /profile` must be `https://`.
- [x] `.dockerignore` excludes worktrees, agent config, specs, and docs; RLS inventory
  text corrected to migrations 016/018/019/025 across 30 tables.
- Evidence: `go test ./...` and `-race` on middleware/router pass; package coverage
  service 90.5%, middleware 91.0%, handler 92.3%, router 100%, config 99.2%;
  golangci-lint 0 issues; Swagger drift none; database-design gate passes;
  `validate-supabase-security.sh` passes against the local migrated schema;
  `smoke.sh` 23/23 against an image built from this branch.
- Accepted/unchanged: public avatar URLs are predictable by design (public bucket);
  migrations still run under the runtime role (least-privilege role remains a target).

### Merge with main — 2026-10-22

- [x] Reconcile with spec 011 (least-privilege runtime role): `./migrate`, development/test boot
  migrations, and the startup schema check all use `DATABASE_MIGRATE_URL` (schema owner); the
  check runs over a short-lived owner connection (`migration.RequireCurrentAt`) because
  `gym_pulse_api` cannot read `schema_migrations`. Railway preserves `DATABASE_MIGRATE_URL`.
- [x] CI integration step keeps main's unfiltered run and adds `./internal/migration`.
- Evidence: lint 0 issues; `go test -race ./...` passes; coverage service 90.5%, middleware 91.0%,
  handler 92.3%, router 100%, config 99.2%; database-design gate and Swagger drift clean;
  actionlint clean. With TLS PostgreSQL and `scripts/ci/runtime-role.sql`: CI layout
  (`ENVIRONMENT=test`, requests as `gym_pulse_api`) passes `smoke.sh` 23/23 and
  `validate-supabase-security.sh`. `ENVIRONMENT=staging` on a fresh database: the server exits
  ("behind … want 26"); `./migrate` over the owner URL reaches 26; `gym_pulse_api` gets
  `permission denied for table schema_migrations`, confirming the owner-connection check is
  required; the server then serves `/health` and `/ready`, and keeps serving with a repair warning
  when the schema is dirty at 27.

## Phase 1: Convergence

- [x] T001 CRITICAL: take `context.Context` first in `migration.Up` (connection via `postgres.WithConnection`, graceful stop between migrations on cancel) and cancel `cmd/migrate` on SIGINT/SIGTERM per Constitution V (contradicts)
- [x] T002 Justify the Dockerfile's `-trimpath` build flag in the plan per plan: Phase 5 (unrequested)
