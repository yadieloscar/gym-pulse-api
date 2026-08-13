# Implementation Plan: Release-Safe Training Workflows

**Branch**: `agent/release-blockers-api` | **Date**: 2026-08-12 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/006-release-safety/spec.md`

## Summary

Close the three audited API release blockers: implement the already-published legacy-adoption workflow, make every goal-training mutation safely replayable with atomic cross-aggregate outcomes, and reject inclusive date ranges above 366 days before expensive work. The implementation stays within the existing handler → service → DAO architecture, adds one transaction-oriented DAO for workflows that span training aggregates, reuses the existing idempotency and legacy-adoption tables, and adds no dependency or migration.

## Technical Context

**Language/Version**: Go 1.26.5

**Primary Dependencies**: chi v5, pgx/v5 + pgxpool, go-playground/validator v10, google/uuid

**Storage**: PostgreSQL 16; existing `legacy_adoptions`, `idempotency_records`, legacy plan/template tables, and goal-training tables

**Testing**: Go unit/table tests, DAO SQL contract tests, `go test -race ./...`, golangci-lint, generated Swagger drift, Docker/PostgreSQL smoke

**Target Platform**: Stateless Linux container behind an HTTP load balancer

**Project Type**: Authenticated JSON web service

**Performance Goals**: Constant-size adoption work bounded to seven assignments; mutation transactions complete without external calls; calendar work bounded to 366 inclusive days per request

**Constraints**: Owner-scoped UUID access; exact scoped replay; consistent program → schedule transaction lock order; short transactions; no legacy-row deletion; additive API deployed before the app dependency

**Scale/Scope**: Per-user operations horizontally distributed across replicas, coordinated through PostgreSQL constraints and transaction advisory locks; eleven existing goal-training mutation families plus one adoption route

## Constitution Check

*GATE: Passed before Phase 0 and re-checked after Phase 1 design.*

- [x] Contract and `docs/CONTRACTS.md` changes are identified: adoption route inventory, mutation operation keys, 366-day range ceiling, exact atomic outcomes.
- [x] Authentication, ownership, idempotency, revision, and error behavior are preserved or specified: all resources remain user-scoped; replays precede revision evaluation; mismatches remain 409.
- [x] Tests are identified before implementation, including contract and smoke checks: model boundaries, handler keys, service replay ordering, SQL atomicity, router inventory, and live smoke.
- [x] Migrations document compatibility, existing-data behavior, and rollback: no migration is required; existing rows and tables remain unchanged.
- [x] New dependencies, layers, and concurrency are justified: no dependency or goroutine is added; one transaction-oriented DAO is required because workout/session changes and participation cross current DAO aggregates.

## Architecture and Transaction Design

1. Add `TrainingMutationDAO`, backed by the existing query pool, for only the workflows that must atomically span current repositories. Each method begins one transaction, obtains domain advisory locks in program → schedule order when both are needed, checks the scoped idempotency record before revision or existence checks, applies the compare-and-swap mutation, stores the complete response, and commits once.
2. Keep validation, request merging, historical policy, status derivation, and response selection in services. The transaction DAO owns SQL atomicity and reloads the authoritative response inside the transaction so its JSON is exactly replayable.
3. Keep current read DAOs and already-atomic materialize/regenerate/recover/sport/block implementations. Move remaining profile, program, scheduled-workout, set, completion, and session mutations onto the transaction DAO.
4. Implement adoption as one program+schedule transaction: load the saved training profile, query only owned non-rest weekly assignments and exercises, map them deterministically into a program, materialize next Monday through Sunday, insert `legacy_adoptions`, store the response, and commit. Existing adoption returns its authoritative resources without copying again.
5. Add `MaxTrainingRangeDays = 366` and enforce it in `ValidateDateRange`; every current goal-training list/generation/transition path already calls this function or will be updated to do so.

## Data and Contract Effects

- **Migrations**: None. `migrations/012_create_training_domain.up.sql` already provides both required durable records and uniqueness constraints.
- **Existing data**: Read-only during adoption. New goal-training rows are additive. Previously adopted athletes replay or receive the existing authoritative adoption.
- **Rollback**: Reverting application code removes route behavior but does not delete adopted data. No down migration exists because no schema changes occur.
- **Compatibility**: The route is additive. Existing header-only profile and
  custom-program calls are normalized at the handler during the migration
  window; the app PR adds the body field and deploys after this API PR.
- **Errors**: Missing/foreign owned resources remain 404, invalid/mismatched keys remain 422, stale revisions and changed idempotency payloads remain 409, oversized ranges return 422 with field `range`.

## Project Structure

### Documentation (this feature)

```text
specs/006-release-safety/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/release-safety.openapi.yaml
├── checklists/requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
internal/
├── model/training.go
├── dao/training_mutation_dao.go
├── dao/*_test.go
├── service/training_svc.go
├── service/schedule_svc.go
├── service/*_test.go
├── handler/training.go
├── handler/*_test.go
└── router/{router.go,training_routes_test.go}
cmd/server/main.go
docs/CONTRACTS.md
scripts/smoke.sh
```

**Structure Decision**: Extend the existing single Go service. The new DAO is a transaction boundary, not a parallel business layer; handlers and services retain their existing roles.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Transaction DAO spans current program, schedule, session, set, participation, and idempotency tables | The user-observable mutations must commit and replay as one outcome across those aggregates | Sequential existing DAO calls can commit half an outcome; duplicating a complete transaction in each existing DAO would scatter lock and replay policy and make ordering inconsistent |
