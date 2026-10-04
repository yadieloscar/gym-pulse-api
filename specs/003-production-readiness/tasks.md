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
