# Tasks

## Phase 1

- [x] Add failing decode tests for omitted, empty, populated, and null log fields.
- [x] Add service tests for notes-only, overrides-only, set-logs-only, and explicit clears.
- [x] Add PostgreSQL tests proving preservation and atomic replacement.
- [x] Implement presence-aware update intent through handler, service, and DAO.
- [x] Update `docs/CONTRACTS.md`, generated docs, and smoke assertions.
- [x] Run the API gates and linked app acceptance flow.

## Phase 2

- [x] Add regeneration rollback and concurrent session-start tests. Evidence: `TestReleaseRegenerationRollbackIntegration` and `TestReleaseSessionStartRegenerationOrderingIntegration` in `internal/dao/release_safety_integration_test.go`; PostgreSQL-backed race run passed 2026-10-05, including both lock acquisition orders.
- [x] Implement one regeneration transaction.
- [x] Add concurrent same-key and payload-mismatch tests for clone, materialize, and recovery. Evidence: `TestReleaseDuplicateMutationsIntegration` in `internal/dao/release_safety_integration_test.go`; PostgreSQL-backed race run passed 2026-10-05 for all three mutation families, exact replay after revision advancement, and mismatch without persisted changes.
- [x] Store mutation and exact response with idempotency state in one transaction.
- [x] Add replay-after-unrelated-change coverage.

## Later phases

- [x] Implement and test atomic program activation.
- [x] Implement and test JWKS cooldown, timeouts, claim, subject, and algorithm validation.
- [x] Implement and test body limits and trailing JSON rejection.
- [x] Batch list queries and record query-count evidence.
- [x] Complete independent review and resolve all concrete findings.
