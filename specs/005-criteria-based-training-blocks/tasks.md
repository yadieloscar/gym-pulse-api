# Tasks: Criteria-Based Training Blocks API

**Input**: `spec.md`, `plan.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`

**Tests**: Required and written before each implementation phase.

## Phase 1: Foundation

- [x] T001 Add migration 019 tables, constraints, indexes, compatibility notes, and rollback in `migrations/019_create_criteria_training_blocks.up.sql` and `migrations/019_create_criteria_training_blocks.down.sql`
- [x] T002 Add request, response, entity, enum, progress, and error models in `internal/model/training_block.go`
- [x] T003 Add model validation and qualification tests in `internal/model/training_block_test.go`

## Phase 2: User Story 1 - Create and read a personal block (P1)

**Independent test**: Create a five-stage block, replay create, list its summary, fetch full ordered detail, and reject a foreign program without a disclosure.

- [x] T004 [US1] Add failing DAO tests for ownership-scoped create/replay/list/detail and program validation in `internal/dao/training_block_dao_test.go`
- [x] T005 [US1] Add failing service tests for create bounds, contiguous stages, payload hashing, pagination, and stable errors in `internal/service/training_block_svc_test.go`
- [x] T006 [US1] Add failing handler/router tests for create/list/detail auth, query bounds, idempotency header, and response shapes in `internal/handler/training_block_test.go` and `internal/router/router_test.go`
- [x] T007 [US1] Implement transactional create and bounded owned reads in `internal/dao/training_block_dao.go`
- [x] T008 [US1] Implement create/list/detail orchestration and validation in `internal/service/training_block_svc.go`
- [x] T009 [US1] Implement JSON handlers and routes in `internal/handler/training_block.go` and `internal/router/router.go`
- [x] T010 [US1] Wire DAO, service, and handler in `cmd/server/main.go`

## Phase 3: User Story 2 - Exposure and next-morning evidence (P2)

**Independent test**: Add one completed-as-planned exposure, attach baseline once, and observe exactly one qualifying exposure while retries and stale revisions remain safe.

- [x] T011 [US2] Add failing DAO tests for atomic exposure/response writes, revision conflicts, one-time response, replay, and rollback in `internal/dao/training_block_dao_test.go`
- [x] T012 [US2] Add failing service tests for timezone-aware dates, performed bounds, enum validation, and server-only qualification in `internal/service/training_block_svc_test.go`
- [x] T013 [US2] Add failing handler tests for exposure and next-morning contracts in `internal/handler/training_block_test.go`
- [x] T014 [US2] Implement transactional exposure and response mutations in `internal/dao/training_block_dao.go`
- [x] T015 [US2] Implement exposure/recovery business rules in `internal/service/training_block_svc.go`
- [x] T016 [US2] Implement exposure/recovery endpoints and route wiring in `internal/handler/training_block.go` and `internal/router/router.go`

## Phase 4: User Story 3 - Explicit stage transitions (P3)

**Independent test**: Reject early advance, advance after sufficient evidence, regress with a reason, complete the final stage, archive, and preserve append-only history.

- [x] T017 [US3] Add failing DAO tests for locked atomic transition/revision/idempotency behavior in `internal/dao/training_block_dao_test.go`
- [x] T018 [US3] Add failing service tests for advance, regress, complete, archive, and prohibited state transitions in `internal/service/training_block_svc_test.go`
- [x] T019 [US3] Add failing handler tests for transition request and conflict shapes in `internal/handler/training_block_test.go`
- [x] T020 [US3] Implement transition persistence and aggregate reloading in `internal/dao/training_block_dao.go`
- [x] T021 [US3] Implement explicit transition rules in `internal/service/training_block_svc.go`
- [x] T022 [US3] Implement the transition endpoint in `internal/handler/training_block.go` and `internal/router/router.go`

## Phase 5: Contract, privacy, and convergence

- [x] T023 Document all routes, bounds, errors, ownership, revision, and idempotency behavior in `docs/CONTRACTS.md`
- [x] T024 Add account deletion, missing/foreign ownership, and cross-domain compatibility tests in affected `internal/dao`, `internal/service`, and `internal/handler` test files
- [x] T025 Run formatting, targeted handler/router contract smoke tests, `golangci-lint run`, `go test ./...`, and `go test -race ./...`
- [x] T026 Verify every requirement and acceptance scenario against implementation, review the migration rollback, and update completed task markers in `specs/005-criteria-based-training-blocks/tasks.md`

## Dependency order

T001–T003 establish the aggregate. US1 enables the resource and reads; US2 depends on US1; US3 depends on US2 evidence. Contract and convergence follow all stories. Tasks marked for one story remain independently testable at its checkpoint.
