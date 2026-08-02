# Feature Specification: Sport Activity Tracking

**Feature Branch**: `codex/sport-activity-tracking`

**Feature ID**: `gympulse-sport-activity-tracking`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/003-sport-activity-tracking/spec.md)

**Created**: 2026-08-02

**Status**: Approved

**Input**: User description: "I want to add sort of a sports model to add like a training or tracking so that doing sports count"

## Clarifications

### Session 2026-08-02

- Q: Should the first release track completed sports, plan sport training, or include sport-specific performance tracking? → A: Track completed sport sessions and count them toward consistency.
- Q: Should weekly consistency count each sport session, duration-based credit, or one active local date? → A: Count once per active local date, regardless of the number or duration of same-day activities.
- Q: What information is required to record a sport activity? → A: Sport and duration are required, date defaults to today, and notes are optional.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Log a completed sport activity (Priority: P1)

An athlete records a sport they completed by choosing a sport, confirming the local date, entering the duration, and optionally adding notes. The record is kept separately from any structured gym workout on the same date.

**Why this priority**: A sport cannot contribute to consistency or history until the athlete can record it as a trustworthy completed activity.

**Independent Test**: Record a 60-minute basketball activity and verify that it is returned with the same sport, local date, duration, and notes for the authenticated athlete.

**Acceptance Scenarios**:

1. **Given** an authenticated athlete, **When** they save a valid sport, local date, and duration, **Then** exactly one completed sport activity is recorded and returned.
2. **Given** an athlete has already completed a gym workout on a date, **When** they record a sport on that date, **Then** both activities remain available without either replacing the other.
3. **Given** a save is retried with the same operation identity, **When** the service receives the retry, **Then** it returns the original activity without creating a duplicate.

---

### User Story 2 - Count sport toward consistency (Priority: P2)

An athlete's completed sport contributes to weekly consistency in the same way as other completed training, while planned workout quality remains a separate fact.

**Why this priority**: The stated product value is that doing a sport should count, without hiding whether a planned workout was completed.

**Independent Test**: Record a sport on an otherwise inactive local date and verify weekly participation increases by one; then add another activity on that date and verify weekly participation does not increase again.

**Acceptance Scenarios**:

1. **Given** a local date with no recorded participation, **When** a sport activity is completed, **Then** that date counts once toward the athlete's weekly goal.
2. **Given** a local date that already counts because of another completed activity, **When** a sport is recorded, **Then** the weekly goal count remains one for that date.
3. **Given** a planned workout was missed on a date, **When** a sport is recorded for that date, **Then** participation is preserved while the planned workout remains missed.

---

### User Story 3 - Review sport history (Priority: P3)

An athlete reviews sport activities alongside training history and can distinguish each activity by sport, date, duration, and notes.

**Why this priority**: A durable, readable history makes the activity useful beyond the moment it is logged.

**Independent Test**: Record two sports on one date and another sport on a different date, then retrieve the date range and verify all three appear in deterministic order with no other athlete's data.

**Acceptance Scenarios**:

1. **Given** an athlete has recorded sport activities, **When** they review a date range, **Then** every owned activity in the range appears in newest-first order with its sport and duration.
2. **Given** another athlete has a sport activity in the same range, **When** the first athlete reviews history, **Then** the other athlete's activity is absent.
3. **Given** an athlete has no sport activities in a range, **When** they review that range, **Then** an empty collection is returned.

### Edge Cases

- A duration below 1 minute or above 1,440 minutes is rejected without recording participation.
- A future local date is rejected; the profile timezone determines what date is considered today.
- Choosing "Other" requires a custom sport name; a blank or whitespace-only name is rejected.
- Sport and custom names are trimmed, bounded, and stored as display snapshots so history does not change when the selection catalog changes.
- Multiple distinct activities may share a date, but a repeated operation identity with a different payload is a conflict.
- Authentication expiry, cancellation, or a server error never reports success and never leaves a partially created activity.
- A sport recorded on a scheduled day does not change, complete, or replace the scheduled workout.

## Non-Goals *(mandatory)*

- GPS routes, live timers, heart-rate data, calories, scores, teams, opponents, or sport-specific performance statistics.
- Device-health, wearable, or third-party activity imports.
- Sport-specific training plans, drills, coaching, or schedule generation.
- Social feeds, public profiles, teams, or leaderboards.
- Changing finalized planned-workout quality or granting more than one weekly participation credit for the same local date.
- Deleting historical sport activities in this first release; correction and deletion behavior requires a follow-up specification that preserves trustworthy history.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST allow an authenticated athlete to record a completed sport activity with a stable sport identifier, display name, local date, and duration in whole minutes.
- **FR-002**: The system MUST offer a small common sport selection and an "Other" choice that accepts a custom display name.
- **FR-003**: The system MUST allow optional notes and MUST preserve a bounded, trimmed value or no value.
- **FR-004**: The system MUST allow multiple sport activities and structured workouts to coexist on the same local date.
- **FR-005**: A valid sport activity MUST count its local date toward weekly participation and weekly goal progress.
- **FR-006**: A local date MUST contribute at most one unit to weekly participation regardless of how many sports or workouts were completed on it.
- **FR-007**: Recording a sport MUST NOT alter the status, performed sets, or history of a planned or structured workout.
- **FR-008**: The system MUST return owned sport activities for a bounded date range in deterministic newest-first order and MUST return an empty collection rather than a missing value.
- **FR-009**: Every create operation MUST be safely retryable: the same operation identity and payload returns the original result, while reuse with a different payload produces a stable conflict.
- **FR-010**: Sport activities MUST be private to the authenticated athlete; missing and foreign activity identifiers MUST be indistinguishable.
- **FR-011**: Activity creation and participation preservation MUST succeed or fail as one observable operation.
- **FR-012**: Invalid dates, durations, sport identifiers, custom names, or notes MUST be rejected without changing activity history or participation.
- **FR-013**: Existing clients and existing workout records MUST continue to behave as before when sport activity support is added.
- **FR-014**: Account deletion MUST remove the athlete's sport activities under the same privacy guarantees as other training data.
- **FR-015**: The first release MUST record completed sport sessions only and MUST NOT create sport schedules, drills, scores, opponents, or sport-specific performance measurements.

### Key Entities

- **Sport activity**: One completed, athlete-owned sport occurrence with identity, local date, stable sport identifier, sport display snapshot, duration, optional notes, operation identity, and creation time.
- **Sport selection**: A stable identifier and display label presented by the client; "Other" adds a bounded custom display name.
- **Day participation**: The durable statement that an athlete participated on a local date; it is shared by sports and workouts but counts a date no more than once.
- **Athlete**: The authenticated owner of every sport activity and participation outcome.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An athlete can record a sport activity with sport, date, and duration in under 30 seconds during a usability check.
- **SC-002**: Every accepted sport activity appears in the athlete's history and weekly participation result immediately after a successful save.
- **SC-003**: Repeating the same save request up to five times produces exactly one sport activity and one participation outcome for the date.
- **SC-004**: Adding multiple activities on one local date changes weekly goal progress by no more than one.
- **SC-005**: Automated ownership checks demonstrate that one athlete cannot list, view, or infer another athlete's sport activities.
- **SC-006**: Existing workout and participation acceptance scenarios continue to pass without changed outcomes.

## Assumptions

- The first release records completed sport sessions only; planning sport practice and sport-specific performance tracking are intentionally deferred.
- The initial sport selection includes common choices and "Other"; the stored display snapshot keeps old records readable as the selection evolves.
- The athlete's saved training-profile timezone governs date validation and participation.
- Existing authentication, account ownership, and participation concepts are reused.
