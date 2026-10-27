# Feature Specification: Daily nutrition totals and a chosen target

**Feature Branch**: `spec/012-nutrition-targets`

**Feature ID**: `gympulse-nutrition-targets`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/015-nutrition-targets/spec.md)

**Created**: 2026-10-25

**Status**: Approved (defaults confirmed by the user on 2026-10-25)

**Input**: User description: "Add a macros and diet feature. Option A: protein-first daily totals with Apple Health import. Amend PRODUCT.md and start the cross-repo spec under feature ID gympulse-nutrition-targets."

Governing product rules: workspace `docs/PRODUCT.md`, section "Nutrition" (added 2026-10-25). Totals not meals; the athlete sets the target; no adaptive coaching; nutrition stays one step away from the daily loop.

## Clarifications

### Session 2026-10-25

- Q: Does the service talk to Apple Health or a food database? → A: No. The device reads the health store and sends GymPulse one set of totals per day, marked as coming from Health. GymPulse holds no food, meal, or portion data.
- Q: What is stored per day? → A: One row per athlete per calendar date: energy (kcal), protein, carbohydrate, and fat (grams). Any of the four may be absent; a day with no values at all is deleted rather than kept empty.
- Q: What happens when the athlete edits a day that Health also reports? → A: The athlete's entry wins. A day the athlete typed or edited is never overwritten by an import; an import for that date is acknowledged and ignored.
- Q: Does GymPulse compute or adjust the target? → A: No. The target is whatever the athlete saved. The suggestion shown before they save (protein per kilogram of body weight) is computed on the device from body weight the athlete already recorded, and only the saved number reaches the service.
- Q: Is the date in the request body or the address? → A: Always in the address: a day is saved by its date. The athlete's local today is used only to reject future dates. (Recorded during planning, 2026-10-25; FR-002 amended.)
- Q: Which date does "today" mean? → A: The athlete's local date in their training-profile timezone, as sport activities already do. Dates in the future are rejected.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Record and read a day's totals (Priority: P1)

An athlete enters their calories and macros for a day from their phone. GymPulse stores one set of totals for that date, returns it with every later read of that date range, and lets the athlete correct it later without losing the rest of their history.

**Why this priority**: Everything else (targets, weekly review, Health import) is meaningless without a trustworthy per-day record.

**Independent Test**: Save totals for today, read the last seven days and see exactly one entry for today, save a corrected protein value with the returned revision, and confirm the other three values are unchanged and the revision advanced.

**Acceptance Scenarios**:

1. **Given** an authenticated athlete with no entry for a date, **When** they save totals for that date, **Then** the response carries the stored totals, the date, a revision of one, and when it was recorded.
2. **Given** an existing entry, **When** the athlete saves new totals with the current revision, **Then** the entry is replaced, the revision advances, and the recorded time updates.
3. **Given** an existing entry, **When** a save arrives with an out-of-date revision, **Then** the request is rejected with the documented stale-revision conflict and the stored entry does not change.
4. **Given** a save that was already applied, **When** the same save is retried with the same operation identity and payload, **Then** the original response is replayed and nothing is written twice.
5. **Given** a date after the athlete's local today, **When** totals are saved for it, **Then** the request is rejected and nothing is stored.
6. **Given** entries across several weeks, **When** the athlete reads a date range, **Then** only entries inside the inclusive range are returned, oldest first, and an empty range returns an empty collection.
7. **Given** another athlete's entry, **When** it is read or changed by its date through a different account, **Then** that account sees only its own data; another athlete's entry can never be observed or changed.

---

### User Story 2 - Keep the athlete's chosen target (Priority: P2)

The athlete saves the daily target they chose: protein in grams, optionally energy, carbohydrate, and fat. GymPulse keeps the current target and when it took effect so past weeks are judged against the target that applied then.

**Why this priority**: "Did I hit my protein today?" is the single question the feature answers; without a stored target the weekly view cannot be computed.

**Independent Test**: Save a protein target, read it back, save a new target a week later, and confirm the first week still reports against the first target.

**Acceptance Scenarios**:

1. **Given** no target, **When** the athlete reads their target, **Then** the response says there is none.
2. **Given** no target, **When** the athlete saves protein 150 g effective today, **Then** the current target is 150 g protein with energy, carbohydrate, and fat unset.
3. **Given** a current target, **When** the athlete saves a different target with today's date, **Then** the new target applies from today and the previous one is retained for earlier dates.
4. **Given** a current target, **When** the athlete saves the same values again, **Then** nothing changes and the response is the current target.
5. **Given** a target that was saved with a wrong value today, **When** the athlete saves a corrected value with the same effective date, **Then** today's target is replaced rather than a second target for the same day being added.
6. **Given** no activity from the athlete, **When** time passes or body weight changes, **Then** the stored target is unchanged.

---

### User Story 3 - Bring in totals the phone read from Health (Priority: P2)

An athlete who logs meals in another app connects Apple Health in GymPulse. The phone reads each day's dietary totals from Health and sends them to GymPulse as that day's entry, marked as coming from Health, so the athlete never retypes a number.

**Why this priority**: Most athletes who care about macros already log them elsewhere. Import removes the main reason not to use GymPulse for the weekly view.

**Independent Test**: Send Health totals for yesterday, read them back marked as from Health, send a manual edit for yesterday, then send Health totals for yesterday again and confirm the manual edit stands.

**Acceptance Scenarios**:

1. **Given** no entry for a date, **When** Health totals are sent for it, **Then** an entry is created marked as from Health.
2. **Given** an entry from Health, **When** newer Health totals are sent for the same date, **Then** the entry is updated and still marked as from Health.
3. **Given** an entry the athlete typed or edited, **When** Health totals are sent for that date, **Then** the request succeeds, the stored entry is returned unchanged, and nothing is written.
4. **Given** an entry from Health, **When** the athlete saves their own totals for that date, **Then** the entry becomes the athlete's and is no longer overwritten by imports.

### Edge Cases

- A value that is negative, non-numeric, or implausibly large (energy above 20,000 kcal or any macro above 2,000 g) is rejected with a field-level validation error.
- A save with all four values absent deletes the day's entry if one exists and otherwise does nothing.
- A date range longer than 366 days or with `to` before `from` is rejected with a field-level validation error, matching other date-range reads.
- A target with protein absent or zero is rejected; protein is the one required part of a target.
- A target effective date in the future is rejected.
- Deleting the account removes every nutrition entry and target with it.
- A Health import for a date in the future is rejected the same way a manual save is. There is no lower bound on how old a date may be.
- An operation retried with the same identity but a different payload is rejected with the documented idempotency conflict.

## Non-Goals *(mandatory)*

- No food, meal, recipe, portion, or barcode data. GymPulse stores totals only.
- No estimate of energy expenditure, no adaptive or recomputed target, no eating recommendation.
- No micronutrients, water, fibre, alcohol, or sugar. Four values only in this release.
- No write-back of nutrition to Apple Health. Health is read on the device; GymPulse never writes dietary data.
- No change to body weight, sport activities, workouts, participation, consistency targets, or records. Nutrition does not count toward training consistency.
- No reminders, notifications, streaks, or weekly recap content about nutrition.
- No sharing, coach access, or export of nutrition data.
- Google Health Connect is out of scope; the source marker is designed to admit it later.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The service MUST store at most one nutrition entry per athlete per calendar date, holding optional energy (kcal), protein, carbohydrate, and fat (grams), a source (athlete or Health), a revision, and the time it was last recorded.
- **FR-002**: Saving an entry MUST address it by its calendar date and MUST reject dates after the athlete's local today in their training-profile timezone.
- **FR-003**: Saving an existing entry MUST require the current revision and MUST reject a stale revision with the documented revision conflict, returning the current revision.
- **FR-004**: Saves MUST honour the documented idempotency behaviour: an identical retry replays the original response; a different payload under the same key is rejected with the documented idempotency conflict.
- **FR-005**: A save in which all four values are absent MUST delete the day's entry if present and succeed without writing otherwise.
- **FR-006**: Reading entries MUST accept an inclusive date range of at most 366 days and MUST return entries oldest first as a collection that is never null.
- **FR-007**: Health imports MUST be addressed by date and marked as from Health. When the stored entry for that date belongs to the athlete, the import MUST succeed, return the stored entry unchanged, and write nothing.
- **FR-008**: An athlete's own save over an entry from Health MUST convert the entry to the athlete's and MUST protect it from later imports.
- **FR-009**: The service MUST store the athlete's chosen target (protein required; energy, carbohydrate, and fat optional) with the date it took effect, MUST keep earlier targets, and MUST replace rather than duplicate a target saved for the same effective date.
- **FR-010**: Reading the target MUST return the target in effect today, or state that there is none. Reading entries for a range MUST make it possible to know which target applied on each date in the range.
- **FR-011**: The service MUST never change a target or an entry without an athlete-initiated request.
- **FR-012**: Every nutrition resource MUST be scoped to the authenticated athlete; a missing date or another athlete's data MUST produce the same documented not-found behaviour, and account deletion MUST remove all nutrition data.
- **FR-013**: All values MUST be validated as non-negative numbers within documented bounds, and validation failures MUST name the offending field.
- **FR-014**: The public contract for every behaviour above MUST be documented in `docs/CONTRACTS.md` in the same change as the implementation, including request fields, validation, statuses, and error codes.

### Key Entities *(include if feature involves data)*

- **Nutrition day**: an athlete's totals for one calendar date. Attributes: date, energy, protein, carbohydrate, fat, source (athlete or Health), revision, recorded time. Unique per athlete and date.
- **Nutrition target**: the daily values the athlete chose, effective from a date. Attributes: effective date, protein (required), energy, carbohydrate, fat, revision. Unique per athlete and effective date; the latest effective date on or before a given day is the target for that day.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An athlete can save a day's totals and see them in a range read within one second on a normal connection, in a single round trip each.
- **SC-002**: Retrying a save after a network timeout never produces a duplicate or changed entry; 100% of identical retries replay the original result in automated tests.
- **SC-003**: A concurrent edit from a second device is never silently lost; the stale save is rejected with a conflict in 100% of automated cases.
- **SC-004**: A Health import never overwrites an athlete-entered day in any automated scenario.
- **SC-005**: An athlete who deletes their account leaves zero nutrition rows behind.
- **SC-006**: Clients that predate this feature keep working unchanged: no existing endpoint, field, or error changes.

## Assumptions

- The athlete's timezone already exists in the training profile; local today is derived from it exactly as for sport activities.
- Energy is exchanged in kilocalories and macros in grams; the device converts kilojoules from Health before sending.
- One entry per day is enough; per-meal history is explicitly not wanted.
- The protein-per-kilogram suggestion lives on the device and is never stored or computed by the service.
- The existing idempotency ledger, revision conflict shape, ownership checks, row-level security, and account-deletion cascade patterns are reused without change.
- The API ships and is deployed before the app surfaces depend on it; the change is purely additive.
