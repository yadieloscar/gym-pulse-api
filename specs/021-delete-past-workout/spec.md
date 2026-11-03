# Feature Specification: Delete a Past Workout

**Feature Branch**: `021-delete-past-workout`

**Feature ID**: `gympulse-delete-past-workout`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/022-delete-past-workout/spec.md)

**Created**: 2026-11-03

**Status**: Implemented (owner direction 2026-11-03: past workouts of all kinds become deletable, API first)

**Input**: A build-vs-buy review found GymPulse lets an athlete edit a past workout but not delete
it. The owner chose to make both off-plan logs and past plan workouts deletable.

## Product change

This spec intentionally reverses an earlier default. Two earlier training PRDs (now historical)
said "past workouts cannot be deleted", and the API answered `409` for a past
`DELETE /api/v1/logs/{date}`. `docs/PRODUCT.md` asks only that destructive changes be explicit and
tested, so an explicit, confirmed delete of the athlete's own record satisfies the "trustworthy
history" invariant. Corrections by editing remain available.

## Clarifications

### Session 2026-11-03

- Q: Which workouts can be deleted? → A: Every off-plan log at any date, and every scheduled
  workout that is finalized or dated before today. Today's and future unfinished plan workouts
  stay plan items: the athlete edits, skips, or regenerates them instead.
- Q: What goes with a deleted plan workout? → A: Its required sets, every workout session linked to
  it, and their performed sets. Records, volume, totals, and history no longer count them.
- Q: What happens to the streak? → A: The day's stored participation is recomputed from what
  remains on that date. Deleting a missed workout can therefore restore a streak; the athlete owns
  that record.
- Q: Apple Health? → A: The API deletes the export record. It cannot reach the device, so the
  workout already written to Apple Health stays there; the app says so.
- Q: Sport activities linked to a deleted sport session? → A: They stay in history, unlinked.
- Q: Undo? → A: No. Deletion is permanent and is confirmed in the app first.

## User Scenarios & Testing

### User Story 1 - Delete a past off-plan log (Priority: P1)

**Acceptance Scenarios**:

1. **Given** a log dated last month, **When** the athlete deletes it, **Then** `DELETE
   /api/v1/logs/{date}` answers 204 and the log, its sets, and its Health export record are gone.
2. **Given** the same date again, **When** it is deleted, **Then** the answer is 404.

### User Story 2 - Delete a past plan workout (Priority: P1)

**Acceptance Scenarios**:

1. **Given** a finished or past scheduled workout, **When** the athlete deletes it, **Then**
   `DELETE /api/v1/scheduled-workouts/{id}` answers 204 and the workout, its sessions, sets, and
   export record are gone, and stats no longer count it.
2. **Given** another scheduled workout remains on that date, **When** one is deleted, **Then** the
   day's participation reflects only the remaining one.
3. **Given** today's unfinished or a future workout, **When** deletion is requested, **Then** the
   answer is `409 CONFLICT` with the workout as `resource`, and nothing changes.
4. **Given** a linked session is active, **Then** `409 ACTIVE_SESSION_CONFLICT`.
5. **Given** another athlete's or a missing workout, **Then** 404.

## Requirements

- **FR-001**: `DELETE /api/v1/logs/{date}` deletes the athlete's log for any valid date.
- **FR-002**: `DELETE /api/v1/scheduled-workouts/{id}` deletes an owned scheduled workout that is
  finalized or dated before today (UTC, as the other historical checks), and rejects others with 409.
- **FR-003**: A plan-workout delete removes its required sets, linked sessions, and their sets in
  one transaction with the workout.
- **FR-004**: Both deletes remove the workout's Apple Health export record atomically.
- **FR-005**: A plan-workout delete recomputes that date's `day_participation` from the remaining
  scheduled workouts, completed sessions, and sport activities, and drops a day with neither an
  opportunity nor participation.
- **FR-006**: Foreign and missing workouts answer the shared 404; a repeated delete answers 404.
- **FR-007**: An active linked session blocks the delete.

## Success Criteria

- **SC-001**: After either delete, total workouts, weekly volume, and the day streak no longer count
  the deleted workout (DAO integration tests).
- **SC-002**: No orphaned sessions, set logs, or export records remain (DAO integration tests).

## Out of Scope

- Removing the workout from Apple Health (device-side; spec 009 already excluded it).
- Deleting off-plan workout sessions that are not linked to a scheduled workout.
- Undo or a trash bin.
