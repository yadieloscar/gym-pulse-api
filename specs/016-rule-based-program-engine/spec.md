# Feature Specification: Rule-Based Program Engine

**Feature Branch**: `spec/016-rule-based-program-engine`

**Feature ID**: `gympulse-rule-based-program-engine`

**Sibling Spec**: App amendment to [019-ai-program-generation](../../../gym-pulse-app/specs/019-ai-program-generation/spec.md) (structured constraints replace the free-text request; to be written at planning)

**Depends on**: [015-ai-program-generation](../015-ai-program-generation/spec.md) (gym-pulse-api#39): the endpoint, draft shape, validation, and the goal guide this engine implements in code.

**Created**: 2026-11-02

**Status**: Draft (owner direction: "what if we move away from an AI generated integration, and we actually build the intelligence into our systems?" → "Yes, spec it")

**Input**: User description: "Move program generation off the AI provider and build the programming intelligence into GymPulse itself: a deterministic engine that builds the draft from the athlete's answers using the published training guidance."

## Clarifications

### Session 2026-11-02

Decisions derived from the owner's direction, `docs/PRODUCT.md` (static like a starter, no
surprising automation, privacy by design, catalog exercises only), spec 015 and its research
"Goal guide", and the current exercise catalog (79 entries in eight categories). The owner can
override any of them.

- Q: Does the endpoint change? → A: No. `POST /api/v1/program-generation` keeps its draft shape,
  validation, and stable answers. The engine is a second implementation of the generator behind
  the same route and becomes the default. The model-backed generator stays available only when a
  provider key is configured and an operator opts into it; without a key the engine serves and
  nothing is "disabled".
- Q: What happens to the free-text request? → A: Rules cannot read prose, so the request becomes
  structured: `avoid` (movement patterns the athlete wants left out, such as hinge, overhead,
  jumping, or knee-dominant squatting) and `focus` (upper, lower, or core emphasis). The app
  shows these as chips in place of the text field. The free-text `request` stays accepted for the
  model path only and is ignored by the engine.
- Q: What does the engine know about each exercise? → A: The catalog gains programming metadata
  per entry: movement pattern, primary muscle group, equipment required, and impact. That is a
  migration with seeded values for every current entry; the catalog stays read-only in the API.
- Q: Is the draft deterministic? → A: Yes. The same answers always produce the same draft. "Try
  another" sends a `variant` number that rotates the exercise chosen within each pattern, so the
  athlete sees a genuinely different but equally valid draft. There is no randomness.
- Q: Does the daily limit still apply? → A: Not to engine-built drafts. The ledger exists to
  bound provider cost; the engine has none. The status answer keeps its fields for
  compatibility and the limit applies only when the model path serves.
- Q: Where does the programming knowledge live? → A: In code tables in the API (split by goal and
  days, rep, set, and rest ranges per goal and experience, cardio share, secondary-goal share),
  with their sources carried over from spec 015 research "Goal guide". Tables are data, reviewed
  like contracts, and every table value is covered by a test.
- Q: Does the rationale stay? → A: Yes, as templated plain-language sentences assembled from the
  choices the engine made (split, main lifts, what was avoided, what could not be honored).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Get a draft instantly from my answers (Priority: P1)

An athlete answers the four questions and taps "Build my program". The draft appears at once: one
workout per chosen day, each with catalog exercises that fit their equipment and session length,
with sets, reps or duration, and rest taken from the goal guide, and a short rationale naming the
split and the main lifts.

**Why this priority**: This replaces the provider path for every athlete and removes the wait, the
cost, and the daily limit.

**Independent Test**: With a hypertrophy profile on Monday, Wednesday, and Friday, intermediate,
barbell and dumbbells, 60 minutes, request a draft and verify three full-body workouts of 5 to 7
exercises each arrive in under 200 ms, every exercise resolves to a catalog entry the equipment
allows, strength exercises carry 6 to 12 reps and 90 to 180 s rest, and the draft is accepted
unchanged by `POST /api/v1/programs`.

**Acceptance Scenarios**:

1. **Given** a valid profile, **When** a draft is requested, **Then** the response satisfies every
   draft rule in spec 015 FR-002 to FR-005 by construction and arrives without a provider call.
2. **Given** the same answers twice, **When** two drafts are requested, **Then** they are
   identical.
3. **Given** a beginner, **When** a draft is built, **Then** each workout holds fewer exercises,
   uses only compound entries plus core, and skips explosive movements whatever the goal.
4. **Given** bodyweight-only equipment, **When** a draft is built, **Then** every exercise is a
   bodyweight entry and the rationale says the plan is built around bodyweight work.

---

### User Story 2 - Steer the draft with structured constraints (Priority: P2)

The athlete marks patterns to avoid (for example hinge and overhead) and a focus (upper body). The
draft leaves those patterns out, fills the slots from other patterns, and gives the focus area a
larger share, saying so in the rationale.

**Why this priority**: This is what the free-text request did; without it the athlete cannot steer.

**Independent Test**: Request a strength draft with `avoid: ["hinge", "overhead_push"]` and
`focus: "upper"` and verify no deadlift, Romanian deadlift, good morning, kettlebell swing,
overhead press, or landmine press appears, at least two thirds of strength exercises are upper
body, and the rationale mentions both.

**Acceptance Scenarios**:

1. **Given** avoided patterns, **When** a draft is built, **Then** no exercise with any avoided
   pattern appears and the slots are filled from the remaining patterns.
2. **Given** a focus, **When** a draft is built, **Then** the focus area gets about two thirds of
   the strength exercises and the other areas keep at least one.
3. **Given** constraints that leave a pattern slot with no eligible exercise, **When** a draft is
   built, **Then** the slot is dropped, the workout still holds at least three exercises, and the
   rationale says what could not be honored.
4. **Given** an unknown pattern or focus value, **When** a draft is requested, **Then** the answer
   is `422 VALIDATION_ERROR` naming `avoid` or `focus`.

---

### User Story 3 - Try another and get a different valid draft (Priority: P3)

From the preview the athlete asks for another draft with the same answers and receives a different
one: the same split and targets, different exercises chosen within each pattern.

**Why this priority**: Determinism must not mean "the only draft".

**Independent Test**: Request `variant: 0` and `variant: 1` with identical answers and verify the
splits and targets match, at least one exercise differs per workout where the catalog allows, and
both drafts are valid.

**Acceptance Scenarios**:

1. **Given** `variant` n, **When** a draft is built, **Then** each pattern slot takes the n-th
   eligible exercise in catalog order, wrapping when the list is shorter.
2. **Given** a pattern with a single eligible exercise, **When** the variant changes, **Then** that
   slot stays the same and the draft is still returned.

---

### User Story 4 - Primary and secondary goals shape the plan (Priority: P3)

The athlete's primary goal chooses the split and the targets; up to two secondary goals take a
share: cardio entries after lifting or on their own day, or one heavier compound per session.

**Independent Test**: Request strength with secondary conditioning on three days and verify the
targets stay in the strength range, one cardio entry of 10 to 20 minutes ends at most two of the
three workouts, and the rationale names the secondary goal.

**Acceptance Scenarios**:

1. **Given** a secondary goal, **When** a draft is built, **Then** at most about one third of the
   exercises or one day serve it and the primary goal's targets are untouched.
2. **Given** no secondary goals, **When** a draft is built, **Then** the draft contains only the
   primary goal's structure.

### Edge Cases

- One available day: one full-body workout; secondary cardio, if any, ends that workout.
- Six or seven days: at most six workouts (spec 015), the split chosen for six, one rest day.
- A 20-minute session: three exercises, compounds only, cardio secondary dropped with a rationale
  note.
- Equipment `bands` only: the catalog may hold few band entries; the engine falls back to
  bodyweight entries, says so, and never returns fewer than three exercises per workout.
- Equipment `cardio_machine` only with a strength goal: strength slots fall back to bodyweight.
- Power goal for a beginner: strength-style draft with no explosive entry (US1/AC3 governs); the
  rationale says beginners get nothing explosive.
- Conditioning goal with no cardio equipment: bodyweight locomotion entries (jump rope, burpee,
  HIIT circuit) serve the cardio slots.
- Avoiding every pattern that a goal needs: the engine builds what it can (topping a thin workout
  up from any remaining pattern, then isolation work, then cardio) and the rationale lists what
  was dropped; if fewer than three exercises remain for any workout the answer is a documented
  `422 VALIDATION_ERROR` on `avoid` saying the constraints leave too little to build from. With
  at most six avoided patterns this is reachable only on bodyweight-only equipment.
- A catalog entry missing programming metadata: the migration seeds all current entries; a
  database check constraint keeps future entries from being added without it.

## Non-Goals *(mandatory)*

- Reading free text with the engine; prose stays with the optional model path.
- Progression, periodization, deloads, or any change driven by logged history (PRODUCT.md:
  static like a starter).
- Editing the draft before saving; that is a program-editing feature.
- Per-athlete learning, ratings, or history of past drafts.
- Replacing the starter-program catalog; starters remain the curated path.
- Changing the custom-program create, the player, records, or history.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `POST /api/v1/program-generation` MUST serve drafts from the rule engine by default;
  the model path MUST be used only when a provider key is configured and the deployment opts in.
  With the engine serving, `GET /api/v1/program-generation` MUST report `enabled: true`.
- **FR-002**: The engine MUST produce drafts that satisfy spec 015 FR-002 to FR-005 by
  construction (shape, one to `min(available_days, 6)` workouts, 3 to 8 unique catalog exercises
  each, valid targets, primary goal equal to the profile's) and the existing validator MUST still
  run on the result.
- **FR-003**: The engine MUST be deterministic: identical input, including `variant`, MUST
  produce an identical draft, and `variant` MUST rotate the exercise chosen within each pattern.
- **FR-004**: The request MUST accept optional `avoid` (a set of movement patterns), `focus`
  (`upper`, `lower`, or `core`), `secondary_goals` (spec 015 FR-014), and `variant` (integer 0 or
  more); unknown values MUST return `422 VALIDATION_ERROR` naming the field. `request` free text
  MUST remain accepted and MUST be ignored by the engine.
- **FR-005**: The exercise catalog MUST carry, for every entry, a movement pattern, a primary
  muscle group, the equipment required, and an impact level, seeded for all current entries and
  enforced for new ones; `GET /api/v1/exercise-catalog` MUST expose them.
- **FR-006**: The programming tables (split by goal and day count; reps, sets, and rest by goal and
  experience; cardio share; beginner and session-length limits; secondary-goal share) MUST live in
  the API as reviewable data with their sources recorded in research.md, and every table value
  MUST be exercised by a test.
- **FR-007**: The engine MUST fit the session length (about 8 to 10 minutes per strength exercise,
  less for isolation; cardio by its duration), the equipment (an exercise is eligible only when
  every piece of equipment it requires is available; `full_gym` allows everything), and the
  experience (beginners: fewer exercises, compounds plus core, no explosive entries).
- **FR-008**: Avoided patterns MUST never appear; a focus MUST shift about two thirds of strength
  slots to its area; when constraints leave a workout with fewer than three eligible exercises the
  answer MUST be `422 VALIDATION_ERROR` on `avoid` with a plain message.
- **FR-009**: The rationale MUST be plain language of at most 500 characters assembled from the
  engine's choices (split, main lifts, secondary share, what was avoided, what could not be
  honored) and MUST contain no medical, diet, or outcome claims.
- **FR-010**: Engine-built drafts MUST NOT consume the daily generation ledger and MUST complete
  within 200 ms at p95 on the acceptance stack; the handler's extended write deadline is kept for
  the model path only.
- **FR-011**: Nothing about the athlete MUST leave the API for an engine-built draft; logs record
  outcome, duration, and the chosen split only.
- **FR-012**: `docs/CONTRACTS.md` and the OpenAPI contract MUST describe `avoid`, `focus`,
  `variant`, the catalog fields, and the provider rule in the same change as the implementation.

### Key Entities

- **Catalog metadata**: per exercise, `pattern` (squat, hinge, horizontal_push, vertical_push,
  horizontal_pull, vertical_pull, carry, core, explosive, locomotion), `muscle_group` (chest,
  back, shoulders, arms, quads, hamstrings, glutes, core, full_body), `equipment_required` (subset
  of the profile equipment values), `impact` (low or high).
- **Programming tables**: split templates, target ranges, and shares keyed by goal, day count, and
  experience.
- **Generation constraints**: `avoid`, `focus`, `secondary_goals`, `variant`.
- **Program draft**: unchanged from spec 015.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For every combination of the six goals, one to six days, three experience levels,
  and each single equipment value plus `full_gym`, the engine returns a valid draft or the
  documented 422, with no provider call, in an exhaustive test.
- **SC-002**: p95 latency under 200 ms on the acceptance stack; no `ANTHROPIC_API_KEY` is needed
  for the smoke run.
- **SC-003**: The same answers produce byte-identical drafts across runs and across restarts.
- **SC-004**: Every catalog entry has all four metadata fields and the validator script reports no
  gaps.
- **SC-005**: Spec 015's contract tests pass unchanged apart from the additive fields.

## Assumptions

- Spec 015 (gym-pulse-api#39) merges first; this feature amends it rather than replacing the
  route. Migration numbers start after 031.
- The eight catalog categories map onto patterns without renaming any entry; the migration adds
  columns and seeds values, nothing else changes in the catalog table.
- The app sends `avoid`, `focus`, and `variant` once its sibling amendment ships; an older app
  sending only `request` still receives a valid engine draft.
- Kettlebell swings and Olympic-lift variants require `barbell` or `full_gym` as the nearest
  profile equipment value; there is no kettlebell equipment value and none is added here.
