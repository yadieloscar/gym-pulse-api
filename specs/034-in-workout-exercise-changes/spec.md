# Feature Specification: Swap or skip an exercise during a workout

**Feature Branch**: `feat/swap-skip-exercise`

**Feature ID**: `gympulse-in-workout-exercise-changes`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/040-in-workout-exercise-changes/spec.md)

**Created**: 2026-11-17

**Status**: Approved (requested by the owner on 2026-11-17; scope chosen in Clarifications)

**Input**: Owner, from the workout player: "we should be able to also swap or do anything to an exercise while doing it here."

Governing product rules: workspace `docs/PRODUCT.md`. Nothing there changes: the program stays the plan, and a dated workout stays that day's own copy of it.

## Clarifications

### Session 2026-11-17 (owner's answers)

- Q: Which changes should an exercise allow during a workout? → A: **Swap** it for another exercise and **skip** it. Adding an exercise and reordering were offered and not chosen.
- Q: Does a change during a workout change the program for future weeks? → A: **No, today only.** This matches `PATCH .../sets/{set_id}/target`. The program is changed in the program editor.

### Drafting defaults

- A set that already has a performed result keeps its identity and the exercise it was performed as. Only open sets are swapped or skipped.
- A swap keeps reps, duration, rest, and notes, and clears the target weight: that weight belonged to the other exercise.
- Swapping an exercise with some sets performed leaves those sets in place and puts the remaining sets, as the new exercise, directly after them.
- A workout keeps at least one set. Skipping its last remaining work is refused; the athlete skips the workout instead (`POST .../skip`).
- Sport sessions have no sets and refuse both routes, like the other set routes.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Swap an exercise for today (Priority: P1)

The machine is taken, so the athlete replaces Dips with Push-ups for today and keeps the planned sets and reps.

**Independent Test**: swap an exercise in today's workout; the workout returns with the new name on that exercise's open sets, and next week's workout still has the original.

**Acceptance Scenarios**:

1. **Given** an exercise with no performed set, **When** it is swapped, **Then** its sets keep their ids, order, reps, and rest, carry the new name and category, and have no target weight.
2. **Given** an exercise with one of three sets performed, **When** it is swapped, **Then** the performed set is unchanged and the two open sets follow it as the new exercise, numbered from 1; later exercises move one place down.
3. **Given** every set of the exercise is performed, **When** it is swapped, **Then** the answer is 422 and nothing changes.

### User Story 2 - Skip an exercise for today (Priority: P1)

The athlete is short on time and drops one exercise so the workout can finish as done.

**Acceptance Scenarios**:

1. **Given** an exercise with no performed set, **When** it is skipped, **Then** its sets are gone and later exercises move one place up.
2. **Given** an exercise with some sets performed, **When** it is skipped, **Then** only its open sets are gone.
3. **Given** the exercise holds the workout's only sets and none is performed, **When** it is skipped, **Then** the answer is 422.

### Edge Cases

- A stale `expected_revision` answers 409 with the authoritative workout.
- A repeated request with the same operation key returns the recorded response.
- A finished workout answers 409.
- An `exercise_order` not in the workout, or a workout owned by someone else, answers 404.

## Requirements *(mandatory)*

- **FR-001**: `POST /api/v1/scheduled-workouts/{id}/exercises/{exercise_order}/swap` replaces the open sets' exercise for that date only.
- **FR-002**: `POST /api/v1/scheduled-workouts/{id}/exercises/{exercise_order}/skip` removes the open sets for that date only.
- **FR-003**: Neither route changes a performed set, the program, the saved workout, or another date.
- **FR-004**: Both routes are idempotent by operation key and guarded by `expected_revision`; each successful change increases the workout revision by one.
- **FR-005**: Both routes answer with the authoritative `ScheduledWorkout`.

## Success Criteria *(mandatory)*

- **SC-001**: After a swap or skip, a workout can be finished as completed without the removed or replaced sets counting as unfinished.
- **SC-002**: No performed set is lost or renamed by either route (integration tests).

## Rollout

Additive routes, contract 1.14.0, no migration. API first; the app sibling treats a missing route as "not available yet".
