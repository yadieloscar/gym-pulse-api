# Implementation Plan: Criteria-Based Training Blocks API

**Branch**: `codex/criteria-based-training-blocks` | **Date**: 2026-08-04 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/005-criteria-based-training-blocks/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

Add an authenticated, athlete-owned criteria-based training-block aggregate without changing programs, schedules, workouts, sport activities, or statistics. PostgreSQL stores immutable stage definitions, append-only exposures and transitions, and one next-morning response per exposure. Go handler/service/DAO layers enforce ownership, bounds, timezone-aware dates, optimistic revisions, transaction-scoped idempotency, and server-derived qualifying progress. Lists are bounded and paginated; detail and mutations return the authoritative aggregate.

## Technical Context

<!--
  ACTION REQUIRED: Replace the content in this section with the technical details
  for the project. The structure here is presented in advisory capacity to guide
  the iteration process.
-->

**Language/Version**: Go 1.26; PostgreSQL migrations

**Primary Dependencies**: chi v5, pgx v5, validator v10, google/uuid; no new dependency

**Storage**: PostgreSQL tables for blocks, stages, exposures, transitions; existing `idempotency_records`

**Testing**: Go unit tests, DAO transaction tests with existing fakes, handler/router tests, full `go test ./...`, race suite, smoke toggle

**Target Platform**: Existing Linux API service and supported PostgreSQL deployment

**Project Type**: Authenticated JSON web service

**Performance Goals**: Bounded list queries (default 20, maximum 100); detail loads one owned aggregate using indexed foreign keys; no unbounded cross-user scans

**Constraints**: Stable error envelopes; authenticated ownership; atomic idempotent mutations; optimistic revision conflicts; no cross-domain side effects; additive migration and routes

**Scale/Scope**: Four new aggregate tables, six routes, one migration pair, one model/service/DAO/handler lane plus contract and tests

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- [x] Contract and `docs/CONTRACTS.md` changes are identified.
- [x] Authentication, ownership, idempotency, revision, and error behavior are preserved or specified.
- [x] Tests are identified before implementation, including contract and smoke checks.
- [x] Migrations document compatibility, existing-data behavior, and rollback.
- [x] New dependencies, layers, and concurrency are justified.

## Project Structure

### Documentation (this feature)

```text
specs/005-criteria-based-training-blocks/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output (/speckit-plan command)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/           # Phase 1 output (/speckit-plan command)
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)
<!--
  ACTION REQUIRED: Replace the placeholder tree below with the concrete layout
  for this feature. Delete unused options and expand the chosen structure with
  real paths (e.g., apps/admin, packages/something). The delivered plan must
  not include Option labels.
-->

```text
cmd/server/main.go
docs/CONTRACTS.md
internal/
├── dao/training_block_dao.go
├── handler/training_block.go
├── model/training_block.go
├── router/router.go
└── service/training_block_svc.go
migrations/
├── 019_create_criteria_training_blocks.up.sql
└── 019_create_criteria_training_blocks.down.sql
```

**Structure Decision**: Extend the existing handler → service → DAO structure and server composition root. Tests live beside changed Go packages, matching current repository conventions. No new layer, dependency, worker, or goroutine is introduced.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No constitution violations.
