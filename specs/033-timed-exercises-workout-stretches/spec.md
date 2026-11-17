# Feature Specification: Timed exercises and chosen stretching routines in saved workouts

**Feature Branch**: `feat/timed-exercises-workout-stretches`

**Feature ID**: `gympulse-workout-stretches`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/039-timed-exercises-workout-stretches/spec.md)

**Created**: 2026-11-17

**Status**: Approved (requested by the owner on 2026-11-17; drafting defaults are listed under Clarifications)

**Input**: Owner: "I also want to be able to customize my workout by adding a necessary stretch that I want to add." Offered two shapes (an exercise counted in seconds; a stretching routine attached to a workout), the owner answered "both".

Governing product rules: workspace `docs/PRODUCT.md`. Nothing there changes: a saved workout stays the athlete's own recipe, stretching routines stay bundled app content, and a finished routine is still recorded as a mobility session that counts toward nothing.

## Clarifications

### Session 2026-11-17 (drafting defaults; the owner's request fixes the rest)

- Q: How is a stretch or hold stored on a saved workout? → A: As a third exercise shape beside reps and cardio minutes: `sets` plus `duration_seconds`, the seconds for **each** set. "2 × 30 s" is `sets: 2, duration_seconds: 30`.
- Q: Is a timed exercise cardio? → A: No. Cardio keeps `duration_minutes` and one long effort. A timed exercise is strength work whose sets are measured in seconds; its logged sets carry `duration_seconds`, no reps, and `exercise_modality` stays `strength`.
- Q: What does "attach a stretching routine" store? → A: The slug of the routine the athlete chose to open the workout and the slug of the one to close it. Routines are bundled in the app (spec 018), so the API stores the slug and does not look it up.
- Q: What happens when no routine is chosen? → A: What happens today: the app offers a warm-up and a cool-down picked by the workout's body area. `null` means that automatic pick.
- Q: Can an older app clear the choice by accident? → A: No. `PUT /templates/{id}` replaces the template, but a routine key that is absent from the body keeps the stored value. `null` or `""` clears it.
- Q: Do programs get the routine choice? → A: Not in this feature. A program unit copied from a saved workout keeps its timed exercises (programs already store seconds per set) but not the routine choice; program sessions keep the automatic pick.
- Q: Did programs already have timed holds? → A: The schema did (`program_exercises.target_duration_seconds`), and nothing used it for strength work: "Spanish squat hold · 45 s" is an exercise **name** with one rep per set. No program content changes here.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A stretch counted in seconds (Priority: P1)

The athlete adds "Couch stretch, 2 sets of 30 seconds" to a saved workout and saves it.

**Independent Test**: `POST /api/v1/templates` with an exercise `{sets: 2, duration_seconds: 30}`; read it back.

**Acceptance Scenarios**:

1. **Given** an exercise with `sets` and `duration_seconds`, **When** the workout is saved, **Then** it is stored and returned with both, and without reps.
2. **Given** `duration_seconds` with `reps`, with `duration_minutes`, without `sets`, at 0, or above 3600, **When** it is saved, **Then** the answer is 422 on `exercises` and nothing is stored.
3. **Given** a finished workout whose timed set is logged with `duration_seconds` and no `actual_reps`, **When** the day log is saved, **Then** it is accepted and the set's modality is `strength`.
4. **Given** a day log made from that workout, **When** it is read, **Then** its embedded template exercise carries `duration_seconds`.

### User Story 2 - My own warm-up and cool-down (Priority: P2)

The athlete picks "Hips and hamstrings" as the cool-down of their leg day.

**Independent Test**: save a template with `cooldown_routine_id`, read it back, update it from a body without the key, read it again.

**Acceptance Scenarios**:

1. **Given** a slug in `warmup_routine_id` or `cooldown_routine_id`, **When** the workout is saved, **Then** `GET /templates/{id}` returns it.
2. **Given** a workout with a chosen routine, **When** it is updated with a body that has no routine keys, **Then** the choice is kept.
3. **Given** the same workout, **When** it is updated with `null` or `""`, **Then** the choice is cleared and the field reads `null`.
4. **Given** a value that is not a 1 to 64 character lowercase slug, **When** it is saved, **Then** the answer is 422 naming the field.

### User Story 3 - Nothing is lost elsewhere (Priority: P2)

**Acceptance Scenarios**:

1. **Given** the data export, **When** it is read, **Then** each template carries both routine fields and each exercise carries `duration_seconds`.
2. **Given** a legacy weekly plan with a timed exercise, **When** it is adopted into a program, **Then** the program exercise is `strength` with the seconds as `target_duration_seconds` and no reps.

### Edge Cases

- A template saved by an older app has neither field: both routine fields read `null`, and no exercise has `duration_seconds`.
- A slug the current app does not know (content removed later): the API returns it; the app falls back to the automatic pick.
- A rep exercise and a timed exercise in one workout: each keeps its own shape.

## Non-Goals *(mandatory)*

- A server-side stretch or routine catalog, or validation of a slug against one.
- A routine choice on programs, program units, or scheduled sessions.
- Recording that the chosen routine was done as part of the workout. A finished routine is a mobility session, as before.
- A timed target on `set_logs` (the day-log path stores the performed seconds only, as it does for reps).
- Changing starter, generated, or Knee Kit program content to use seconds.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A template exercise MUST be exactly one of: strength (`sets` and `reps`), timed (`sets` and `duration_seconds`), or cardio (`duration_minutes`). Any other combination MUST be refused with 422 on `exercises`.
- **FR-002**: `duration_seconds` MUST be a whole number from 1 to 3600 and means seconds per set. The database MUST refuse a value outside that range.
- **FR-003**: `intensity` MUST stay valid only with `duration_minutes`.
- **FR-004**: A template MUST carry `warmup_routine_id` and `cooldown_routine_id`, each `null` or a 1 to 64 character slug of lowercase letters, digits, and single hyphens. The database MUST refuse anything else.
- **FR-005**: `GET /templates/{id}` and the `POST` and `PUT` responses MUST always include both routine keys.
- **FR-006**: On `PUT`, an absent routine key MUST keep the stored value; `null` or `""` MUST clear it.
- **FR-007**: The day-log template, the data export, and legacy weekly-plan adoption MUST carry the new fields.
- **FR-008**: The change MUST be additive: every request an app older than contract 1.13.0 sends keeps its meaning, and no existing row is rewritten.

### Key Entities

- **Template exercise** (`exercises`): gains `duration_seconds`.
- **Saved workout** (`workout_templates`): gains `warmup_routine_id` and `cooldown_routine_id`.

## Success Criteria *(mandatory)*

- **SC-001**: A workout with a timed exercise and a chosen cool-down saves and reads back unchanged.
- **SC-002**: The released app (before its update) saves, edits, and logs workouts exactly as before, and an edit from it does not clear a chosen routine.
- **SC-003**: Migration 049 applies, rolls back, and re-applies cleanly.

## Assumptions

- The app owns routine content and its ids; the slug rule matches `mobility_sessions.routine_id`.
- One athlete uses the service today, so no backfill or staged rollout is needed beyond API first.

## Rollout

1. API: migration 049 runs in the pre-deploy step, then the server starts (the startup schema check needs 049).
2. App: uses the new fields only after it sees the routine keys on a template it reads.
