---
description: "Implementation tasks for sport activity tracking API"
---

# Tasks: Sport Activity Tracking API

**Input**: Design documents from `/specs/004-sport-activity-tracking/`

**Tests**: Required by the API constitution and listed before each implementation task.

## Phase 1: Setup

- [X] T001 Confirm migration number and preserve unrelated work with `git status --short` and `migrations/`
- [X] T002 [P] Add the additive sport contract to `docs/CONTRACTS.md`

## Phase 2: Foundational

- [X] T003 [P] Add migration assertions for `sport_activities` privacy, constraints, and index
- [X] T004 Add `migrations/018_create_sport_activities.up.sql` and `.down.sql`
- [X] T005 [P] Add sport activity request/response models in `internal/model/sport_activity.go`

## Phase 3: User Story 1 - Log a completed sport (P1)

**Goal**: Create one valid athlete-owned sport atomically and replay it safely.

**Independent Test**: Create a 60-minute basketball activity, retry it, and verify one identical result.

- [X] T006 [P] [US1] Add DAO atomic-create/list/get tests in `internal/dao/sport_activity_dao_test.go`
- [X] T007 [P] [US1] Add validation, timezone, idempotency, and normalization tests in `internal/service/sport_activity_svc_test.go`
- [X] T008 [P] [US1] Add create/get/list HTTP behavior tests in `internal/handler/sport_activity_test.go`
- [X] T009 [US1] Implement owner-scoped atomic persistence in `internal/dao/sport_activity_dao.go`
- [X] T010 [US1] Implement date/range/request validation and hashing in `internal/service/sport_activity_svc.go`
- [X] T011 [US1] Implement `SportActivityHandler` in `internal/handler/sport_activity.go`
- [X] T012 [US1] Wire DAO/service/handler in `cmd/server/main.go` and routes in `internal/router/router.go`
- [X] T013 [US1] Extend route registration tests in `internal/router/training_routes_test.go` and authenticated wiring coverage in `scripts/smoke.sh`

## Phase 4: User Story 2 - Count sport toward consistency (P2)

**Goal**: A sport preserves participation and adds at most one performed date to weekly progress.

**Independent Test**: Add two sports on one inactive date and observe one weekly unit.

- [X] T014 [P] [US2] Add weekly aggregation expectations to `internal/dao/sport_activity_dao_test.go`
- [X] T015 [US2] Include `sport_activities` dates in `internal/dao/stats_dao.go` without changing workout totals or volume
- [X] T016 [US2] Verify atomic participation preservation and scheduled-opportunity retention in DAO/service tests

## Phase 5: User Story 3 - Review private sport history (P3)

**Goal**: Return owned activities for bounded ranges and hide foreign identifiers.

**Independent Test**: List three owned activities newest-first and verify foreign data is absent.

- [X] T017 [P] [US3] Add deterministic empty/range/ownership tests to sport DAO, service, and handler tests
- [X] T018 [US3] Complete list/get ownership and stable not-found behavior across DAO/service/handler
- [X] T019 [US3] Add `sport_activities` to account cleanup in `internal/dao/account_dao.go` and related tests

## Phase 6: Polish and Verification

- [X] T020 Run `gofmt` on changed Go files
- [X] T021 Run focused sport, stats, account, handler, and router tests
- [X] T022 Run `go test ./...`, `golangci-lint run`, and `./scripts/smoke-toggle.sh`
- [X] T023 Validate `quickstart.md`, migration rollback notes, and sibling app contract compatibility

## Dependencies

T001–T005 establish the contract/model. US1 supplies the atomic creation path required by US2 and the
resources read by US3. Verification follows all stories. The API deploys before the app.
