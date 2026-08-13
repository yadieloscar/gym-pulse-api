# Tasks: Release-Safe Training Workflows

**Input**: Design documents from `specs/006-release-safety/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: Tests and executable verification are required for every changed behavior.

**Organization**: Tasks are grouped by independently testable user story and execute in rollout-risk order.

## Phase 1: Setup and Contract Lock

**Purpose**: Freeze the published behavior and test boundaries before implementation.

- [x] T001 Create and validate specification, research, plan, data model, contract delta, and quickstart in specs/006-release-safety/
- [x] T002 Update mutation identity and 366-day inclusive range rules in docs/CONTRACTS.md
- [x] T003 Add release-safety request/response types and shared range constant in internal/model/training.go

---

## Phase 2: Foundational Transaction Boundary

**Purpose**: Establish shared replay and transaction behavior before user workflows.

**⚠️ CRITICAL**: No mutation story can ship until this phase is complete.

- [x] T004 Add failing request-hash/replay-order and operation-key validation tests in internal/service/training_domain_svc_test.go and internal/handler/guided_training_handlers_test.go
- [x] T005 Add failing transaction SQL contract tests for owner scoping, idempotency-first evaluation, lock order, exact response storage, and rollback in internal/dao/training_mutation_dao_test.go
- [x] T006 Implement the training mutation transaction interface and PostgreSQL DAO in internal/dao/training_mutation_dao.go
- [x] T007 Wire the production transaction DAO into all training services in cmd/server/main.go and internal/service/training_svc.go

**Checkpoint**: The production service has one short, owner-scoped, replay-aware transaction boundary.

---

## Phase 3: User Story 1 - Carry Forward Existing Plans (Priority: P1) 🎯 MVP

**Goal**: Existing athletes can adopt their owned recurring legacy plan into one active goal-based program and the next future week.

**Independent Test**: Seed owned weekly assignments, adopt once, replay concurrently, and prove one authoritative result with unchanged legacy rows.

### Tests for User Story 1

- [x] T008 [P] [US1] Add failing adoption model and mapping tests for strength, cardio, repeated templates, weekday order, and next-week dates in internal/model/goal_based_training_acceptance_test.go
- [x] T009 [P] [US1] Add failing handler and route inventory tests for POST /api/v1/programs/adopt-legacy in internal/handler/guided_training_handlers_test.go and internal/router/training_routes_test.go
- [x] T010 [US1] Add failing service/DAO adoption tests for ownership, no-plan, exact replay, changed payload, concurrent calls, and rollback in internal/service/guided_training_services_test.go and internal/dao/training_mutation_dao_test.go

### Implementation for User Story 1

- [x] T011 [US1] Implement deterministic legacy-template to program mapping and next-future-week calculation in internal/model/training.go
- [x] T012 [US1] Implement atomic legacy adoption and authoritative replay in internal/dao/training_mutation_dao.go
- [x] T013 [US1] Add ProgramService adoption orchestration in internal/service/training_svc.go
- [x] T014 [US1] Add the authenticated adoption handler, Swagger annotation, and route in internal/handler/training.go and internal/router/router.go
- [x] T015 [US1] Add live adoption/replay/legacy-preservation coverage to scripts/smoke.sh

**Checkpoint**: User Story 1 is independently deployable as additive API support.

---

## Phase 4: User Story 2 - Retry Training Actions Safely (Priority: P1)

**Goal**: Every published goal-training mutation replays exactly and multi-aggregate outcomes commit together.

**Independent Test**: For each mutation family, prove exact replay after revision advancement, changed-payload conflict, concurrent first attempts, and all-or-nothing failure injection.

### Tests for User Story 2

- [x] T016 [P] [US2] Add failing profile and program create/update exact-replay tests in internal/service/training_domain_svc_test.go and internal/service/guided_training_services_test.go
- [x] T017 [P] [US2] Add failing snapshot, target, required-set, extra-set, and scheduled-completion replay tests in internal/service/guided_training_services_test.go
- [x] T018 [P] [US2] Add failing workout-session create/update replay and completion-participation atomicity tests in internal/service/guided_training_services_test.go
- [x] T019 [US2] Add failing SQL contract tests for workout/participation and session/participation one-transaction writes in internal/dao/training_mutation_dao_test.go

### Implementation for User Story 2

- [x] T020 [US2] Require matching operation keys for training profile and custom program create/update handlers and requests in internal/model/training.go and internal/handler/training.go
- [x] T021 [US2] Move training profile and custom program create/update mutations to exact atomic replay in internal/service/training_svc.go and internal/dao/training_mutation_dao.go
- [x] T022 [US2] Move scheduled snapshot and target edits to exact atomic replay in internal/service/schedule_svc.go and internal/dao/training_mutation_dao.go
- [x] T023 [US2] Move required-set and extra-set workflows, including session creation and live status refresh, into one exact-replay transaction in internal/service/schedule_svc.go and internal/dao/training_mutation_dao.go
- [x] T024 [US2] Move scheduled finalization and day participation into one exact-replay transaction in internal/service/schedule_svc.go and internal/dao/training_mutation_dao.go
- [x] T025 [US2] Move workout-session create/update and completion participation into one exact-replay transaction in internal/service/schedule_svc.go and internal/dao/training_mutation_dao.go
- [x] T026 [US2] Extend wire-level exact-replay and changed-payload smoke coverage in scripts/smoke.sh

**Checkpoint**: Published mutation semantics are safe across timeouts, retries, restarts, and load-balanced replicas.

---

## Phase 5: User Story 3 - Keep Date Work Bounded (Priority: P1)

**Goal**: All goal-training calendar paths accept at most 366 inclusive dates and reject larger work before repository access.

**Independent Test**: Run every range path at 366 and 367 inclusive days and assert that oversized requests invoke no DAO.

### Tests for User Story 3

- [x] T027 [P] [US3] Add failing shared inclusive boundary tests in internal/model/training_test.go
- [x] T028 [P] [US3] Add failing no-repository-call range tests for schedule, session, participation, materialization, regeneration, and plan transition in internal/service/release_coverage_test.go and internal/service/guided_experience_contract_test.go

### Implementation for User Story 3

- [x] T029 [US3] Enforce the 366-day inclusive maximum in internal/model/training.go and route every goal-training range through it in internal/service/schedule_svc.go and internal/service/plan_transition_svc.go
- [x] T030 [US3] Add 366/367-day wire checks to scripts/smoke.sh

**Checkpoint**: Expensive date work is uniformly bounded before database or generation work.

---

## Phase 6: Polish and Cross-Repository Handoff

**Purpose**: Complete executable evidence and make the additive API safe for the dependent app PR.

- [x] T031 [P] Regenerate Swagger outputs in docs/docs.go, docs/swagger.json, and docs/swagger.yaml
- [x] T032 Run gofmt and goimports on every changed Go file
- [x] T033 Run go test ./..., go test -race ./..., golangci-lint run, govulncheck ./..., and ./scripts/smoke-toggle.sh
- [x] T034 Run the quickstart acceptance scenarios and record any unavailable external gate in specs/006-release-safety/quickstart.md
- [x] T035 Perform an independent final diff review for contract drift, ownership leaks, replay ordering, deadlocks, partial commits, and unbounded work

---

## Dependencies & Execution Order

- **Phase 1** establishes the contract and request models.
- **Phase 2** depends on Phase 1 and blocks both mutation-heavy stories.
- **User Story 1** depends on Phase 2 and ships first because the app already calls the missing route.
- **User Story 2** depends on Phase 2; tasks within the story are sequential by shared transaction files.
- **User Story 3** may be tested in parallel with User Story 2 but its shared model edit integrates after operation-key model changes.
- **Phase 6** depends on all selected stories.

## Parallel Opportunities

- T008 and T009 affect disjoint test packages.
- T016, T017, and T018 start in disjoint focused test sections but converge before shared implementation.
- T027 and T028 affect model and service tests independently.
- T031 may run after all handler/model annotations stabilize while documentation review proceeds.

## Implementation Strategy

1. Deliver the additive adoption route and range ceiling without waiting for the app PR.
2. Complete mutation atomicity before declaring the API release-safe; do not partially enable only selected public mutation semantics.
3. Deploy and smoke the API branch first.
4. Implement the sibling app safety specification against this exact contract.

## Notes

- No schema migration or dependency installation is planned.
- Tests are written failing before each implementation group.
- Runtime training recommendations remain deterministic and outside this feature.
