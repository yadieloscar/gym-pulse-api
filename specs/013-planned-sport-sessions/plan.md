# Implementation Plan: Planned Sport Sessions API

**Branch**: `spec/013-planned-sport-sessions` | **Date**: 2026-10-25 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/013-planned-sport-sessions/spec.md`

## Summary

Make a program unit either an exercise workout (today's behavior, `kind: exercises`) or a sport
session (`kind: sport`) with a sport snapshot and planned targets. One additive migration adds the
kind and sport columns to `program_workouts` and `scheduled_workouts`, and an optional link from
`sport_activities` to the scheduled session it fulfilled. Materialization snapshots sport sessions
onto the schedule with no required sets; lazy finalization marks an unfulfilled past session
`missed`; skip and unskip work unchanged. Recording is the existing `POST /api/v1/sport-activities`
with an optional `scheduled_workout_id`: in the one existing create transaction the activity is
inserted, the session becomes `completed`, both directions of the link are readable, and
participation is credited to the performed date. An imported activity auto-fulfils the earliest
open session for the same sport and date. Every existing route keeps its shape; `kind` defaults to
`exercises` for old clients and old rows.

## Workflow and Specialist Scope

- **Feature**: `gympulse-planned-sport-sessions`, new scope, sibling
  [app spec](../../../gym-pulse-app/specs/016-planned-sport-sessions/spec.md). This repository
  ships first; the app hides "Add a sport day" until the API exposes `kind`.
- **gym-pulse-api-engineering**: owns contract, validation placement, transaction and lock order,
  migration safety, and test evidence (this plan, `research.md`, `data-model.md`, `contracts/`).
- **gym-pulse-cross-repo-delivery**: owns additive compatibility and rollout order (`quickstart.md`).
- **supabase-postgres-best-practices**: informs the kind/sport pair checks, the partial unique link
  index, and the `ON DELETE SET NULL` choice that keeps account-deletion order valid.
- **gym-pulse-engineering-lead**: resolved the "extend the activity create route versus a new
  record route" and "recover ignores sport sessions" decisions recorded in `research.md`.
  Single-agent execution; one read-only code-mapping subagent produced the facts this plan cites.

## Technical Context

**Language/Version**: Go 1.26+

**Primary Dependencies**: chi, pgx/pgxpool, google/uuid (no additions)

**Storage**: PostgreSQL; altered `program_workouts`, `scheduled_workouts`, `sport_activities`; existing `day_participation` and `idempotency_records`

**Testing**: Go unit/handler/DAO tests, model acceptance tests, source-text DAO contract tests, `go test ./...`, `./scripts/smoke-toggle.sh`, `./scripts/validate-database-design.sh`, `./scripts/validate-supabase-security.sh`

**Target Platform**: Linux HTTP service and Supabase-hosted PostgreSQL

**Project Type**: Go web service

**Performance Goals**: Program and schedule reads gain columns on the same rows, no extra queries; recording adds one `FOR UPDATE` point read and one `UPDATE` inside the existing sport-activity transaction; auto-match is one indexed lookup on `(user_id, date)`

**Constraints**: Additive contract; ownership; `[]` collections; idempotency and revision rules unchanged; no Data API exposure; lock order `program:` → `schedule:` → `sport-activity:` → row locks

**Scale/Scope**: One migration, model and validation changes, materialization branch, finalize branch, sport-activity transaction branch, four guard checks on set and complete routes, contract and schema documentation

## Constitution Check

*GATE: Passed before Phase 0 and re-checked after Phase 1 design.*

- [x] Contract changes are identified in `contracts/planned-sport-sessions.openapi.yaml` and land in `docs/CONTRACTS.md` with the code (programs, schedule, sport activities, error codes).
- [x] Authentication, ownership, idempotency, and revision behavior are preserved. The new link path is owner-scoped through the existing `scheduled_workouts.user_id` lookup, and the one new conflict (`SPORT_SESSION_CONFLICT`) returns the authoritative session like the existing conflicts.
- [x] Tests are identified before implementation in `quickstart.md`, including contract smoke, source-text DAO contracts, and schema-document validation.
- [x] Migration 027 is additive with column defaults, needs no backfill, and has a documented down migration; `ON DELETE SET NULL` keeps the existing account-deletion order valid.
- [x] No new dependency, layer, or concurrency. The sport-activity DAO gains the `schedule:` lock ahead of its own lock, which follows the documented order.

## Project Structure

### Documentation (this feature)

```text
specs/013-planned-sport-sessions/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/planned-sport-sessions.openapi.yaml
└── tasks.md
```

### Source Code (repository root)

```text
docs/CONTRACTS.md
docs/DATABASE_DESIGN.md
migrations/027_add_planned_sport_sessions.{up,down}.sql
internal/
├── model/{training.go,training_materialization.go,sport_activity.go,
│          training_test.go,sport_activity_acceptance_test.go}
├── service/{training_svc.go,schedule_svc.go,sport_activity_svc.go,
│            training_domain_svc_test.go,sport_activity_svc_test.go,schedule_resolution_test.go}
├── dao/{program_dao.go,schedule_dao.go,training_mutation_dao.go,sport_activity_dao.go,
│        health_export_dao.go,sport_activity_dao_test.go,training_mutation_dao_test.go}
├── handler/{training.go,sport_activity.go,response.go,
│            training_test.go,sport_activity_test.go}
└── router/router.go            # no route changes; wiring passes the schedule DAO to the sport service
scripts/smoke.sh                # new step: sport unit → materialize → record → replay → conflict
```

**Structure Decision**: No new packages or routes. The feature extends the program, schedule, and
sport-activity owners in place so each resource keeps one handler → service → DAO path. The link
step is resolved inside the sport-activity DAO's existing create transaction (`lockSportSession`),
so the service signature and `cmd/server/main.go` are unchanged (implementation note, 2026-10-25).

## Complexity Tracking

No constitution violations.
