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
| FR-001, FR-002 | T005, T006, T011, T013 | `internal/model/training_materialization_test.go`, `internal/handler/training_test.go` | Passed (see Evidence) |
| FR-003, SC-001 | T005, T007, T012, T014 | 12-week model test; integration Q1 | Passed (see Evidence) |
| FR-004, SC-005 | T002, T008, T009 | Concurrent-fill integration test; recover onto an occupied date | Passed (see Evidence) |
| FR-005, SC-003 | T015, T017 | Service and integration Q2 | Passed (see Evidence) |
| FR-006, FR-007 | T016, T018, T019 | Integration Q3 | Passed (see Evidence) |
| FR-008 | T016, T018, T030 | Integration Q4 | Passed (see Evidence) |
| FR-009 to FR-011, SC-002 | T020, T021, T022 | Service tests; integration Q5 | Passed (see Evidence) |
| FR-012, FR-013, SC-004 | T008, T009, T023, T024 | Fixed-clock service test; integration Q6 | Passed (see Evidence) |
| FR-014 | T025, T026, T027 | Handler and integration Q7 | Passed (see Evidence) |
| FR-015, FR-016 | T005, T028, T029, T030 | `DiffUnits` model tests; integration Q8 | Passed (see Evidence) |
| FR-017 | T031, T032 | Integration Q9 | Passed (see Evidence) |
| FR-018, SC-007 | T034 | Ownership table test Q10; five identical retries | Passed (see Evidence) |
| FR-019, SC-006 | T010, T033 | Golden tests on pre-feature bodies Q11 | Passed (see Evidence) |
| FR-020 | T035, T036 | `docs/CONTRACTS.md` diff; `./scripts/smoke-toggle.sh` | Passed (see Evidence) |
| FR-021 | T002, T003, T004 | `internal/dbmigrate` lint and round trip; `./scripts/validate-database-design.sh` | Passed (see Evidence) |
| FR-022 | T037 | `EXPLAIN` output recorded under Evidence | Passed (see Evidence) |
| Coverage ≥90% (`internal/service`, `internal/middleware`) | T038 | `go test -coverprofile` | Passed (see Evidence) |

## Phase 1: Setup

- [X] T001 Fetch `origin/main`; confirm migration number 045 and contract version 1.10.0 are free (rename files and references here if not); record a green baseline for `go test ./...`

## Phase 2: Foundational (blocks every story)

- [X] T002 Create `migrations/045_program_weekdays_and_rolling_schedule.up.sql` and `.down.sql` per [data-model.md](data-model.md): `program_workouts.weekdays` with `program_workouts_weekdays_check`, `programs.scheduled_through`, `scheduled_workouts.slot_date`, `uq_scheduled_workouts_unit_slot`, `idx_scheduled_workouts_program_planned`; re-runnable and one transaction per file (`docs/RELEASE_RUNBOOK.md` "Writing a migration")
- [X] T003 Document the three columns, the constraint, both indexes, and migration 045 in `docs/DATABASE_DESIGN.md`; run `./scripts/validate-database-design.sh`
- [X] T004 Run the `internal/dbmigrate` lint and the integration round trip for 045; add an integration assertion that existing program, unit, and session rows are unchanged after the migration in `internal/dao/release_migration_integration_test.go`
- [X] T005 [P] Model tests first in `internal/model/training_materialization_test.go`: `PlaceUnits` (units equal to, fewer than, and more than days; start mid-week; one day), `LayoutSessions` (12 weeks, five units, two weekdays on one unit, a sport unit, `SlotDate` set), `DiffUnits` (removed, added, weekdays changed, content changed, unchanged), `OffAvailability`, `ValidateWeekdays` (range, duplicate, empty, mixed placed and unplaced)
- [X] T006 Implement `Weekdays` on `ProgramWorkout` (always an array in JSON), `SlotDate` on `ScheduledWorkout` (not in JSON), `ScheduledThrough` on `Program` (not in JSON), `ScheduleHorizonDays`, and `ValidateWeekdays` in `internal/model/training.go`
- [X] T007 Implement `PlaceUnits`, `LayoutSessions`, `DiffUnits`, and `OffAvailability` in `internal/model/training_materialization.go` until T005 passes
- [X] T008 Integration tests first in new `internal/dao/schedule_fill_integration_test.go`: fill inserts only after `scheduled_through`, never before today, once per unit per slot under two concurrent fills, and starts after the latest planned session when `scheduled_through` is `NULL`; recover onto a date that holds the same unit still succeeds
- [X] T009 Implement `fillProgram`, `fillActiveProgram`, and exported `EnsureFilled` in new `internal/dao/schedule_fill.go` (program lock then schedule lock, placement of an unplaced program, `ON CONFLICT (program_workout_id, slot_date) ... DO NOTHING`, advance `scheduled_through`); write `slot_date` in `insertScheduledWorkouts` in `internal/dao/schedule_dao.go`
- [X] T010 Read and write `weekdays` in `internal/dao/program_dao.go` (`insertProgramWorkouts`, unit selects) and add `weekdays` to every program fixture and mock in `internal/service/mocks_test.go`; existing tests stay green with unplaced units returning `[]`

**Checkpoint**: migration applied, model rules proven, the fill usable by every story.

## Phase 3: User Story 1 - A program is a week with fixed weekdays (P1)

**Independent test**: quickstart Q1.

- [X] T011 [P] [US1] Handler tests in `internal/handler/training_test.go`: create and replace accept `weekdays`; bad weekday, duplicate, empty list, and mixed placed/unplaced return 422 `workouts`; every unit in every program response has a `weekdays` array
- [X] T012 [P] [US1] Integration test Q1 in new `internal/dao/one_active_program_integration_test.go`: five units on chosen weekdays, four weeks, a workout and a sport unit sharing a weekday
- [X] T013 [US1] Validate weekdays in `programService.Create` and `Replace` in `internal/service/training_svc.go`; set `preferred_weekday` to the first weekday of a placed unit
- [X] T014 [US1] Switch `MaterializeIdempotent` and `RegenerateIdempotent` (and their service callers in `internal/service/schedule_svc.go`) to `LayoutSessions` with placement of an unplaced program, `ON CONFLICT DO NOTHING`, and a raised `scheduled_through` (research R11); delete `MaterializeProgramForWeekdays` and the unused `materializeProgram` helper once no caller remains

## Phase 4: User Story 2 - Saving never starts; one explicit start (P1)

**Independent test**: quickstart Q2, Q3, Q4.

- [X] T015 [P] [US2] Service tests in `internal/service/training_domain_svc_test.go`: `active: false` saves without deactivating; omitted `active` behaves as today
- [X] T016 [P] [US2] Service tests for the transition in `internal/service/guided_training_services_test.go`: optional `proposed_profile` and `to`; `start_date` is `max(from, today)`; `replaces_program`; preview writes nothing; a stale token still conflicts; an unplaced target with no training profile is 422 `available_days` (spec edge case)
- [X] T017 [US2] Add `Active *bool` to `CreateProgramRequest` in `internal/model/training.go` and honor it in `programService.Create` and the create DAO paths in `internal/dao/program_dao.go` and `internal/dao/training_mutation_dao.go`
- [X] T018 [US2] Make `ProposedProfile` a pointer and `To` and `ExpectedProfileRevision` optional in `internal/model/training.go`; in `internal/service/plan_transition_svc.go` load the current profile when none is proposed, compute `start_date` and `replaces_program`, and preview with `PlaceUnits` and `LayoutSessions`; inject the profile DAO and clock in `cmd/server/main.go`
- [X] T019 [US2] Rework `planTransitionDAO.Apply` in `internal/dao/plan_transition_dao.go`: profile write only when proposed; place and store weekdays for an unplaced target; activate; delete planned unstarted sessions dated on or after the start date with no upper bound; call `fillProgram`; integration tests Q3 and Q4 in `internal/dao/one_active_program_integration_test.go`

## Phase 5: User Story 3 - Profile changes keep the program (P1)

**Independent test**: quickstart Q5.

- [X] T020 [P] [US3] Service tests for target resolution order (named program, named starter, active program, top starter) and `off_availability` in `internal/service/guided_training_services_test.go`
- [X] T021 [US3] Implement the resolution order and `off_availability` (always an array) in `internal/service/plan_transition_svc.go` and the preview model in `internal/model/training.go`
- [X] T022 [US3] Add the no-op branch to `planTransitionDAO.Apply` (target is the placed active program: no session changes) with integration test Q5, including "no active program → top starter is placed"

## Phase 6: User Story 4 - The schedule rolls forward (P1)

**Independent test**: quickstart Q6.

- [X] T023 [P] [US4] Service tests with a fixed clock in `internal/service/training_domain_svc_test.go`: a read 40 days after the start fills; a second read does not call the fill; no profile or no active program means no fill
- [X] T024 [US4] Call `EnsureFilled` at the start of `scheduleService.List` in `internal/service/schedule_svc.go`; integration test Q6 (skipped, moved, and deleted sessions are not re-created; nothing before today)

## Phase 7: User Story 5 - Move one date (P2)

**Independent test**: quickstart Q7.

- [X] T025 [P] [US5] Handler tests in `internal/handler/schedule_resolution_handlers_test.go`: 200, replay, stale revision 409, wrong state 409 with `resource`, bad date 422 `date`, 404, header and key mismatch, a sport session moves
- [X] T026 [US5] Implement `MoveIdempotent` in `internal/dao/schedule_dao.go` (schedule lock, replay, row lock, state and date checks, update `date` and `revision` only, `recomputeParticipation` for both dates, ledger) with integration test Q7
- [X] T027 [US5] Implement `Move` in `internal/service/schedule_svc.go` and `internal/handler/training.go`; route `POST /scheduled-workouts/{id}/move` in `internal/router/router.go` with a router test

## Phase 8: User Story 6 - Editing the active program keeps the schedule in step (P2)

**Independent test**: quickstart Q8.

- [X] T028 [P] [US6] Service tests in `internal/service/training_domain_svc_test.go`: a unit `id` that is not in the program is 422 `workouts`; a request without unit IDs is accepted
- [X] T029 [US6] Rework `programDAO.Replace` in `internal/dao/program_dao.go`: update matched units in place with the two-pass renumber, insert new units, delete absent units (research R2)
- [X] T030 [US6] In the same transaction, take the schedule lock and apply `DiffUnits` to planned unstarted sessions dated after today when the program is active (research R7); integration test Q8, including a surviving one-off move, an untouched in-progress session, and a released-app save followed by regenerate giving one schedule with no duplicates

## Phase 9: User Story 7 - Remove a saved program (P3)

**Independent test**: quickstart Q9.

- [X] T031 [P] [US7] Handler and service tests: 204, 404, active → 409 `CONFLICT` with the program in `resource`, in `internal/handler/training_test.go` and `internal/service/training_domain_svc_test.go`
- [X] T032 [US7] Implement `Delete` in `internal/dao/program_dao.go` (program lock then schedule lock; delete planned unstarted sessions, then the program), `internal/service/training_svc.go`, and `internal/handler/training.go`; route `DELETE /programs/{id}` with a router test; integration test Q9 (finished sessions keep snapshots; a training block's program link reads null)

## Phase 10: Cross-cutting

- [X] T033 Golden tests on pre-feature request bodies in `internal/handler/guided_training_handlers_test.go` and `internal/dao/one_active_program_integration_test.go` (Q11): create, replace then regenerate, materialize twice, preview and apply with `proposed_profile` and `to`; "create, then transition with `program_id`" gives the same first week as `main`
- [X] T034 Ownership table test Q10 across every new ID field and route, and five identical retries of create, replace, apply, move, and delete, in `internal/dao/one_active_program_integration_test.go`
- [X] T035 Document contract 1.10.0 in `docs/CONTRACTS.md` in the same commit as the code: unit `weekdays` and `id`, create `active`, optional transition fields, `start_date`, `replaces_program`, `off_availability`, the changed statements listed in the spec's "Changes to approved specs and contracts", both new routes, and the detection rule; regenerate Swagger if the repository generates it
- [X] T036 Add a smoke step to `scripts/smoke.sh`: save with weekdays and `active: false` → preview → apply → schedule on the chosen weekdays → move → edit a weekday → remove the old program
- [X] T037 Record `EXPLAIN` for the `EnsureFilled` check, the start delete, and the edit delete against the integration database under Evidence below
- [X] T038 If the data export includes programs, add `weekdays` to `internal/model/export.go` and `internal/dao/export_dao.go` with test updates; otherwise record that no change is needed

## Phase 11: Verification

- [X] T039 Run and record: `gofmt`, `go test ./...`, `golangci-lint run`, `go test -race ./...`, the tagged integration suite, `./scripts/validate-database-design.sh`, `./scripts/smoke-toggle.sh`, `govulncheck ./...`, and service coverage
- [X] T040 Independent read-only review of the final diff (transactions and lock order, the fill under concurrency, released-app compatibility, ownership); resolve or record each finding
- [X] T041 Update the evidence table above with actual results and tell the app session the deployed contract version

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

### Implementation run (2026-11-16, branch `feat/031-one-active-program`)

Commands run from the repository root against a disposable PostgreSQL 16 container with
`scripts/ci/runtime-role.sql` mounted and migrations applied by `go run ./cmd/migrate`.

| Gate | Command | Result |
| --- | --- | --- |
| Baseline before changes | `go test ./...`; tagged integration suite | Passed |
| Format | `gofmt -l internal cmd` | No files |
| Unit tests | `go test ./...` | Passed |
| Race | `go test -race ./...` | Passed |
| Lint | `golangci-lint run` | 0 issues |
| Integration | `go test -race -tags=integration ./internal/dao -count=1` | Passed 6 runs in a row after the review fixes; 0 skipped tests |
| Integration, both packages in one command | `go test -race -tags=integration ./internal/dao ./internal/dbmigrate -count=1` | Passed 4 of 5 runs. 1 failure: a deadlock between a DAO test's account cleanup and the migration re-run test altering the same tables (confirmed in the PostgreSQL log). The two packages now run with `-p 1` locally; CI already runs them as separate steps |
| Migration rules | `internal/dbmigrate` lint and round trip (part of the two lines above) | Passed for 045 |
| Database design | `./scripts/validate-database-design.sh` | Passed: 43 tables, 450 columns, 51 indexes |
| Row-level security | `scripts/validate-supabase-security.sh` inside the database container | Passed |
| Coverage | `go test -coverprofile ./internal/service ./internal/middleware` | service 90.9%, middleware 92.2% |
| Vulnerabilities | `govulncheck ./...` | No vulnerability in called code |
| Smoke | `scripts/smoke.sh` against this branch's server running as `gym_pulse_api` | 40 passed, 0 failed, including new step 13l; rerun after the review fixes |
| Swagger | `swag init -g cmd/server/main.go --output docs` | Regenerated and committed |

Not run: `./scripts/smoke-toggle.sh` itself. It recreates the shared development container on port
8080, which other sessions use. `scripts/smoke.sh`, the script it wraps, ran against a server built
from this branch on another port instead.

### Independent review (T040, 2026-11-16)

A read-only reviewer with fresh context read the final diff against the spec. It reported no
blocker, and confirmed lock order (program, then schedule) on every two-lock path, owner scoping of
every statement, and the `ON CONFLICT` partial-index target.

| # | Severity | Finding | Resolution |
| --- | --- | --- | --- |
| 1 | Major | A program started again through `PUT active:true` was filled only from its stale fill mark, leaving about seven weeks empty. | Fixed: a program that loses its future has its mark cleared; a save that starts a program lays it out from tomorrow. `TestSaveThatRestartsAProgramFillsItFromTomorrow` |
| 2 | Major | Creating an active program (the default for older clients) left the replaced program's future sessions, so weeks held two programs. | Fixed in `CreateProgram`, `Create`, and `CreateIdempotent`: the replaced program's sessions after today are removed. `TestCreatingAnActiveProgramRetiresTheReplacedProgramsFuture` |
| 3 | Major | A retried start returned 409 "stale preview" instead of replaying, because the token was checked before the idempotency record. | Fixed: the record is looked up first. `TestRetriedStartIsReplayedNotJudgedStale`, `TestStartRecordAnswersARetry` |
| 4 | Minor | Removing a program deleted past sessions that were still `planned`. | Fixed: only today's and later sessions go. `TestRemoveProgramKeepsPastSessionsThatAreStillPlanned` |
| 5 | Minor | Starting a program through a save doubled today. | Fixed with finding 1: the fill starts tomorrow. Same test |
| 6 | Minor | An older client echoing unit IDs without `weekdays` re-dealt a placed week. | Fixed: known units keep their stored weekdays. `TestSaveWithoutWeekdaysKeepsThePlacedWeek` |
| 7 | Minor | Regenerate can drop a session moved into its range from a slot outside it. | Accepted and documented in CONTRACTS: older clients cannot move, and clients on 1.10.0 do not call regenerate. |
| 8 | Minor | The preview token did not cover the target program's revision. | Fixed: the revision is part of the token. `TestStartPreviewGoesStaleWhenTheProgramIsEdited` |
| 9 | Minor | A start naming an active, placed, never-filled program returned no sessions. | Fixed: that branch fills. `TestStartOfAnUnfilledActiveProgramFillsIt` |
| Tests | | `TestConcurrentFillsCreateEachSlotOnce` cannot make two fills race, because the advisory locks serialize them. | Comment corrected to say so; the index is proven by `TestSlotIndexRejectsASecondSessionForTheSameSlot`. |
| Contract | | Three places in CONTRACTS did not say what the code did. | Rewritten with the fixes above. |

Not changed: a replayed start returns fewer fields than the first response (no recommended starter,
no `replaces_program`). CONTRACTS states exactly what a replay returns.

### Rollout note

Migration 045 alters three tables that requests write to. Each `ALTER` is fast, but if it runs
while requests are in flight PostgreSQL can pick a deadlock victim: either the migration job
(re-run it; it is re-runnable) or one request (a 500 that the client's idempotent retry repeats
safely). Run the pre-deploy migrate job at a quiet moment.

### Query plans (T037)

Captured with 300 athletes and 126,600 sessions loaded inside a rolled-back transaction, after
`ANALYZE`.

| Query | Plan |
| --- | --- |
| Fill check (`EnsureFilled`) | Index Scan on `idx_programs_one_active_per_user` |
| Start delete (all future planned of one athlete) | Index Scan on `idx_scheduled_workouts_user_date` (`user_id`, `date >=`) |
| Edit delete by unit | Index Scan on `idx_scheduled_workouts_user_date` (`user_id`, `date >=`), unit as filter |
| Removal delete by program | Bitmap Index Scan on `idx_scheduled_workouts_program_planned` and `idx_scheduled_workouts_user_date` |
| Latest session of a program (first fill) | Index Scan on `idx_scheduled_workouts_user_date` |
| Fill insert | Conflict arbiter index `uq_scheduled_workouts_unit_slot` |

### Differences from the plan

- `ReplaceProgram` and `DeleteProgram` live in `internal/dao/training_mutation_dao.go`, the
  idempotent path the service already used; the plan named `program_dao.go`.
- A unit `id` the program does not own is treated as a new unit, not a 422 (research R2, finding
  A3): a stale client ID must not fail a save that used to succeed, and no foreign row is touched.
- T014: `MaterializeProgramForWeekdays` and the unused `materializeProgram` helper are deleted.
- T038: the data export does not include programs, so no export change was needed.
- T004: the "existing rows unchanged" assertion is covered by the migration round trip plus
  `TestEnsureFilledContinuesAfterSessionsFromBeforeTheWatermark`; no separate test was added to
  `release_migration_integration_test.go`.
- Found while testing: `POST /programs` and `PUT /programs/{id}` without the optional `roadmap`
  returned 500 (a NOT NULL violation). Both now store `{}`.
- Found while building the app's mock server: a unit sent back with the `weekdays: []` it was read
  with was rejected. An empty list now means unplaced, so a program round-trips (spec User Story 1,
  scenario 5 amended).
- `scripts/validate-database-design.sh` did not parse `ADD COLUMN IF NOT EXISTS` or
  `CREATE INDEX IF NOT EXISTS`, which strict migrations require. It now does.

### Follow-up fix (2026-11-17): the unit-replacing save without a clock

`programDAO.Replace` deletes every unit of a program and inserts the saved ones. Deleting a unit
only clears `program_workout_id` on its sessions (`ON DELETE SET NULL`, kept so finished workouts
survive), which is how production came to show sessions of a workout the program no longer had
(repaired by migration 046). The service has used `ReplaceProgram` in
`training_mutation_dao.go` since this feature, so `Replace` is reached only when no mutation DAO
is wired, but it still produced that state. It now removes the program's untouched sessions dated
after the athlete's today and moves the fill mark back to today, as 046 does, and takes the
schedule lock. Restores FR-010 (saving the active program updates its future) on that path.

- Test: `TestProgramReplaceLeavesNoSessionWithoutAUnit` in
  `internal/dao/program_replace_integration_test.go` (pass, 2026-11-17).
- Not changed: the `ON DELETE SET NULL` foreign key. `CASCADE` would delete finished and started
  sessions with their unit, and the current save path already removes the future first.
- Already enforced in the data: one active program per athlete, by the partial unique index
  `idx_programs_one_active_per_user` from migration 014. Every path that activates a program
  deactivates the other one first, inside the per-athlete program lock.

### Follow-up fix (2026-11-17): saving the active program timed out in production

The owner replaced a workout and added a sport session on the phone and got "Couldn't save your
program". The server logged `context canceled`: the app gave up at its 15-second timeout and the
transaction rolled back, so nothing was saved. Laying the changed units out to the 56-day horizon
wrote 24 sessions and 288 sets with one statement each, about 360 round trips. The API runs in
US East and the database in us-west-1, so each one costs tens of milliseconds.

`insertLayoutSessions`, `insertScheduledSets`, and `insertProgramExercises` now send their rows as
pgx batches: one round trip for the sessions, one for all their sets, one per workout's exercises.
No behavior, contract, or schema change. Restores FR-010 on a remote database.

| Check | Result |
| --- | --- |
| Same save through a proxy adding 60 ms per round trip | 23.9 s before, 3.5 s after (the 3.5 s includes four reads the script makes first) |
| `TestSavingTheActiveProgramStaysWithinARoundTripBudget` | pass: 24 sessions, 288 sets, 19 statements and 4 batches (budget 60) |
| Full integration suite, unit tests, lint | pass |
| The save on the owner's phone against production | not verified until deployed |

Not changed: a 60-day schedule read still runs about 56 statements (3.7 s through the same
proxy), and four malformed payloads (duplicate exercise order, unknown catalog id, an exercise
with neither reps nor time, duplicate workout position) return 500 where 422 is intended.

### Consistency analysis (2026-11-16, before implementation)

Read-only pass over spec, plan, research, data model, contract draft, and tasks.

| # | Finding | Severity | Resolution |
| --- | --- | --- | --- |
| A1 | Spec FR-019 and SC-006 named one behavior change for a released app; the plan's rollout names two (FR-009 and weeks no longer rotating). | Medium | Spec amended to name both. |
| A2 | The spec edge case "unplaced program and no training profile → 422" had no task. | Low | Added to T016. |
| A3 | A unit `id` that is not in the program returns 422 `workouts`, while FR-018 says foreign IDs share 404. | Low | Kept: the ID is a body field of an owned program, and 422 names the field. The contract draft states it. |
| A4 | No requirement lacks a task; no task lacks a requirement; no constitution conflict. | None | None needed. |
