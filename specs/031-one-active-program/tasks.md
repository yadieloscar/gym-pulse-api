# Tasks: One Active Program on Fixed Weekdays

**Input**: [spec.md](spec.md), [plan.md](plan.md), [data-model.md](data-model.md), [research.md](research.md), [quickstart.md](quickstart.md), [contract](contracts/one-active-program.openapi.yaml) | **Feature ID**: `gympulse-one-active-program`

Tests are required by the constitution (Principle IV) and by SC-001 to SC-007. Write each story's
tests first and see them fail for the intended gap before implementing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: can run in parallel (different files, no dependency on an unfinished task)
- **[Story]**: the user story in [spec.md](spec.md) the task belongs to

## Requirement and Verification Evidence

| Requirement / acceptance | Tasks | Verification | Result / limitation |
| --- | --- | --- | --- |
| FR-001, FR-002 | T005, T006, T011, T013 | `internal/model/training_materialization_test.go`, `internal/handler/training_test.go` | Planned |
| FR-003, SC-001 | T005, T007, T012, T014 | 12-week model test; integration Q1 | Planned |
| FR-004, SC-005 | T002, T008, T009 | Concurrent-fill integration test; recover onto an occupied date | Planned |
| FR-005, SC-003 | T015, T017 | Service and integration Q2 | Planned |
| FR-006, FR-007 | T016, T018, T019 | Integration Q3 | Planned |
| FR-008 | T016, T018, T030 | Integration Q4 | Planned |
| FR-009 to FR-011, SC-002 | T020, T021, T022 | Service tests; integration Q5 | Planned |
| FR-012, FR-013, SC-004 | T008, T009, T023, T024 | Fixed-clock service test; integration Q6 | Planned |
| FR-014 | T025, T026, T027 | Handler and integration Q7 | Planned |
| FR-015, FR-016 | T005, T028, T029, T030 | `DiffUnits` model tests; integration Q8 | Planned |
| FR-017 | T031, T032 | Integration Q9 | Planned |
| FR-018, SC-007 | T034 | Ownership table test Q10; five identical retries | Planned |
| FR-019, SC-006 | T010, T033 | Golden tests on pre-feature bodies Q11 | Planned |
| FR-020 | T035, T036 | `docs/CONTRACTS.md` diff; `./scripts/smoke-toggle.sh` | Planned |
| FR-021 | T002, T003, T004 | `internal/dbmigrate` lint and round trip; `./scripts/validate-database-design.sh` | Planned |
| FR-022 | T037 | `EXPLAIN` output recorded under Evidence | Planned |
| Coverage ≥90% (`internal/service`, `internal/middleware`) | T038 | `go test -coverprofile` | Planned |

## Phase 1: Setup

- [ ] T001 Fetch `origin/main`; confirm migration number 045 and contract version 1.10.0 are free (rename files and references here if not); record a green baseline for `go test ./...`

## Phase 2: Foundational (blocks every story)

- [ ] T002 Create `migrations/045_program_weekdays_and_rolling_schedule.up.sql` and `.down.sql` per [data-model.md](data-model.md): `program_workouts.weekdays` with `program_workouts_weekdays_check`, `programs.scheduled_through`, `scheduled_workouts.slot_date`, `uq_scheduled_workouts_unit_slot`, `idx_scheduled_workouts_program_planned`; re-runnable and one transaction per file (`docs/RELEASE_RUNBOOK.md` "Writing a migration")
- [ ] T003 Document the three columns, the constraint, both indexes, and migration 045 in `docs/DATABASE_DESIGN.md`; run `./scripts/validate-database-design.sh`
- [ ] T004 Run the `internal/dbmigrate` lint and the integration round trip for 045; add an integration assertion that existing program, unit, and session rows are unchanged after the migration in `internal/dao/release_migration_integration_test.go`
- [ ] T005 [P] Model tests first in `internal/model/training_materialization_test.go`: `PlaceUnits` (units equal to, fewer than, and more than days; start mid-week; one day), `LayoutSessions` (12 weeks, five units, two weekdays on one unit, a sport unit, `SlotDate` set), `DiffUnits` (removed, added, weekdays changed, content changed, unchanged), `OffAvailability`, `ValidateWeekdays` (range, duplicate, empty, mixed placed and unplaced)
- [ ] T006 Implement `Weekdays` on `ProgramWorkout` (always an array in JSON), `SlotDate` on `ScheduledWorkout` (not in JSON), `ScheduledThrough` on `Program` (not in JSON), `ScheduleHorizonDays`, and `ValidateWeekdays` in `internal/model/training.go`
- [ ] T007 Implement `PlaceUnits`, `LayoutSessions`, `DiffUnits`, and `OffAvailability` in `internal/model/training_materialization.go` until T005 passes
- [ ] T008 Integration tests first in new `internal/dao/schedule_fill_integration_test.go`: fill inserts only after `scheduled_through`, never before today, once per unit per slot under two concurrent fills, and starts after the latest planned session when `scheduled_through` is `NULL`; recover onto a date that holds the same unit still succeeds
- [ ] T009 Implement `fillProgram`, `fillActiveProgram`, and exported `EnsureFilled` in new `internal/dao/schedule_fill.go` (program lock then schedule lock, placement of an unplaced program, `ON CONFLICT (program_workout_id, slot_date) ... DO NOTHING`, advance `scheduled_through`); write `slot_date` in `insertScheduledWorkouts` in `internal/dao/schedule_dao.go`
- [ ] T010 Read and write `weekdays` in `internal/dao/program_dao.go` (`insertProgramWorkouts`, unit selects) and add `weekdays` to every program fixture and mock in `internal/service/mocks_test.go`; existing tests stay green with unplaced units returning `[]`

**Checkpoint**: migration applied, model rules proven, the fill usable by every story.

## Phase 3: User Story 1 - A program is a week with fixed weekdays (P1)

**Independent test**: quickstart Q1.

- [ ] T011 [P] [US1] Handler tests in `internal/handler/training_test.go`: create and replace accept `weekdays`; bad weekday, duplicate, empty list, and mixed placed/unplaced return 422 `workouts`; every unit in every program response has a `weekdays` array
- [ ] T012 [P] [US1] Integration test Q1 in new `internal/dao/one_active_program_integration_test.go`: five units on chosen weekdays, four weeks, a workout and a sport unit sharing a weekday
- [ ] T013 [US1] Validate weekdays in `programService.Create` and `Replace` in `internal/service/training_svc.go`; set `preferred_weekday` to the first weekday of a placed unit
- [ ] T014 [US1] Switch `MaterializeIdempotent` and `RegenerateIdempotent` (and their service callers in `internal/service/schedule_svc.go`) to `LayoutSessions` with placement of an unplaced program, `ON CONFLICT DO NOTHING`, and a raised `scheduled_through` (research R11); delete `MaterializeProgramForWeekdays` and the unused `materializeProgram` helper once no caller remains

## Phase 4: User Story 2 - Saving never starts; one explicit start (P1)

**Independent test**: quickstart Q2, Q3, Q4.

- [ ] T015 [P] [US2] Service tests in `internal/service/training_domain_svc_test.go`: `active: false` saves without deactivating; omitted `active` behaves as today
- [ ] T016 [P] [US2] Service tests for the transition in `internal/service/guided_training_services_test.go`: optional `proposed_profile` and `to`; `start_date` is `max(from, today)`; `replaces_program`; preview writes nothing; a stale token still conflicts
- [ ] T017 [US2] Add `Active *bool` to `CreateProgramRequest` in `internal/model/training.go` and honor it in `programService.Create` and the create DAO paths in `internal/dao/program_dao.go` and `internal/dao/training_mutation_dao.go`
- [ ] T018 [US2] Make `ProposedProfile` a pointer and `To` and `ExpectedProfileRevision` optional in `internal/model/training.go`; in `internal/service/plan_transition_svc.go` load the current profile when none is proposed, compute `start_date` and `replaces_program`, and preview with `PlaceUnits` and `LayoutSessions`; inject the profile DAO and clock in `cmd/server/main.go`
- [ ] T019 [US2] Rework `planTransitionDAO.Apply` in `internal/dao/plan_transition_dao.go`: profile write only when proposed; place and store weekdays for an unplaced target; activate; delete planned unstarted sessions dated on or after the start date with no upper bound; call `fillProgram`; integration tests Q3 and Q4 in `internal/dao/one_active_program_integration_test.go`

## Phase 5: User Story 3 - Profile changes keep the program (P1)

**Independent test**: quickstart Q5.

- [ ] T020 [P] [US3] Service tests for target resolution order (named program, named starter, active program, top starter) and `off_availability` in `internal/service/guided_training_services_test.go`
- [ ] T021 [US3] Implement the resolution order and `off_availability` (always an array) in `internal/service/plan_transition_svc.go` and the preview model in `internal/model/training.go`
- [ ] T022 [US3] Add the no-op branch to `planTransitionDAO.Apply` (target is the placed active program: no session changes) with integration test Q5, including "no active program → top starter is placed"

## Phase 6: User Story 4 - The schedule rolls forward (P1)

**Independent test**: quickstart Q6.

- [ ] T023 [P] [US4] Service tests with a fixed clock in `internal/service/training_domain_svc_test.go`: a read 40 days after the start fills; a second read does not call the fill; no profile or no active program means no fill
- [ ] T024 [US4] Call `EnsureFilled` at the start of `scheduleService.List` in `internal/service/schedule_svc.go`; integration test Q6 (skipped, moved, and deleted sessions are not re-created; nothing before today)

## Phase 7: User Story 5 - Move one date (P2)

**Independent test**: quickstart Q7.

- [ ] T025 [P] [US5] Handler tests in `internal/handler/schedule_resolution_handlers_test.go`: 200, replay, stale revision 409, wrong state 409 with `resource`, bad date 422 `date`, 404, header and key mismatch, a sport session moves
- [ ] T026 [US5] Implement `MoveIdempotent` in `internal/dao/schedule_dao.go` (schedule lock, replay, row lock, state and date checks, update `date` and `revision` only, `recomputeParticipation` for both dates, ledger) with integration test Q7
- [ ] T027 [US5] Implement `Move` in `internal/service/schedule_svc.go` and `internal/handler/training.go`; route `POST /scheduled-workouts/{id}/move` in `internal/router/router.go` with a router test

## Phase 8: User Story 6 - Editing the active program keeps the schedule in step (P2)

**Independent test**: quickstart Q8.

- [ ] T028 [P] [US6] Service tests in `internal/service/training_domain_svc_test.go`: a unit `id` that is not in the program is 422 `workouts`; a request without unit IDs is accepted
- [ ] T029 [US6] Rework `programDAO.Replace` in `internal/dao/program_dao.go`: update matched units in place with the two-pass renumber, insert new units, delete absent units (research R2)
- [ ] T030 [US6] In the same transaction, take the schedule lock and apply `DiffUnits` to planned unstarted sessions dated after today when the program is active (research R7); integration test Q8, including a surviving one-off move, an untouched in-progress session, and a released-app save followed by regenerate giving one schedule with no duplicates

## Phase 9: User Story 7 - Remove a saved program (P3)

**Independent test**: quickstart Q9.

- [ ] T031 [P] [US7] Handler and service tests: 204, 404, active → 409 `CONFLICT` with the program in `resource`, in `internal/handler/training_test.go` and `internal/service/training_domain_svc_test.go`
- [ ] T032 [US7] Implement `Delete` in `internal/dao/program_dao.go` (program lock then schedule lock; delete planned unstarted sessions, then the program), `internal/service/training_svc.go`, and `internal/handler/training.go`; route `DELETE /programs/{id}` with a router test; integration test Q9 (finished sessions keep snapshots; a training block's program link reads null)

## Phase 10: Cross-cutting

- [ ] T033 Golden tests on pre-feature request bodies in `internal/handler/guided_training_handlers_test.go` and `internal/dao/one_active_program_integration_test.go` (Q11): create, replace then regenerate, materialize twice, preview and apply with `proposed_profile` and `to`; "create, then transition with `program_id`" gives the same first week as `main`
- [ ] T034 Ownership table test Q10 across every new ID field and route, and five identical retries of create, replace, apply, move, and delete, in `internal/dao/one_active_program_integration_test.go`
- [ ] T035 Document contract 1.10.0 in `docs/CONTRACTS.md` in the same commit as the code: unit `weekdays` and `id`, create `active`, optional transition fields, `start_date`, `replaces_program`, `off_availability`, the changed statements listed in the spec's "Changes to approved specs and contracts", both new routes, and the detection rule; regenerate Swagger if the repository generates it
- [ ] T036 Add a smoke step to `scripts/smoke.sh`: save with weekdays and `active: false` → preview → apply → schedule on the chosen weekdays → move → edit a weekday → remove the old program
- [ ] T037 Record `EXPLAIN` for the `EnsureFilled` check, the start delete, and the edit delete against the integration database under Evidence below
- [ ] T038 If the data export includes programs, add `weekdays` to `internal/model/export.go` and `internal/dao/export_dao.go` with test updates; otherwise record that no change is needed

## Phase 11: Verification

- [ ] T039 Run and record: `gofmt`, `go test ./...`, `golangci-lint run`, `go test -race ./...`, the tagged integration suite, `./scripts/validate-database-design.sh`, `./scripts/smoke-toggle.sh`, `govulncheck ./...`, and service coverage
- [ ] T040 Independent read-only review of the final diff (transactions and lock order, the fill under concurrency, released-app compatibility, ownership); resolve or record each finding
- [ ] T041 Update the evidence table above with actual results and tell the app session the deployed contract version

## Dependencies

- Phase 2 blocks everything. T002 → T003, T004. T005 → T006, T007. T008 → T009. T010 follows T006.
- US1 (Phase 3) needs Phase 2. US2 needs US1's T014 for a shared layout path. US3 builds on US2's
  transition changes (T018, T019). US4 needs only Phase 2.
- US5 and US7 need only Phase 2. US6 needs T009 and T010.
- Phase 10 needs every story it tests. T035 lands with the code it documents.

## Parallel examples

- After Phase 2: T011 and T012 together; T015 and T016 together; T023, T025, T028, and T031 are all
  test-only tasks in different files.
- US4, US5, and US7 can proceed in parallel with US2 and US3 once Phase 2 is done; they touch
  `schedule_svc.go`, `schedule_dao.go`, and `program_dao.go`, so merge order matters for US5, US6,
  and US7.

## Implementation strategy

1. **Minimum useful slice**: Phases 1 to 6 (the four P1 stories). After them a custom program lands
   on chosen weekdays, saving does not start, profile edits keep the program, and the schedule
   rolls forward. The released app already benefits.
2. **Then** US5 (move), US6 (edit in step), US7 (remove).
3. **One API pull request** for the feature, as the plan's rollout states. Commit per phase so the
   review can follow the stories.

## Evidence

Record command output, `EXPLAIN` plans, and review findings here as tasks complete.
