# Feature Specification: Knee Kit (daily knee checklist and pain log)

**Feature Branch**: `spec/027-knee-kit`

**Feature ID**: `gympulse-knee-kit`

**Sibling Spec**: `gym-pulse-app/specs/031-knee-kit/spec.md`

**Created**: 2026-11-14

**Status**: Approved (owner, 2026-11-14: "Approve, build it")

**Input**: Owner request (2026-11-14) for a daily knee checklist and a daily knee pain log in the
app. The API stores one record per athlete per local date and nothing more.

## Clarifications

### Session 2026-11-14

- Q: Does the API compute guidance, a traffic light, or a streak? → A: No. The API stores facts
  (ticked item ids, two optional pain scores, two flags) and never diagnoses. The app computes the
  traffic-light guidance and the daily streak from the range read.
- Q: Does a Knee Kit day feed any derived read model? → A: No. It is a plain owned resource.
  Participation, stats, streaks, records, volume, distribution, plan, schedule, and day status
  never read it. Only the data export and account deletion touch the table besides its own routes.
- Q: Spec 005 lists "pain scoring" as a non-goal. Does this contradict it? → A: By owner direction
  this narrows that non-goal only for this self-monitoring resource: the athlete records their own
  0–10 score and the API stores it. The API still does not score, interpret, gate, or progress
  anything from pain, and criteria-based training blocks are unchanged.
- Q: Does the API own the checklist content? → A: No. The checklist is bundled in the app. The API
  validates the shape of item ids (slug, count, no duplicates) and not their meaning.
- Q: Is an empty day deleted? → A: No. A day with no items and no pain values is still stored,
  because the athlete unticked things. Only account deletion removes a day.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Save a day (Priority: P1)

The app PUTs the whole record for a date whenever the athlete ticks an item or logs pain. Retries
with the same operation key replay the original result.

**Why this priority**: Without it the checklist and pain log do not survive reinstall or a second
device.

**Independent Test**: PUT a date with `expected_revision` 0 → 201; the same key again → the same
201; a new key with the stored revision → 200 and `revision` 2.

**Acceptance Scenarios**:

1. **Given** no record for a date, **When** PUT with `expected_revision` 0, **Then** 201 returns
   the stored day with `revision` 1.
2. **Given** a stored day, **When** PUT with its revision, **Then** 200 returns the fully replaced
   day with the revision incremented; omitted pain values become `null` and omitted flags `false`.
3. **Given** a stale `expected_revision`, **Then** 409 `REVISION_CONFLICT` carries the stored day
   under `resource` (omitted when no record exists).
4. **Given** the same key and payload, **Then** the original status and body replay; the same key
   with a different date or body is 409 `IDEMPOTENCY_CONFLICT`.
5. **Given** a future date, more than 12 items, a non-slug or duplicate item, or a pain value
   outside 0–10, **Then** 422 `VALIDATION_ERROR` names the field.

### User Story 2 - Read a range (Priority: P1)

The app reads a date range to draw the checklist, the pain history, the guidance, and the streak.

**Independent Test**: Save two dates, read a range covering both, receive them oldest first.

**Acceptance Scenarios**:

1. **Given** days owned by another athlete, **Then** the range never returns them.
2. **Given** no days in the range, **Then** `{"days":[]}`.
3. **Given** a range longer than 366 days, a malformed date, or `from` after `to`, **Then** 422.

### User Story 3 - Knee Kit never moves consistency (Priority: P1)

**Independent Test**: With and without Knee Kit days on scheduled and rest dates, stats and
participation responses are identical.

**Acceptance Scenarios**:

1. **Given** a rest day with only a Knee Kit day, **Then** participation omits that date as before
   and stats are unchanged.
2. **Given** account deletion, **Then** the athlete's Knee Kit days are removed with the account.
3. **Given** a data export, **Then** it carries the athlete's Knee Kit days and no one else's.

### Edge Cases

- `completed_items` absent or `null` is 422; `[]` is valid and stored.
- The same items in a different order are the same payload; items are stored and returned sorted.
- `expected_revision` absent is 422, so a missing field cannot pass as "no record exists".
- An athlete without a training profile receives the shared 404 on PUT (the local today cannot be
  resolved); the range read needs no profile.
- Very old app versions never call these routes; no compatibility shim is needed (additive).

## Non-Goals *(mandatory)*

- Guidance, traffic lights, thresholds, streaks, diagnosis, or any interpretation of pain.
- A server-side checklist catalog or validation of item ids against one.
- Including Knee Kit days in participation, stats, streaks, records, volume, plan, schedule, or
  day status.
- Deleting a single day, or a partial (PATCH) update.
- New exercise-catalog entries (see plan: no way to keep them out of program generation without
  changing the engine).
- Any notification or job.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The API MUST add `PUT /api/v1/knee-kit/days/{date}` requiring `Idempotency-Key`
  equal to `operation_key`, fully replacing that date's record, with body fields
  `completed_items` (required array, at most 12, each a lowercase slug of 1–64 characters, no
  duplicates), `morning_pain` and `activity_pain` (optional, nullable, whole 0–10), `sharp_pain`
  and `limping` (boolean, default false), `expected_revision` (required, ≥ 0), and
  `operation_key` (1–128). `date` MUST be a valid date not after the athlete's local today.
- **FR-002**: A save MUST answer 201 when created and 200 when replaced, MUST enforce
  `expected_revision` (409 `REVISION_CONFLICT` with the stored day), and MUST replay an identical
  retry and reject a changed payload under the same key (409 `IDEMPOTENCY_CONFLICT`).
- **FR-003**: The API MUST add `GET /api/v1/knee-kit/days?from=&to=` (inclusive, ≤ 366 days,
  oldest first, `{"days":[]}` when none).
- **FR-004**: Storage MUST be a new table with an owner foreign key that cascades on account
  deletion, a unique (owner, date) key, CHECK constraints mirroring the validation, a revision,
  and row-level security consistent with existing athlete tables. Every query MUST be scoped by
  the authenticated subject.
- **FR-005**: No existing query or read model MUST read the new table, apart from account
  deletion and the data export.
- **FR-006**: Account deletion MUST delete the athlete's Knee Kit days explicitly, and the data
  export MUST carry them in an additive `knee_kit_days` section (`schema_version` stays 1).
- **FR-007**: `docs/CONTRACTS.md` MUST document the routes and the export section as contract
  1.7.0 (additive).

### Key Entities

- **Knee Kit day**: id, owner, date, completed_items, morning_pain, activity_pain, sharp_pain,
  limping, revision, created_at, updated_at.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Service, handler, and repository tests cover create, replace, idempotent replay,
  both conflicts, validation, the range read, ownership, and the account-deletion cascade.
- **SC-002**: A regression test proves stats and participation are unchanged by Knee Kit days,
  and a static test fails if any other DAO names the table.
- **SC-003**: The database-design and RLS validators pass with the new table.

## Assumptions

- Migration 037 is used because 035 and 036 are claimed by other open branches. golang-migrate
  applies versions in ascending order and tolerates the gap, but it never goes back: this branch
  must not be deployed before 035 and 036, or those must renumber past 037.
- Contract 1.7.0 is used because 1.6.0 is claimed by other open branches.
- Ships and deploys before the app version that records Knee Kit days (API first).
