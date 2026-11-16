# Feature Specification: One Active Program on Fixed Weekdays

**Feature Branch**: `spec/031-one-active-program`

**Feature ID**: `gympulse-one-active-program`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/037-one-active-program/spec.md)

**Amends**: the program, schedule, and plan-transition sections of `docs/CONTRACTS.md`, and
[Planned sport sessions](../013-planned-sport-sessions/spec.md) where it describes how a program is
placed on dates. See "Changes to approved specs and contracts" below.

**Created**: 2026-11-16 · **Revised**: 2026-11-16 (planning: placement of existing programs)

**Status**: Approved (owner answered D1 to D6 and accepted every drafting default on 2026-11-16;
see Clarifications)

**Input**: Owner direction after an audit of the plan and program workflow: "The plan and programs
logic are not making sense." "I also would like to be able to change the days for each workout, as
well like create my custom program, like what if I want to create a custom pull push leg program
and add a bike ride and also volleyball?"

## Audit findings this spec resolves

Verified in code on `main` on 2026-11-16:

| # | Finding | Evidence |
| --- | --- | --- |
| 1 | The weekday stored on a program workout is ignored. Workouts are dealt out in sequence across the profile's available days. | `internal/model/training_materialization.go` |
| 2 | A profile change without a named program replaces the active program with the top recommended starter, even when the athlete follows a custom program. | `internal/service/plan_transition_svc.go` |
| 3 | Creating a program always makes it active and deactivates the previous one. | `internal/service/training_svc.go`, `internal/dao/program_dao.go` |
| 4 | Three operations place a program on dates (plan-transition apply, regenerate, materialize) with three different rules for kept workouts and active sessions. | `internal/dao/plan_transition_dao.go`, `internal/dao/schedule_dao.go` |
| 5 | Every change restarts the sequence at the first workout, and nothing extends the schedule past the range filled at the last change. | same files |
| 6 | Two planned sessions of the same program workout can land on one date. | no uniqueness rule; materialize never deletes |

## Clarifications

### Session 2026-11-16 (owner answers)

- **D1** Q: How is an athlete's training organized? → A: One active program at a time. "Plan" is
  only the app's view of the current week of that program.
- **D2** Q: How do days work? → A: Each workout in the program has its own fixed weekday. The week
  repeats as laid out. The athlete can also move one specific date as a one-off.
- **D3** Q: What does editing goal or available days do? → A: It keeps the active program. A
  different program is only ever suggested, never applied.
- **D4** Q: What does saving a program do? → A: It saves it. A program becomes active only through
  one explicit start with a preview, the same for every entry point.
- **D5** Q: What happens over time? → A: The schedule rolls forward with no end date until the
  athlete switches. Editing the program keeps future dates in step instead of restarting.
- **D6** Q: What must a custom program allow? → A: The athlete's own workouts (for example push,
  pull, legs) and sport sessions (for example a bike ride and volleyball) in one week, each on the
  weekday the athlete picks.

### Session 2026-11-16 (drafting defaults)

- Q: Can one workout sit on more than one weekday? → A: Yes. Starters such as the two-workout
  foundation programs and push/pull/legs on six days already repeat a workout inside a week. A
  program unit therefore has one or more weekdays.
- Q: Does a week still alternate (A-B-A, then B-A-B)? → A: No. The week repeats exactly as laid
  out. An athlete who wants a different second week edits the program.
- Q: What does the athlete's `available_days` mean now? → A: It is a preference. It decides the
  first placement of a program that has no weekdays yet and feeds recommendations. After that the
  program's own weekdays decide the schedule.
- Q: How does a released app keep working? → A: Every change is additive. A request that carries no
  weekdays is placed by the server exactly once, in sequence across the available days, as today.

### Session 2026-11-16 (planning)

- Q: How do existing programs get weekdays? → A: Not in the migration. They are placed by the same
  one-time rule as any unplaced program, the first time a session would be created for them
  (research R5). FR-001 and FR-021 are amended to match.
- Q: How does the app detect support? → A: Every program unit carries `weekdays` as an array
  (research R10).

### Session 2026-11-16 (owner accepted the defaults)

- Q: Can two sessions share a day? → A: Yes (FR-004). Lifting and volleyball can share a Saturday.
- Q: After a long break, are unfilled past dates missed? → A: No. They stay empty (User Story 4,
  scenario 4).
- Q: Is the legacy weekly plan removed here? → A: No. It stays until a separate cleanup
  (Non-Goals).

## Changes to approved specs and contracts

| Source | Statement | Change |
| --- | --- | --- |
| CONTRACTS, plan-transition preview | "When no program/starter is supplied, the highest-ranked goal-matching starter is selected." | Applies only when the athlete has no active program. Otherwise the active program is the target and stays active. |
| CONTRACTS, plan-transition preview | "Materialization uses the exact ISO weekdays in `proposed_profile.available_days`." | Applies only to a target that has no weekdays yet (first placement). A placed program uses its own weekdays. |
| CONTRACTS, plan-transition apply | "replaces only unstarted workouts in the requested range" | Also removes unstarted planned sessions of any other program on or after `from`, so two programs never share the future. |
| CONTRACTS, `POST /programs` | Creates the program active. | Unchanged by default. A new optional `active: false` saves without starting. |
| CONTRACTS, `PUT /programs/{id}` | "Replacing the program never changes already materialized scheduled workouts." | Narrowed: saving the **active** program updates its unstarted future sessions in the same operation. Started, finished, and past sessions never change. |
| CONTRACTS, schedule | A schedule exists only for ranges an operation filled. | The active program's sessions exist for every future date the athlete can read (FR-012). |
| API spec 013 | "Materialize, regenerate, plan transitions, skip, and unskip treat sport sessions exactly like workouts." | Unchanged. Sport sessions gain weekdays and one-off moves exactly as workouts do. |
| API spec 017 | Switching a stage's program uses the plan-transition contract. | Unchanged. That contract is the single start operation defined here. |

Skip, unskip, recover, complete, set logging, participation, streaks, records, and delete keep
their contracts.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A program is a week with fixed weekdays (Priority: P1)

The athlete's program lists workouts and sport sessions, each on the weekdays the athlete chose.
The schedule follows that week exactly: push on Monday is always on Monday.

**Why this priority**: Every other rule depends on the program, not the profile, deciding the days.
It is the owner's first complaint and first request.

**Independent Test**: Save a program with Push on Monday, Pull on Wednesday, Legs on Friday, a
cycling session on Thursday, and volleyball on Saturday; start it; read four weeks of schedule and
verify each unit appears only on its weekdays, every week.

**Acceptance Scenarios**:

1. **Given** a program whose units carry weekdays, **When** it is started, **Then** each unit is
   scheduled on each of its weekdays and on no other day, regardless of `available_days`.
2. **Given** a unit with two weekdays, **When** the schedule is read, **Then** it appears on both
   every week.
3. **Given** a workout and a sport session on the same weekday, **When** the schedule is read,
   **Then** both appear on that date.
4. **Given** a program saved without weekdays (a released app or a starter), **When** it is first
   placed, **Then** the server deals its units in sequence across the athlete's available days once,
   stores the result as the program's weekdays, and returns them.
5. **Given** a unit with a weekday outside 1 to 7, a repeated weekday, or an empty weekday list,
   **When** it is saved, **Then** the API returns 422 `VALIDATION_ERROR` with
   `details.field = "workouts"`.

---

### User Story 2 - Saving never starts; one explicit start (Priority: P1)

The athlete saves a program (built by hand, generated, or copied from a starter or a multi-phase
series) and nothing about their current training changes. They start it through one operation that
previews the first week, then applies it.

**Why this priority**: Today the active program changes before the confirmation that promises
"nothing changes until you start it".

**Independent Test**: With program P active, save program Q with `active: false`; verify P is still
active and the schedule is unchanged. Preview starting Q; verify nothing changed. Apply; verify Q is
the only active program, P's unstarted future sessions are gone, and P's finished sessions remain.

**Acceptance Scenarios**:

1. **Given** an active program, **When** another program is created with `active: false`, **Then**
   it is saved inactive, the active program and the schedule are unchanged.
2. **Given** a saved program, **When** its start is previewed, **Then** the response shows the
   target program, its weekdays, and the dated sessions, and nothing is written.
3. **Given** a preview, **When** it is applied, **Then** in one transaction the target becomes the
   only active program and every unstarted planned session of any other program on or after the
   start date is replaced by the target's sessions.
4. **Given** a session that is in progress, finished, or a resolved missed workout, **When** a
   program is started, **Then** that session is unchanged.
5. **Given** the start request omits `proposed_profile`, **When** it is applied, **Then** the
   training profile is unchanged.
6. **Given** a request that creates a program without `active`, **When** it is saved, **Then** it is
   created active as today (released-app behavior).

---

### User Story 3 - Profile changes keep the program (Priority: P1)

The athlete changes their goal or available days. Their active program stays. The response tells
the app which sessions now sit on days the athlete said they do not train, and which starter the
new goal would recommend, as information only.

**Why this priority**: Silent replacement of a custom program is the most damaging current
behavior: the athlete loses the program they built.

**Independent Test**: With a custom program active on Monday, Wednesday, Friday, apply a profile
change to Tuesday, Thursday, Saturday with no program named; verify the custom program is still
active, the schedule is unchanged, and the response lists all three units as off the new days.

**Acceptance Scenarios**:

1. **Given** an active program, **When** a profile change is previewed or applied with no program
   or starter named, **Then** the target is the active program and no session changes.
2. **Given** the proposed available days exclude a unit's weekday, **When** the change is previewed,
   **Then** the response lists that unit and weekday as off the athlete's days.
3. **Given** a changed goal, **When** the change is previewed, **Then** the recommended starter is
   returned as a suggestion and is not applied.
4. **Given** no active program, **When** a profile change is applied with no program named,
   **Then** the top recommended starter is placed, as today (onboarding behavior).

---

### User Story 4 - The schedule rolls forward (Priority: P1)

The active program always has upcoming sessions. The athlete never reaches the end of the schedule
and never has to "change something" to get next week.

**Why this priority**: Today the schedule runs out 7 or 28 days after the last change.

**Independent Test**: Start a program, advance the clock 40 days without any mutation, read the
next two weeks, and verify the program's sessions are present on their weekdays.

**Acceptance Scenarios**:

1. **Given** an active program, **When** any future range within 56 days of today is read, **Then**
   it contains the program's sessions on their weekdays without any athlete action.
2. **Given** the same range is read twice or concurrently, **When** sessions are filled, **Then**
   each unit appears once per date.
3. **Given** a session the athlete moved, skipped, or deleted, **When** the schedule is filled
   again, **Then** it is not re-created on its original date.
4. **Given** the athlete was away longer than the filled range, **When** they return, **Then** the
   unfilled past dates stay empty and are not reported as missed.
5. **Given** no active program, **When** the schedule is read, **Then** nothing is filled.

---

### User Story 5 - Move one date (Priority: P2)

The athlete moves one planned session to another date this once (legs from Friday to Saturday this
week). The program's weekdays do not change and next week is back to normal.

**Why this priority**: It answers "change the day" for a single week without editing the program.

**Independent Test**: Move a planned session from Friday to Saturday; verify Friday is empty,
Saturday holds it, next Friday still holds the regular session, and a retry with the same key
replays the response.

**Acceptance Scenarios**:

1. **Given** a planned, unstarted session, **When** it is moved to today or a later date with a
   current revision and an operation key, **Then** its date changes, its revision increments once,
   and its original date is not refilled.
2. **Given** a session that is in progress, finished, missed, or in the past, **When** a move is
   requested, **Then** the API returns 409 `CONFLICT` with the current session.
3. **Given** a target date in the past or beyond the readable range, **When** a move is requested,
   **Then** the API returns 422 `VALIDATION_ERROR` with `details.field = "date"`.
4. **Given** a foreign or missing session, **When** a move is requested, **Then** the API returns
   the standard 404.
5. **Given** the target date already holds a session, **When** the move is applied, **Then** both
   sessions are on that date.

---

### User Story 6 - Editing the active program keeps the schedule in step (Priority: P2)

The athlete changes the active program: a weekday, an exercise, a new sport session, a removed
workout. Future unstarted sessions follow the edit in the same save. Nothing restarts.

**Why this priority**: Today an edit needs a second request that can fail and leave the week stale.

**Independent Test**: With Push on Monday, change it to Tuesday and add volleyball on Saturday in
one save; verify future Mondays are empty, future Tuesdays hold Push, future Saturdays hold
volleyball, and today's in-progress session is unchanged.

**Acceptance Scenarios**:

1. **Given** the active program, **When** it is saved with changed weekdays, **Then** its unstarted
   planned sessions from tomorrow onward match the new week in the same transaction.
2. **Given** changed exercises on a unit, **When** the program is saved, **Then** that unit's
   unstarted future sessions snapshot the new targets and earlier sessions keep theirs.
3. **Given** a one-off move of a unit that was not edited, **When** another unit is edited,
   **Then** the move is kept.
4. **Given** an inactive program, **When** it is saved, **Then** no session changes.
5. **Given** a stale revision, **When** the save is attempted, **Then** the API returns 409 and
   neither the program nor the schedule changes.

---

### User Story 7 - Remove a saved program (Priority: P3)

The athlete removes a saved program they no longer want. History is untouched.

**Why this priority**: Saving without starting, and multi-phase series, leave programs behind.

**Independent Test**: Remove an inactive program that has finished sessions; verify it is gone from
the program list, the sessions remain with their names, and removing the active program is refused.

**Acceptance Scenarios**:

1. **Given** an inactive owned program, **When** it is removed, **Then** it no longer appears and
   its finished sessions keep their snapshots.
2. **Given** the active program, **When** removal is requested, **Then** the API returns 409
   `CONFLICT` and nothing changes.
3. **Given** a training-block stage that names the removed program, **When** the block is read,
   **Then** that link reads as empty (API spec 017 edge case).

### Edge Cases

- A program with no weekdays on any unit and an athlete with no training profile: the start is
  refused with 422 naming `available_days`, because there are no days to place on.
- More units than available days at first placement: units are dealt in sequence and may share a
  weekday; nothing is dropped.
- The profile timezone decides "today" and "tomorrow" for starts, fills, and moves.
- A start whose start date is today while today already holds a started session: that session
  stays; the target's session for today is still planned beside it.
- Two requests fill the same range at once: each unit still appears once per date.
- A released app calls regenerate or materialize after a save that already updated the schedule:
  the result is the same schedule, not a duplicate.
- Account deletion removes programs, weekdays, and sessions as today.

## Non-Goals *(mandatory)*

- More than one active program, or merging programs into one week.
- Programs with a fixed length, automatic phase advance, or automatic program switching.
- Weeks that differ from one another (A/B alternation, deload weeks).
- Moving a missed workout (recover already plans a copy) or a finished one.
- Swapping two sessions in one action; the athlete moves each.
- Removing the legacy weekly-plan routes or `adopt-legacy`. That is a separate cleanup after
  production use is checked.
- Changing how participation, the weekly target, streaks, records, or reminders are calculated.
- Any change to training blocks beyond the removed-program edge case.

## Requirements *(mandatory)*

### Functional Requirements

**Weekdays**

- **FR-001**: Every placed program unit (workout or sport session) MUST carry one or more unique
  ISO weekdays. Every program read MUST return each unit's `weekdays` as an array, empty while the
  unit is unplaced, so its presence signals support. The existing single `preferred_weekday` MUST
  remain in responses as a placed unit's first weekday.
- **FR-002**: A unit saved without weekdays MUST be accepted. The server MUST place such a program
  once, in sequence across the athlete's available days, when it is first started or filled, and
  MUST store and return the result.
- **FR-003**: Scheduling MUST place each unit on each of its weekdays and nowhere else. Available
  days MUST NOT move a placed unit.
- **FR-004**: A date MAY hold more than one session. No operation MUST leave two planned sessions of
  the same program unit on one date; the database MUST enforce this.

**Saving and starting**

- **FR-005**: Program creation MUST accept an optional `active`. `false` saves the program without
  changing the active program or any session. Omitted keeps today's behavior.
- **FR-006**: The plan-transition preview and apply operations MUST be the single way to start a
  program. `proposed_profile` MUST be optional; when omitted the profile is unchanged.
- **FR-007**: Apply MUST, in one transaction, make the target the only active program and replace
  every unstarted planned session of any other program on or after the start date, whatever `to`
  the request carries.
- **FR-008**: No start, save, fill, or move MUST change a session that is in progress, finished, or
  a missed workout. Every schedule-writing operation MUST share this one rule.

**Profile changes**

- **FR-009**: When the athlete has an active program and the request names no program or starter,
  the transition target MUST be the active program and no session MUST change.
- **FR-010**: The transition response MUST list the units whose weekdays fall outside the proposed
  available days, and MUST keep returning the recommended starter as a suggestion.
- **FR-011**: With no active program and no named target, the top recommended starter MUST be
  placed as today.

**Rolling schedule**

- **FR-012**: For an active program, a schedule read MUST return its sessions for every date from
  today through at least 56 days ahead that falls inside the requested range, with no prior
  mutation by the athlete.
- **FR-013**: Filling MUST be idempotent and safe under concurrent reads, MUST NOT create sessions
  before today, and MUST NOT re-create a session the athlete moved, skipped, or deleted.

**Moves and edits**

- **FR-014**: A new mutation MUST move one planned, unstarted session to a date from today through
  the filled range, with `expected_revision` and `operation_key`. It MUST change only that session.
- **FR-015**: Saving the active program MUST update its unstarted planned sessions from tomorrow
  onward to match the saved week and targets, in the same transaction as the save.
- **FR-016**: A one-off move MUST survive a program save unless that unit was removed or its
  weekdays changed.

**Removal**

- **FR-017**: A new mutation MUST remove an inactive owned program and its unstarted planned
  sessions. Removing the active program MUST return 409 `CONFLICT`. Finished sessions MUST keep
  their snapshots.

**Cross-cutting**

- **FR-018**: Every new or changed mutation MUST follow the existing ownership, idempotency,
  revision, atomicity, and stable-error rules. Foreign and missing resources MUST return the same
  404.
- **FR-019**: All changes MUST be additive. Requests a released app sends today MUST stay valid, and
  materialize and regenerate MUST keep their contracts. The one intended behavior change for a
  released app is FR-009.
- **FR-020**: `docs/CONTRACTS.md` MUST document the new fields, mutations, and rules as the next
  free minor contract version at plan time, and MUST state how a client detects support.
- **FR-021**: The migration MUST be additive and re-runnable under the strict migration rules, MUST
  NOT rewrite any session, and its down migration MUST remove only what it added. Existing
  programs MUST be placed by the FR-002 rule on first use (amended 2026-11-16 in planning, research
  R5: one placement rule, in one place).
- **FR-022**: Fill, move, and edit queries MUST be index-backed, bounded, and owner-scoped.

### Key Entities

- **Program**: The athlete's training week. At most one is active. Others are saved.
- **Program unit**: A workout or a sport session in a program, with one or more weekdays.
- **Session**: A dated copy of a unit on the schedule (today's `scheduled_workout`).
- **Start**: The one previewed operation that makes a program active and places it on dates.
- **One-off move**: A change to one session's date that leaves the program's weekdays alone.
- **Off-availability unit**: Derived. A unit whose weekday is not among the proposed available days.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In a 12-week simulated run of a five-unit program, every unit lands on its chosen
  weekdays in every week, with zero sessions on other days.
- **SC-002**: A profile change with no named program leaves an active custom program active in 100%
  of tested cases.
- **SC-003**: Saving a program with `active: false` changes zero rows of the active program and the
  schedule.
- **SC-004**: With no mutations for 40 days, the next 14 days of the schedule are complete.
- **SC-005**: Five identical retries of any new or changed mutation produce one durable effect, and
  concurrent fills never produce two planned sessions of one unit on one date.
- **SC-006**: Compatibility tests prove a released app's create, edit, regenerate, materialize, and
  transition requests still succeed, and that the only changed outcome is FR-009.
- **SC-007**: Ownership tests prove an athlete cannot read, start, move, edit, or remove another
  athlete's programs or sessions.

## Assumptions

- Migration number and contract version are confirmed at plan time after fetching `main`
  (`main` holds migration 044 and contract 1.9.0 on 2026-11-16).
- The 56-day range matches the furthest ahead the app's calendar reads today; the plan confirms it.
- Vertical jump phases and training-block stages stay separate saved programs that the athlete
  starts by hand through the same start operation.
- The weekly target keeps reading "the plan's training days" from scheduled sessions.

## Rollout

1. **API first** (this spec): migration, model, services, handlers, tests, and CONTRACTS. Additive;
   a released app keeps working, and FR-009 already stops it replacing a custom program.
2. **Deploy the API** and verify with the smoke script.
3. **App second** ([app spec 037](../../../gym-pulse-app/specs/037-one-active-program/spec.md)):
   detects support, sends weekdays and `active: false`, and stops calling regenerate.
4. **Cleanup later**: retire materialize, regenerate, and the legacy weekly plan in a separate
   change once no supported app calls them.

## Open Questions for the Owner

None. All were answered on 2026-11-16 and are recorded under Clarifications.
