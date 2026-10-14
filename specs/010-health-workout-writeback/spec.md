# Feature Specification: Remember a workout's health export

**Feature Branch**: `spec/010-health-workout-writeback`

**Feature ID**: `gympulse-health-workout-writeback`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/009-health-workout-writeback/spec.md)

**Created**: 2026-10-13

**Status**: Approved

**Input**: User description: "If we start a workout in our app, do we have to do something else so that it's tied with Health? Spec the write-back."

## Clarifications

### Session 2026-10-13

- Q: Does the service talk to Apple Health? → A: No. The device writes the workout to the health store and tells the service the identity it received. The service only remembers that identity so the export never happens twice.
- Q: Which workouts are exported? → A: Finished workout sessions only, planned and off-plan alike. Discarded, in-progress, and legacy day logs are not exported. Sport activities are never exported because they are either typed in or already came from Health.
- Q: What is a "finished workout session" in GymPulse's data? → A: A finalized planned workout with a completed or incomplete outcome, or an off-plan log. A missed outcome and a rest log never export. Exports are read for a date range rather than embedded in session reads, because the device addresses planned workouts and off-plan logs by their own ids. (Recorded during planning, 2026-10-14.)
- Q: Is the export reversible? → A: The record of the export can be cleared by the athlete deleting the workout in Health and GymPulse detecting it later; that is a follow-up. In this release the identity is written once and only read.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Remember the export once (Priority: P1)

An athlete finishes a gym workout in GymPulse with "Save gym workouts to Apple Health" turned on. The phone writes the workout to Health and tells GymPulse the identity Health assigned. GymPulse keeps that identity on the workout so a reinstall, a second device, or a retry never writes the same workout to Health again.

**Why this priority**: Without a durable record, the device cannot know whether a finished workout already exists in Health, and duplicates in Health are the one failure athletes notice.

**Independent Test**: Record an export identity for a finished session, read the session back and see it, then record it again under a new retry identity and confirm the identity is unchanged and nothing else moved.

**Acceptance Scenarios**:

1. **Given** a finished, owned workout session with no export identity, **When** the device records one, **Then** the session reports that identity and the time it was recorded.
2. **Given** a session that already has an export identity, **When** the device records the same identity again, **Then** the session is returned unchanged.
3. **Given** a session that already has an export identity, **When** the device tries to record a different identity, **Then** the request is rejected with a stable conflict and the stored identity does not change.
4. **Given** a session that is not finished, **When** the device tries to record an export, **Then** the request is rejected and nothing is stored.
5. **Given** another athlete's session, **When** the device records an export for it, **Then** the response is the same not-found answer as for a missing session.

---

### User Story 2 - Read which workouts are already in Health (Priority: P2)

The device asks for the athlete's recent finished sessions and can tell which ones already have an export identity, so it exports only the ones that are missing, including sessions finished while export was off if the athlete chooses to catch up.

**Why this priority**: The device must be able to rebuild its view after reinstall without a local ledger.

**Independent Test**: Finish two sessions, record an export for one, list sessions in the range, and confirm one carries an identity and the other does not.

**Acceptance Scenarios**:

1. **Given** sessions with and without export identities, **When** the athlete's sessions are read, **Then** each carries its export identity and recorded time, or nothing.
2. **Given** a client that predates this feature, **When** it reads sessions, **Then** it keeps working and ignores the new fields.

### Edge Cases

- An export identity that is blank, too long, or not a plausible health identifier is rejected.
- Recording an export never changes the session's performed sets, status, revision for performance edits, participation, records, or volume.
- Concurrent record attempts for one session yield one stored identity; the loser receives the conflict or the unchanged replay.
- Account deletion removes export identities with the session.

## Non-Goals *(mandatory)*

- Any server-side connection to Apple, Google, or a health provider.
- Exporting sport activities, legacy day logs, body weight, or anything other than finished workout sessions.
- Detecting that the athlete deleted the workout in Health, or clearing an export identity.
- Exporting sets, reps, weights, heart rate, or energy; the device writes only type, start, end, and a GymPulse reference.
- Android Health Connect.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A finished workout session MAY carry one health export identity and the time it was recorded; both are absent until the device records them.
- **FR-002**: The athlete's device MUST be able to record the export identity for an owned, finished session exactly once; the same identity replays unchanged, a different identity is a stable conflict.
- **FR-003**: Recording an export for a session that is not finished, is missing, or belongs to another athlete MUST fail with the documented not-found or validation behavior and store nothing.
- **FR-004**: The identity MUST be trimmed and bounded (1–128 characters).
- **FR-005**: The athlete's device MUST be able to read its export identities for a date range, each with its workout's date; the additions MUST be additive for older clients.
- **FR-006**: Recording an export MUST NOT change performance data, participation, records, volume, or planned-workout outcomes.
- **FR-007**: Account deletion MUST remove export identities with the rest of the athlete's data.

### Key Entities

- **Finished workout**: A finalized planned workout (completed or incomplete) or an off-plan log; the thing an export refers to.
- **Health export**: The health store's identifier for one exported finished workout, with the time it was recorded; write-once per workout and origin.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Recording the same export any number of times leaves exactly one identity on the session.
- **SC-002**: A client built before this feature reads and mutates sessions exactly as before.
- **SC-003**: Ownership checks show one athlete cannot read or set another athlete's export identity.
- **SC-004**: Existing session, participation, and stats acceptance scenarios pass unchanged.

## Assumptions

- The device performs the Health write and conflict check; the service is a durable, private ledger.
- Finished means the session's documented completed state; the plan maps this to the current status field.
- The companion import feature (`gympulse-health-workout-import`) is deployed first; its skip list keeps exported strength workouts from being imported back.
