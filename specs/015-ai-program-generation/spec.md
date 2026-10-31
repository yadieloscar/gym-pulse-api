# Feature Specification: Generate a Program Draft

**Feature Branch**: `spec/015-ai-program-generation`

**Feature ID**: `gympulse-ai-program-generation`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/019-ai-program-generation/spec.md)

**Created**: 2026-10-31

**Status**: Draft (owner direction: "start the cross-repository spec for AI program generation using the approach you recommended")

**Input**: User description: "Can we integrate some sort of AI somehow? Kind of it creates a basic program for you?"

## Clarifications

### Session 2026-10-31

Decisions derived from the owner's request, the workspace `docs/PRODUCT.md` (consistency lens, no
surprising automation, privacy by design, athlete-set targets), and the existing goal-based training
contract. The owner can override any of them.

- Q: What does "AI creates a program" mean in GymPulse's data? → A: A one-shot draft of a personal
  program built from the athlete's saved training profile (goal, weekdays, experience, equipment,
  session length). The draft uses the same shape as a custom program and only exercises from the
  exercise catalog, so the player, records, and history work unchanged once it is saved.
- Q: Is the draft saved automatically? → A: No. Generation is read-only. The athlete reviews the
  draft and chooses to use it; only then is it saved through the existing custom-program create,
  which activates it like any other chosen program. Leaving the review discards the draft.
- Q: Where does the generation run? → A: In the API. The app never talks to an AI provider and never
  holds a provider key. The API sends the provider only the training profile fields, the exercise
  catalog, and the athlete's optional one-line request; never identity, email, history, body
  metrics, nutrition, or health data.
- Q: Can the athlete steer it? → A: Yes, with one optional short request such as "upper body
  focus" or "no barbell squats", and by asking for another draft. There is no chat.
- Q: Is it a recommendation? → A: It is presented as a starting point the athlete reviews, in the
  same way the protein suggestion and the knee tendon starter are. The response carries a short
  rationale in plain language and the app labels it as a suggestion with a not-medical-advice line.
- Q: What happens when the provider is down or generation is not configured? → A: A stable
  "unavailable" answer; the athlete can still choose a starter or build a program by hand. Nothing
  about the athlete's plan changes.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Get a draft program from my profile (Priority: P1)

An athlete with a saved training profile asks GymPulse for a program. Within a short wait they
receive a draft: a name, one workout per training day they chose (up to the days they have), each
workout with a handful of catalog exercises carrying sets, reps or duration, and rest, plus a short
rationale explaining the split. The draft fits their equipment and session length.

**Why this priority**: This is the feature. Without a usable draft the entry point has no value.

**Independent Test**: Save a profile (hypertrophy, Monday/Wednesday/Friday, intermediate, barbell
and dumbbells, 60 minutes), request a draft, and verify three workouts arrive, every exercise names
an exercise-catalog entry, every strength exercise has sets and reps, and the response is accepted
unchanged by the custom-program create.

**Acceptance Scenarios**:

1. **Given** an athlete with a saved training profile, **When** they request a draft, **Then** the
   response contains a program draft whose workout count is between one and the number of
   available days, in the custom-program shape, with a rationale.
2. **Given** the draft, **When** each exercise is checked, **Then** it references an existing
   exercise-catalog entry by identity and name, its modality matches the catalog, strength exercises
   carry sets and reps, cardio exercises carry sets and a duration, and rest is present.
3. **Given** the draft, **When** it is submitted unchanged as a custom program create with an
   operation key, **Then** a program is created exactly as any other custom program.
4. **Given** an athlete with no saved training profile, **When** they request a draft, **Then** the
   request is rejected with the documented validation answer naming the missing profile and nothing
   is generated.
5. **Given** the generator returns an exercise that is not in the catalog, a workout with no
   exercises, or more workouts than available days, **When** the draft is checked, **Then** the API
   repairs what it safely can (drops the unknown exercise) or rejects the draft and answers
   "unavailable" rather than returning an invalid draft.

---

### User Story 2 - Ask for a different draft (Priority: P2)

The athlete adds one short request ("upper body focus", "no barbell squats", "shorter sessions")
and asks again. The new draft reflects the request within the same catalog and profile limits.

**Why this priority**: The first draft will not suit everyone; a bounded steer avoids an edit-every-
exercise flow while keeping the athlete in control.

**Independent Test**: Request a draft with the request "no deadlifts" and verify no workout names
a deadlift; request one with "upper body focus" and verify the rationale and exercise categories
reflect it.

**Acceptance Scenarios**:

1. **Given** a saved profile, **When** the athlete sends a request text of 1 to 200 characters,
   **Then** the draft is produced with that request considered and echoed back.
2. **Given** a request text over 200 characters or containing only whitespace, **When** it is
   sent, **Then** it is rejected with the documented validation answer.
3. **Given** the request asks for something outside GymPulse's scope (a diet, a medical opinion,
   a sport the catalog does not cover), **When** the draft is produced, **Then** it still contains
   only catalog exercises and its rationale says in one line what could not be honored.

---

### User Story 3 - Fair use and safe failure (Priority: P3)

Generation costs money and depends on an outside service. GymPulse limits how many drafts one
athlete can request per day and fails clearly when the service is unavailable.

**Why this priority**: Protects the product from runaway cost and abuse without making the normal
athlete notice.

**Independent Test**: Request six drafts in one local day and verify the sixth receives the
documented limit answer; disable generation and verify the request receives the documented
"unavailable" answer with no change to the athlete's data.

**Acceptance Scenarios**:

1. **Given** an athlete who has received five drafts in their current local day, **When** they
   request a sixth, **Then** the request is rejected with a stable limit answer stating when they
   can try again, and no provider call is made.
2. **Given** generation is not configured, the provider fails, or the provider does not answer
   within the wait limit, **When** a draft is requested, **Then** the request receives a stable
   "unavailable" answer within the wait limit, nothing is stored, and the attempt does not count
   against the daily limit.
3. **Given** another athlete, **When** they request drafts, **Then** the limit is counted for each
   athlete separately.

### Edge Cases

- A profile with one available day yields a single full-body workout.
- A profile with seven available days yields at most six workouts; the generator is told to leave
  rest and the draft never exceeds the available days.
- Equipment "bodyweight" only: every exercise must be performable without equipment. The catalog
  has no equipment tags, so the API instructs the generator and the athlete reviews; see
  Assumptions.
- Session length 20 minutes: the draft contains fewer exercises per workout; the API bounds
  exercises per workout at 3 to 8.
- Duplicate exercises inside one workout are collapsed to the first occurrence.
- Target weight is never generated; it stays empty and the athlete fills it while training, as with
  starters.
- Two requests arriving at once from the same athlete are both served or both limited correctly;
  the daily count never undercounts.
- Account deletion removes any generation records with the account.

## Non-Goals *(mandatory)*

- Persisting drafts, a draft history, or resuming an unsaved draft after leaving the review.
- Chat, follow-up questions, or multi-turn refinement; one optional request line per draft.
- Editing exercises in the draft before saving; the athlete regenerates, saves and later edits, or
  chooses a starter instead. Exercise-level editing of owned programs is a separate specification.
- Adaptive programming: no progression rules, no changes based on logged history, records, body
  weight, or nutrition. The draft is static like a starter copy.
- Any change to the schedule, the active program, or the training profile as a side effect of
  generating a draft.
- Equipment or muscle tags on the exercise catalog; a follow-up may add them to tighten validation.
- Generating training blocks, sport days, templates, nutrition targets, or anything other than a
  program of catalog exercises.
- Provider choice or model tuning as product behavior; those are design decisions recorded in the
  plan and may change without a spec change as long as this behavior holds.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: An authenticated athlete with a saved training profile MUST be able to request a
  program draft built from that profile, optionally with one request text of 1 to 200 characters.
- **FR-002**: The draft MUST use the custom-program create shape (name, primary goal, workouts with
  ordered exercises) so it can be submitted unchanged, plus a rationale of at most 500 characters
  and the request text echoed back when one was sent.
- **FR-003**: The draft MUST contain between one and the athlete's number of available days of
  workouts (at most six), each with 3 to 8 exercises, each exercise naming an existing exercise-
  catalog entry by identity, name, category, and modality, with sets, reps for strength or duration
  for cardio, rest, no target weight, and no duplicate exercises within a workout.
- **FR-004**: The draft's primary goal MUST equal the profile's primary goal, and the draft name
  MUST be 1 to 80 characters.
- **FR-005**: The API MUST validate every generated draft against FR-003 and FR-004, dropping
  unknown or duplicate exercises and surplus workouts where the result still satisfies FR-003, and
  otherwise MUST answer "unavailable" without returning the draft.
- **FR-006**: Generation MUST NOT create, change, or deactivate programs, schedules, scheduled
  workouts, templates, or the training profile.
- **FR-007**: The API MUST send the provider only the training profile fields, the exercise
  catalog, and the request text; it MUST NOT send the athlete's identity, contact details, workout
  history, records, body metrics, nutrition, health, or training-block data.
- **FR-008**: An athlete MUST receive at most five drafts per local calendar day in their profile
  timezone; further requests that day MUST receive a stable limit answer that states when the limit
  resets, and MUST NOT reach the provider.
- **FR-009**: When generation is not configured, the provider fails, the draft fails validation, or
  the provider does not answer within 45 seconds, the request MUST receive a stable "unavailable"
  answer, the attempt MUST NOT count toward FR-008, and nothing MUST be stored for it.
- **FR-010**: A missing training profile MUST produce the documented validation answer naming the
  profile; no provider call is made.
- **FR-011**: The provider key MUST live only in API configuration; it MUST NOT appear in
  responses, logs, fixtures, documentation, or the app.
- **FR-012**: Generation records needed for FR-008 MUST be scoped to the athlete and MUST be
  removed by account deletion.
- **FR-013**: `docs/CONTRACTS.md` MUST describe the request, response, and each stable answer in
  the same change as the implementation.

### Key Entities

- **Program draft**: An unsaved program in the custom-program create shape, with a rationale and
  the optional request text; exists only in the response.
- **Generation attempt**: A per-athlete record of one successful draft in a local day, used only
  for the daily limit; carries no draft content.
- **Exercise catalog**: The existing read-only list of exercises; the only source of draft
  exercises.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For the six valid goals crossed with 1, 3, and 6 available days, every draft in a
  fixed test set passes validation and is accepted unchanged by the custom-program create.
- **SC-002**: 100% of exercises in accepted drafts resolve to an exercise-catalog entry.
- **SC-003**: A draft arrives within 45 seconds, and the "unavailable" answer arrives within the
  same limit when the provider does not respond.
- **SC-004**: No request to the provider contains an athlete identifier, email, or any field
  outside the training profile, catalog, and request text, as shown by a recorded test request.
- **SC-005**: The sixth draft request in one local day is refused before any provider call.
- **SC-006**: Existing program, starter, schedule, and plan-transition acceptance scenarios pass
  unchanged.

## Assumptions

- The exercise catalog is small enough to send whole with each request; if it grows beyond a few
  hundred entries the plan may filter by modality or category without changing this behavior.
- Equipment fit cannot be verified by the API because the catalog has no equipment tags; the
  generator is instructed with the profile's equipment and the athlete reviews the draft. Adding
  catalog tags is a follow-up specification.
- The daily limit of five and the 45-second wait are product defaults the plan may expose as
  configuration; changing the numbers is not a spec change.
- The app presents the draft as "A starting point from your profile" with the line "For your own
  training, not medical advice"; copy lives in the app specification.
- The feature is off when no provider key is configured; the app learns this through the
  "unavailable" answer and hides or disables the entry accordingly (app specification).
- Idempotency keys are not required because generation is read-only and non-deterministic; saving
  the draft uses the existing idempotent custom-program create.
