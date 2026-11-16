# Implementation Plan: One Active Program on Fixed Weekdays

**Branch**: `spec/031-one-active-program` | **Date**: 2026-11-16 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/031-one-active-program/spec.md`

## Summary

Make the program, not the profile, decide which days an athlete trains. Each program unit stores
its weekdays. One fill function creates sessions from that week and keeps the active program filled
56 days ahead, called from the schedule read and from every operation that starts or edits the
active program. Plan-transition preview and apply become the single start: they keep the active
program when none is named, and stop requiring a profile change. Saving the active program updates
its future sessions per unit in the same transaction. Two new routes move one session and remove a
saved program. One additive migration, no backfill, no new dependency, no background worker.

## Workflow and Specialist Scope

- **Feature**: `gympulse-one-active-program`, new scope, sibling app spec 037. Spec status Approved.
  This plan corrects two requirements found in planning (R5: FR-001 and FR-021), recorded in the
  spec in this change.
- **`gym-pulse-engineering-lead`**: cross-repository scope, single-agent decision, readiness.
- **`gym-pulse-api-engineering`**: handler → service → DAO lanes, ownership, idempotency, revision,
  transaction boundaries, contract and smoke gates (all FRs).
- **`gym-pulse-cross-repo-delivery`**: additive compatibility, the released app's mixed-version
  behavior, and the detection rule the app depends on (FR-019, R10, R11, Rollout).
- **`supabase-postgres-best-practices`**: partial unique index, array check, lock order, delete
  predicates (R3, R4, [data-model.md](data-model.md)).
- **`google-go-style`**: implementation and review only.
- **Delegation**: single agent for this plan. The design decisions are sequential and share the
  same files. An independent read-only review is required before the implementation PR merges.

## Technical Context

**Language/Version**: Go 1.26.11; PostgreSQL SQL migrations

**Primary Dependencies**: chi v5, pgx v5, google/uuid, go-playground/validator; no new dependency

**Storage**: PostgreSQL. One column on each of `program_workouts`, `programs`, and
`scheduled_workouts`; one check constraint; two indexes (see [data-model.md](data-model.md))

**Testing**: model and service unit tests with manual DAO mocks; handler and router tests; DAO
integration tests (`-tags=integration`); `go test ./...`; race suite; `./scripts/smoke-toggle.sh`;
`./scripts/validate-database-design.sh`; `internal/dbmigrate` lint and round trip

**Target Platform**: Existing Linux API service and its PostgreSQL

**Project Type**: Authenticated JSON web service

**Performance Goals**: A schedule read on an already-filled day adds one indexed single-row lookup.
The fill runs at most once per athlete per day and inserts at most 14 units × 8 weeks of rows in
one statement batch (SC-004, FR-022)

**Constraints**: Additive contract; released-app requests stay valid (FR-019); no session that is
started, finished, or missed is ever changed (FR-008); no session created before today (FR-013);
owner scoping on every ID

**Scale/Scope**: One migration pair, two new routes, four changed request bodies, three changed
response shapes, no new error code

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- [x] Contract and `docs/CONTRACTS.md` changes are identified: unit `weekdays` and `id`, create
  `active`, optional transition fields, three new preview fields, two routes, contract minor
  version ([contract draft](contracts/one-active-program.openapi.yaml)).
- [x] Authentication, ownership, idempotency, revision, and error behavior are preserved or
  specified: shared 404 on every new ID; move uses `expected_revision` and a new idempotency scope;
  state conflicts reuse `CONFLICT` with `resource` (R8, R9).
- [x] Tests are identified before implementation, including contract and smoke checks (see
  Verification).
- [x] Migrations document compatibility, existing-data behavior, and rollback: columns are nullable
  and unset on existing rows; no session is rewritten; the down migration removes only added
  objects ([data-model.md](data-model.md)).
- [x] New dependencies, layers, and concurrency are justified: none added. The rolling schedule
  uses the read path that already writes, not a worker (R4).

**Post-design re-check**: still passing. The design changes two requirement wordings (FR-001,
FR-021) without changing owner decisions D1 to D6.

## Design

### Model (`internal/model`)

- `ProgramWorkout` gains `Weekdays []int` (`json:"weekdays"`, always an array) and accepts `ID` on
  replace requests. `Program` gains unexported-from-JSON `ScheduledThrough`.
- New pure functions in `training_materialization.go`:
  - `PlaceUnits(units, availableDays, start)`: the one placement rule (R5).
  - `LayoutSessions(program, from, to)`: sessions for each unit on each of its weekdays, with
    `SlotDate` set. Replaces the dealing in `MaterializeProgramForWeekdays`, which is deleted once
    its callers move.
  - `DiffUnits(before, after)`: removed, added, weekdays-changed, content-changed, unchanged (R7).
  - `OffAvailability(program, availableDays)`.
- `ValidateWeekdays`: 1 to 7, unique, ascending after normalization; all-or-none per program.
- `CreateProgramRequest.Active *bool`; `PreviewPlanTransitionRequest.ProposedProfile` becomes a
  pointer, `To` optional; `ApplyPlanTransitionRequest.ExpectedProfileRevision` optional.
- `ScheduleHorizonDays = 56`.

### DAO (`internal/dao`)

- `program_dao.go`: select and insert `weekdays`; `Replace` updates matched units in place with the
  two-pass renumber (R2), takes the schedule lock, and when the program is active applies the unit
  diff to planned unstarted sessions dated after today (R7). New `Delete` (R9).
- New `schedule_fill.go`: `fillActiveProgram(ctx, tx, userID, today)` and
  `fillProgram(ctx, tx, userID, program, from, through)` shared by every caller (R4). Inserts with
  `ON CONFLICT (program_workout_id, slot_date) ... DO NOTHING` and advances `scheduled_through`.
  New exported `EnsureFilled(ctx, userID, today)` for the read path: a cheap check, then the fill
  in its own transaction under the program and schedule locks.
- `plan_transition_dao.go`: profile write only when a profile is supplied; the no-op branch when
  the target is the placed active program; otherwise place, activate, delete planned unstarted
  sessions dated on or after the start date with no upper bound, and call `fillProgram` (R6).
- `schedule_dao.go`: `MaterializeIdempotent` and `RegenerateIdempotent` receive layout sessions,
  place the program if unplaced, insert with `ON CONFLICT DO NOTHING`, and raise
  `scheduled_through` (R11). New `MoveIdempotent` (R8). `scanScheduledWorkout` is unchanged:
  `slot_date` is internal and not returned.
- Lock order everywhere: program namespace, then schedule namespace (already the order in
  plan-transition apply, materialize, and regenerate).

### Service (`internal/service`)

- `planTransitionService`: target resolution order (named program, named starter, active program,
  top starter); loads the current profile when none is proposed; computes `start_date`,
  `replaces_program`, and `off_availability`; previews with `PlaceUnits` and `LayoutSessions`
  without writing. Needs a `TrainingProfileDAO` and the clock, as `scheduleService` has.
- `scheduleService.List`: calls `EnsureFilled` before the existing read and lazy finalize. A
  missing training profile means no timezone and no fill, as today's lazy finalize already
  requires a profile. New `Move`.
- `programService`: `Create` honors `active`; `Replace` validates weekdays and unit IDs; new
  `Delete`.

### Handler and routes (`internal/handler`, `internal/router/router.go`)

| Method | Path | Handler | Note |
| --- | --- | --- | --- |
| `POST` | `/scheduled-workouts/{id}/move` | `ScheduleHandler.Move` | `decodeMutation`, `matchIdempotencyKey`, `pathUUID` |
| `DELETE` | `/programs/{id}` | `ProgramHandler.Delete` | 204; active → 409 `CONFLICT` |
| `POST` | `/programs` | `Create` (changed) | Optional `active`, unit `weekdays` |
| `PUT` | `/programs/{id}` | `Replace` (changed) | Unit `id`, `weekdays` |
| `POST` | `/plan-transitions/preview`, `/apply` | changed | Optional `proposed_profile`, `to` |
| `GET` | `/schedule` | `List` (changed behavior) | Fills before reading |

Errors go through `handleServiceError`; no new error type or code is needed.

### Unchanged on purpose

- Skip, unskip, recover, complete, set logging, session, participation calculation, statistics,
  health export, sport activity linking, training blocks, `from-starter`, `adopt-legacy`.
- The legacy weekly-plan routes (spec Non-Goals).

## Verification

| Requirement | Test |
| --- | --- |
| FR-001, FR-002 | Model tests for `PlaceUnits` (equal, fewer, and more units than days; start mid-week) and `ValidateWeekdays`; handler golden test: `weekdays` is always an array |
| FR-003, SC-001 | Model test: 12 weeks of `LayoutSessions` for a five-unit program, every session on its weekdays; DAO integration Q1 in [quickstart.md](quickstart.md) |
| FR-004, SC-005 | DAO integration: two concurrent fills produce one row per unit per date (race suite); recover onto an occupied date still succeeds |
| FR-005, SC-003 | Service and DAO tests: `active: false` leaves the active program and schedule rows unchanged; omitted `active` behaves as today (Q2) |
| FR-006, FR-007 | DAO integration Q3: apply replaces other programs' planned sessions past `to`; profile untouched when not proposed |
| FR-008 | DAO integration: start, save, and move with an in-progress, a completed, and a missed session present; none change (Q4) |
| FR-009 to FR-011, SC-002 | Service tests for target resolution; DAO integration Q5: custom program survives a profile change, `off_availability` lists the right units; no active program → top starter |
| FR-012, FR-013, SC-004 | Service test with a fixed clock: read 40 days later fills; second read writes nothing; skipped, moved, and deleted sessions are not re-created; nothing before today (Q6) |
| FR-014 | Handler and DAO tests Q7: move, replay, stale revision, bad date, wrong state, sport session |
| FR-015, FR-016 | Model tests for `DiffUnits`; DAO integration Q8: weekday change, content change, add, remove, and a surviving one-off move |
| FR-017 | DAO integration Q9: remove inactive (history kept, block link null), active → 409 |
| FR-018, SC-007 | Ownership table test across every new ID field and route (Q10); five identical retries |
| FR-019, SC-006 | Handler golden tests on pre-feature request bodies for create, replace, regenerate, materialize, preview, apply (Q11); released-app sequence "create, then transition with program_id" yields the same first week as before |
| FR-020 | `docs/CONTRACTS.md` updated in the same commit; `./scripts/smoke-toggle.sh` |
| FR-021 | `internal/dbmigrate` lint and round trip; `./scripts/validate-database-design.sh`; integration: existing rows unchanged after migration |
| FR-022 | `EXPLAIN` of the fill check, start delete, and edit delete recorded in tasks evidence |
| Coverage | `internal/service` and `internal/middleware` stay ≥90% |

## Project Structure

### Documentation (this feature)

```text
specs/031-one-active-program/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/one-active-program.openapi.yaml
├── checklists/requirements.md
└── tasks.md             # next stage (/speckit-tasks)
```

### Source Code (repository root)

```text
docs/CONTRACTS.md
docs/DATABASE_DESIGN.md
internal/
├── dao/program_dao.go, plan_transition_dao.go, schedule_dao.go (+ tests)
├── dao/schedule_fill.go (new, + integration test)
├── handler/program.go, schedule.go, plan_transition.go (+ tests)
├── model/training.go, training_materialization.go (+ tests)
├── router/router.go (+ test)
└── service/training_svc.go, plan_transition_svc.go, schedule_svc.go (+ tests)
migrations/
├── 045_program_weekdays_and_rolling_schedule.up.sql
└── 045_program_weekdays_and_rolling_schedule.down.sql
```

**Structure Decision**: Extend the existing program, schedule, and plan-transition lanes in place.
The fill gets its own DAO file because four callers share it.

## Rollout

1. This feature ships as one API PR: migration, code, CONTRACTS, DATABASE_DESIGN, tests. Additive.
2. Deploy the API (the `./migrate` pre-deploy job applies 045) and run the smoke script.
3. A released app keeps working. Two outcomes change for it, both intended: a profile edit no
   longer replaces a placed active program (FR-009), and its weeks stop rotating once a program is
   placed.
4. App spec 037 implements against the deployed contract, detecting support by `weekdays` (R10).
5. Retiring materialize, regenerate, and the legacy weekly plan is a later, separate change.

**Rollback**: Roll the image back; the columns are nullable and ignored by the previous image.
Sessions the fill created remain valid planned sessions. Run the down migration only if the columns
must go.

## Risks

| Risk | Mitigation |
| --- | --- |
| The schedule read now runs a write transaction once a day per athlete | Cheap pre-check; same locks as existing writes; integration test for concurrent reads |
| In-place unit updates change `Replace`, the most-used program write | Released-app path (no unit IDs) keeps delete-and-insert semantics; golden tests on old bodies |
| A long absence leaves up to 56 days of sessions that finalize as missed on return | Existing behavior for the old 28-day window; left as is and noted for the app spec |
| Weeks no longer rotate for programs with fewer units than days | Owner decision D2; called out in Rollout |

## Complexity Tracking

No constitution violations.
