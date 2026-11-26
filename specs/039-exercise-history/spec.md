# Feature Specification: Exercise history

**Feature Branch**: `feat/039-exercise-history`

**Feature ID**: `gympulse-exercise-history`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/053-exercise-history/spec.md)

**Created**: 2026-11-25

**Status**: Approved 2026-11-25 (owner picked "exercise history page" from a brainstorm and asked
for it to be built end to end)

**Input**: One page per lift listing the athlete's past sessions of that exercise, newest first,
each with its date and the sets actually performed, and the heaviest set marked. Read-only. Prefer
an existing API read; add one only if needed, and page it so a long history does not load at once.

Governing product rules: workspace `docs/PRODUCT.md`, "Trustworthy history", "Plan versus
performance", "Records are sets the athlete actually lifted, never estimates", and "Review … without
changing the underlying training record".

## What already exists, and what is missing

| Read | Returns | Why it is not enough |
| --- | --- | --- |
| `GET /exercises/history?ids` | The completed sets of the **most recent** day per exercise | One day only |
| `GET /exercises/records?ids` | All-time heaviest set and best estimate per exercise | No sessions |
| `GET /exercises/progression?ids&weeks` | Heaviest set per week, at most 52 weeks | One set per week |
| `GET /logs?week` + `GET /logs/{date}` | A day's whole log | The app's lift page reads 12 weeks this way: 12 requests plus one per trained day, no paging, and no planned workouts |

All three per-exercise reads match `set_logs.exercise_id` only. A set logged in a planned (program)
workout has no saved-workout exercise id, so planned work is invisible to them. No existing read
returns a lift's sessions, so this feature adds one.

## Decisions

- **One new read, `GET /api/v1/exercises/sessions`, no schema change.** Every fact it needs is
  already stored on `set_logs`, `day_logs`, `workout_sessions`, `exercises`, and `scheduled_sets`.
- **A lift is named by any of three identities, and a set matches on any of them**: the saved-workout
  exercise id, the catalog id, and the name stored with the set. The app sends every identity it
  holds. This is the only way planned work (catalog id or name), off-plan work (exercise id), and
  imported history (name only) land on one page. Rejected: a new "lift" table that unifies
  identities; it is a larger data change than a read-only page justifies.
- **One entry per calendar day.** Two workouts on one day are one entry. The cursor is then a plain
  date and a page can never split a day.
- **Paged by date, newest first**: `limit` days (default 20, at most 50) and `before`.
- **The heaviest set is computed over the whole history and returned with every page**, so the mark
  is right on page one and never moves as older pages load. Same rule as `/exercises/records`:
  weight and positive reps, equal weight → more reps → earlier date. Never an estimate.
- **Completed sets only.** A set that was planned or entered but not completed is not performance.
- **Existing reads are unchanged.** Teaching records and progression about planned work would change
  numbers already on screen and is out of scope.

## User journeys

1. **Read a lift's history.** The athlete opens a lift and sees each day they did it, newest first,
   with every completed set.
2. **Go further back.** A long history arrives one page at a time.
3. **Planned and off-plan together.** A lift done in a program workout and in a saved workout shows
   both.

## Requirements

- **FR-001**: `GET /api/v1/exercises/sessions` MUST return
  `{ "sessions": [{ "date", "sets": [{ "actual_reps", "actual_weight", "duration_seconds" }] }], "next_before", "heaviest" }`
  for the authenticated athlete, with `sessions` as `[]` when there is nothing.
- **FR-002**: The lift MUST be named by `exercise_id`, `catalog_id`, `name`, or any mix. None →
  422 `VALIDATION_ERROR` with `field: "exercise_id"`; a malformed UUID → 422 naming its field; a
  `name` over 200 characters → 422 with `field: "name"`.
- **FR-003**: A set MUST be included when it is completed and matches any identity sent: its
  exercise id; its saved-workout exercise's or planned set's catalog id; or its stored name,
  compared trimmed and without case. Sets come from day logs and from workout sessions that are not
  discarded.
- **FR-004**: Sessions MUST be one per calendar day, newest first, with sets in logged order.
- **FR-005**: `limit` MUST default to 20 and allow 1 to 50; `before` MUST be a `YYYY-MM-DD` date and
  keep days strictly earlier. Otherwise 422 naming the field. `next_before` MUST be the next page's
  `before`, and `null` on the last page.
- **FR-006**: `heaviest` MUST be the heaviest completed set with a weight and positive reps across
  the lift's whole history (equal weight → more reps → earlier date), or `null`. It MUST be the same
  on every page.
- **FR-007**: The read MUST return only the authenticated athlete's sets. Another athlete's exercise
  id returns the empty page, not an error.
- **FR-008**: The route MUST be read-only and additive: no stored row, existing route, or existing
  field changes.

## Non-goals

- No change to `/exercises/history`, `/exercises/records`, or `/exercises/progression`.
- No editing or deleting from this read, no notes, no estimates, no volume totals.
- No new table, column, or index.

## Compatibility and rollout

Additive (contract 1.18.0), no migration. Deploy the API first. An older app never calls the route.
A newer app against an older API gets a 404 without the JSON envelope and says the page is not
available yet.

## Acceptance

| Requirement | Evidence |
| --- | --- |
| FR-001 | `TestLogHandler_ExerciseSessions` (success body, empty body), `TestExerciseSessions` |
| FR-002 | `TestLogService_ExerciseSessions` (invalid input table), smoke step 8b2 |
| FR-003 | `TestExerciseSessions` (every identity together; each identity alone; not completed; discarded) |
| FR-004 | `TestExerciseSessions` (order of days and of sets) |
| FR-005 | `TestLogService_ExerciseSessions` (limit and before), `TestExerciseSessions` (pages walk the history; a page that ends on the last day) |
| FR-006 | `TestExerciseSessions` (tie on weight; same heaviest on every page) |
| FR-007 | `TestExerciseSessions` (another athlete; the runtime role) |
| FR-008 | Full unit and integration suites unchanged apart from the new tests |
