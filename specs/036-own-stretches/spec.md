# Feature Specification: The athlete's own stretches, and routines on program workouts

**Feature Branch**: `feat/own-stretches`

**Feature ID**: `gympulse-own-stretches`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/044-own-stretches/spec.md)

**Created**: 2026-11-18

**Status**: Approved 2026-11-18 (owner request, scope chosen in Clarifications)

**Input**: Owner, 2026-11-18: "I also want to be able to create my own stretches AND I want the
kneee kit should be integrated with a workout like its a stretch too".

Governing product rules: workspace `docs/PRODUCT.md`, "Stretching". This feature changes one line of
it, "no custom routine builder": an athlete may now keep their own stretch moves and routines. They
stay text and timers, GymPulse never supplies or suggests them, and they never count toward the
weekly target, streak, records, or volume.

## Clarifications

### Session 2026-11-18 (owner's answers)

- Q: What do you want to create? → A: **Both**: your own stretch moves, and your own routines built
  from them and from the bundled moves.
- Q: A saved workout can already pick Knee Kit as its warm-up or cool-down. What is missing? →
  A: **Program workouts.** A workout that comes from a program cannot choose a warm-up or cool-down.

### Drafting defaults

- Stretching content stays in the app. The API stores the athlete's own moves and routines as one
  small document per account and does not know the bundled catalog.
- The document is read and written whole. It is small (at most 50 moves and 30 routines), always
  used together, and never queried by field, so it is one JSONB value with a revision, like
  `programs.roadmap`. Rejected: normalized move, routine, and item tables with a route per row,
  which would be several times the code for no behaviour the app needs.
- A program unit's routine choice lives on the unit, not on each dated session. The app reads it
  through the session's `program_workout_id`. Changing it never changes a session's sets.
- Finished routines are recorded as before through `/api/v1/mobility-sessions`, which already accepts
  any slug as `routine_id`.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Keep my own stretches (Priority: P1)

**Independent Test**: PUT a library with one move and one routine that uses it and a bundled move;
GET returns the same content at revision 1.

**Acceptance Scenarios**

1. **Given** an account that never saved a library, **When** it GETs the library, **Then** it
   receives revision 0 with empty lists.
2. **Given** revision N, **When** it PUTs with `expected_revision` N, **Then** the whole document is
   replaced and returned at revision N+1.
3. **Given** a stale `expected_revision`, **Then** 409 `REVISION_CONFLICT` with the current library
   under `resource`, and nothing changes.
4. **Given** the same `operation_key` and body again, **Then** the stored answer replays with 200; a
   different body under that key is 409 `IDEMPOTENCY_CONFLICT`.
5. **Given** another athlete, **Then** they never see or change this library.
6. **Given** account deletion, **Then** the library is deleted.

### User Story 2 - A program workout chooses its warm-up and cool-down (Priority: P1)

**Independent Test**: PUT a program whose unit carries `warmup_routine_id: "knee-kit"`; GET returns
it; a later PUT that omits the key keeps it; a PUT with `null` clears it.

**Acceptance Scenarios**

1. **Given** any program or starter response, **Then** every unit carries `warmup_routine_id` and
   `cooldown_routine_id`, `null` when unset.
2. **Given** a create or update with a slug in either field of an exercises unit, **Then** it is
   stored and returned.
3. **Given** an update where a unit that already exists (by `id`) omits a key, **Then** the stored
   value is kept; `null` or `""` clears it.
4. **Given** a value that is not a slug, or a routine on a sport unit, **Then** 422
   `VALIDATION_ERROR` with `details.field` `workouts`.
5. **Given** a routine-only change, **Then** no dated session's sets, status, or revision change.

## Requirements

- **FR-001**: `GET /api/v1/stretching/library` → 200
  `{ "revision": 0, "moves": [], "routines": [], "updated_at": null }` for a new account, else the
  stored document with its revision and `updated_at` (RFC 3339).
- **FR-002**: `PUT /api/v1/stretching/library` with
  `{ "expected_revision": <int ≥ 0>, "operation_key": "<1..128>", "moves": [Move], "routines": [Routine] }`
  and an `Idempotency-Key` header equal to `operation_key` → 200 with the same shape as GET.
  `expected_revision` 0 creates the document.
- **FR-003**: `Move`: `id` (slug, 4..64, starts with `my-`), `name` (1..60 after trimming), `cue`
  (0..200), `areas` (0..60), `mode` (`"hold"` or `"reps"`), `sided` (bool), `hold_seconds` (5..600,
  required for `hold`, `null` for `reps`), `reps` (1..100, required for `reps`, `null` for `hold`).
  At most 50, ids unique.
- **FR-004**: `Routine`: `id` (slug, 4..64, starts with `my-`), `name` (1..60 after trimming),
  `items` (1..30) of `{ "move_id": <slug 1..64>, "hold_seconds": <5..600 or null>, "reps": <1..100 or null> }`.
  At most 30, ids unique. An item whose `move_id` starts with `my-` must name a move in the same
  body. Other `move_id`s are bundled moves the API does not check. At most one of `hold_seconds` and
  `reps` is set on an item; it replaces the move's own dose in that routine.
- **FR-005**: Any rule in FR-003 or FR-004 that fails is 422 `VALIDATION_ERROR` with `details.field`
  `moves` or `routines`. A slug is the pattern the mobility and template routes already use.
- **FR-006**: The mutation follows the saved-meals pattern: one transaction, the idempotency record
  and the write together, a stale revision is 409 `REVISION_CONFLICT` with `resource`.
- **FR-007**: One new table, `stretch_libraries`, keyed by `user_id` (cascade on account delete),
  with `revision`, `content` (JSONB), `updated_at`, secured with `gym_pulse_private.secure_table`.
  It is added to account deletion.
- **FR-008**: `program_workouts` gains nullable `warmup_routine_id` and `cooldown_routine_id` with
  the same check as `workout_templates` (migration 049). Program and starter unit JSON always carries
  both keys, `null` when unset; a client may use their presence to detect this contract.
- **FR-009**: On program create and update the fields follow User Story 2. A unit copied from a
  starter has both `null`.
- **FR-010**: `docs/CONTRACTS.md` documents both parts as contract 1.16.0, including how a client
  detects an older server (the library route answers 404 without the JSON error body; program units
  lack the keys). Swagger, the Bruno collection, and the smoke script cover the library route.

## Non-goals

- A server-side catalog of bundled stretches, or validating bundled move or routine ids.
- Routine fields on dated sessions (`scheduled_workouts`) or on the rules engine and generated drafts.
- The library in the data export (finished routines are not exported today either).
- Sharing stretches between athletes.

## Success Criteria

- **SC-001**: Two requests cover the library (one read, one write); program routes gain fields only.
- **SC-002**: Unit tests cover every validation rule and the absent-key rule; integration tests cover
  the library DAO (ownership, revision, idempotency) and the program unit columns; `go test ./...`,
  lint, the migration lint, and the smoke script pass.
