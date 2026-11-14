# Feature Specification: Training Blocks Linked to Real Training

**Feature Branch**: `spec/017-training-block-program-link`

**Feature ID**: `gympulse-training-block-program-link`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/030-training-block-program-link/spec.md)

**Amends**: [Criteria-based training blocks](../005-criteria-based-training-blocks/spec.md)
(`gympulse-criteria-based-training-blocks`). See "Changes to approved specs and contracts" below.

**Created**: 2026-11-03 · **Revised**: 2026-11-07 (review fixes), 2026-11-14 (planning: detection)

**Status**: Approved (owner answered every open question on 2026-11-07; see Clarifications)

**Input**: Owner direction relayed from the coordinating session: "What are blocks even for? They
duplicate logging." Today a block is fully separate from programs and workout logs. The athlete
taps "Record exposure" by hand after every session and switches programs by hand when a stage
completes. Link blocks to real training: (1) a completed workout from the block's program offers
or creates the exposure, (2) the next-morning check is prompted on Today, (3) each stage can name
the program it runs, and moving stage offers to switch the active program as an explicit athlete
action. Keep the guardrails: no medical prescription, no automatic stage moves.

## Clarifications

### Session 2026-11-03 (drafting defaults)

- Q: Which program does a stage run? → A: Each stage may name an owned program (`program_id` on the
  stage). A stage without one inherits the block's associated program. This is the stage's
  **effective program**. A stage with neither has no link and keeps today's manual recording.
- Q: Does the server create the exposure on its own? → A: No. The server lists matching workouts
  as **exposure suggestions**, and the athlete confirms one with a single action through the
  existing create-exposure mutation, which gains an optional workout reference.
- Q: What does a suggestion prefill? → A: `performed_on` = the scheduled workout date,
  `activity_label` = the workout name, `load_level` = the stage load level, and `session_outcome` =
  `completed_as_planned` for a `completed` workout and `modified` for an `incomplete` one. Optional
  count, duration, and intensity stay empty. The athlete may change any value before saving.
- Q: Can one workout count twice? → A: No. A workout backs at most one exposure per block. It may
  back one exposure in each of two active blocks that share the program.
- Q: Does the block API switch programs? → A: No. Switching stays in the existing plan-transition
  preview/apply contract, called by the app after an explicit athlete action.

### Session 2026-11-07 (owner answers)

- Q: Offer or auto-create the exposure? → A: Offer. Nothing is recorded without the athlete's
  press.
- Q: Are `incomplete` workouts suggested? → A: Yes, prefilled as `modified`: kept in history, never
  qualifying.
- Q: Can stage programs change on an existing block? → A: Yes, for the current and later stages
  only. Every other stage field stays immutable.
- Q: Do manual sport activities or off-plan sessions match? → A: No.
- Q: Two active blocks share the program? → A: The workout is suggested to each block; each can
  record it once.

### Session 2026-11-07 (review fixes, owner answers)

- Q: How does the athlete decline a suggestion? → A: "Not this one" stores a dismissal for that
  workout in that block, and it is never suggested to that block again. Suggestions also include
  only workouts finalized **after** the stage's program link took effect, so sessions finished (and
  possibly recorded by hand) before linking never resurface.
- Q: Do sport days in the stage program match? → A: No. Only `exercises` units are suggested.
  Planned sport units (for example the knee tendon program's Saturday volleyball) are never
  suggested; the athlete can still record them by hand.
- Q: When does a stage's window start? → A: At the later of two timestamps: the moment the stage
  became current (the transition's `created_at`, or block creation for stage one) and the moment
  its effective program link last changed. Comparison uses the workout's `finalized_at`, not dates,
  so a workout finished at 07:00 is never suggested to a stage entered at 09:00.
- Q: Which error codes? → A: State conflicts (stage earlier than current, block read-only) reuse
  the existing 409 `CONFLICT` with `resource`, as the block DAO already does for read-only blocks.
  A second exposure for the same workout returns the new 409 `EXPOSURE_SOURCE_ALREADY_RECORDED`, so
  the app can say "Already counted".
- Q: How does the app detect this feature? → A: There is no runtime contract version. Amended
  2026-11-14 in planning (research R1): an older API answers `GET /training-blocks/due` with
  `400 BAD_REQUEST` because it parses "due" as a block ID, so a status code cannot signal support.
  The app detects support by fields: every block summary carries `pending_next_morning` and every
  aggregate carries `exposure_suggestions`, both always arrays. The app calls the due read only
  after seeing one of them.

## Changes to approved specs and contracts

| Source | Statement | Change |
| --- | --- | --- |
| 005 / CONTRACTS | "An optional owned `program_id` is context only." | The block program and the new per-stage program select which finished workouts are suggested. They still never change the program. |
| 005 FR-018 | "MUST NOT alter programs, schedules, workouts…" | Unchanged. Blocks now **read** finished scheduled workouts; they never write them. |
| 005 Assumption | "A stage's definition becomes immutable after block creation" | Narrowed: the stage program can be set or cleared on the current or a later stage. Every other stage field stays immutable. |
| 005 Non-goal | "Replacing workout logging" | Kept: sets are logged only in the player; an exposure now references that workout. |
| CONTRACTS `DELETE /api/v1/scheduled-workouts/{id}` (API spec 021) | Deletes the workout and its sessions in one transaction | Extended: the same transaction clears the reference on any exposure that points at it (the exposure and its facts stay, because block history is append-only) and removes any dismissal of it. |

The guardrails stay: no diagnosis, prescription, or clearance; qualification stays server-derived
from `completed_as_planned` plus `baseline`; stage movement stays an explicit athlete mutation.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Count a logged workout toward the stage (Priority: P1)

An athlete in an active block finishes a scheduled gym workout from the current stage's program.
The API returns it as an exposure suggestion. The athlete confirms it and the API creates one
exposure that references the workout, with a pending next-morning response.

**Why this priority**: This removes the duplicated logging that makes blocks unusable today.

**Independent Test**: Create a block whose stage one names program P, complete a scheduled workout
of P, read suggestions, create the exposure from the suggestion, and verify one exposure with the
reference and no remaining suggestion. Repeat with the same workout under a new key and verify
`EXPOSURE_SOURCE_ALREADY_RECORDED` and no second exposure.

**Acceptance Scenarios**:

1. **Given** an active block whose current stage's effective program is P, **When** an
   `exercises` scheduled workout of P is finalized after the stage window started, **Then** the
   aggregate's `exposure_suggestions` include it with ID, date, name, finalized status, and the
   prefill values.
2. **Given** a suggestion, **When** the athlete creates an exposure with its
   `scheduled_workout_id`, **Then** the exposure stores the reference, the suggestion disappears,
   and the block revision increments once.
3. **Given** a workout already backs an exposure in this block, **When** another exposure with the
   same reference is created under a new operation key, **Then** the API returns 409
   `EXPOSURE_SOURCE_ALREADY_RECORDED` with the current aggregate and creates nothing.
4. **Given** a workout that is not suggestible (another program, a sport unit, `planned`,
   `in_progress` or `missed`, finalized before the window started, or dismissed in this block),
   **When** suggestions are read, **Then** it is absent, and an exposure that references it returns
   422 `VALIDATION_ERROR` with `details.field = "scheduled_workout_id"`.
5. **Given** a foreign or missing scheduled workout ID, **When** it is referenced, **Then** the API
   returns the standard 404 `NOT_FOUND`.
6. **Given** an exposure created without a reference, **When** it is saved, **Then** behavior is
   exactly as in contract 005.

---

### User Story 2 - Decline a suggestion (Priority: P1)

The athlete does not want a suggested workout to count (for example a session they cut short for
an unrelated reason). They dismiss it, and it is never suggested to that block again.

**Why this priority**: Without a decline, an unwanted suggestion stays on Today until the stage
moves.

**Independent Test**: Dismiss a suggestion, verify it is gone from the aggregate and the due read,
replay the same dismissal and verify the same response, and verify the workout cannot be
referenced by a new exposure in that block.

**Acceptance Scenarios**:

1. **Given** a suggestion, **When** the athlete dismisses it with a current revision and an
   operation key, **Then** the dismissal is stored, the suggestion disappears, and the revision
   increments once.
2. **Given** the same dismissal is retried, **When** key and payload match, **Then** the original
   response is replayed.
3. **Given** a workout that is not currently suggested to this block, **When** it is dismissed,
   **Then** the API returns 422 `VALIDATION_ERROR` with `details.field = "scheduled_workout_id"`.
4. **Given** two blocks share the program, **When** the workout is dismissed in one, **Then** it is
   still suggested to the other.

---

### User Story 3 - List due next-morning checks across blocks (Priority: P1)

The API returns, across active blocks, every exposure whose next-morning response is pending and
whose `performed_on` is before today in the training-profile timezone, plus open suggestions, in
one request.

**Why this priority**: The qualifying rule depends on the next-morning check; buried inside the
block, it is forgotten and nothing qualifies.

**Independent Test**: Record exposures dated yesterday and today; read the due list and verify only
yesterday's is due, with block name, stage name, activity label, and date; record its response and
verify it is gone.

**Acceptance Scenarios**:

1. **Given** pending exposures dated yesterday and today, **When** the due read runs, **Then** only
   the exposure dated before today is listed as due.
2. **Given** an exposure counted today for a workout dated yesterday, **When** the due read runs,
   **Then** that check is due immediately.
3. **Given** a recorded response, **When** the read runs again, **Then** that exposure is gone.
4. **Given** completed or archived blocks, **When** the read runs, **Then** their pending
   exposures and suggestions are excluded.
5. **Given** block summaries, **When** they are listed, **Then** each carries
   `pending_next_morning` (at most 20, oldest first) alongside the existing count.

---

### User Story 4 - Name a program per stage (Priority: P2)

The athlete creates a block whose stages name owned programs, or sets the program of an existing
block's current or later stage. The aggregate reports each stage's program and effective program.

**Why this priority**: Multi-phase plans need the block to know which program belongs to which
stage before the app can offer the switch.

**Independent Test**: Create a four-stage block with four owned programs and verify each stage;
set stage three's program on an existing block and verify only that link and its link time
changed.

**Acceptance Scenarios**:

1. **Given** owned programs, **When** a block is created with `program_id` on stages, **Then** the
   aggregate returns each stage's `program_id` and `effective_program_id`.
2. **Given** a foreign or missing stage program, **When** creation or update is attempted, **Then**
   the standard 404 is returned and nothing changes.
3. **Given** an active block, **When** the athlete sets or clears the program of the current or a
   later stage with a current revision, **Then** only that link changes, the stage's link time
   moves to now, the revision increments once, and a matching retry replays the response.
4. **Given** a stage earlier than the current stage, or a completed or archived block, **When** its
   program is changed, **Then** the API returns 409 `CONFLICT` with the current aggregate.
5. **Given** the current stage's program is set mid-stage, **When** suggestions are read, **Then**
   workouts finalized before the change are not suggested.

---

### User Story 5 - Stage moves stay explicit and program-neutral (Priority: P2)

Advance, regress, complete, and archive keep their 005 behavior. The response includes the new
current stage's effective program so the app can offer a switch; the API changes no program.

**Independent Test**: Advance a block whose next stage names Q while P is active; verify P stays
active and the aggregate reports Q.

**Acceptance Scenarios**:

1. **Given** criteria are complete, **When** the athlete advances, **Then** the next stage becomes
   current and the active program is unchanged.
2. **Given** an uncounted workout finalized before the advance, **When** suggestions are read for
   the new stage, **Then** it is not suggested, even if it shares the program and the date.

### Edge Cases

- A workout's finalized status changes later: an existing exposure keeps its stored outcome;
  suggestions always reflect the current status.
- A workout logged late (scheduled date in the past, finalized now) is suggested if finalized inside
  the window; its `performed_on` prefill is the scheduled date, so its check is due at once.
- A deleted scheduled workout (CONTRACTS `DELETE /api/v1/scheduled-workouts/{id}`): its exposure
  stays with a null reference; its dismissal is removed; nothing about it is suggested.
- Two active blocks share the effective program: suggested to each; each records or dismisses it
  once.
- A plan transition replaces unstarted workouts only; finished workouts and exposures are untouched.
- A stage program that becomes unavailable reads back as null and falls back to the block program;
  its link time moves to that moment, so older workouts do not resurface.
- Account deletion removes stage links, references, and dismissals with the block.

## Non-Goals *(mandatory)*

- Creating exposures without an explicit athlete action.
- Next-morning notifications or reminders.
- Advancing, regressing, or completing a stage automatically, or prompting the athlete to.
- Switching, activating, or rescheduling programs from any block endpoint.
- Matching sport units, off-plan sessions, sport activities, or imported health workouts.
- Deriving performed count, duration, or intensity from logged sets.
- Undoing a dismissal, editing stage criteria, or editing or deleting exposures.
- Any diagnosis, treatment, healing, or clearance language.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A stage MUST accept an optional owned `program_id` at creation; the aggregate MUST
  return `program_id`, server-derived `effective_program_id`, and the link time for every stage.
- **FR-002**: A new mutation MUST set or clear the program of the current or a later stage of an
  active block, with `expected_revision` and `operation_key`, MUST update that stage's link time,
  and MUST NOT change any other stage field. Other states return 409 `CONFLICT` with `resource`.
- **FR-003**: Program references MUST be ownership-scoped; foreign and missing programs MUST return
  the same 404; an unavailable program MUST read back as null.
- **FR-004**: The aggregate MUST include `exposure_suggestions` for the current stage: owned
  scheduled workouts of kind `exercises` whose `program_id` is the effective program, with status
  `completed` or `incomplete`, `finalized_at` after the stage window start (FR-005), and neither an
  exposure nor a dismissal in this block referencing them. Each suggestion MUST carry the workout
  ID, date, name, finalized status, and prefill values.
- **FR-005**: The stage window start MUST be the later of the moment the stage became current and
  the stage's effective-link time, compared with `finalized_at`.
- **FR-006**: Create-exposure MUST accept an optional `scheduled_workout_id`; when present the
  server MUST accept it only if the workout is currently suggested to this block, and MUST store
  the reference.
- **FR-007**: A workout MUST back at most one exposure per block, enforced by the database; a
  duplicate under a new key MUST return 409 `EXPOSURE_SOURCE_ALREADY_RECORDED` with the aggregate.
- **FR-008**: A new dismiss mutation MUST store one dismissal per block and workout, accept only a
  currently suggested workout, and follow the revision and idempotency rules.
- **FR-009**: Exposures MUST return their optional `scheduled_workout_id`; all other fields and the
  qualifying rule MUST be unchanged.
- **FR-010**: Block summaries MUST include `pending_next_morning`, at most 20 entries oldest first,
  each with exposure ID, performed date, activity label, and stage name.
- **FR-011**: A new read MUST return, across active blocks only and in one request, due checks
  (pending, `performed_on` before today) and open suggestions, bounded and oldest first, each with
  block ID, name, revision, and stage name.
- **FR-012**: Transition responses MUST include the new current stage's effective program; no
  transition MUST change programs, schedules, or workouts.
- **FR-013**: Deleting a scheduled workout MUST, in its existing transaction, null the reference on
  exposures that point at it and remove its dismissals.
- **FR-014**: All new mutations MUST use the 005 idempotency, revision, atomicity, and stable-error
  rules.
- **FR-015**: All changes MUST be additive. Existing requests stay valid; an app that ignores new
  fields keeps its behavior; `pending_next_morning` and `exposure_suggestions` MUST always be present
  as arrays (never omitted or null) so their presence signals support.
- **FR-016**: `docs/CONTRACTS.md` MUST document the new fields, mutations, read, and error code as
  contract 1.6.0 (or the next free minor version at plan time).
- **FR-017**: The migration MUST be additive (nullable columns, link-time column defaulting to
  block creation, dismissal table, partial unique index) with no data rewrite, and its down
  migration MUST remove only what it added.
- **FR-018**: Suggestion and due queries MUST be index-backed, bounded, and owner-scoped.

### Key Entities

- **Stage program link**: Optional owned program on a stage, with the time the effective link last
  changed. Effective program = stage program, else block program, else none.
- **Exposure source**: Optional reference from an exposure to a scheduled workout; unique per block.
- **Suggestion dismissal**: Athlete's stored "not this one" for a workout in a block.
- **Exposure suggestion**: Derived. A finished gym workout of the current stage's effective program,
  finalized inside the stage window, with no exposure or dismissal in this block.
- **Due next-morning check**: Derived. A pending exposure in an active block dated before today.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Recording a suggested workout needs one request and zero re-entered fields.
- **SC-002**: Five identical retries of any new or changed mutation produce one durable effect; a
  second exposure for the same workout in one block is impossible (database-enforced, test-proven).
- **SC-003**: Ownership tests prove an athlete cannot reference, dismiss, be suggested, or infer
  another athlete's programs, workouts, or exposures.
- **SC-004**: Compatibility tests prove existing block, program, schedule, workout, sport,
  participation, delete, and statistics responses are unchanged for requests without new fields.
- **SC-005**: The due read serves Today in one request within the existing block list latency
  budget for 5 active blocks with 50 exposures each.
- **SC-006**: A test proves workouts finalized before a stage transition or a link change are never
  suggested to the new stage or link.
- **SC-007**: Contract and API terminology contain zero diagnosis, treatment, healing, or clearance
  claims.

## Assumptions

- `scheduled_workouts.program_id` and `finalized_at` (migration 012) identify the program and the
  finalization moment; no new workout column is needed.
- The training-profile timezone decides "today" exactly as it does for exposure dates in 005.
- Migration number and contract version are confirmed at plan time after fetching `main`
  (`main` holds migration 033 and contract 1.5.0 on 2026-11-07).

## Rollout

1. **API first** (this spec): migration, model, service, handlers, tests, CONTRACTS 1.6.0, and the
   delete-transaction change. Additive; the released app ignores the new fields.
2. **Deploy the API** and verify with the smoke script.
3. **App second** ([app spec 030](../../../gym-pulse-app/specs/030-training-block-program-link/spec.md)):
   detects the feature (FR-015) and keeps the manual flow against an older API.
4. **Starters** (app spec 030 User Story 5): the knee tendon block links its program; the vertical
   jump plan can create a four-stage block linked to its phase programs. No further API change.

## Open Questions for the Owner

None. All were answered on 2026-11-07 and are recorded under Clarifications.
