# Implementation Plan: Imported health workouts API

**Branch**: `spec/008-health-workout-import` | **Date**: 2026-10-12 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/008-health-workout-import/spec.md`

## Summary

Extend the existing `sport_activities` resource so a device can submit workouts read from Apple
Health. One additive migration adds an origin (`source`), an external identity, and three optional
measurements, plus a per-athlete uniqueness rule on origin and identity. The existing handler →
service → DAO path validates the new fields, and the create transaction returns the existing
activity with `200` when the same health workout is submitted again, so re-imports across retries,
reinstalls, and devices never duplicate history or re-credit participation. Reads include the new
fields; older clients ignore them.

## Workflow and Specialist Scope

- **Feature**: `gympulse-health-workout-import`, new scope, sibling
  [app plan](../../../gym-pulse-app/specs/008-health-workout-import/plan.md). This repository ships
  first.
- **gym-pulse-api-engineering**: owns contract, validation placement, transaction order, migration
  safety, and test evidence (this plan, `research.md`, `data-model.md`, `contracts/`).
- **gym-pulse-cross-repo-delivery**: owns additive compatibility and rollout order (`quickstart.md`).
- **supabase-postgres-best-practices**: informs the partial unique index and check constraints.
- **gym-pulse-engineering-lead**: resolved the duplicate-submission status and migration numbering
  decisions recorded in `research.md`. Single-agent execution; the work is one sequential lane.

## Technical Context

**Language/Version**: Go 1.26+

**Primary Dependencies**: chi, pgx/pgxpool, google/uuid (no additions)

**Storage**: PostgreSQL; altered `sport_activities`; existing `day_participation` and `idempotency_records`

**Testing**: Go unit/handler/DAO tests, model acceptance test, `go test ./...`, `./scripts/smoke-toggle.sh`, `./scripts/validate-database-design.sh`

**Target Platform**: Linux HTTP service and Supabase-hosted PostgreSQL

**Project Type**: Go web service

**Performance Goals**: Duplicate check is one indexed point lookup inside the existing create transaction; a 60-workout first import is 60 short requests

**Constraints**: Additive contract; ownership; `[]` collections; retry and re-import safety; no Data API exposure; participation rules unchanged

**Scale/Scope**: One migration, model and validation changes, one DAO branch, handler status mapping, contract and schema documentation

## Constitution Check

*GATE: Passed before Phase 0 and re-checked after Phase 1 design.*

- [x] Contract changes are identified in `contracts/sport-activities-import.openapi.yaml` and will land in `docs/CONTRACTS.md` with the code.
- [x] Authentication, ownership, idempotency, and error behavior are preserved; the new duplicate path is specified (`200`, no side effect) and stays owner-scoped.
- [x] Tests are identified before implementation in `quickstart.md`, including contract smoke and schema-document validation.
- [x] Migration 022 is additive with a column default, needs no backfill, and has a documented down migration.
- [x] No new dependency, layer, or concurrency. The DAO gains one query in the existing transaction.

## Project Structure

### Documentation (this feature)

```text
specs/008-health-workout-import/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/sport-activities-import.openapi.yaml
└── tasks.md
```

### Source Code (repository root)

```text
docs/CONTRACTS.md
docs/DATABASE_DESIGN.md
migrations/022_add_sport_activity_source.{up,down}.sql
internal/
├── dao/{sport_activity_dao.go,sport_activity_dao_test.go}
├── handler/{sport_activity.go,sport_activity_test.go}
├── model/{sport_activity.go,sport_activity_acceptance_test.go}
└── service/{sport_activity_svc.go,sport_activity_svc_test.go}
scripts/smoke.sh   # only if a wire assertion for the new fields is added
```

**Structure Decision**: No new files beyond the migration pair. Every change extends the existing
sport-activity handler, service, DAO, and model so the resource keeps one owner.

## Complexity Tracking

No constitution violations.
