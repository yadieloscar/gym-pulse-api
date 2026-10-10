# Feature Specification: Complete Database Design

**Feature Branch**: `agent/codex-natural-language-guide`

**Created**: 2026-07-27

**Status**: Approved for implementation

**Input**: User description: "Let's do a complete and documented database design for this system"

## User Scenarios & Testing

### User Story 1 - Understand the system of record (Priority: P1)

As an engineer changing GymPulse, I need one authoritative description of the persisted data,
ownership boundaries, and relationships so that I can make changes without reverse-engineering the
entire application.

**Why this priority**: A correct shared model is the prerequisite for safe feature, migration, and
contract work.

**Independent Test**: A reviewer can use only the design to identify every persisted entity, its
owner, its identity, and its relationships, then confirm those facts against the current system.

**Acceptance Scenarios**:

1. **Given** the current GymPulse product, **When** an engineer looks up any persisted entity,
   **Then** the design states its purpose, fields, identity, ownership, and relationships.
2. **Given** a user-facing API field, **When** an engineer traces where it is stored,
   **Then** the design either identifies the authoritative persisted field or explicitly identifies
   the value as derived, cached, external, or not persisted.
3. **Given** both legacy workout logging and goal-based training, **When** an engineer compares them,
   **Then** the design explains their coexistence, compatibility boundary, and source-of-truth rules.

---

### User Story 2 - Protect integrity and privacy (Priority: P2)

As a maintainer reviewing a data change, I need the invariants, ownership rules, deletion behavior,
and concurrency rules documented so that user data remains isolated, consistent, and recoverable.

**Why this priority**: GymPulse stores private health and activity data; incorrect ownership,
mutation, or lifecycle behavior has a high user impact.

**Independent Test**: A reviewer can evaluate a proposed data change against a complete list of
integrity, authorization, transaction, retry, and deletion rules without inferring policy from code.

**Acceptance Scenarios**:

1. **Given** a user-owned record, **When** access or deletion behavior is reviewed, **Then** the
   design states how ownership is established, enforced, and cascaded.
2. **Given** a retry or concurrent mutation, **When** its persistence behavior is reviewed, **Then**
   the design states the applicable idempotency, revision, locking, and transaction rules.
3. **Given** an account deletion, **When** its data impact is reviewed, **Then** every user-owned
   entity has an explicit delete or retention outcome.

---

### User Story 3 - Operate and evolve the database safely (Priority: P3)

As an operator or data engineer, I need access patterns, indexes, growth assumptions, monitoring,
backup, and migration guidance so that the data store can evolve without avoidable outages or
performance regressions.

**Why this priority**: A logical model is incomplete without an operational and migration model.

**Independent Test**: A reviewer can identify the index supporting each important read/write path,
the expected scale threshold for redesign, and a safe rollout sequence for each documented gap.

**Acceptance Scenarios**:

1. **Given** a critical query pattern, **When** its performance support is reviewed, **Then** the
   design identifies the expected index and deterministic ordering.
2. **Given** a proposed schema change, **When** rollout is planned, **Then** the design provides
   expand, backfill, validation, cutover, and cleanup guidance.
3. **Given** a database incident or growth concern, **When** operators consult the design, **Then**
   it identifies health signals, backup expectations, recovery objectives, and capacity triggers.

### Edge Cases

- The authenticated user exists in the external identity provider but has not yet created a local
  profile or identity-anchor row.
- A mutable template or program is deleted after historical activity took a snapshot from it.
- Multiple workout sessions occur on one local calendar date while legacy endpoints still address a
  log by date.
- A request is retried with the same operation key and either the same or a different payload.
- A plan regeneration overlaps an active session or another program/schedule mutation.
- An account deletion occurs while some user-owned tables cascade from the identity anchor and
  legacy tables still require explicit deletion.
- An identifier reference becomes null while its immutable snapshot must remain reportable.
- A high-volume history query grows beyond the assumptions under which the current indexes were
  selected.
- A deployment runs while old and new application versions coexist with an expanded schema.

## Requirements

### Functional Requirements

- **FR-001**: The design MUST identify the database boundary, external identity dependency, and
  authoritative ownership key.
- **FR-002**: The design MUST inventory every current persisted table and every current column,
  including type, nullability, default, purpose, and sensitivity.
- **FR-003**: The design MUST document every primary key, foreign key, unique rule, check rule,
  deletion action, and index currently present.
- **FR-004**: The design MUST show all entity relationships in both a readable relationship diagram
  and a detailed table catalog.
- **FR-005**: The design MUST separate identity, preferences/profile, catalog, legacy
  template/logging, planning, goal-based programs, scheduled execution, participation,
  idempotency, and compatibility concerns.
- **FR-006**: The design MUST state which data is authoritative, snapshotted, derived, cached,
  external, or compatibility-only.
- **FR-007**: The design MUST trace persisted product behavior to the user-facing contract and
  identify data that is not persisted by the API database.
- **FR-008**: The design MUST document authenticated ownership enforcement and the expected behavior
  for missing, foreign, orphaned, and deleted resources.
- **FR-009**: The design MUST document transaction boundaries, optimistic revisions, idempotency
  records, lock ordering, retry behavior, and known race protections.
- **FR-010**: The design MUST derive an index strategy from current filters, joins, ordering,
  uniqueness, cascades, and expected growth.
- **FR-011**: The design MUST document data lifecycle rules for creation, mutation, snapshots,
  account deletion, retention, archival, and expired operational records.
- **FR-012**: The design MUST document database access roles, least-privilege expectations,
  encryption boundaries, secret handling, and the reason authorization remains in the API.
- **FR-013**: The design MUST document capacity assumptions, pagination rules, partitioning
  thresholds, maintenance, query diagnostics, backup, restore, and recovery expectations.
- **FR-014**: The design MUST distinguish the schema that exists today from the target design and
  provide a prioritized, forward-only migration roadmap for closing material gaps.
- **FR-015**: Each proposed migration stage MUST describe existing-data handling, mixed-version
  compatibility, validation evidence, roll-forward behavior, and cleanup timing.
- **FR-016**: The design MUST include a repeatable validation procedure that detects drift between
  the documentation, migrations, query access patterns, and public contract.
- **FR-017**: The design deliverable MUST NOT change public API behavior or production data as part
  of this documentation-only scope.

### Key Entities

- **Identity anchor**: The local representation of an externally authenticated user and the root for
  user-owned data.
- **Profile and preferences**: Display, onboarding, measurement, appearance, and training
  preference data belonging to one user.
- **Exercise catalog**: Curated, globally readable exercise definitions.
- **Legacy template and log model**: Reusable workout templates plus date-oriented workout history
  retained for compatibility.
- **Training program model**: Versioned starter content and user-owned program snapshots.
- **Scheduled execution model**: Dated workout snapshots, planned sets, workout sessions, and
  performed-set results.
- **Participation model**: Finalized daily opportunity and participation outcomes used for streaks.
- **Idempotency model**: Stored mutation fingerprints and responses used to make retries safe.
- **Compatibility adoption model**: The one-time link between a user's legacy data and goal-based
  training.

## Success Criteria

### Measurable Outcomes

- **SC-001**: 100% of current tables, columns, keys, constraints, and indexes are represented in the
  design with no unresolved placeholders.
- **SC-002**: 100% of persisted user-facing product concepts can be classified as authoritative,
  snapshotted, derived, cached, external, or compatibility-only.
- **SC-003**: Every user-owned table has an explicit ownership path and account-deletion outcome.
- **SC-004**: Every high-frequency or growth-sensitive access pattern has a documented deterministic
  order and supporting current or proposed index.
- **SC-005**: Every material schema gap is assigned a priority and a safe evolution stage rather
  than being silently treated as current behavior.
- **SC-006**: An engineer unfamiliar with the persistence code can locate the owner, lifecycle, and
  source of truth for a named entity in under 10 minutes using the design alone.
- **SC-007**: The documented validation procedure can be executed without modifying production data
  and produces an unambiguous pass, drift, or environment-unavailable result.

## Assumptions

- The design covers the complete behavior currently represented by the API migrations, persistence
  code, public contract, and mobile client types as reconciled on 2026-10-05.
- Supabase remains the external authentication authority; application data continues to flow through
  the Go API.
- PostgreSQL remains a single logical primary database for the current scale; multi-writer or
  multi-region persistence is outside this design.
- User workout, profile, settings, body-weight, program, and participation data is retained until
  the user deletes it; legal holds and regulated medical-record retention are outside current scope.
- Account deletion is intended to remove all application data promptly and permanently. Backup
  copies age out according to infrastructure retention and are not selectively rewritten.
- Historical performance must survive deletion or editing of mutable planning sources through
  snapshots, while optional provenance references may become null.
- This deliverable documents and validates the design. Any production schema changes identified by
  the gap analysis require separate reviewed forward migrations and PostgreSQL-backed tests.
