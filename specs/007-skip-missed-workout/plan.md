# Implementation Plan: Skip a missed workout

**Branch**: `spec/007-skip-missed-workout` | **Date**: 2026-10-11 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/007-skip-missed-workout/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command. See `docs/SPEC_WORKFLOW.md` and `.agents/skills/speckit-plan/SKILL.md` for the workflow.

## Summary

Record how an athlete resolved a missed scheduled workout. Two nullable columns (`resolution`,
`resolved_at`) mark a missed workout as skipped or recovered without touching its outcome. New
`POST /scheduled-workouts/{id}/skip` and `/unskip` routes follow the existing `/complete` mutation
pattern. `POST /schedule/recover` now selects only unresolved misses and marks its source as
recovered in the same transaction, fixing the existing behavior where the same miss could be copied
repeatedly. A migration backfills sources already recovered.

## Workflow and Specialist Scope

- **Feature**: `gympulse-today-next-action`, new scope. Sibling app plan:
  [../../../gym-pulse-app/specs/007-today-next-action/plan.md](../../../gym-pulse-app/specs/007-today-next-action/plan.md).
- **gym-pulse-api-engineering**: contract, migration, mutation layering, tests, CI gates.
- **google-go-style**: receiver names, error wrapping, keyed fields in new code.
- **gym-pulse-cross-repo-delivery**: API ships and deploys before the dependent app release;
  contract updated first.
- Spec amended during planning (Clarifications, "planning discovery") to define *resolved* as
  skipped or recovered after mapping found recovery never recorded its source (research R2).

## Technical Context

**Language/Version**: Go 1.26.9

**Primary Dependencies**: chi router, pgx v5, golang-migrate, swag v1.16.4 (existing; no new dependencies)

**Storage**: PostgreSQL (Supabase); `scheduled_workouts` gains `resolution`, `resolved_at`

**Testing**: `go test ./...` with service fakes (`internal/service/guided_training_services_test.go`, `release_coverage_test.go`), handler stubs (`internal/handler/guided_training_handlers_test.go`), router tests, `-tags=integration` DAO tests (`internal/dao/release_safety_integration_test.go`), `scripts/smoke-toggle.sh`

**Target Platform**: Linux container API

**Project Type**: web service

**Performance Goals**: One indexed single-row update per skip/unskip; recovery adds one predicate and one update inside its existing transaction

**Constraints**: Additive, backward-compatible JSON; idempotent and revision-checked; advisory-lock serialized per user like other schedule mutations

**Scale/Scope**: 1 migration, 2 routes, 1 changed query path (2 code paths), 1 new error code

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- [x] Contract and `docs/CONTRACTS.md` changes are identified. ([contracts/scheduled-workout-resolution.md](contracts/scheduled-workout-resolution.md): shape, two routes, recover side effect, `RESOLUTION_CONFLICT`.)
- [x] Authentication, ownership, idempotency, revision, and error behavior are preserved or specified. (Reuses `/complete` pattern: advisory lock, `findIdempotency`, `user_id` scoping, revision check; research R4–R5.)
- [x] Tests are identified before implementation, including contract and smoke checks. ([quickstart.md](quickstart.md) scenarios 1–12; smoke additions for skip and unskip.)
- [x] Migrations document compatibility, existing-data behavior, and rollback. (Additive nullable columns, documented backfill, down migration drops columns; research R3, R7.)
- [x] New dependencies, layers, and concurrency are justified. (None added.)

Post-design re-check: passes; no violations to track.

## Project Structure

### Documentation (this feature)

```text
specs/007-skip-missed-workout/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── scheduled-workout-resolution.md
└── tasks.md             # /speckit-tasks
```

### Source Code (repository root)

```text
migrations/
├── 020_add_scheduled_workout_resolution.up.sql     # columns, check, backfill
└── 020_add_scheduled_workout_resolution.down.sql

internal/model/training.go          # ScheduledWorkout.Resolution, ResolvedAt; resolution constants; ResolutionConflictError
internal/dao/schedule_dao.go        # select lists; RecoverIdempotent filter + source update
internal/dao/training_mutation_dao.go  # loadScheduledWorkout columns; SkipScheduledWorkout, UnskipScheduledWorkout
internal/service/schedule_svc.go    # Skip, Unskip; RecoverToday fallback filter
internal/handler/training.go        # Skip, Unskip handlers + swag annotations
internal/handler/response.go        # RESOLUTION_CONFLICT mapping
internal/router/router.go           # two routes

docs/CONTRACTS.md, docs/DATABASE_DESIGN.md, docs/{docs.go,swagger.json,swagger.yaml}
scripts/smoke.sh                    # skip + unskip wire checks

tests: internal/service/*_test.go, internal/handler/guided_training_handlers_test.go,
       internal/router/training_routes_test.go, internal/dao/release_safety_integration_test.go
```

**Structure Decision**: Existing handler → service → DAO layering for scheduled-workout mutations;
no new packages.

## Delivery Order

1. Contract and database design docs (contract drift gate).
2. Migration with backfill and its integration test.
3. Model fields and every `scheduled_workouts` select/scan path.
4. Recovery filter and source resolution (both paths) with tests proving no double copy.
5. Skip and unskip DAO, service, handler, router, error mapping, swagger.
6. Smoke checks; coverage gate; deploy before the app release.

## Complexity Tracking

No constitution violations.
