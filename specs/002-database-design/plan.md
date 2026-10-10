# Implementation Plan: Complete Database Design

**Branch**: `agent/codex-natural-language-guide` | **Date**: 2026-07-27 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/002-database-design/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

Create one authoritative, as-built database design for GymPulse and a prioritized target-state
roadmap. The implementation is documentation plus a non-mutating static drift check. It inventories
all migrations, reconciles them with DAO access patterns, the public API contract, and mobile types,
then documents ownership, snapshots, constraints, indexes, transactions, security, lifecycle,
capacity, operations, and safe schema evolution. It does not apply a production migration or alter a
public API.

## Technical Context

**Language/Version**: Go 1.26.8+ application; PostgreSQL SQL migrations; Markdown and POSIX shell for
this deliverable

**Primary Dependencies**: pgx/pgxpool, golang-migrate, PostgreSQL, Supabase Auth; no new dependency

**Storage**: PostgreSQL hosted by Supabase, with Supabase Auth as the external identity authority

**Testing**: Static design drift script, migration inspection, `go test ./...`, and optional
read-only PostgreSQL catalog queries

**Target Platform**: Railway-hosted Go API connecting directly to Supabase PostgreSQL; Expo
iOS/Android client accesses application data only through the API

**Project Type**: Web service with a separate mobile client repository

**Performance Goals**: Deterministic indexed user/date and user/identity reads; bounded list queries;
no N+1 persistence paths; evidence-driven partitioning only after large-table thresholds

**Constraints**: Preserve authenticated ownership, legacy compatibility, UUID public identity,
optimistic revisions, idempotency responses, immutable history snapshots, and forward-only mixed
version rollout

**Scale/Scope**: 30 current tables, 277 current columns, 19 migrations, all current DAO query paths,
and the full `/api/v1/*` persistence contract; current design remains unpartitioned below roughly
100 million rows per high-growth table unless measured evidence justifies earlier action

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- [X] Contract and `docs/CONTRACTS.md` changes are identified: no observable API change.
- [X] Authentication, ownership, idempotency, revision, and error behavior are preserved and
  documented.
- [X] Verification is identified before implementation: static drift validation and repository
  tests; contract smoke is not triggered because the contract is unchanged.
- [X] Proposed migrations document compatibility, existing-data behavior, validation, roll-forward,
  and later cleanup; no migration is applied in this scope.
- [X] No dependency, application layer, or concurrency mechanism is introduced.

## Project Structure

### Documentation (this feature)

```text
specs/002-database-design/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output (/speckit-plan command)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/
│   └── design-artifact.md
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
docs/
└── DATABASE_DESIGN.md

migrations/
├── 001_create_workout_templates.up.sql
└── … 019_create_criteria_training_blocks.up.sql

internal/dao/
└── *.go

scripts/
└── validate-database-design.sh

specs/002-database-design/
└── planning and traceability artifacts
```

**Structure Decision**: Keep the durable design in `docs/`, the executable drift check with existing
repository scripts, and feature-specific evidence in `specs/002-database-design/`. The app
repository remains read-only because no client contract or behavior changes.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No constitution violations.
