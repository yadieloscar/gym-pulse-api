# Feature Specification: Skip a missed workout

**Feature Branch**: `spec/007-skip-missed-workout`

**Feature ID**: `gympulse-today-next-action`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/007-today-next-action/spec.md)

**Created**: 2026-10-11

**Status**: Draft

**Input**: User description: "An athlete can explicitly skip a missed planned workout so the next workout in the sequence becomes next, and can undo that skip. The missed date stays recorded as missed; skipping never rewrites history, participation, or records."

## Clarifications

### Session 2026-10-11

Decisions from the experience blueprint review with the product owner (workspace `docs/PRODUCT.md`:
"Missed work waits for a choice").

- Q: What does skipping change? → A: Only whether the missed workout is still offered as next. Its missed outcome, participation, and history stay exactly as recorded.
- Q: Can a skip be undone? → A: Yes, unless the workout has since been performed through recovery.
- Q: Is moving a missed workout to another future day included? → A: No.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Skip the earliest missed workout (Priority: P1)

An athlete who missed a planned workout tells GymPulse to skip it. From then on, that workout is no
longer the next workout to do, and the following workout in the sequence becomes next. The day it was
missed stays recorded as missed.

**Why this priority**: The app's Today screen needs an authoritative, cross-device record of the
athlete's choice; without it, a missed workout stays next forever or the choice is lost on another
device.

**Independent Test**: For an athlete with one missed workout, skip it, then read the schedule and
confirm the workout is marked as skipped, its outcome is still missed, and recovery no longer selects it.

**Acceptance Scenarios**:

1. **Given** the athlete's earliest unresolved missed workout, **When** the athlete skips it, **Then** the workout is recorded as skipped by the athlete, its outcome remains missed, and the response returns the updated workout.
2. **Given** a skipped workout, **When** the athlete asks to recover a missed workout for a date, **Then** recovery selects the earliest missed workout that is not skipped, or reports that none remains.
3. **Given** the same skip request is repeated with the same operation key, **When** it is received again, **Then** the original result is returned and nothing changes twice.
4. **Given** a workout that is not missed (planned, in progress, completed, or incomplete), **When** the athlete tries to skip it, **Then** the request is rejected with a stable error and nothing changes.

---

### User Story 2 - Undo a skip (Priority: P2)

Right after skipping, or later, the athlete can undo the skip so the missed workout is offered again.

**Why this priority**: The product promises that missed-workout choices are reversible.

**Independent Test**: Skip a missed workout, undo the skip, and confirm the workout is no longer marked
as skipped and recovery selects it again.

**Acceptance Scenarios**:

1. **Given** a skipped missed workout, **When** the athlete undoes the skip, **Then** the skip mark is removed and the workout is again eligible to be next and to be recovered.
2. **Given** a skipped workout whose sequence item has since been performed through recovery, **When** the athlete tries to undo the skip, **Then** the request is rejected with a stable error and nothing changes.
3. **Given** the same undo request is repeated with the same operation key, **When** it is received again, **Then** the original result is returned.

### Edge Cases

- Another athlete's workout: skipping or undoing it is indistinguishable from a missing workout and changes nothing.
- A stale revision: the request is rejected as a conflict and the current workout can be read again.
- A workout already skipped: skipping again with a new operation key returns the current state without change.
- Several missed workouts: each can be skipped independently; skipping one never affects another.
- An off-plan session on the missed date: participation for that date is unaffected by skipping.
- A program change after a skip: replacing unstarted workouts never removes a skip mark from a historical missed workout.

## Non-Goals *(mandatory)*

- Moving a missed workout to a chosen future date.
- Changing missed, completed, or incomplete outcomes, participation, streaks, records, volume, or history.
- Deleting scheduled workouts.
- Automatic skipping or schedule shifting of any kind.
- App behavior, which the sibling app specification owns.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The API MUST let an authenticated athlete mark one of their own missed scheduled workouts as skipped.
- **FR-002**: Skipping MUST NOT change the workout's missed outcome, its date, its sets, the athlete's participation, streaks, records, volume, or history.
- **FR-003**: Each scheduled workout MUST expose whether and when it was skipped, so every client can exclude it from "next".
- **FR-004**: Recovering a missed workout MUST select the earliest missed workout that is not skipped.
- **FR-005**: The API MUST let the athlete undo a skip, making the workout eligible again, unless its sequence item has since been performed through recovery.
- **FR-006**: Skipping a workout that is not missed MUST be rejected with a stable, documented error.
- **FR-007**: Skip and undo MUST be idempotent per operation key and protected by the workout's revision, consistent with other scheduled-workout mutations.
- **FR-008**: Workouts owned by another athlete MUST be treated as not found.
- **FR-009**: Replacing or regenerating unstarted future workouts MUST preserve skip marks on historical missed workouts.
- **FR-010**: The client-facing contract MUST document the skip and undo operations, the skipped field, error cases, and their interaction with recovery before the app depends on them.
- **FR-011**: Existing clients that ignore the new field MUST keep working unchanged.

### Key Entities

- **Scheduled workout**: Gains a record of whether the athlete skipped it and when. Its outcome remains derived exactly as before.
- **Skip operation**: An athlete's explicit, idempotent decision to stop offering a missed workout as next, reversible by an undo operation.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After a skip, the skipped workout is excluded from recovery selection in 100% of contract tests.
- **SC-002**: Skipping and undoing leave participation, streak, record, and volume results identical to before, verified by comparison tests.
- **SC-003**: Repeated skip or undo requests with the same operation key never change state twice.
- **SC-004**: Existing clients pass their current contract checks without modification.

## Assumptions

- "Missed" keeps its current definition: a finalized scheduled workout with no checked required sets.
- The app derives "next" from the schedule as it does today and will exclude skipped workouts using the new field.
- The change is additive: a nullable skipped timestamp on the scheduled workout plus skip and undo operations.
- The API ships and is deployed before the dependent app behavior.
