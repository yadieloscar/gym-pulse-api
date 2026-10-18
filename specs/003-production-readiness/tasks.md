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
- [ ] Owner: provision Railway (Pro), GHCR read token, GitHub `staging`/`production` environments,
  and run the first deploy. Unverified until then.

Evidence (2026-10-18, local):
- `golangci-lint run` 0 issues; `go test ./...` passes; coverage service 90.5%, middleware 90.4%,
  handler 92.1%, router/config 100%; `govulncheck` no reachable findings; `actionlint` clean.
- `-tags=integration` `TestRelease*` in `internal/dao` and `internal/migration` pass on PostgreSQL 16.
- Container, `ENVIRONMENT=staging`: server exits on an unmigrated schema ("behind … want 25");
  `./migrate` reaches 25 and repeats as a no-op; server then serves `/health` with the build
  version and `/ready` 200. `scripts/wait-for-deploy.sh` passes on a match and fails on a stale build.
- `scripts/smoke.sh` 23/23 against the new image in `ENVIRONMENT=test` (boot migration path).
  `smoke-toggle.sh` not run: it recreates the shared workspace API container; CI's smoke job covers it.
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
