# Tasks: Complete Database Design

**Input**: Design documents from `/specs/002-database-design/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`,
`quickstart.md`

**Tests**: Static drift validation and repository quality gates are required. There is no observable
API behavior change.

**Organization**: Tasks are grouped by user story so the current model, integrity model, and
operational model remain independently reviewable.

## Phase 1: Setup

**Purpose**: Establish the documentation and validation boundaries.

- [X] T001 Create the authoritative design structure and status vocabulary in docs/DATABASE_DESIGN.md
- [X] T002 Create a non-mutating migration-to-document drift check in scripts/validate-database-design.sh

---

## Phase 2: Foundational

**Purpose**: Establish the evidence inventory used by every story.

- [X] T003 Reconcile migrations 001–019, DAO queries, docs/CONTRACTS.md, and app types in specs/002-database-design/research.md

**Checkpoint**: Every current physical fact and behavioral source has an identified authority.

---

## Phase 3: User Story 1 - Understand the system of record (Priority: P1)

**Goal**: Make the complete current schema and source-of-truth model understandable without reading
the persistence implementation.

**Independent Test**: The drift script finds all 30 tables and named migration indexes, and a reviewer
can trace every persisted product concept to an authoritative, snapshotted, derived, external, or
compatibility-only source.

- [X] T004 [US1] Document architecture boundaries, source-of-truth classifications, and the current ERD in docs/DATABASE_DESIGN.md
- [X] T005 [US1] Document all 30 current tables, 277 columns, constraints, indexes, relationships, and migration provenance in docs/DATABASE_DESIGN.md
- [X] T006 [US1] Document API persistence traceability and legacy/session coexistence in docs/DATABASE_DESIGN.md

**Checkpoint**: The as-built schema is complete and reviewable independently of target recommendations.

---

## Phase 4: User Story 2 - Protect integrity and privacy (Priority: P2)

**Goal**: Make ownership, snapshot, transaction, concurrency, idempotency, deletion, and security
rules explicit.

**Independent Test**: A reviewer can determine the owner, deletion outcome, mutation guard, and
historical behavior of every user-owned record from the design alone.

- [X] T007 [US2] Document ownership, sensitivity, constraint, snapshot, and deletion matrices in docs/DATABASE_DESIGN.md
- [X] T008 [US2] Document transaction boundaries, lock order, optimistic revisions, idempotency, retries, and race protections in docs/DATABASE_DESIGN.md
- [X] T009 [US2] Document Supabase Auth/Data API boundaries, least-privilege roles, encryption, secret handling, and deletion-resurrection risk in docs/DATABASE_DESIGN.md

**Checkpoint**: Integrity and privacy rules are explicit, and current gaps are not presented as deployed.

---

## Phase 5: User Story 3 - Operate and evolve safely (Priority: P3)

**Goal**: Provide an evidence-derived performance, operations, and migration model.

**Independent Test**: Each important query path has a current or target index, and each gap has a
safe rollout sequence and validation rule.

- [X] T010 [US3] Document access patterns, index audit, pagination, capacity, partition thresholds, and query diagnostics in docs/DATABASE_DESIGN.md
- [X] T011 [US3] Document connection management, maintenance, backup, restore, recovery objectives, and incident signals in docs/DATABASE_DESIGN.md
- [X] T012 [US3] Document prioritized expand/backfill/validate/cutover/cleanup migration stages with compatibility and rollback guidance in docs/DATABASE_DESIGN.md

**Checkpoint**: The operational and evolution design can be reviewed independently of runtime changes.

---

## Phase 6: Polish and cross-cutting validation

**Purpose**: Verify completeness, traceability, formatting, and repository health.

- [X] T013 Run scripts/validate-database-design.sh and resolve all documentation drift
- [X] T014 Run go test ./... and golangci-lint run, recording unavailable gates in specs/002-database-design/quickstart.md
- [X] T015 Perform an independent read-only final review of docs/DATABASE_DESIGN.md against specs/002-database-design/spec.md, plan.md, and tasks.md

---

## Dependencies and execution order

- Phase 1 has no dependencies.
- Phase 2 depends on Phase 1 and blocks all user stories.
- User Story 1 establishes the current model and must complete before User Stories 2 and 3.
- User Story 2 and User Story 3 are conceptually independent after User Story 1, but edits to the
  same authoritative document remain sequential.
- Phase 6 depends on all selected user stories.

## Parallel opportunities

The research areas are independently reviewable, but implementation remains single-agent because
T004–T012 edit the same authoritative file and architectural consistency is more valuable than
parallel writes.

## Implementation strategy

1. Deliver the as-built catalog first (US1).
2. Add integrity and privacy policy (US2).
3. Add operational and migration policy (US3).
4. Run drift validation and repository gates.
5. Review every **Target** claim to ensure it is not described as current behavior.


## Current-schema reconciliation — 2026-10-05

Original T001–T015 IDs and historical results are preserved. The owning spec/catalog scope now
tracks migrations 001–019; historical Go/lint results do not establish current runtime readiness.

- [X] T016 Reconcile the as-built catalog to migrations 016–019: 30 tables, 277 supported columns, 24 named indexes.
- [X] T017 Reconcile shipped RLS, legacy ownership, sports/criteria-block lifecycle, deletion, and pool configuration; retain future proposals separately.
- [X] T018 Validate exact table/column inventory, named-index coverage, declared counts, shell syntax, and documentation formatting.
- [X] T019 Complete independent source review and disposable migrated-catalog comparison; record hosted catalog/security evidence as an open promotion gate.

See quickstart.md for current static evidence and explicitly unrun environment/runtime gates.
