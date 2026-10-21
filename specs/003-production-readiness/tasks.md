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
