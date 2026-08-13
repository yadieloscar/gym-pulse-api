# Feature Specification: Release-Safe Training Workflows

**Feature Branch**: `agent/release-blockers-api`

**Feature ID**: `gympulse-release-safety`

**Sibling Spec**: `../gym-pulse-app/specs/006-release-client-safety/spec.md`

**Created**: 2026-08-12

**Status**: Approved for implementation

**Input**: Ship the audited release blockers across both GymPulse repositories, beginning with additive API support and preserving deterministic training behavior.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Carry Forward Existing Plans (Priority: P1)

An existing athlete can move their saved templates and weekly assignments into the goal-based training experience without rebuilding their plan or losing historical data.

**Why this priority**: The mobile app already offers this path. Without server support, an existing athlete reaches a dead end during setup.

**Independent Test**: Start with owned legacy templates and a weekly plan, adopt them once, and verify that one active program plus the next future training week appear while every legacy row remains unchanged.

**Acceptance Scenarios**:

1. **Given** an athlete with legacy templates and weekly assignments who has not adopted them, **When** they adopt their plan, **Then** one active program and the next complete future week of dated workouts are created from the owned legacy data.
2. **Given** the same athlete repeats the same adoption request, **When** the request is replayed, **Then** the same authoritative program and schedule are returned without creating duplicates.
3. **Given** an athlete has no adoptable workout assignment, **When** they request adoption, **Then** they receive an actionable not-found outcome and no data changes.

---

### User Story 2 - Retry Training Actions Safely (Priority: P1)

An athlete can log or finalize training through an unreliable mobile connection without a retry duplicating work or leaving participation inconsistent with the workout.

**Why this priority**: Mobile requests can be retried after timeouts, app restarts, and connectivity transitions. Incorrect retry behavior can silently corrupt the athlete's record.

**Independent Test**: Repeat each supported training mutation with the same operation identity, including injected failures, and verify an exact replay, a conflict for changed input, and all-or-nothing workout and participation outcomes.

**Acceptance Scenarios**:

1. **Given** a successful training mutation, **When** the identical operation is retried, **Then** the original status and authoritative response are returned with no second state change.
2. **Given** an operation identity already used for one payload, **When** it is reused with changed input, **Then** the mutation is rejected as a conflict and existing data remains unchanged.
3. **Given** a scheduled workout is finalized, **When** any part of finalization fails, **Then** neither the workout outcome nor its participation record is committed.
4. **Given** an off-plan session becomes complete, **When** any part of completion fails, **Then** neither the session completion nor its participation preservation is committed.
5. **Given** concurrent duplicate attempts, **When** they use the same operation identity, **Then** at most one state transition occurs and both callers observe the same result.

---

### User Story 3 - Keep Date Work Bounded (Priority: P1)

An authenticated athlete can request normal calendar ranges while accidentally or maliciously large ranges are rejected before expensive generation, loading, or lazy-finalization work begins.

**Why this priority**: Unbounded inclusive ranges can consume database connections, memory, and CPU and undermine horizontal scaling for every user.

**Independent Test**: Exercise every goal-training range boundary at the documented maximum and one day above it, verifying the maximum succeeds and the oversized request makes no repository call.

**Acceptance Scenarios**:

1. **Given** a valid inclusive range at or below the maximum, **When** it is requested, **Then** normal processing continues.
2. **Given** a valid-looking range above the maximum, **When** it is requested, **Then** it is rejected with a field-specific validation outcome before data access or generation.
3. **Given** a reversed or malformed range, **When** it is requested, **Then** existing validation behavior remains stable.

### Edge Cases

- Adoption includes only templates referenced by non-rest weekly assignments and orders workouts by ISO weekday.
- Multiple weekdays may reference the same template; each assigned day remains a distinct program workout.
- Adoption starts on the next Monday strictly after the athlete's current local date, so it never rewrites the current or historical week.
- Legacy cardio exercises without strength sets become duration-based scheduled work; strength exercises retain their legacy sets, repetitions, weight, rest, and notes.
- A failed exact replay lookup must not fall through into a second mutation.
- Existing participation on a date is preserved while scheduled-opportunity and participated flags are combined according to the documented rules.
- The range maximum is inclusive and is measured in calendar days, including both endpoints.

## Non-Goals *(mandatory)*

- Removing, rewriting, or deprecating legacy templates, weekly plans, overrides, or historical logs.
- Changing starter-program ranking, plan-transition policy, training recommendations, or any runtime AI/LLM behavior.
- Adding new workout types, analytics events, administrative endpoints, or background jobs.
- Solving app-side offline authentication, password recovery, request cancellation, or onboarding ownership; those belong to the sibling app specification.
- Replacing the existing authentication provider or ownership model.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST let an authenticated athlete adopt only their own referenced legacy templates and weekly assignments into one active program.
- **FR-002**: Adoption MUST create dated snapshots for the next Monday-through-Sunday week strictly after the athlete's current local date.
- **FR-003**: Adoption MUST preserve all legacy rows and MUST not alter historical or current-week workouts.
- **FR-004**: Adoption MUST return the authoritative program, authoritative dated schedule, and whether this call performed the adoption.
- **FR-005**: An identical adoption replay MUST return the previously committed resources without creating another program or schedule; changed input under the same operation identity MUST conflict.
- **FR-006**: Every published goal-training mutation MUST require a client operation identity. When both body and header identities are supplied they MUST match; the three pre-existing header-only profile/custom-program calls MAY normalize a missing body identity during the API-first migration window.
- **FR-007**: Every supported mutation MUST atomically store its state transition and replayable response under the authenticated athlete and mutation scope.
- **FR-008**: Exact mutation replays MUST return the original committed status and body, while changed input under the same scoped operation identity MUST return the documented idempotency conflict.
- **FR-009**: Scheduled-workout finalization and its day-participation outcome MUST commit or roll back together.
- **FR-010**: Workout-session completion and participation preservation MUST commit or roll back together.
- **FR-011**: Concurrent mutations for one athlete MUST use consistent domain lock ordering and short transactions without network calls inside the transaction.
- **FR-012**: All goal-training list, materialization, regeneration, transition, and participation date ranges MUST reject an inclusive span above 366 calendar days before repository work.
- **FR-013**: Oversized ranges MUST return the standard validation error shape with the range identified as the invalid field.
- **FR-014**: All created and loaded resources MUST remain scoped to the authenticated owner; a foreign identifier MUST remain indistinguishable from a missing one.
- **FR-015**: The client-facing contract, generated documentation, route inventory, smoke coverage, and automated tests MUST describe and prove the delivered behavior.

### Key Entities

- **Legacy adoption**: The durable one-per-athlete record connecting an operation identity to the adopted program and replayable result.
- **Program**: The athlete-owned ordered training plan created from assigned legacy templates.
- **Scheduled workout**: A dated immutable snapshot for one assigned day in the next future week.
- **Idempotency record**: A scoped, athlete-owned operation identity, request fingerprint, status, and exact response used for safe replay.
- **Day participation**: The durable per-local-date participation outcome that must remain consistent with finalized workouts and completed sessions.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An athlete with an adoptable weekly plan receives exactly one active program and the expected future-week workouts on the first request and on 100 consecutive identical replays.
- **SC-002**: Automated failure-injection checks demonstrate zero cases where a workout or completed session is committed without its required participation outcome, or vice versa.
- **SC-003**: Concurrent duplicate-operation checks demonstrate one observable state transition and one authoritative response for every tested mutation scope.
- **SC-004**: Every audited goal-training range accepts an inclusive 366-day request and rejects a 367-day request before data access.
- **SC-005**: Contract, unit, integration, race, lint, vulnerability, generated-documentation, and smoke gates complete successfully with no reduced thresholds or skipped release-safety scenarios.

## Assumptions

- The next eligible future week means the next Monday-through-Sunday block strictly after the athlete's current local date, based on their saved training-profile timezone.
- The 366-day inclusive maximum supports leap-year history and planning while bounding per-request work; normal app views use much smaller ranges.
- An athlete can adopt legacy data once. Later legacy edits remain available through legacy screens but do not mutate the adopted goal-based program.
- Existing tables and constraints are sufficient; any schema extension will be forward-only and compatible with existing rows.
- The sibling app continues to use deterministic, manually chosen training behavior; no model-generated plan changes are introduced.
