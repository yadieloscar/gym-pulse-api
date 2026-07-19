# Implementation Plan

## Constitution check

- Contract First: every observable API change updates `docs/CONTRACTS.md` and contract tests.
- Owned Authenticated Data: all queries remain scoped by authenticated user ID.
- Idempotent Evolution: mutations and stored replay results share one transaction.
- Executable Evidence: failing regression tests precede implementation where practical.
- Simple Go Boundaries: handlers decode transport intent, services own policy, DAOs own transactions.

## Phase 1 — Lossless day-log updates

Change `UpdateDayLogRequest` to preserve field presence for overrides, set logs, and notes. Pass an
explicit update intent through the service to one DAO transaction. Omission preserves data; present
empty arrays clear; present values replace. Workout replacement clears incompatible omitted detail.

Files expected to change:

- `internal/model/log.go`
- `internal/handler/logs.go` and handler tests
- `internal/service/log_svc.go` and log service tests
- `internal/dao/log_dao.go` and PostgreSQL integration tests
- `docs/CONTRACTS.md` and generated API documentation when applicable

Compatibility: deploy the API first. Existing app requests become safe immediately. The linked app
PR also sends complete detail defensively and remains compatible with the old API.

## Phase 2 — Schedule and idempotency integrity

Introduce workflow-specific transactional DAO operations. Regeneration locks/checks affected rows,
deletes eligible snapshots, inserts replacements and sets, stores the idempotency response, and
commits once. Clone, materialize, and recovery claim/check their operation key and store the exact
response in the same transaction as mutation. Replay decodes the stored response rather than reading
the current range.

Primary files:

- `internal/service/schedule_svc.go`
- `internal/service/training_svc.go`
- `internal/dao/schedule_dao.go`
- `internal/dao/training_log_dao.go`
- workflow-specific DAO tests and service tests

No migration is expected unless the stored response schema proves insufficient after inspection.

## Phase 3 — Program activation

Create the new program and deactivate the previous active program in one transaction. Preserve the
existing partial unique index. Map foreseeable conflicts to a stable public error and test concurrent
creation.

## Phase 4 — Authentication and HTTP hardening

Add a JWKS refresh cooldown/negative lookup, single bounded refresh behavior, injected HTTP client
with timeout, exact signing-method checks, configured issuer/audience validation, and UUID subject
validation. Add a 1 MiB request-body limit, stable 413 error, and trailing-JSON rejection.

## Phase 5 — Query performance

Batch-load scheduled and performed sets for list results, preserve deterministic ordering and empty
arrays, and add query-count evidence. Keep this in a separate PR from integrity work.

## Verification

- Focused handler/service tests during development.
- PostgreSQL rollback, constraint, ownership, and concurrency integration tests.
- `go test ./...`
- `go test -race ./...`
- `golangci-lint run` when available.
- `./scripts/smoke-toggle.sh` for contract changes.
- `govulncheck ./...` when available for authentication changes.
- Live cross-repo acceptance against the app flows.
- Independent read-only final review.

## PR and deployment order

1. API lossless log updates; deploy and smoke-test.
2. Linked app compatibility PR; release after API verification.
3. API schedule atomicity.
4. API idempotency and exact replay.
5. API program activation.
6. API authentication hardening.
7. API HTTP hardening.
8. API query performance.

Rollback after the app release should be roll-forward for the log contract because reverting the API
would reintroduce data loss for older app behavior.

