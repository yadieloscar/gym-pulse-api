# Implementation Plan: Sport Activity Tracking API

**Branch**: `codex/sport-activity-tracking` | **Date**: 2026-08-02 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/004-sport-activity-tracking/spec.md`

## Summary

Add an authenticated `sport_activities` resource for completed sport sessions. A new handler →
service → DAO path validates athlete-local dates and performs activity creation, participation
preservation, and idempotency recording in one PostgreSQL transaction. Range reads are owner-scoped
and deterministic. Weekly performed-day stats include sport dates, while workout totals, workout
volume, and scheduled-workout quality retain their current meaning.

## Technical Context

**Language/Version**: Go 1.26+

**Primary Dependencies**: chi, pgx/pgxpool, go-playground/validator, google/uuid

**Storage**: PostgreSQL; new `sport_activities`, existing `day_participation`, and `idempotency_records`

**Testing**: Go unit/handler/router tests, migration assertions, `go test ./...`, contract smoke toggle

**Target Platform**: Linux HTTP service and Supabase-hosted PostgreSQL

**Project Type**: Go web service

**Performance Goals**: Indexed owner/date reads; one short create transaction with no network calls

**Constraints**: Additive API; ownership; deterministic `[]`; retry safety; no Data API; preserve workout semantics

**Scale/Scope**: One table, three routes, one service/DAO boundary, and date aggregation changes

## Constitution Check

*GATE: Passed before Phase 0 and re-checked after Phase 1 design.*

- [x] Contract and `docs/CONTRACTS.md` changes are identified in `contracts/sport-activities.openapi.yaml`.
- [x] Authentication, ownership, idempotency, revision, and error behavior are specified.
- [x] Tests are identified before implementation, including full tests and contract smoke checks.
- [x] Migration 018 is additive, requires no backfill, preserves clients, and has a table-drop rollback.
- [x] No new dependency, architectural layer, or concurrency is introduced.

## Project Structure

### Documentation (this feature)

```text
specs/004-sport-activity-tracking/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
└── tasks.md
```

### Source Code (repository root)

```text
cmd/server/main.go
docs/CONTRACTS.md
migrations/018_create_sport_activities.{up,down}.sql
internal/
├── dao/{sport_activity_dao.go,stats_dao.go,account_dao.go}
├── handler/sport_activity.go
├── model/sport_activity.go
├── router/router.go
└── service/sport_activity_svc.go
```

**Structure Decision**: Extend the existing handler → service → DAO structure. The DAO owns the
activity, participation, and idempotency transaction.

## Complexity Tracking

No constitution violations.
