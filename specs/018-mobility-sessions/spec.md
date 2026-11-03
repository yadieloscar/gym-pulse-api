# Feature Specification: Mobility Sessions (recorded stretching)

**Feature Branch**: `spec/018-mobility-sessions`

**Feature ID**: `gympulse-stretching-routines`

**Sibling Spec**: `gym-pulse-app/specs/026-stretching-routines/spec.md`

**Created**: 2026-11-03

**Status**: Approved (owner, 2026-11-03: "Approve, implement")

**Input**: Owner request (2026-11-03) for a guided stretching feature; the owner chose to record
finished stretching without counting it toward consistency.

## Clarifications

### Session 2026-11-03

- Q: Why a new resource instead of a workout session or sport activity? → A: Workout sessions feed
  participation, stats, records, and volume, and sport activities are completed sports that can
  fulfil planned sport sessions. Stretching must do none of that, so it gets its own owned
  resource that no derived read model consumes.
- Q: Does the API own the stretching catalog? → A: No. Routines are bundled in the app. The API
  stores what was performed (routine id, display name, counts, duration) as facts and does not
  validate routine ids against a catalog.
- Q: Can an entry be edited? → A: No. Entries are created once and may be deleted by their owner.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Record a finished routine (Priority: P1)

The app posts one entry when a stretching routine ends with at least one completed move. Retries
with the same operation key return the original entry.

**Why this priority**: Without it nothing the athlete does in a routine survives reinstall or a
second device.

**Independent Test**: POST an entry twice with the same `Idempotency-Key`; receive 201 then 200
with the same id; GET the date range and see one entry.

**Acceptance Scenarios**:

1. **Given** a valid body, **When** POSTed, **Then** 201 returns the stored entry with `id` and
   `created_at`.
2. **Given** the same operation key again, **Then** 200 returns the original entry unchanged.
3. **Given** a future `date`, an out-of-range count, `moves_completed` > `moves_total`, or an
   unknown `context`, **Then** 422 `VALIDATION_ERROR` names the field.

### User Story 2 - Read and delete entries (Priority: P1)

The app lists an athlete's entries for a date range and deletes one by id.

**Independent Test**: Create entries on two dates, list a range covering one, delete it, list
again.

**Acceptance Scenarios**:

1. **Given** entries owned by another athlete, **Then** list never returns them and GET/DELETE by
   their id is the shared 404.
2. **Given** an owned entry, **When** DELETE is called, **Then** 204; a repeat DELETE is 404.
3. **Given** a range longer than 366 days or `from` after `to`, **Then** 422.

### User Story 3 - Stretching never moves consistency (Priority: P1)

**Independent Test**: With and without mobility entries on scheduled and rest dates, stats
(`this_week`, `streak_weeks`), participation, records, history, and volume responses are byte-for-
byte identical.

**Acceptance Scenarios**:

1. **Given** a rest day with only a mobility entry, **Then** participation omits that date as
   before and stats are unchanged.
2. **Given** account deletion, **Then** the athlete's mobility entries are removed with the account.

### Edge Cases

- Same operation key with a different body: 409 `IDEMPOTENCY_CONFLICT` (follow the existing
  idempotency convention).
- `date` omitted: defaults to today in the athlete's training-profile timezone, as sport activities.
- `routine_name` with surrounding whitespace is trimmed; empty after trim is 422.
- Very old app versions never call these routes; no compatibility shim is needed (additive).

## Non-Goals *(mandatory)*

- A server-side stretch catalog or routine endpoint.
- Updates (PATCH/PUT) to an entry.
- Including entries in stats, participation, records, history, volume, plan, or schedule.
- Health-store import of flexibility or cooldown workouts (they stay skipped by the app).
- Any notification or job.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The API MUST add `POST /api/v1/mobility-sessions` requiring `Idempotency-Key`
  equal to `operation_key`, with body fields: `date` (optional, `YYYY-MM-DD`, not in the future),
  `routine_id` (lowercase slug, 1–64), `routine_name` (trimmed, 1–80), `context`
  (`warmup` | `cooldown` | `rest_day` | `anytime`), `duration_seconds` (1–7,200), `moves_completed`
  (1–100), `moves_total` (1–100, ≥ `moves_completed`), `content_version` (1–32 chars), and
  `operation_key` (1–128).
- **FR-002**: The API MUST add `GET /api/v1/mobility-sessions?from=&to=` (inclusive, ≤ 366 days,
  ordered by `date`, `created_at`, `id` newest first; `[]` when none), `GET /{id}`, and
  `DELETE /{id}` → 204.
- **FR-003**: Every route MUST be owner-scoped by the authenticated subject and protected by
  row-level security consistent with existing athlete tables; a foreign or missing id is the
  shared 404.
- **FR-004**: Storage MUST be a new table with an owner foreign key that cascades on account
  deletion and a unique (owner, operation_key) constraint.
- **FR-005**: No existing query or read model (stats, participation, records, exercise history,
  volume, plan, schedule, sport activities, health exports) MUST read the new table.
- **FR-006**: `docs/CONTRACTS.md` MUST document the routes and bump the contract minor version
  (additive).

### Key Entities

- **Mobility session**: id, owner, date, routine_id, routine_name, context, duration_seconds,
  moves_completed, moves_total, content_version, operation_key, created_at.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Repository and handler tests cover create, idempotent replay, conflict, validation,
  list range, ownership 404, delete, and account-deletion cascade.
- **SC-002**: A regression test proves stats and participation are unchanged by mobility entries.
- **SC-003**: The RLS validator passes with the new table.

## Assumptions

- The next free migration number is taken from `origin/main` at implementation time (currently
  after 031) and rechecked before merge.
- Ships and deploys before the app version that records entries (API first).
