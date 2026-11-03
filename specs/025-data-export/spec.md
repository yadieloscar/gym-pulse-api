# Feature Specification: Export My Data

**Feature Branch**: `spec/025-data-export`

**Feature ID**: `gympulse-data-export`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/025-data-export/spec.md)

**Created**: 2026-11-03

**Status**: Implemented, pending owner review (owner direction: close the "data ownership and export" gap from the 2026-11-03 build-vs-buy review)

**Input**: "Let the user export all their data (at minimum workout logs with sets, templates, sport
activities, body weight, nutrition days) as CSV and/or JSON from the app."

## Clarifications

### Session 2026-11-03

Decisions derived from the owner's request, `docs/PRODUCT.md` (user ownership, trustworthy history,
privacy by design), and the existing contract. The owner can override any of them.

- Q: Which format? → A: Both. One complete JSON document for everything (the portable, lossless
  copy) and one flat CSV of workout sets (what Hevy and Strong export, readable in a spreadsheet).
- Q: Where is the export built? → A: In the API, from one consistent snapshot. The app never reads
  the database another way, so the API is the only place that sees all sections at once.
- Q: Which data is "all their data"? → A: Everything the athlete authored or performed: profile
  name, settings, training profile, workouts with sets (goal-based sessions and legacy day logs),
  saved workouts (templates), sport activities, body weights, nutrition days, nutrition targets,
  food entries, and own foods. Programs, the schedule, weekly plans, criteria-based training blocks,
  and Health export receipts are derived or planning data and are left for a later additive version.
- Q: Are discarded sessions exported? → A: No. The athlete threw them away; they are not history.
- Q: What unit are set weights in? → A: Set and template weights carry no stored unit; the export
  states the athlete's `settings.weight_unit`. Body weights keep the unit they were entered in.
- Q: Is the export stored or emailed? → A: No. It is generated on request, returned once, and never
  persisted, cached, or sent anywhere else.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Take a complete copy of my data (Priority: P1)

An athlete asks for their data and receives one JSON file that holds every workout with its sets,
their saved workouts, sport activities, body weights, and nutrition records.

**Independent Test**: Seed two athletes with rows in every exported table, export the first, and
verify every section of the first is present and nothing of the second appears.

**Acceptance Scenarios**:

1. **Given** an athlete with data in every section, **When** they export, **Then** each section is
   present with its rows, oldest first.
2. **Given** another athlete's data in the same tables, **When** the first athlete exports,
   **Then** none of it appears.
3. **Given** a new athlete with no data, **When** they export, **Then** every collection is `[]`
   and absent singletons are `null`.
4. **Given** a discarded session, **When** the athlete exports, **Then** it and its sets are absent.
5. **Given** a workout finished while the export is reading, **When** the export completes,
   **Then** that workout appears whole or not at all.

### User Story 2 - Open my sets in a spreadsheet (Priority: P1)

An athlete asks for a CSV and receives one row per recorded set that opens in a spreadsheet.

**Acceptance Scenarios**:

1. **Given** workouts with sets, **When** the athlete exports CSV, **Then** each set is one row with
   date, workout, exercise, targets, actuals, unit, duration, and flags, oldest first.
2. **Given** a workout with notes but no sets, **When** exported, **Then** it still has one row.
3. **Given** athlete text starting with `=`, `+`, `-`, or `@`, **When** exported, **Then** the cell is
   prefixed with `'` so a spreadsheet does not run it as a formula.

### Edge Cases

- Database failure → standard JSON error, no attachment header, no partial file.
- Repeated taps → per-user limit of 6 per minute with `429 RATE_LIMITED`.
- Deleted identity → existing active-user check rejects with `401` before any read.

## Requirements *(mandatory)*

- **FR-001**: `GET /api/v1/export` MUST return the athlete's data as one JSON document with
  `schema_version` and `exported_at`.
- **FR-002**: `GET /api/v1/export/workout-sets` MUST return the athlete's sets as RFC 4180 CSV.
- **FR-003**: Every query MUST be scoped to the authenticated user; child rows MUST be reached only
  through an owned parent.
- **FR-004**: All sections MUST be read in one read-only repeatable-read transaction.
- **FR-005**: The export MUST work under the least-privilege runtime role without new grants.
- **FR-006**: Both endpoints MUST be rate limited per user and MUST NOT persist the export.
- **FR-007**: Empty collections MUST encode as `[]`.
- **FR-008**: The change MUST be additive: no migration, no change to existing endpoints.

## Success Criteria *(mandatory)*

- **SC-001**: An athlete can obtain a complete copy of their data in one request.
- **SC-002**: The CSV opens in a spreadsheet with one row per set and no evaluated formulas.
- **SC-003**: No test or review finds another athlete's data in an export.
