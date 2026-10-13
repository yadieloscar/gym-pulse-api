---

description: "Task list for skipping and resolving missed workouts"
---

# Tasks: Skip a missed workout

**Input**: Design documents from `/specs/007-skip-missed-workout/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/scheduled-workout-resolution.md](contracts/scheduled-workout-resolution.md), [quickstart.md](quickstart.md)

**Tests**: Tests and executable verification are REQUIRED for changed behavior by the constitution.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2)

## Requirement and Verification Evidence

| Requirement / acceptance | Tasks | Verification | Result / limitation |
| --- | --- | --- | --- |
| FR-001, FR-003, US1-1 | T011, T015–T020 | service + handler tests; quickstart #1 | Passed 2026-10-11: service, handler, integration skip tests; smoke 21/21 |
| FR-002, SC-002 | T012, T014 | integration comparison test; quickstart #10 | Passed: history and stats DAO outputs identical before skip, after skip, after unskip (TestReleaseSkipAndUnskipResolveWithoutChangingHistory) |
| FR-004, US1-2, US1-5, SC-001 | T008, T009, T013 | recover service + integration tests; quickstart #2, #5 | Passed: TestReleaseRecoverResolvesSourceOnce (second recover 404); fallback unit tests |
| FR-006, US1-4 | T011, T016, T019 | service + handler tests; quickstart #4 | Passed: planned and recovered skips return ResolutionConflictError; smoke 409 RESOLUTION_CONFLICT on incomplete |
| FR-007, US1-3, US2-3, SC-003 | T011, T017, T024 | idempotency replay tests; quickstart #3 | Passed: replay returns stored body without revision change |
| FR-008 | T012 | integration ownership test; quickstart #8 | Passed: foreign user skip returns NotFoundError; smoke 404 |
| FR-005, US2-1, US2-2 | T022–T027 | service + handler + integration tests; quickstart #6, #7 | Passed: unskip restores and recover selects it; recovered unskip conflicts |
| FR-009 | T014 | integration test across regenerate and transition apply; quickstart #9 | Passed: TestReleaseResolutionSurvivesScheduleReplacement |
| FR-010 | T001, T002, T021, T027 | contract drift gate; swagger diff | Passed: CONTRACTS.md updated; swagger regenerated (routes, fields, apiError only) |
| FR-011, SC-004 | T007, T030 | existing suites unchanged; smoke | Passed: go test ./... green; existing integration suite green; smoke 21/21 |
| FR-012 | T004, T006 | migration integration test; quickstart #11, #12 | Passed: TestReleaseMigration020BackfillsOnlyRecoveredSources incl. later-cycle and same-day cases, down/up |
| FR-013 | T010 | integration correction test; quickstart #13 | Passed: TestReleaseCorrectionClearsResolution |

## Path Conventions

Go service at repository root: `migrations/`, `internal/{model,dao,service,handler,router}/`, `docs/`, `scripts/`.

---

## Phase 1: Setup (Contract first)

**Purpose**: Publish the client-facing contract before code, as the constitution requires

- [x] T001 Document `resolution` and `resolved_at` on `ScheduledWorkout`, the clarified recover behavior, the skip and unskip routes, and `409 RESOLUTION_CONFLICT` in docs/CONTRACTS.md per contracts/scheduled-workout-resolution.md
- [x] T002 [P] Document the two new columns, their check constraint, and the backfill rule in docs/DATABASE_DESIGN.md (scheduled_workouts section, around lines 700–725) and confirm ./scripts/validate-database-design.sh passes

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Schema, model, and read paths that every story uses

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T003 Create migrations/020_add_scheduled_workout_resolution.up.sql adding `resolution TEXT NULL CHECK (resolution IN ('skipped','recovered'))`, `resolved_at TIMESTAMPTZ NULL`, and `CHECK ((resolution IS NULL) = (resolved_at IS NULL))`
- [x] T004 Add the provenance backfill to migrations/020_add_scheduled_workout_resolution.up.sql: for each idempotency_records row with scope 'schedule/recover', mark the source missed row that recovery copied (data-model.md "Backfill") as recovered with resolved_at = the earliest matching record time
- [x] T005 [P] Create migrations/020_add_scheduled_workout_resolution.down.sql dropping the constraint and both columns
- [x] T006 Add a golang-migrate helper to internal/dao/release_safety_integration_test.go that can apply 020 down and up, and tests: a recovery record marks exactly its source; a later-cycle occurrence of the same program workout stays unresolved; a same-day recovery is matched; misses without a record stay unresolved; down/up round-trips
- [x] T007 Add `Resolution *string` (`json:"resolution,omitempty"`), `ResolvedAt *time.Time` (`json:"resolved_at,omitempty"`), resolution constants, and a typed `ResolutionConflictError` to internal/model/training.go; add the new columns to every scheduled_workouts select and scan in internal/dao/schedule_dao.go (lines ~61, ~167, ~449) and loadScheduledWorkout in internal/dao/training_mutation_dao.go (~486)
- [x] T008 Make the service test fake in internal/service/release_coverage_test.go implement RecoverIdempotent and add a failing test that a second recover with a new operation key does not copy the same missed workout again; add a fallback-path test that resolved misses are ignored
- [x] T009 Change RecoverIdempotent in internal/dao/schedule_dao.go to select `status='missed' AND resolution IS NULL` and, in the same transaction, set the source's `resolution='recovered'`, `resolved_at=now()`, `revision=revision+1`; add only the unresolved filter to the legacy fallback in RecoverToday in internal/service/schedule_svc.go (~306–318) and comment it as legacy
- [x] T010 Clear `resolution` and `resolved_at` in updateScheduledStatus in internal/dao/training_mutation_dao.go (~781–807) when the re-derived status is not missed, with an integration test where a correction completes a skipped workout (FR-013)
- [x] T011 Map `ResolutionConflictError` to 409 `RESOLUTION_CONFLICT` with `details` and the authoritative `resource` in handleServiceError in internal/handler/response.go, with a handler test

**Checkpoint**: Schema migrated, fields serialized, recovery no longer double-copies

---

## Phase 3: User Story 1 - Skip the earliest missed workout (Priority: P1) 🎯 MVP

**Goal**: An athlete marks an owned missed workout as skipped; it stops being next and recovery ignores it, with history unchanged.

**Independent Test**: Skip a missed workout, read the schedule, and confirm `resolution: "skipped"`, `status: "missed"`, and recover returns 404 when no other unresolved miss exists (quickstart #1, #2).

### Tests for User Story 1 (REQUIRED) ⚠️

- [x] T012 [P] [US1] Integration tests in internal/dao/release_safety_integration_test.go: skip sets resolution and bumps revision; another user's id returns not found; /stats (including streaks and volume), participation, and records are identical before skip, after skip, and after unskip
- [x] T013 [P] [US1] Integration test in internal/dao/release_safety_integration_test.go: recover marks its source recovered and a second recover returns 404; skipping a recovered workout conflicts; recover then skip with the old revision returns REVISION_CONFLICT and with the new revision returns RESOLUTION_CONFLICT
- [x] T014 [P] [US1] Integration test in internal/dao/release_safety_integration_test.go: a skipped miss keeps its resolution after schedule regenerate and plan-transition apply
- [x] T015 [P] [US1] Service tests in internal/service/guided_training_services_test.go for Skip: missed → skipped, already skipped → unchanged without revision bump, not missed → ResolutionConflictError, idempotent replay, stale revision conflict
- [x] T016 [P] [US1] Handler tests in internal/handler/guided_training_handlers_test.go for POST /scheduled-workouts/{id}/skip: 200 body, Idempotency-Key mismatch 422, 404, 409 RESOLUTION_CONFLICT; extend the guidedScheduleService stub for the new interface methods

### Implementation for User Story 1

- [x] T017 [US1] Add SkipScheduledWorkout to the TrainingMutationDAO interface and implement it in internal/dao/training_mutation_dao.go following FinalizeScheduledWorkout: advisory lock, findIdempotency (scope `scheduled-workouts/skip`), owned row FOR UPDATE, revision check, status and resolution guards, UPDATE with revision+1, loadScheduledWorkout, insertIdempotency
- [x] T018 [US1] Add Skip to the schedule service in internal/service/schedule_svc.go hashing `{WorkoutID, Request}` like Complete (~425–448) and to the ScheduleService interface
- [x] T019 [US1] Add the Skip handler with swag annotations in internal/handler/training.go using decodeMutation and matchIdempotencyKey
- [x] T020 [US1] Register `POST /api/v1/scheduled-workouts/{id}/skip` in internal/router/router.go with a route test in internal/router/training_routes_test.go
- [x] T021 [US1] Regenerate docs/docs.go, docs/swagger.json, and docs/swagger.yaml with `swag init -g cmd/server/main.go --output docs`

**Checkpoint**: Skip works end to end; recovery fix verified

---

## Phase 4: User Story 2 - Undo a skip (Priority: P2)

**Goal**: The athlete can clear a skip so the workout is unresolved again; recovered workouts cannot be un-skipped.

**Independent Test**: Skip, unskip, and confirm no resolution and that recover selects the workout again (quickstart #6, #7).

### Tests for User Story 2 (REQUIRED) ⚠️

- [x] T022 [P] [US2] Service tests in internal/service/guided_training_services_test.go for Unskip: skipped → unresolved with revision bump, unresolved → unchanged, recovered → ResolutionConflictError, idempotent replay
- [x] T023 [P] [US2] Handler tests in internal/handler/guided_training_handlers_test.go for POST /scheduled-workouts/{id}/unskip
- [x] T024 [P] [US2] Integration test in internal/dao/release_safety_integration_test.go: skip then unskip, recover selects the workout; replaying the same unskip key returns the stored result

### Implementation for User Story 2

- [x] T025 [US2] Implement UnskipScheduledWorkout in internal/dao/training_mutation_dao.go (scope `scheduled-workouts/unskip`) clearing resolution and resolved_at only when resolution is skipped
- [x] T026 [US2] Add Unskip to internal/service/schedule_svc.go and the Unskip handler with swag annotations in internal/handler/training.go
- [x] T027 [US2] Register `POST /api/v1/scheduled-workouts/{id}/unskip` in internal/router/router.go with a route test, and regenerate swagger docs

**Checkpoint**: Both stories work independently

---

## Phase 5: Polish & Cross-Cutting Concerns

- [x] T028 [P] Add skip and unskip wire checks to scripts/smoke.sh and run ./scripts/smoke-toggle.sh
- [x] T029 Run `go test ./...` with coverage and confirm ≥ 90% for service, handler, router, and middleware packages
- [x] T030 Run `go test -race -tags=integration ./internal/dao -run '^TestRelease'` against a disposable database and record results in this file's evidence table
- [x] T031 Review the final diff against docs/CONTRACTS.md and the app sibling contract, confirm `git diff --check`, and record the deploy-before-app rollout note in the PR description

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: none; contract first
- **Foundational (Phase 2)**: depends on Phase 1; blocks both stories (T003 → T004 → T006; T007 before T008–T011)
- **US1 (Phase 3)**: depends on Phase 2
- **US2 (Phase 4)**: depends on Phase 2; reuses US1 wiring patterns but is independently testable
- **Polish (Phase 5)**: after both stories

### Within Each User Story

- Tests first (T012–T016, T022–T024) and confirmed failing for the gap
- DAO → service → handler → router → swagger

### Parallel Opportunities

- T002 with T001; T005 with T003–T004
- All [P] test tasks within a story
- US1 and US2 test writing can overlap once Phase 2 is done

---

## Parallel Example: User Story 1

```bash
Task: "Integration tests for skip in internal/dao/release_safety_integration_test.go"
Task: "Service tests for Skip in internal/service/guided_training_services_test.go"
Task: "Handler tests for POST /scheduled-workouts/{id}/skip in internal/handler/guided_training_handlers_test.go"
```

---

## Implementation Strategy

### MVP First

1. Phases 1–2 (contract, migration, recovery fix)
2. Phase 3 (skip) → validate quickstart #1–5, #8–11
3. Phase 4 (undo) → validate quickstart #6, #7
4. Phase 5 gates; deploy API before the app release that calls skip/unskip

### Rollout

Additive fields and routes; older apps unaffected. The recovery fix applies to all clients on deploy.
