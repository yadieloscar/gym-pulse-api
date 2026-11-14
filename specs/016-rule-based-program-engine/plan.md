# Implementation Plan: Rule-Based Program Engine

**Branch**: `spec/016-rule-based-program-engine` | **Date**: 2026-11-02 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/016-rule-based-program-engine/spec.md`

## Summary

Make a deterministic, in-process engine the default generator behind `POST /api/v1/program-generation`.
The engine is a pure Go package (`internal/engine`) that builds the draft from the athlete's
answers using programming tables sourced from the spec 015 goal guide (split by day count, slot
order per workout kind, exercises per session, targets by goal and experience, cardio share,
secondary-goal share, beginner limits). It plugs into the existing `DraftGenerator` seam, so the
service, validator, handler, and route stay as spec 015 left them. The catalog gains four
programming fields (migration 032); the request gains `avoid`, `focus`, and `variant`; the status
and response gain `provider`. The model path stays opt-in through `PROGRAM_GENERATION_PROVIDER=model`.

## Workflow and Specialist Scope

- **Feature**: `gympulse-rule-based-program-engine`, new scope that amends spec 015's contract
  additively. Sibling app amendment to spec 019 (constraint chips, `provider` read) follows this
  repository.
- **gym-pulse-api-engineering**: package boundary, migration, contract, tests (this plan,
  `research.md`, `data-model.md`, `contracts/`, `quickstart.md`).
- **gym-pulse-engineering-lead**: the provider switch and the ledger exemption are the risk
  points; both are covered by config and service tests before wiring.
- **gym-pulse-cross-repo-delivery**: additive rollout; an older app keeps working on the engine.
- **supabase-postgres-best-practices**: migration ordering (update rows, then `NOT NULL`), CHECK
  constraints, one new index.
- Single-agent execution; one sequential lane; read-only review pass after implementation.

## Technical Context

**Language/Version**: Go 1.26+

**Primary Dependencies**: chi, pgx/pgxpool, google/uuid (existing). No new dependency.

**Storage**: PostgreSQL; four columns and one index on `exercise_catalog`; no new table

**Testing**: Go unit tests for the engine (exhaustive matrix, table coverage, determinism),
model, service, config, handler; PostgreSQL migration test; `go test ./...`, `golangci-lint`,
`./scripts/smoke-toggle.sh`, `./scripts/validate-database-design.sh`,
`./scripts/validate-supabase-security.sh`

**Target Platform**: Linux HTTP service on Railway, Supabase-hosted PostgreSQL

**Project Type**: Go web service

**Performance Goals**: p95 under 200 ms per draft; O(slots × catalog) with 79 entries

**Constraints**: Additive contract; deterministic; no provider call, no ledger use, nothing leaves
the process; spec 015 invariants hold by construction and by validation

**Scale/Scope**: One migration, one new package (three files plus tests), one adapter, model and
config additions, contract docs, smoke steps

## Constitution Check

*GATE: Passed before Phase 0 and re-checked after Phase 1 design.*

- [x] Contract changes identified in `contracts/program-generation-engine.openapi.yaml`; land in
  `docs/CONTRACTS.md` ("Program generation" and "Exercise catalog") with the code; Swagger
  regenerated.
- [x] Authentication, ownership, idempotency, and error behavior preserved: same authenticated
  routes, read-only POST, no `Idempotency-Key`; new 422 fields `avoid`, `focus`, `variant`; the
  rate limit on the route stays.
- [x] Tests identified before implementation in `quickstart.md`, including the exhaustive matrix,
  determinism, migration, and smoke timing.
- [x] Migration 032 is additive, updates existing rows before constraining, and its down drops
  the columns; the catalog stays read-only.
- [x] No new dependency; one new package justified below; no new concurrency.

## Project Structure

### Documentation (this feature)

```text
specs/016-rule-based-program-engine/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/program-generation-engine.openapi.yaml
├── checklists/requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
cmd/server/main.go                              # provider selection
.env.example, README.md                         # PROGRAM_GENERATION_PROVIDER
docs/CONTRACTS.md, docs/DATABASE_DESIGN.md
docs/{docs.go,swagger.json,swagger.yaml}
migrations/032_add_exercise_catalog_programming.{up,down}.sql
internal/
├── config/{config.go,config_test.go}            # provider enum, fallback
├── dao/{exercise_catalog_dao.go,...}            # select the four fields
├── engine/{tables.go,engine.go,rationale.go,    # pure package, no I/O
│           tables_test.go,engine_test.go,rationale_test.go,matrix_test.go}
├── handler/{program_generation.go,_test.go,exercise_catalog_test.go}
├── model/{exercise_catalog.go,program_generation.go,program_generation_test.go}
└── service/{program_generation_svc.go,_test.go,
             program_generator_rules.go,program_generator_rules_test.go}
scripts/smoke.sh
```

**Structure Decision**: one pure package for the intelligence, one adapter behind the seam spec
015 already defined, nothing else moves. The tables are typed Go data with a source comment per
table, reviewed like a contract; the engine never reads the database or the clock.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| New package `internal/engine` | Programming tables and the build algorithm are a coherent unit with many tests and no I/O | Putting them in `service` mixes pure logic with DAO wiring and makes the exhaustive tests heavier; putting them in `model` mixes contract types with programming data |
| Migration 032 seeds 79 rows | The engine cannot choose exercises without pattern, muscle group, equipment, and impact | Inferring from `category` and name in code is brittle and untestable against future catalog edits |
