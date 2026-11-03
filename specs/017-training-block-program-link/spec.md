# Feature Specification: Training Blocks Linked to Real Training

**Feature Branch**: `spec/017-training-block-program-link`

**Feature ID**: `gympulse-training-block-program-link`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/021-training-block-program-link/spec.md)

**Amends**: [Criteria-based training blocks](../005-criteria-based-training-blocks/spec.md)
(`gympulse-criteria-based-training-blocks`). See "Changes to the approved block spec" below.

**Created**: 2026-11-03

**Status**: Draft. Awaiting owner answers to the open questions at the end of this document.

**Input**: Owner direction relayed from the coordinating session: "What are blocks even for? They
duplicate logging." Today a block is fully separate from programs and workout logs. The athlete
taps "Record exposure" by hand after every session and switches programs by hand when a stage
completes. Link blocks to real training: (1) a completed workout from the block's program offers
or creates the exposure, (2) the next-morning check is prompted on Today, (3) each stage can name
the program it runs, and moving stage offers to switch the active program as an explicit athlete
action. Keep the guardrails: no medical prescription, no automatic stage moves.

## Clarifications

### Session 2026-11-03 (defaults chosen while drafting)

These are drafting defaults. Each one that changes user value is repeated as an owner question at
the end, with its alternative.

- Q: Which program does a stage run? → A: Each stage may name an owned program (`program_id` on the
  stage). A stage without one inherits the block's associated program. This is the stage's
  **effective program**. A stage with neither has no link and keeps today's manual recording.
- Q: Which workouts count as "from the block's program"? → A: A scheduled workout of the current
  stage's effective program, finalized as `completed` or `incomplete`, dated on or after the day the
  stage became current, and not in the future. Off-plan workout sessions and unlinked sport
  activities never match; the athlete can still record them by hand.
- Q: Does the server create the exposure on its own? → A: No. The server lists matching workouts
  that have no exposure yet as **exposure suggestions**, and the athlete confirms one with a single
  action. The exposure is created by the existing create-exposure mutation with a new optional
  workout reference. This keeps "no hidden data changes" from `docs/PRODUCT.md`.
- Q: What does a suggestion prefill? → A: `performed_on` = the workout date, `activity_label` = the
  workout name, `load_level` = the stage load level, and `session_outcome` = `completed_as_planned`
  for a `completed` workout and `modified` for an `incomplete` one. Optional performed count,
  duration, and intensity stay empty unless the athlete enters them. The athlete may change any
  prefilled value before saving.
- Q: Can one workout count twice? → A: No. A workout can back at most one exposure per block. It may
  back one exposure in each of two active blocks that share the program.
- Q: How does Today learn about due next-morning checks without loading every block? → A: Block
  summaries gain a bounded list of pending checks with their dates, and one new read lists due
  checks and open suggestions across all active blocks in one call.
- Q: Does the block API switch programs? → A: No. Program switching stays in the existing
  plan-transition preview/apply contract, called by the app after an explicit athlete action. The
  block API keeps its rule that it never changes programs, schedules, or workouts.
- Q: Stages are immutable after creation. How does an existing block get stage programs? → A: One
  narrow new mutation sets or clears the program of a stage that is current or still ahead. It
  changes link context only. Name, instructions, targets, load level, and required exposures stay
  immutable.

## Changes to the approved block spec

This feature intentionally changes these statements in `005-criteria-based-training-blocks`:

| 005 statement | Change |
| --- | --- |
| "An optional owned `program_id` is context only." | The block program, and a new per-stage program, now select which completed workouts are suggested as exposures. They still never change the program. |
| FR-018 "MUST NOT alter programs, schedules, workouts…" | Unchanged. Blocks now **read** completed scheduled workouts; they still never write them. |
| Assumption "A stage's definition becomes immutable after block creation" | Narrowed: the stage program can be set or cleared on a stage that is current or still ahead. Every other stage field stays immutable. |
| Non-goal "Replacing workout logging" | Unchanged in spirit: workout logging stays the only place sets are recorded; an exposure now references that workout instead of duplicating it. |

The guardrails stay: no diagnosis, prescription, or clearance; qualification stays server-derived
from `completed_as_planned` plus `baseline`; stage movement stays an explicit athlete mutation.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Count a logged workout toward the stage (Priority: P1)

An athlete in an active block finishes a scheduled workout from the current stage's program. The
API returns that workout as an exposure suggestion. The athlete confirms it, and the API creates one
exposure that references the workout, with a pending next-morning response.

**Why this priority**: This removes the duplicated logging that makes blocks unusable today.

**Independent Test**: Create a block whose stage one names program P, complete a scheduled workout
of P, read suggestions, create the exposure from the suggestion, and verify one exposure exists
with the workout reference and the suggestion is gone. Repeat with the same workout and verify a
stable conflict and no second exposure.

**Acceptance Scenarios**:

1. **Given** an active block whose current stage's effective program is P, **When** a scheduled
   workout of P dated today is completed, **Then** the block's suggestions include that workout
   with its date, name, finalized status, and the prefill values above.
2. **Given** a suggestion, **When** the athlete creates an exposure with its
   `scheduled_workout_id`, **Then** the exposure stores the reference, the suggestion disappears,
   and the block revision increments once.
3. **Given** a workout already backs an exposure in this block, **When** another exposure with the
   same reference is created under a new operation key, **Then** the API returns 409
   `EXPOSURE_SOURCE_ALREADY_RECORDED` with the current aggregate and creates nothing.
4. **Given** a workout of another program, an off-plan session, a future workout, a `missed` or
   `skipped` workout, or a workout dated before the stage became current, **When** suggestions are
   read, **Then** it is not suggested, and creating an exposure that references it returns 422
   `VALIDATION_ERROR` with `details.field = "scheduled_workout_id"`.
5. **Given** a foreign or missing scheduled workout ID, **When** it is referenced, **Then** the API
   returns the standard 404 `NOT_FOUND` without revealing ownership.
6. **Given** an exposure created without a reference, **When** it is saved, **Then** behavior is
   exactly as in contract 005 (manual recording still works).

---

### User Story 2 - List due next-morning checks across blocks (Priority: P1)

The app needs to show due checks on Today. The API returns, across the athlete's active blocks,
every exposure whose next-morning response is still pending and whose date is before today in the
athlete's training-profile timezone, plus open exposure suggestions.

**Why this priority**: The qualifying rule depends on the next-morning check. If it is reachable
only inside the block, it is forgotten and nothing qualifies.

**Independent Test**: Record an exposure dated yesterday and one dated today; read the due list
and verify only yesterday's appears, with block name, stage name, activity label, and date. Record
its response and verify the list is empty.

**Acceptance Scenarios**:

1. **Given** pending exposures dated yesterday and today, **When** the due list is read, **Then**
   only the exposure dated before today appears as due, and the one dated today is counted as not
   yet due.
2. **Given** a recorded response, **When** the list is read again, **Then** that exposure is gone.
3. **Given** completed or archived blocks with pending exposures, **When** the list is read,
   **Then** they are excluded.
4. **Given** block summaries, **When** they are listed, **Then** each summary carries
   `pending_next_morning` (at most 20 entries, oldest first) alongside the existing count.

---

### User Story 3 - Name a program per stage (Priority: P2)

The athlete creates a block whose stages each name the owned program they run, or sets the program
of an existing block's current or upcoming stage. The block aggregate reports each stage's program
and effective program so the app can offer the switch on stage moves.

**Why this priority**: Multi-phase plans (vertical jump: four phase programs) need the block to know
which program belongs to which stage before the app can offer the switch.

**Independent Test**: Create a four-stage block with four owned programs and verify each stage
returns its program; set stage three's program on an existing block and verify only that link
changed.

**Acceptance Scenarios**:

1. **Given** owned programs, **When** a block is created with `program_id` on stages, **Then** the
   aggregate returns each stage's `program_id` and `effective_program_id`.
2. **Given** a foreign or missing stage program, **When** creation or update is attempted, **Then**
   the request returns the standard 404 and changes nothing.
3. **Given** an active block, **When** the athlete sets or clears the program of the current or a
   later stage with a current revision, **Then** only that link changes, the revision increments
   once, and a matching retry replays the original response.
4. **Given** a stage earlier than the current stage, or a completed or archived block, **When** the
   program is changed, **Then** the API returns 409 `TRAINING_BLOCK_STATE_CONFLICT`.
5. **Given** a stage program that later becomes unavailable, **When** the block is read, **Then**
   the stage's `program_id` is null and its effective program falls back to the block program.

---

### User Story 4 - Stage moves stay explicit and program-neutral (Priority: P2)

Advance, regress, complete, and archive keep their 005 behavior. The response includes the new
current stage's effective program so the app can offer a switch, but the API changes no program.

**Independent Test**: Advance a block whose next stage names program Q while P is active; verify P
is still active and the aggregate reports Q as the new current stage's effective program.

**Acceptance Scenarios**:

1. **Given** criteria are complete, **When** the athlete advances, **Then** the next stage becomes
   current and the active program is unchanged.
2. **Given** suggestions existed for the previous stage, **When** the stage changes, **Then** they
   are no longer suggested; workouts already recorded keep their exposure and stage.

### Edge Cases

- A workout's finalized status changes later through a replay or recovery path: an existing
  exposure keeps its stored session outcome; suggestions always reflect the current status.
- A completed workout whose scheduled date is in the past (logged late) is suggested if its date is
  on or after the stage start and not in the future.
- Two active blocks share the effective program: the workout is suggested to each; each block may
  record it once.
- A stage program equal to the block program is valid and needs no special handling.
- A plan transition replaces unstarted scheduled workouts only; completed workouts and their
  exposures are untouched.
- Planned sport units: a scheduled sport workout finalized `completed` matches like any other
  scheduled workout of the program.
- Account deletion removes stage program links and source references with the block.
- The block API never deletes a referenced scheduled workout. If another path removes it, the
  exposure keeps its facts and its reference becomes null.

## Non-Goals *(mandatory)*

- Creating exposures without an explicit athlete action (pending owner answer to question 1).
- Recording the next-morning response from a notification, or sending reminder notifications.
- Advancing, regressing, or completing a stage automatically, or suggesting that the athlete should.
- Switching, activating, or rescheduling programs from any block endpoint.
- Matching off-plan workout sessions, unlinked sport activities, or imported health workouts
  (pending owner answer to question 4).
- Deriving performed count, duration, or intensity from logged sets.
- Editing stage criteria after creation, or editing or deleting exposures.
- Any diagnosis, treatment, healing, or clearance language.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A stage MUST accept an optional owned `program_id` at block creation; the aggregate
  MUST return `program_id` and server-derived `effective_program_id` for every stage.
- **FR-002**: A new mutation MUST set or clear the program of the current stage or a later stage of
  an active block, with `expected_revision` and `operation_key`, and MUST NOT change any other stage
  field.
- **FR-003**: Program references MUST be ownership-scoped; foreign and missing programs MUST return
  the same 404 shape; an unavailable program MUST read back as null.
- **FR-004**: The block aggregate MUST include `exposure_suggestions` for the current stage: each
  owned scheduled workout of the effective program, finalized `completed` or `incomplete`, dated
  from the stage start date (local, training-profile timezone) through today, with no exposure in
  this block referencing it. Each suggestion MUST carry the scheduled workout ID, date, name,
  finalized status, and the prefill values.
- **FR-005**: Create-exposure MUST accept an optional `scheduled_workout_id`. When present, the
  server MUST verify ownership, the effective-program match, finalized status, and the date window,
  and MUST store the reference with the exposure.
- **FR-006**: A scheduled workout MUST back at most one exposure per block, enforced by the
  database; a duplicate under a new operation key MUST return 409
  `EXPOSURE_SOURCE_ALREADY_RECORDED` with the current aggregate.
- **FR-007**: Exposures MUST return their optional `scheduled_workout_id`; all other exposure fields
  and the qualifying rule MUST be unchanged.
- **FR-008**: Block summaries MUST include `pending_next_morning`, at most 20 entries oldest first,
  each with exposure ID, performed date, activity label, and stage name.
- **FR-009**: A new read MUST return, across active blocks only, due next-morning checks (pending,
  dated before today) and open exposure suggestions, bounded and ordered oldest first, with block
  ID, block name, revision, and stage name per entry.
- **FR-010**: Transition responses MUST include the new current stage's effective program; no
  transition MUST change programs, schedules, or workouts.
- **FR-011**: All new mutations MUST use the existing idempotency, revision, atomicity, and
  stable-error rules of contract 005.
- **FR-012**: All changes MUST be additive: existing request bodies stay valid, new response fields
  are optional for clients, and an app that ignores them keeps its current behavior.
- **FR-013**: `docs/CONTRACTS.md` MUST document the new fields, mutation, read, and error code under
  a new contract minor version.
- **FR-014**: The migration MUST be additive (nullable columns, a partial unique index, no backfill)
  and its down migration MUST remove only what it added.
- **FR-015**: Reads MUST remain bounded; the suggestion and due queries MUST be index-backed and
  MUST NOT scan other athletes' rows.

### Key Entities

- **Stage program link**: Optional owned program on a stage. Effective program = stage program,
  else block program, else none.
- **Exposure source**: Optional reference from an exposure to the scheduled workout it records;
  unique per block.
- **Exposure suggestion**: Derived, not stored. A finalized scheduled workout of the current stage's
  effective program with no exposure in this block.
- **Due next-morning check**: Derived. A pending exposure in an active block dated before today.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For a workout of the stage program, recording the exposure needs one confirm request
  and zero re-entered fields.
- **SC-002**: Five identical retries of any new or changed mutation produce one durable effect; a
  second exposure for the same workout in one block is impossible (database-enforced, test-proven).
- **SC-003**: Ownership tests prove an athlete cannot reference, be suggested, or infer another
  athlete's programs, scheduled workouts, or exposures.
- **SC-004**: Compatibility tests prove existing block, program, schedule, workout, sport,
  participation, and statistics responses are unchanged for requests that do not use the new
  fields.
- **SC-005**: Today's due list and suggestions arrive in one request whose latency stays within the
  existing block list budget for 5 active blocks with 50 exposures each.
- **SC-006**: Contract and API terminology contain zero diagnosis, treatment, healing, or clearance
  claims.

## Assumptions

- Scheduled workouts carry their program through the existing program and schedule model; no new
  program relation is needed beyond the stage link.
- "Stage start date" is the local date of the last transition into the current stage, or the block
  creation date for stage one.
- The training-profile timezone decides "today" exactly as it does for exposure dates in 005.
- Migration numbering is decided at plan time after fetching `main`; open branches already use 032.

## Rollout

1. **API first** (this spec): migration, model, service, handlers, tests, and a `docs/CONTRACTS.md`
   minor version. Additive; the released app keeps working because it ignores the new fields.
2. **Deploy the API** to Railway and verify with the smoke script against the new contract version.
3. **App second** ([app spec 021](../../../gym-pulse-app/specs/021-training-block-program-link/spec.md)):
   consumes suggestions, the due list, and stage programs. It must tolerate an API without them
   (linking hidden, current manual flow kept) during the mixed-version window.
4. **Starter amendments** (app, later): the vertical jump starter (app spec 020) and the knee tendon
   starter (app spec 018) set stage programs at creation instead of naming the program in stage
   text.

## Open Questions for the Owner

1. **Offer or auto-create?** Default: the athlete confirms each suggested exposure with one tap.
   Alternative: the server creates the exposure inside workout completion, and the athlete can only
   correct its outcome later. Auto-create saves one tap but adds a hidden data change, and it needs
   an undo rule, because exposures cannot be deleted today.
2. **Incomplete workouts.** Default: an `incomplete` workout is suggested with outcome `modified`
   (recorded, does not qualify). Alternative: suggest only `completed` workouts.
3. **Editing stage programs on existing blocks.** Default: allowed for the current and later stages
   only. Alternative: stage programs only at creation. That keeps stages fully immutable, but
   existing blocks (including vertical jump blocks already created) would never link.
4. **Sport sessions and off-plan workouts.** Default: only scheduled workouts of the stage program
   match, including planned sport units once completed. Should a manual sport activity (for example
   volleyball not on the plan) also be suggested when the stage is sport-based?
5. **Two blocks, one program.** Default: the workout is suggested to each block. Alternative: only
   to the most recently updated block.
