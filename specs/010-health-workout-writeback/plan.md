# Implementation Plan: Remember a workout's health export

**Branch**: `spec/010-health-workout-writeback` | **Date**: 2026-10-14 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/010-health-workout-writeback/spec.md`

## Summary

Add a small owner-scoped ledger, `health_exports`, that remembers the health-store identity the
device received when it wrote a finished workout to Apple Health. One idempotent `PUT` records the
identity for a finalized planned workout or an off-plan log; a second call with the same identity
replays, a different identity is a stable conflict. One `GET` lists exports for a date range so a
device can rebuild its view after reinstall. Nothing else changes: no session, participation, or
stats behavior moves.

## Workflow and Specialist Scope

- **Feature**: `gympulse-health-workout-writeback`, new scope, sibling
  [app plan](../../../gym-pulse-app/specs/009-health-workout-writeback/plan.md). This repository ships first.
- **gym-pulse-api-engineering**: contract, validation placement, transaction, migration (this plan,
  `research.md`, `data-model.md`, `contracts/`).
- **gym-pulse-cross-repo-delivery**: additive rollout (`quickstart.md`).
- **supabase-postgres-best-practices**: unique index and check constraints; RLS and grant revocation on the new table.
- Single-agent execution; one sequential lane.

## Technical Context

**Language/Version**: Go 1.26+

**Primary Dependencies**: chi, pgx/pgxpool, google/uuid (no additions)

**Storage**: PostgreSQL; new `health_exports`; reads `scheduled_workouts` and `day_logs` for ownership and finished state

**Testing**: Go unit/handler/DAO tests, model acceptance test for the migration, `go test ./...`, `./scripts/smoke-toggle.sh`, `./scripts/validate-database-design.sh`

**Target Platform**: Linux HTTP service and Supabase-hosted PostgreSQL

**Project Type**: Go web service

**Performance Goals**: One indexed point lookup plus one insert per record; a 30-day range read is one indexed query per workout kind

**Constraints**: Additive; owner-scoped; `[]` collections; write-once identity; no Data API exposure

**Scale/Scope**: One migration, one model file, one DAO, one service, one handler, two routes, contract and schema documentation

## Constitution Check

*GATE: Passed before Phase 0 and re-checked after Phase 1 design.*

- [x] Contract changes are identified in `contracts/health-exports.openapi.yaml` and land in `docs/CONTRACTS.md` with the code.
- [x] Authentication, ownership, idempotency, and error behavior are specified: owner-scoped lookups, shared 404 for missing or foreign workouts, verbatim replay through the ledger, `HEALTH_EXPORT_CONFLICT` for a different identity.
- [x] Tests are identified before implementation in `quickstart.md`, including contract smoke and schema-document validation.
- [x] Migration 025 creates one new table with RLS enabled and Data API grants revoked; the down migration drops it.
- [x] No new dependency, layer, or concurrency.

## Project Structure

### Documentation (this feature)

```text
specs/010-health-workout-writeback/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/health-exports.openapi.yaml
└── tasks.md
```

### Source Code (repository root)

```text
cmd/server/main.go
docs/CONTRACTS.md
docs/DATABASE_DESIGN.md
migrations/025_create_health_exports.{up,down}.sql
internal/
├── dao/{health_export_dao.go,health_export_dao_test.go,account_dao.go}
├── handler/{health_export.go,health_export_test.go,response.go}
├── model/{health_export.go,health_export_acceptance_test.go}
├── router/{router.go,training_routes_test.go}
└── service/{health_export_svc.go,health_export_svc_test.go}
scripts/smoke.sh
```

**Structure Decision**: Mirror the sport-activity feature: one handler → service → DAO path owning
the new table, wired in `main.go`, with the DAO owning the transaction.

## Complexity Tracking

No constitution violations.
