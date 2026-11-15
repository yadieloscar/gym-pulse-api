# Implementation Plan: Training Blocks Linked to Real Training

**Branch**: `spec/017-training-block-program-link-plan` | **Date**: 2026-11-14 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/017-training-block-program-link/spec.md`

## Summary

Link criteria-based training blocks to finished gym workouts without the block ever writing a
program, schedule, or workout. Stages gain an optional owned program and a link time. The block
aggregate derives exposure suggestions (finished `exercises` workouts of the current stage's
effective program, finalized inside the stage window, not yet recorded or dismissed). Create-exposure
accepts the workout reference, a new dismiss mutation stores "not this one", a new stage-program
mutation sets or clears a link, and one new read returns due next-morning checks and suggestions
across active blocks for Today. One additive migration; no backfill; no new dependency.

## Workflow and Specialist Scope

- **Feature**: `gympulse-training-block-program-link`, new scope, sibling app spec 030. Spec status
  Approved; this plan also corrects one spec defect found in planning (R1, feature detection).
- **`gym-pulse-api-engineering`**: handler → service → DAO lanes, ownership, idempotency, revision,
  contract and smoke gates (all FRs, tasks, verification).
- **`gym-pulse-cross-repo-delivery`**: additive compatibility, mixed-version window, and the
  detection rule the app depends on (FR-015, R1, Rollout).
- **`supabase-postgres-best-practices`**: partial indexes, FK delete behavior, anti-join cost, RLS on
  the new table (R3, R6, data-model.md).
- **`google-go-style`**: implementation and review only.

## Technical Context

**Language/Version**: Go 1.26.11 (after PR #49); PostgreSQL SQL migrations

**Primary Dependencies**: chi v5, pgx v5, google/uuid; no new dependency

**Storage**: PostgreSQL. Two columns on `criteria_training_stages`, one on
`criteria_training_exposures`, one new table, two indexes (see [data-model.md](data-model.md))

**Testing**: model and service unit tests with manual DAO mocks; handler and router tests; DAO
integration tests (`-tags=integration`); `go test ./...`; race suite; `./scripts/smoke-toggle.sh`;
`govulncheck`

**Target Platform**: Existing Linux API service and its PostgreSQL

**Project Type**: Authenticated JSON web service

**Performance Goals**: The due read answers Today in one request of two statements, bounded to 20
due checks and 20 suggestions; aggregate reads add one indexed query (SC-005)

**Constraints**: No write to programs, schedules, or workouts from block code; additive contract;
stable errors; atomic idempotent mutations; owner scoping on every new ID

**Scale/Scope**: One migration pair, three new routes, two changed request bodies, four changed
response shapes, one new error code

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- [x] Contract and `docs/CONTRACTS.md` changes are identified: new fields, three routes, error code
  `EXPOSURE_SOURCE_ALREADY_RECORDED`, contract minor version ([contract draft](contracts/training-block-program-link.openapi.yaml)).
- [x] Authentication, ownership, idempotency, revision, and error behavior are preserved or
  specified: shared 404 for every new ID; new idempotency scopes include path IDs (R8); state
  conflicts reuse `CONFLICT` (R4).
- [x] Tests are identified before implementation, including contract and smoke checks (see
  Verification).
- [x] Migrations document compatibility, existing-data behavior, and rollback: existing stages get
  `program_linked_at` = migration time, which is the intended "no old sessions" rule (R2); down
  migration removes only added objects (data-model.md).
- [x] New dependencies, layers, and concurrency are justified: none added.

**Post-design re-check**: still passing. The only design change to approved intent is R1 (detection
by fields), recorded in the spec in this change.

## Design

### Routes (`internal/router/router.go`)

| Method | Path | Handler | Note |
| --- | --- | --- | --- |
| `GET` | `/training-blocks/due` | `TrainingBlockHandler.Due` | Registered before `/{block_id}`; chi matches the static segment first. |
| `PUT` | `/training-blocks/{block_id}/stages/{stage_id}/program` | `SetStageProgram` | |
| `POST` | `/training-blocks/{block_id}/suggestions/{scheduled_workout_id}/dismiss` | `DismissSuggestion` | |
| `POST` | `/training-blocks/{block_id}/exposures` | `AddExposure` (changed) | Optional `scheduled_workout_id`. |
| `POST` | `/training-blocks` | `Create` (changed) | Optional stage `program_id`. |

### DAO (`internal/dao/training_block_dao.go`)

- `loadTrainingStages` selects `program_id` and `program_linked_at`; `effective_program_id` is
  derived in Go from stage and block.
- New `loadExposureSuggestions(ctx, q, userID, block, workoutID *uuid.UUID)`: the single R5
  predicate; `workoutID` filters to one row for mutation validation.
- `loadTrainingBlock` attaches suggestions for active blocks (empty slice otherwise) and
  `pending_next_morning`.
- `AddExposure`: when a reference is present, under the existing `FOR UPDATE` lock, check ownership
  (404), existing reference (409 `EXPOSURE_SOURCE_ALREADY_RECORDED`), and suggestion membership
  (422), then insert; map `23505` and `23503` as backstops (R3, R4).
- New `SetStageProgram` and `DismissSuggestion` following the `Transition` shape: lock, revision,
  validate, write, `bumpTrainingBlock`, reload, `storeTrainingBlockIdempotency`.
- New `Due(ctx, userID, today)`: due-check join plus `LATERAL` suggestions (R7).
- `List` adds `pending_next_morning` (bounded subquery) and stage program fields (R11).

### Service (`internal/service/training_block_svc.go`)

- Validates new request fields and resolves "today" from the training-profile timezone for `Due`,
  exactly as `AddExposure` does for dates.
- Builds prefill (R9) in the model so the DAO and tests share it.

### Handler (`internal/handler/training_block.go`)

- Three handlers using `decodeMutation`, `matchIdempotencyKey`, and `pathUUID`; errors go through
  `handleServiceError` (`internal/handler/response.go:37`). The new code follows the
  `model.SportSessionConflictError` pattern: a typed `model.ExposureSourceConflictError` carrying the
  authoritative block, mapped to 409 `EXPOSURE_SOURCE_ALREADY_RECORDED`.

### Unchanged on purpose

- `scheduleDAO.Delete`: foreign keys clear references and dismissals (R3).
- Program, schedule, plan-transition, session, participation, and stats code: no edits.

## Verification

| Requirement | Test |
| --- | --- |
| FR-001, FR-003 | Model tests for `effective_program_id`; DAO integration: foreign stage program → 404 |
| FR-002 | DAO integration: current/later stage allowed, earlier → `CONFLICT`, link time moves; replay |
| FR-004, FR-005, SC-006 | DAO integration: Q1, Q3, Q4, Q5 in [quickstart.md](quickstart.md) |
| FR-006, FR-007 | DAO integration: Q2; concurrent duplicate inserts → one exposure (race suite) |
| FR-008 | DAO integration: Q6; dismiss in one block leaves the other's suggestion |
| FR-010, FR-011 | DAO integration: Q7, bounds of 20, active blocks only |
| FR-012 | Existing transition tests extended: active program unchanged |
| FR-013 | DAO integration: Q8 through `scheduleDAO.Delete` |
| FR-014, SC-002 | Handler tests: key mismatch, stale revision, five identical retries |
| FR-015, SC-004 | Router/handler golden tests on pre-feature bodies; arrays present and `[]` when empty |
| FR-016 | `docs/CONTRACTS.md` updated in the same commit; `./scripts/smoke-toggle.sh` |
| FR-017 | Migration up/down applied in CI smoke job; `scripts/validate-database-design.sh` |
| FR-018 | `EXPLAIN` in quickstart run uses the partial index (recorded in tasks evidence) |
| SC-003 | Ownership table test across every new ID field and route (Q9) |
| SC-007 | Contract terminology review |
| Coverage | `internal/service` stays ≥90% |

## Project Structure

### Documentation (this feature)

```text
specs/017-training-block-program-link/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/training-block-program-link.openapi.yaml
├── checklists/requirements.md
└── tasks.md             # next stage (/speckit-tasks)
```

### Source Code (repository root)

```text
docs/CONTRACTS.md
internal/
├── dao/training_block_dao.go (+ training_block_dao_integration_test.go)
├── handler/training_block.go (+ _test.go)
├── model/training_block.go (+ _test.go)
├── router/router.go (+ _test.go)
└── service/training_block_svc.go (+ _test.go)
migrations/
├── 0NN_link_training_blocks_to_workouts.up.sql
└── 0NN_link_training_blocks_to_workouts.down.sql
```

**Structure Decision**: Extend the existing training-block lane in place. `0NN` is the next free
migration number at implementation time (R10).

## Rollout

1. PR #49 (Go 1.26.11) merges so CI is green.
2. This feature: migration, code, CONTRACTS, tests in one API PR. Additive; the released app ignores
   the new fields and never calls the new routes.
3. Deploy the API and run the smoke script.
4. App spec 030 implements against the deployed contract, detecting support by fields (R1).

## Cross-repo follow-ups

- App spec 030 FR-011 and its detection clarification say "a 404 from the due read"; correct them to
  field detection (R1) in the app repository before app planning.

## Complexity Tracking

No constitution violations.
