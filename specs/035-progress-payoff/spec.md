# Feature Specification: Weekly heaviest set per exercise

**Feature Branch**: `feat/progress-payoff`

**Feature ID**: `gympulse-progress-payoff`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/042-progress-payoff/spec.md)

**Created**: 2026-11-18

**Status**: Approved 2026-11-18 (the owner approved the Progress design that needs it)

**Input**: The app's Progress screen shows, for each of up to five lifts, the heaviest set per week
over 12 weeks. Today the app can only build that by reading 12 weekly log lists and then every
trained day, which is dozens of requests. The API and the database are in different regions, so each
request is slow. This feature answers the question in one request.

Governing product rules: workspace `docs/PRODUCT.md`. "Records are sets the athlete actually lifted,
never estimates": this route returns performed sets only.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - One request for a lift's weekly trend (Priority: P1)

The app asks for several exercises at once and receives, per exercise, the heaviest completed
weighted set in each week that has one.

**Independent Test**: seed completed sets for two exercises across three weeks, call the route, and
compare each week's weight, reps, and date.

**Acceptance Scenarios**

1. **Given** completed weighted sets in three different weeks, **When** the route is called with
   `weeks=12`, **Then** the exercise has three week entries, oldest first, each with the heaviest
   weight, its reps, and the date it was lifted.
2. **Given** two sets of the same weight in one week, **Then** the entry carries the one with more
   reps; equal reps keep the earlier date.
3. **Given** a week with no completed weighted set, **Then** that week has no entry.
4. **Given** an exercise with no completed weighted set in the window, or an id the caller does not
   own or that does not exist, **Then** it is left out of the response.
5. **Given** sets from a discarded workout, or sets not marked completed, **Then** they are ignored.
6. **Given** another athlete's sets for the same exercise id, **Then** they never appear.

## Requirements

- **FR-001**: `GET /api/v1/exercises/progression?ids=<uuid,uuid>&weeks=N` returns
  `ExerciseProgression[]` for the authenticated athlete.
- **FR-002**: `ids` follows the rules of `/exercises/records`: a malformed id or more than 100 ids
  is 422 `VALIDATION_ERROR` with `details.field` `ids`; empty `ids` returns `[]`.
- **FR-003**: `weeks` defaults to 12, allows 1 to 52, and is 422 with `details.field` `weeks`
  otherwise. The window is the current week and the `N-1` weeks before it, using the same week
  boundary as `GET /api/v1/stats/volume`.
- **FR-004**: The set source is the one `/exercises/records` uses: completed sets with a weight and
  positive reps, from day logs and from workout sessions that are not discarded.
- **FR-005**: Each exercise carries `weeks`, oldest first; each entry has `week_start` (Monday,
  `YYYY-MM-DD`), `max_weight`, `max_weight_reps`, and `date`. Exercises appear in the order
  requested.
- **FR-006**: The route is read-only: no idempotency key, no revision, no write.
- **FR-007**: The route needs no migration. It reads through the runtime role under the existing
  row-level rules.
- **FR-008**: `docs/CONTRACTS.md` documents the route as contract 1.15.0 and says how a client
  detects an older server (404 without the JSON error body). Swagger, the Bruno and Postman
  collections, and the smoke script cover it where those already cover `/exercises/records`.

## Non-goals

- Estimated max, volume per exercise, or per-set history.
- Changing `/exercises/records` or `/exercises/history`.

## Success Criteria

- **SC-001**: One request, one query, for up to 100 exercises.
- **SC-002**: Unit tests cover the reduce and validation rules; an integration test covers the query
  against PostgreSQL, including ownership and discarded workouts; `go test ./...`, lint, and the
  smoke script pass.
