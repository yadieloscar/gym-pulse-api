# Implementation Plan: Daily nutrition totals and a chosen target

**Branch**: `spec/012-nutrition-targets` | **Date**: 2026-10-25 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/012-nutrition-targets/spec.md`

## Summary

Add two owner-scoped tables, `nutrition_days` and `nutrition_targets`, behind one handler → service →
DAO path. `PUT /api/v1/nutrition/days/{date}` upserts one set of totals per athlete and date with the
existing revision and idempotency rules; a Health-sourced upsert never overwrites an athlete-entered
day. `GET /api/v1/nutrition/days?from&to` returns the days in range plus the targets that applied to
them. `PUT /api/v1/nutrition/target` saves the athlete's chosen target effective from a date; `GET`
returns the current one. Nothing else moves: no participation, stats, body weight, or workout change.

## Workflow and Specialist Scope

- **Feature**: `gympulse-nutrition-targets`, new scope, sibling
  [app plan](../../../gym-pulse-app/specs/015-nutrition-targets/plan.md). This repository ships first.
- **gym-pulse-api-engineering**: contract, validation placement, transaction, migration (this plan,
  `research.md`, `data-model.md`, `contracts/`).
- **gym-pulse-cross-repo-delivery**: additive rollout and mixed-version window (`quickstart.md`).
- **supabase-postgres-best-practices**: numeric types, check constraints, unique indexes, RLS plus
  the runtime-role policy that migration 026 requires of every new table.
- **gym-pulse-engineering-lead**: owns the two-repo sequence; single-agent execution, one lane.

## Technical Context

**Language/Version**: Go 1.26+

**Primary Dependencies**: chi, pgx/pgxpool, google/uuid (no additions)

**Storage**: PostgreSQL; new `nutrition_days` and `nutrition_targets`; reads `training_profiles.timezone`

**Testing**: Go unit, handler, and DAO tests; migration acceptance test; `go test ./...`,
`./scripts/smoke-toggle.sh`, `./scripts/validate-database-design.sh`, security SQL on a migrated database

**Target Platform**: Linux HTTP service and Supabase-hosted PostgreSQL

**Project Type**: Go web service

**Performance Goals**: One indexed upsert per save; a 366-day range read is two indexed queries (days, targets)

**Constraints**: Additive; owner-scoped; `[]` collections; revisioned athlete saves; Health saves cannot overwrite athlete saves; no Data API exposure

**Scale/Scope**: One migration (two tables), one model file, one DAO, one service, one handler, four routes, contract and schema documentation, smoke step

## Constitution Check

*GATE: Passed before Phase 0 and re-checked after Phase 1 design.*

- [x] Contract changes are identified in `contracts/nutrition.openapi.yaml` and land in `docs/CONTRACTS.md` ("Nutrition" section after "Body weight") with the code.
- [x] Authentication, ownership, idempotency, revision, and error behavior are specified: owner-scoped queries, shared 404 for a missing target, verbatim replay through the ledger, `REVISION_CONFLICT` with the current revision, `IDEMPOTENCY_CONFLICT` on payload mismatch, 200-unchanged for a Health save over an athlete day.
- [x] Tests are identified before implementation in `quickstart.md`, including contract smoke and schema-document validation.
- [x] Migration 027 creates two tables with RLS enabled, Data API grants revoked, and the `gym_pulse_api_access` policy and DML grant for the runtime role; the down migration drops both tables.
- [x] No new dependency, layer, or concurrency.

## Project Structure

### Documentation (this feature)

```text
specs/012-nutrition-targets/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/nutrition.openapi.yaml
└── tasks.md
```

### Source Code (repository root)

```text
cmd/server/main.go
docs/CONTRACTS.md
docs/DATABASE_DESIGN.md
migrations/027_create_nutrition.{up,down}.sql
internal/
├── dao/{nutrition_dao.go,nutrition_dao_test.go,account_dao.go,supabase_security_migration_test.go}
├── handler/{nutrition.go,nutrition_test.go}
├── model/{nutrition.go,nutrition_acceptance_test.go}
├── router/{router.go,training_routes_test.go}
└── service/{nutrition_svc.go,nutrition_svc_test.go}
scripts/smoke.sh
```

**Structure Decision**: Mirror the sport-activity and health-export features: one handler → service →
DAO path owning both tables, wired in `main.go`, with the DAO owning each transaction. The service
loads the training profile for the athlete's timezone exactly as sport activities do.

## Complexity Tracking

No constitution violations.
