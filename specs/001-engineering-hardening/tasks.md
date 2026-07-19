# Tasks

## Phase 1

- [ ] Add failing decode tests for omitted, empty, populated, and null log fields.
- [ ] Add service tests for notes-only, overrides-only, set-logs-only, and explicit clears.
- [ ] Add PostgreSQL tests proving preservation and atomic replacement.
- [ ] Implement presence-aware update intent through handler, service, and DAO.
- [ ] Update `docs/CONTRACTS.md`, generated docs, and smoke assertions.
- [ ] Run the API gates and linked app acceptance flow.

## Phase 2

- [ ] Add regeneration rollback and concurrent session-start tests.
- [ ] Implement one regeneration transaction.
- [ ] Add concurrent same-key and payload-mismatch tests for clone, materialize, and recovery.
- [ ] Store mutation and exact response with idempotency state in one transaction.
- [ ] Add replay-after-unrelated-change coverage.

## Later phases

- [ ] Implement and test atomic program activation.
- [ ] Implement and test JWKS cooldown, timeouts, claim, subject, and algorithm validation.
- [ ] Implement and test body limits and trailing JSON rejection.
- [ ] Batch list queries and record query-count evidence.
- [ ] Complete independent review and resolve all concrete findings.

