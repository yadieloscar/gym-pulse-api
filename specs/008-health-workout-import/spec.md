# Feature Specification: Imported health workouts

**Feature Branch**: `spec/008-health-workout-import`

**Feature ID**: `gympulse-health-workout-import`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/008-health-workout-import/spec.md)

**Created**: 2026-10-12

**Status**: Approved

**Input**: User description: "Import workouts from Apple Health (HealthKit) into sport activities. API: additive migration (source, external_id, optional distance/calories/heart-rate) and contract changes."

## Clarifications

### Session 2026-10-12

- Q: Is an imported workout a new kind of record or a sport activity? → A: It is a sport activity with a recorded origin. It counts toward consistency exactly like a manually logged sport and never changes a planned workout's result.
- Q: Should strength-training workouts recorded by a watch be imported? → A: No in this release. GymPulse is the record of gym work; only activity types that map to a sport are imported. Recorded as an assumption the user can override.
- Q: Can an athlete remove an imported activity? → A: Not in this release. Deletion of sport activities, imported or manual, remains deferred to a history-preserving follow-up specification.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Keep an imported workout exactly once (Priority: P1)

An athlete's phone sends a workout it found in the device health store. GymPulse keeps it as a completed sport activity that remembers where it came from, and sending the same workout again never produces a second copy.

**Why this priority**: Sync runs repeatedly and across reinstalls. Without a durable origin identity the athlete's history fills with duplicates and consistency becomes untrustworthy.

**Independent Test**: Submit the same imported workout five times, with and without the same retry identity, and verify exactly one sport activity exists for it and weekly participation changed by at most one for its date.

**Acceptance Scenarios**:

1. **Given** an authenticated athlete with no activity for a health workout identity, **When** the app submits that workout with its origin and identity, **Then** one sport activity is created with the origin recorded and returned.
2. **Given** the activity already exists for that origin and identity, **When** the app submits the same workout again under a new retry identity, **Then** the existing activity is returned unchanged and no second activity is created.
3. **Given** the activity already exists, **When** the app submits the same identity with different details, **Then** the existing activity is returned unchanged and the response makes clear nothing new was recorded.
4. **Given** two different athletes import workouts that happen to share an identity, **When** both submit, **Then** each athlete receives their own activity and neither can see the other's.

---

### User Story 2 - Count imported sport toward consistency (Priority: P2)

An imported run, ride, swim, or game counts its local date toward the athlete's weekly goal in the same way as a sport the athlete typed in, including when the workout happened on an earlier day in the week.

**Why this priority**: The product value of connecting a health app is that doing sport counts without administrative logging.

**Independent Test**: Import a workout for a date earlier this week with no other participation and verify weekly participation increases by one; import a second workout on the same date and verify it does not increase again.

**Acceptance Scenarios**:

1. **Given** a past local date in the athlete's timezone with no participation, **When** a workout for that date is imported, **Then** the date counts once toward the weekly goal.
2. **Given** a date that already counts because of a finished workout or another sport, **When** a workout is imported for it, **Then** the weekly count for that date remains one.
3. **Given** a planned workout was missed on a date, **When** a workout is imported for that date, **Then** participation is preserved and the planned workout remains missed.

---

### User Story 3 - Review imported activities with their facts (Priority: P3)

An athlete reviews history and sees each imported activity with its sport, date, duration, and, when the health store supplied them, distance, energy, and average heart rate. The origin is visible so the athlete knows which entries came from the health app.

**Why this priority**: Imported data is only useful if it reads clearly alongside manual entries and does not break clients that predate this feature.

**Independent Test**: Import one workout with distance and heart rate and one without, then read a date range and verify both appear newest-first, each with its origin, and the optional facts are present only where supplied.

**Acceptance Scenarios**:

1. **Given** imported and manual activities in a range, **When** the athlete reads that range, **Then** every activity carries an origin value and imported ones carry their identity and any supplied measurements.
2. **Given** an activity created before this feature, **When** it is read, **Then** it reports a manual origin and no measurements, and its other fields are unchanged.
3. **Given** a client that predates this feature, **When** it reads or creates sport activities, **Then** it continues to work without supplying or understanding the new fields.

### Edge Cases

- A workout whose local date, in the athlete's saved training-profile timezone, is in the future is rejected without recording anything.
- A workout shorter than one minute or longer than 1,440 minutes is rejected.
- Distance, energy, or heart-rate values that are negative or implausibly large are rejected; absent values are stored as absent, never zero.
- An origin value other than the supported set, or an identity without an origin, is rejected.
- A manual activity never carries an identity; an imported activity always does.
- Concurrent submissions of the same identity by the same athlete yield one activity and one participation outcome.
- Authentication expiry, cancellation, or a server error never reports success and never leaves a partial activity or participation.
- Deleting the workout in the health app does not change GymPulse history in this release.
- Account deletion removes imported activities together with all other training data.

## Non-Goals *(mandatory)*

- Reading, storing, or forwarding raw health samples, routes, GPS, heart-rate series, or any health data beyond the per-workout summary fields named here.
- Writing GymPulse workouts back to the device health store.
- Importing strength or functional-training workouts; GymPulse remains the record of gym work.
- Server-side connection to Apple, Google, or any third-party health service; the device is the only reader of the health store.
- Editing or deleting sport activities, imported or manual.
- Changing weekly participation rules, planned-workout outcomes, workout counts, distribution, volume, or records.
- Android Health Connect behavior; the origin field is designed to admit it in a later specification.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Every sport activity MUST record an origin. Existing and newly typed activities report a manual origin; imported activities report the health-app origin they came from.
- **FR-002**: An imported activity MUST carry the stable identity the health store assigned to the workout, bounded in length and unique per athlete and origin.
- **FR-003**: Submitting an imported workout whose athlete, origin, and identity already exist MUST return the existing activity without creating, changing, or re-crediting anything, regardless of retry identity.
- **FR-004**: An imported activity MAY carry distance in whole meters, active energy in whole kilocalories, and average heart rate in whole beats per minute. Each is optional, validated for plausibility, and stored only when supplied.
- **FR-005**: Imported activities MUST reuse the sport identifier and display-name rules of manual activities. Sports the catalog does not know use the "Other" identifier with the health app's display name.
- **FR-006**: An imported activity's local date MUST be derived in the athlete's saved training-profile timezone and MUST NOT be in the future; past dates are accepted.
- **FR-007**: A valid imported activity MUST count its date toward weekly participation at most once, atomically with its creation, and MUST NOT change a planned workout's result.
- **FR-008**: Imported activities MUST appear in the existing date-range history and single-activity reads, ordered with manual activities, with origin, identity, and measurements included.
- **FR-009**: The additions MUST be additive: clients that do not send or read the new fields MUST keep working, and existing rows need no backfill beyond the manual default.
- **FR-010**: Imported activities MUST be private to the athlete; missing and foreign identifiers MUST remain indistinguishable.
- **FR-011**: Invalid origin, identity, date, duration, sport, or measurement values MUST be rejected with the stable validation error and no side effect.
- **FR-012**: Account deletion MUST remove imported activities under the same guarantees as other training data.

### Key Entities

- **Sport activity**: Extended with an origin, an optional external identity, and optional distance, energy, and average heart rate.
- **Activity origin**: Where an activity came from. Manual in this release plus one health-app origin; designed to accept further origins later.
- **External identity**: The health store's stable identifier for one workout, unique per athlete and origin, used to make imports idempotent across retries and reinstalls.
- **Day participation**: Unchanged; shared by workouts, manual sports, and imported sports, counting a date once.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Submitting the same health workout any number of times yields exactly one activity and at most one participation credit for its date.
- **SC-002**: A 30-day first import of up to 60 workouts completes with every workout stored and visible in history.
- **SC-003**: Automated ownership checks show one athlete cannot read, duplicate, or infer another athlete's imported activities.
- **SC-004**: Every existing sport-activity, participation, and workout acceptance scenario passes unchanged after the additions.
- **SC-005**: A client built before this feature can list and create sport activities with the same results as before.

## Assumptions

- The device app performs the health-store read, type mapping, and submission; the service stores and validates. No server credential or outbound call to a health provider exists.
- Only one health-app origin ships in this release. Its name is fixed in the contract and a second origin is additive later.
- Strength and functional-training workout types are not imported. If the user prefers to import them as a distinct sport, the app specification's mapping changes and this specification is unaffected.
- The existing idempotency mechanism continues to protect retries; the origin plus identity uniqueness protects re-imports across sessions and devices.
- Measurements are display facts only. They do not feed records, volume, distribution, or any insight in this release.
- Workout timestamps are converted to the athlete's saved training-profile timezone, matching manual sport activities.
