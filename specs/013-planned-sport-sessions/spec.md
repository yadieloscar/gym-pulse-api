# Feature Specification: Planned Sport Sessions

**Feature Branch**: `spec/013-planned-sport-sessions`

**Feature ID**: `gympulse-planned-sport-sessions`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/016-planned-sport-sessions/spec.md)

**Created**: 2026-10-25

**Status**: Approved

**Input**: User description: "I want to do cycling plus volleyball. How can I make this app support my training programs?" Follow-up: "Create the cross-repo spec for planned sport sessions in programs (API spec first, then app spec)."

## Clarifications

### Session 2026-10-25

Decisions from the workspace review with the product owner. They extend, and in one place
supersede, the sport-activity-tracking feature (`gympulse-sport-activity-tracking`), whose first
release deliberately excluded sport planning.

- Q: Where does a planned sport belong: inside a program next to gym workouts, or as a separate
  sport calendar? → A: Inside the program. A program workout is either an exercise workout or a
  sport session, and both follow the same sequence, weekday preference, schedule generation, missed
  handling, and weekly counting. One plan describes the whole week.
- Q: What does finishing a planned sport session mean? → A: Recording a completed sport activity
  for it. The planned session and the recorded activity stay linked, so the plan says what was
  intended and the activity says what was done.
- Q: Does a sport activity imported from the health store fulfil a planned sport session? → A: Yes,
  when it is the same sport on the same local date and the session is still open. The link is made
  once, is visible, and never changes the activity's own facts. Recorded as an assumption the owner
  can override.
- Q: Do starter programs gain sport sessions in this release? → A: No. Sport sessions are added by
  the athlete to a custom or copied program.
- Q: What does a planned sport session count toward? → A: Weekly planned-versus-done counts and
  date participation, like any planned workout. Lifted volume, strength records, and exercise
  distribution are untouched, exactly as for sport activities today.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Plan a sport session inside a program (Priority: P1)

An athlete adds a cycling ride and a volleyball practice to their program alongside their gym
workouts. Each sport session names the sport, a planned duration, an optional planned distance, an
optional note, a preferred weekday, and a position in the program's sequence. The program is saved
and returned with gym workouts and sport sessions in one ordered list.

**Why this priority**: Nothing else in the feature is possible until a plan can describe a sport
session as a first-class planned unit, and this story alone lets an athlete see their whole week in
one program.

**Independent Test**: Create a program with two exercise workouts and two sport sessions (cycling 90
minutes with a 40 km target, volleyball 120 minutes), read it back, and verify every unit keeps its
kind, order, weekday preference, sport, and targets unchanged, visible only to its owner.

**Acceptance Scenarios**:

1. **Given** an authenticated athlete creating or replacing a program, **When** the program
   contains sport sessions and exercise workouts, **Then** the saved program returns both kinds in
   the submitted sequence with their targets intact.
2. **Given** a sport session with a sport the athlete typed as "Other", **When** it is saved,
   **Then** the custom display name is kept as a snapshot and returned unchanged.
3. **Given** a sport session with no planned duration, an out-of-range duration, or a distance
   without a sport that can have one, **When** the program is saved, **Then** the request is rejected
   with a field-level reason and the previously saved program is unchanged.
4. **Given** an existing program with no sport sessions, **When** the athlete reads or replaces it
   as before, **Then** nothing about its behavior or shape changes for existing clients.

---

### User Story 2 - See planned sport sessions on the schedule (Priority: P1)

When the athlete's week is generated from the program, planned sport sessions appear on the schedule
with the same dates, sequence, and statuses as planned workouts. A session is planned until the
athlete records it, becomes missed when its date passes unrecorded, and can be skipped or done later
exactly like a missed workout.

**Why this priority**: The product promise is one source of truth per day. A sport in the program
that never reaches the schedule would leave Today, the calendar, and weekly progress disagreeing.

**Independent Test**: Materialize a week for a program whose sequence is gym, ride, gym, volleyball;
verify four scheduled units with correct dates and kinds; let the ride's date pass and verify it is
missed and unresolved; skip it and verify the skip is recorded and undoable without changing the
other three.

**Acceptance Scenarios**:

1. **Given** a program containing sport sessions, **When** a date range is materialized, **Then**
   each sport session yields one scheduled sport session carrying the sport, planned duration,
   optional planned distance, note, and a snapshot of the program unit it came from.
2. **Given** a scheduled sport session whose local date has passed without a recorded activity,
   **When** the schedule is read, **Then** the session is missed and unresolved.
3. **Given** a missed sport session, **When** the athlete skips it, **Then** the skip is recorded,
   undoable, and does not change participation, other scheduled units, or history.
4. **Given** a program is regenerated for future dates, **When** the preview and apply run,
   **Then** unstarted future sport sessions are replaced together with unstarted future workouts and
   recorded sessions are retained.
5. **Given** a scheduled sport session, **When** a client that predates this feature reads the
   schedule, **Then** it can still read every exercise workout without error.

---

### User Story 3 - Record a planned sport session as done (Priority: P1)

The athlete did the ride. They record it from the plan: the sport is already chosen, the planned
duration is prefilled, and they confirm what actually happened (duration, optional distance, notes).
One sport activity is created, linked to the scheduled session, and the session becomes completed.
Weekly progress counts the day once and counts the session as done.

**Why this priority**: Recording is the daily loop. Without a one-step finish the plan is a list
the athlete has to reconcile by hand, which the product forbids.

**Independent Test**: Record a 95-minute activity against a planned 90-minute ride; verify one
activity exists with the actual values, the session is completed and references it, the week's
done count rose by one, and repeating the request with the same operation identity creates nothing
new.

**Acceptance Scenarios**:

1. **Given** a planned or missed sport session, **When** the athlete records a sport activity
   against it, **Then** exactly one activity is created, the session becomes completed and
   references the activity, and the activity references the session.
2. **Given** a session is already completed, **When** another activity is recorded against it,
   **Then** the request is refused with a stable conflict that returns the current session, and no
   activity is created.
3. **Given** a missed session recorded on a later date, **When** the activity is created with
   today's date, **Then** the session is completed with that later date as its performed date, its
   scheduled date is unchanged, and participation is credited to the performed date.
4. **Given** the actual sport differs from the planned sport, **When** the athlete records against
   the session, **Then** the request is rejected; the athlete may instead record an unlinked sport
   activity as today.
5. **Given** a recorded activity, **When** the athlete reads the activity, **Then** it shows which
   planned session it fulfilled, if any.

---

### User Story 4 - Let an imported workout fulfil the plan (Priority: P2)

The athlete's watch recorded the ride. When the phone imports it, GymPulse notices an open planned
session for the same sport on the same local date and links the two, so the plan shows done without
any typing.

**Why this priority**: Connecting a health app exists so that doing sport counts without
administrative logging; a plan that ignores the import would reintroduce that work.

**Independent Test**: Plan a cycling session for a date, import a cycling workout for that date, and
verify the session is completed and linked. Import a second cycling workout for the same date and
verify it is kept as an unlinked activity and the session is unchanged.

**Acceptance Scenarios**:

1. **Given** one open planned session for a sport on a date, **When** an activity with that sport
   and date is imported, **Then** the session is completed by that activity and both are linked.
2. **Given** no open planned session matches, **When** an activity is imported, **Then** it is
   kept as an ordinary unlinked sport activity, exactly as today.
3. **Given** two open sessions for the same sport on the same date, **When** one activity is
   imported, **Then** only the earliest in sequence is fulfilled.
4. **Given** an imported activity already fulfilled a session, **When** the same import is
   submitted again, **Then** nothing changes and no second link is made.

---

### User Story 5 - Review planned versus done (Priority: P3)

The athlete reviews a week and sees, for each day, what sport was planned, whether it was done,
missed, or skipped, and what was actually recorded. Weekly progress reports planned-versus-done
including sport sessions. Account deletion removes every planned and scheduled sport session.

**Why this priority**: Review makes planning trustworthy and closes the loop, but it depends on the
earlier stories.

**Independent Test**: Over one week plan two sport sessions, complete one, miss and skip the other;
verify the schedule, participation, weekly summary, and activity list all describe the same facts.

**Acceptance Scenarios**:

1. **Given** sport sessions in a range, **When** the schedule and participation are read, **Then**
   every surface reports the same status per date.
2. **Given** a completed sport session, **When** records, lifted volume, and exercise distribution
   are read, **Then** they are unchanged by it.
3. **Given** account deletion, **When** it completes, **Then** no planned, scheduled, or linked
   sport session data remains for that athlete.

### Edge Cases

- A sport session's planned duration is a whole number of minutes from 1 through 1,440; distance,
  when given, is 1 through 1,000,000 metres and only for sports that are distance-based (cycling,
  running, swimming, and "Other"). Volleyball and other court or team sports cannot carry a distance.
- A sport session has no sets, exercises, rest timer, or in-progress state; it is planned, completed,
  missed, or skipped. There is nothing to resume.
- Recording against a session on a future local date is rejected; the performed date can never be
  later than today in the athlete's profile timezone.
- Replacing a program never changes already materialized sessions, including their sport snapshot.
- Deleting or renaming the program unit behind a scheduled session leaves the session intact with
  its snapshot; provenance identifiers become absent, never dangling.
- A sport activity linked to a session cannot be re-linked to another session.
- Recording, linking, and participation preservation succeed or fail as one observable operation.
- An authentication expiry, cancellation, or server error never reports success and never leaves a
  session half-completed.
- Weekly participation still counts a local date at most once however many sessions and activities
  share it.

## Non-Goals *(mandatory)*

- Sport-specific structure inside a session: intervals, power or heart-rate zones, drills, routes,
  scores, opponents, teams, or coaching guidance.
- Starter-program catalog changes; starter programs remain exercise-only in this release.
- Editing or deleting sport activities, which still requires a history-preserving follow-up.
- Automatic schedule shifting, reminders beyond the existing planned-day reminder, or any new
  nudges.
- Counting sport sessions in lifted volume, strength records, or exercise distribution.
- Writing planned sessions to the device health store; health writeback remains limited to finished
  gym workouts.
- Removing or changing any existing route, field, or status for exercise workouts.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A program unit MUST be one of two kinds, exercise workout or sport session, and a
  program MUST hold them in one ordered sequence with preferred weekdays.
- **FR-002**: A sport session MUST carry a stable sport identifier, a sport display snapshot, a
  planned whole-minute duration, an optional planned distance, and an optional bounded note.
- **FR-003**: Existing exercise workouts and programs MUST keep their current shape and behavior;
  a program without sport sessions MUST be indistinguishable from today for existing clients.
- **FR-004**: Schedule generation, regeneration, missed detection, skip, undo skip, and the
  366-date range rule MUST treat sport sessions like exercise workouts, with status values planned,
  completed, missed, and the existing resolutions.
- **FR-005**: A scheduled sport session MUST snapshot its sport, targets, and note at generation
  time and MUST keep provenance to the program unit as optional, non-dangling identifiers.
- **FR-006**: An athlete MUST be able to record a completed sport activity against an owned open
  (planned or missed) scheduled sport session in one operation that creates the activity, completes
  the session, links both directions, and preserves participation for the performed date.
- **FR-007**: Recording against a session MUST require the same sport as planned, a performed date
  no later than the athlete's local today, and the same duration, distance, measurement, and note
  rules as a sport activity.
- **FR-008**: Recording against a completed session MUST fail with a stable conflict that returns
  the current session, and a linked activity MUST never be re-linked.
- **FR-009**: A sport activity MUST expose which scheduled session it fulfilled, if any, and a
  scheduled sport session MUST expose the activity that completed it, if any.
- **FR-010**: Importing a sport activity MUST fulfil at most one open scheduled session for the
  same sport on the same local date, choosing the earliest in sequence, and MUST leave the activity
  unlinked when none matches; replaying the import MUST change nothing.
- **FR-011**: Weekly planned-versus-done and date participation MUST include sport sessions;
  lifted volume, records, and exercise distribution MUST exclude them.
- **FR-012**: Every mutation MUST be safely retryable under the existing operation-identity and
  revision rules; a replay returns the original result and a changed payload is a stable conflict.
- **FR-013**: All planned, scheduled, and linked sport data MUST be private to the owning athlete;
  missing and foreign identifiers MUST be indistinguishable.
- **FR-014**: Invalid kinds, sports, durations, distances, dates, or notes MUST be rejected with a
  field-level reason without changing any stored program, schedule, or activity.
- **FR-015**: Account deletion MUST remove planned, scheduled, and linked sport session data under
  the same guarantees as other training data.
- **FR-016**: The public contract document MUST describe every new field, status, validation rule,
  response, and error in the same change, and the change MUST be additive.

### Key Entities

- **Program unit**: One ordered entry in a program. Kind exercise workout (existing) or sport
  session (new). Shares identity, name, preferred weekday, and sequence position.
- **Sport session (planned)**: A program unit with a stable sport identifier, sport display
  snapshot, planned duration, optional planned distance, and optional note.
- **Scheduled sport session**: The dated copy generated from a sport session, with status,
  resolution, revision, snapshots, optional provenance, and an optional link to the activity that
  completed it and the date it was performed.
- **Sport activity**: Existing completed-sport record, now with an optional reference to the
  scheduled session it fulfilled.
- **Day participation**: Unchanged; still counts a local date at most once.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An athlete can add a sport session to a program and see it on the next generated
  week without any step other than saving the program.
- **SC-002**: Recording a planned sport session as done is one operation; repeating it up to five
  times yields exactly one activity, one completed session, and at most one participation credit.
- **SC-003**: An imported health workout that matches an open planned session completes it with
  zero athlete actions, and a non-matching import behaves exactly as before.
- **SC-004**: Every existing program, schedule, sport-activity, participation, record, and volume
  acceptance scenario continues to pass with unchanged outcomes.
- **SC-005**: Automated ownership checks show one athlete cannot read, record against, or infer
  another athlete's sport sessions.
- **SC-006**: Schedule, participation, and weekly summary agree on the status of every sport
  session date in a 366-date range.

## Assumptions

- Sport sessions reuse the existing sport selection (basketball, soccer, tennis, swimming, running,
  cycling, volleyball, martial arts, climbing, other); distance applies to cycling, running,
  swimming, and other.
- An open planned session is fulfilled by an imported activity with the same sport and local date;
  the owner may change this to an explicit confirmation in the app spec, in which case the API link
  operation from US3 is used instead and FR-010 is dropped.
- The athlete's saved training-profile timezone governs dates, as everywhere else.
- Existing idempotency, revision, ownership, participation, missed, and skip concepts are reused
  rather than duplicated.
- This feature supersedes the "no sport training plans" non-goal recorded in
  `gympulse-sport-activity-tracking`; that spec's recorded behavior otherwise stands.
- The app may ship after the API; until it does, sport sessions are simply never created.
