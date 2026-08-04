# Feature Specification: Criteria-Based Training Blocks

**Feature Branch**: `codex/criteria-based-training-blocks`

**Feature ID**: `gympulse-criteria-based-training-blocks`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/004-criteria-based-training-blocks/spec.md)

**Created**: 2026-08-04

**Status**: Approved

**Input**: User description: "Create the recommended criteria-based training-block feature so an athlete can incorporate an externally prepared plan without turning GymPulse into a medical-prescription or return-to-play clearance system."

## Clarifications

### Session 2026-08-04

- Q: Should GymPulse supply or medically validate rehabilitation protocols? → A: No. Blocks and stage criteria are athlete-authored or copied from advice obtained elsewhere; GymPulse records them without diagnosis, prescription, or medical clearance.
- Q: What makes an exposure count toward a stage? → A: It counts only after the athlete records that it was completed as planned and that the following-morning response returned to their chosen baseline.
- Q: Can the app move stages automatically? → A: No. It may show that recorded criteria are complete, but stage movement always requires an explicit athlete action.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Create a personal training block (Priority: P1)

An authenticated athlete creates a private criteria-based training block, optionally associates it with one of their programs, and defines an ordered sequence of stages. Each stage can describe the planned exposure, optional count, duration, and intensity targets, and the number of qualifying exposures required before the next stage can be considered.

**Why this priority**: The feature must first preserve the athlete's own plan faithfully without publishing it as a GymPulse recommendation.

**Independent Test**: Create a five-stage "Return to spiking" block and verify its name, purpose, optional owned program, stage order, targets, and required exposure counts are returned unchanged only to its owner.

**Acceptance Scenarios**:

1. **Given** an authenticated athlete, **When** they create a valid block with two or more ordered stages, **Then** the block is stored privately with the first stage current and no recorded exposures.
2. **Given** the athlete selects an owned program, **When** the block is created, **Then** the association is retained without changing that program or any scheduled workout.
3. **Given** a create request is retried with the same operation identity and payload, **When** it is received again, **Then** the original block is returned without a duplicate.
4. **Given** a foreign or missing program identifier, **When** block creation is attempted, **Then** the request fails without revealing whether another athlete owns the program.

---

### User Story 2 - Record exposure and recovery (Priority: P2)

The athlete records what they actually performed in the current stage, including an activity label, local date, load level, optional count, duration, intensity, session outcome, and notes. Later, they record the following-morning response. The system derives whether the exposure qualifies toward the stage while preserving the original planned stage targets separately.

**Why this priority**: Criteria-based progression depends on trustworthy plan-versus-performance and next-day evidence rather than a fixed calendar.

**Independent Test**: Record a demanding volleyball exposure as completed as planned, add a baseline next-morning response, and verify it becomes one qualifying exposure without changing weekly participation, the linked program, or workout history.

**Acceptance Scenarios**:

1. **Given** an active block, **When** the athlete records a valid exposure for its current stage, **Then** the exposure is returned with its performed facts and a pending next-morning response.
2. **Given** an exposure completed as planned, **When** the athlete records a baseline next-morning response, **Then** that exposure becomes qualifying exactly once.
3. **Given** an exposure was modified, stopped, or followed by an above-baseline response, **When** it is reviewed, **Then** it remains in history but does not qualify toward the stage.
4. **Given** the athlete retries an exposure or response mutation, **When** the operation identity and payload match, **Then** the original result is replayed without double counting.
5. **Given** stale block state, **When** an exposure or recovery response is written with an old revision, **Then** the system rejects the mutation and returns a stable conflict without losing either record.

---

### User Story 3 - Review criteria and move deliberately (Priority: P3)

The athlete reviews stage progress and exposure history, sees whether the self-authored criteria are complete, and explicitly advances to the next stage, returns to an earlier stage, completes the final stage, or archives the block. The system describes recorded criteria only and never labels the athlete medically cleared.

**Why this priority**: Review turns raw logs into a usable decision aid while preserving athlete control and a trustworthy transition history.

**Independent Test**: Complete the required qualifying exposures for a stage, explicitly advance, move back with a reason, and verify the transition history preserves both actions and the block revision.

**Acceptance Scenarios**:

1. **Given** the current stage has fewer qualifying exposures than required, **When** the athlete requests advancement, **Then** advancement is rejected and the authoritative progress is returned.
2. **Given** the current stage has enough qualifying exposures, **When** the athlete explicitly advances, **Then** exactly the next ordered stage becomes current and the transition is recorded.
3. **Given** the athlete chooses an earlier stage and provides a reason, **When** the regression is saved, **Then** the earlier stage becomes current without deleting later-stage exposure history.
4. **Given** the final stage criteria are complete, **When** the athlete explicitly completes the block, **Then** its status becomes completed without claiming medical clearance.
5. **Given** an active or completed block, **When** the athlete archives it, **Then** it leaves active views but remains available in private history.

### Edge Cases

- A block requires 2–12 ordered stages; each stage order is unique and contiguous.
- Stage names, instructions, block names, purposes, activity labels, reasons, and notes are trimmed and bounded; blank required text is rejected.
- Required qualifying exposures are whole numbers from 1 through 20.
- Count targets and performed counts are whole numbers from 1 through 10,000 when supplied; duration is 1–1,440 minutes; intensity is 1–100 percent.
- An exposure date cannot be in the future according to the athlete's saved timezone and must belong to the current stage when created.
- A next-morning response can be recorded only once per exposure in the first release; an idempotent replay is safe and a changed retry conflicts.
- Completed or archived blocks reject new exposures and forward stage movement.
- Returning to an earlier stage never deletes exposures, transitions, or qualifying evidence from any stage.
- Account deletion removes blocks, stages, exposures, transitions, and idempotency records under existing privacy guarantees.

## Non-Goals *(mandatory)*

- Diagnosing an injury, identifying pathology, prescribing treatment, or clearing an athlete for practice or competition.
- Publishing the referenced shoulder plan as a GymPulse starter program or claiming that its contact counts are medically validated.
- Clinician accounts, remote clinician editing, team workflows, messaging, signatures, or medical-record interoperability.
- Automatic stage advancement, automatic regression, or hidden plan changes.
- Pain scoring, body-part diagnosis fields, red-flag triage, or emergency decision support.
- Generating sport drills, exercise technique, target intensity, or recovery thresholds.
- Automatically modifying the associated program, scheduled workouts, sport activities, participation, streaks, or statistics.
- Importing arbitrary documents in this release.
- Deleting individual exposures or transition history in the first release.
- Cross-activity workload forecasting or warnings outside the records explicitly captured in a block; that requires a later specification.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST allow an authenticated athlete to create a private criteria-based training block with a name, optional purpose, optional owned program association, and 2–12 ordered stages.
- **FR-002**: Each stage MUST preserve its name, optional instructions, optional count target, optional duration target, optional intensity target, load level, and required qualifying exposure count.
- **FR-003**: Creation MUST set the first stage as current, set the block active, initialize revision 1, and create no exposure or transition implicitly.
- **FR-004**: Block creation MUST be idempotent; a matching retry returns the original resource and a changed payload returns a stable conflict.
- **FR-005**: Block, stage, exposure, and transition data MUST be owned by the authenticated athlete, and foreign and missing owned identifiers MUST be indistinguishable.
- **FR-006**: The system MUST list owned blocks in deterministic updated-newest order through a bounded, offset-paginated summary response with status filtering, and return full block detail with ordered stages, exposures, transitions, and derived current-stage progress.
- **FR-007**: The system MUST allow an athlete to record an exposure only against the current stage of an active block, preserving planned stage targets separately from performed exposure facts.
- **FR-008**: An exposure MUST capture date, activity label, load level, optional performed count, optional duration, optional performed intensity, session outcome, optional notes, and a pending next-morning response.
- **FR-009**: The system MUST allow exactly one next-morning response of baseline or above-baseline to be attached to an exposure, using safe idempotent replay behavior.
- **FR-010**: An exposure MUST qualify only when its session outcome is completed-as-planned and its next-morning response is baseline; qualification MUST be server-derived and never client-supplied.
- **FR-011**: The system MUST derive current-stage progress as qualifying exposures divided by the stage's required count without describing the result as diagnosis, treatment success, or medical clearance.
- **FR-012**: Advancing MUST require an explicit athlete mutation, sufficient qualifying exposures, the immediately following stage, a current revision, and an idempotent operation identity.
- **FR-013**: Returning to an earlier stage MUST require an explicit athlete mutation, a bounded reason, a current revision, and must preserve all prior exposure and transition history.
- **FR-014**: Completing a block MUST require an explicit athlete mutation after the final stage criteria are complete; archiving MUST be explicit and preserve history.
- **FR-015**: Every exposure, recovery, stage movement, completion, and archive mutation MUST use optimistic revision checks and safe idempotent replay.
- **FR-016**: Mutations that change a block and create an exposure, recovery response, or transition MUST commit atomically.
- **FR-017**: Invalid stages, bounds, dates, enum values, revisions, ownership, or state transitions MUST fail without partial data changes.
- **FR-018**: Criteria-based training data MUST NOT alter programs, schedules, workouts, sport activities, participation, streaks, volume, or records.
- **FR-019**: Existing clients and all existing training behavior MUST continue unchanged when no criteria-based training endpoint is used.
- **FR-020**: Account deletion MUST remove all owned criteria-based training data.

### Key Entities

- **Criteria-based training block**: One athlete-owned, optionally program-associated sequence with a name, purpose, status, current stage, revision, and timestamps.
- **Training stage**: One immutable ordered definition inside a block with athlete-authored instructions, optional targets, load level, and a required qualifying-exposure count.
- **Training exposure**: One immutable performed occurrence for a stage with session facts, a later next-morning response, and a derived qualifying result.
- **Stage transition**: An append-only record of explicit advancement, regression, completion, or archival with from/to stage context, reason when applicable, and creation time.
- **Current-stage progress**: A server-derived summary of required and qualifying exposures plus whether recorded criteria are complete.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An athlete can create a five-stage personal block with targets in under 5 minutes during a usability check.
- **SC-002**: An athlete can record an exposure in under 30 seconds and its next-morning response in under 15 seconds.
- **SC-003**: Five identical retries of any create, exposure, response, or transition mutation produce one durable effect.
- **SC-004**: Automated ownership checks demonstrate that one athlete cannot list, read, mutate, or infer another athlete's blocks or nested records.
- **SC-005**: Every displayed stage progress value matches the authoritative qualifying-exposure rule, including modified, stopped, above-baseline, and pending examples.
- **SC-006**: Automated compatibility checks demonstrate that existing program, schedule, workout, sport, participation, and statistics outcomes are unchanged.
- **SC-007**: All user-facing and API terminology describes athlete-recorded criteria and contains zero medical-clearance claims.

## Assumptions

- Athletes may transcribe a plan prepared elsewhere, but they remain responsible for deciding whether it is appropriate and for seeking professional guidance when needed.
- The existing authenticated identity, profile timezone, program ownership, idempotency, revision, and account-deletion boundaries are reused.
- A stage's definition becomes immutable after block creation in the first release; correction uses archival plus a new block so recorded history remains interpretable.
- Multiple blocks may be active because an athlete can follow unrelated training objectives.
- "Baseline" is an athlete-chosen comparison term and is not a clinical measurement or diagnosis.
