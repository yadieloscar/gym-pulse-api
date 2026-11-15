# Feature Specification: Saved meals (store an athlete's own meals and log one in a single request)

**Feature Branch**: `spec/028-saved-meals`

**Feature ID**: `gympulse-saved-meals`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/033-saved-meals/spec.md)

**Depends on**: API spec 014 `gympulse-food-logging` (foods, food entries, day totals), shipped on `main`.

**Created**: 2026-11-15

**Status**: Approved (owner, 2026-11-15: scope "Yes, saved meals", then "Confirm defaults")

**Input**: User description: "For the macros, I would like to also have predetermined meals I can log." Then, after the two readings of "predetermined" were set out (meals the athlete saves, or meals GymPulse supplies): "Yes, saved meals".

Governing product rules: workspace `docs/PRODUCT.md`, section "Nutrition" (amended 2026-11-15): "Saved meals are the athlete's own" and "Logging stays fast". Unchanged and still binding: "Foods add up to the day", "Trustworthy food data", and "Not a diet product" (no meal plans, recipes, or prescribed diets).

## Clarifications

### Session 2026-11-15

- Q: Who makes a meal? → A: Only the athlete. A saved meal is a name and a list of foods, each with a serving and an amount. GymPulse supplies, suggests, and shares no meals.
- Q: What is in a meal? → A: Foods the service already knows (USDA, Open Food Facts, or the athlete's own). A quick add has no food behind it, so it cannot be a meal item; a meal's values are always the sum of real foods.
- Q: What does logging a meal store? → A: One ordinary food entry per item, on the requested date, exactly as if the athlete had logged each food. The day keeps no "meal" grouping and the entries keep no link to the meal, so spec 014's entry, total, edit, and delete rules apply unchanged and there are still no meal slots.
- Q: Which facts does a logged meal use? → A: Each food's current facts at the moment of logging, as spec 014 already does for a recent food. A meal stores which food, serving, and amount; it never stores its own copy of energy or macros.
- Q: Is logging a meal all or nothing? → A: Yes. Every item is logged in one transaction, or none is, and a retry of the same operation never logs it twice.
- Q: Can the athlete change the amounts for one log without changing the saved meal? → A: Yes. The log request may carry its own item list (changed amounts, items left out). The saved meal stays as it was.
- Q: What if a food in a meal was archived? → A: A private food the athlete archived still logs from a meal that already contains it, as it does from recents. It cannot be added to a meal afterwards.
- Q: Limits? → A: Up to 50 saved meals per athlete, 1 to 20 items per meal, names of 1 to 60 characters that are unique for that athlete ignoring case.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Log a saved meal in one request (Priority: P1)

An athlete who saved "Usual breakfast" (oats, whey, a banana) logs it to today. Three food entries appear on the day and the day's totals rise by their sum.

**Why this priority**: Logging the same few meals without re-entering each food is the whole reason for the feature.

**Independent Test**: Save a meal with three foods, log it to today, read the day's entries, and see three new entries whose values equal each food's facts times its serving and amount, and totals equal to the sum of all entries.

**Acceptance Scenarios**:

1. **Given** a saved meal, **When** the athlete logs it to a date that is today or earlier, **Then** one food entry per item is stored on that date with the food's facts at that moment, in the meal's item order, and the day's totals equal the sum of the day's entries.
2. **Given** a log request that carries its own item list, **When** it is accepted, **Then** those items are logged and the saved meal is unchanged.
3. **Given** a retry with the same operation identity, **When** it arrives, **Then** the original answer is returned and nothing is logged twice.
4. **Given** one item cannot be logged (for example its food no longer resolves), **When** the meal is logged, **Then** nothing is logged and the answer names the item at fault.
5. **Given** a date that held typed totals and no entries, **When** a meal is logged to it, **Then** the typed totals become one quick-add entry first, as spec 014 already does, so nothing the athlete saved disappears.

---

### User Story 2 - Save a meal (Priority: P1)

The athlete names a set of foods with servings and amounts and saves it.

**Why this priority**: Nothing can be logged until a meal exists.

**Independent Test**: Create a meal with two foods, read saved meals, and see it with its items, each item's food, serving, amount, and the meal's current totals.

**Acceptance Scenarios**:

1. **Given** a name and 1 to 20 items that each name a food the athlete can use, a serving of that food, and an amount from 0.1 to 50, **When** the athlete saves, **Then** the meal is stored as theirs and returned with its items in the order given.
2. **Given** a name the athlete already uses for another saved meal (ignoring case), **When** they save, **Then** the save is rejected with a field-level validation error on the name.
3. **Given** 50 saved meals, **When** the athlete saves another, **Then** the save is rejected with a stable limit error and nothing changes.
4. **Given** a retry with the same operation identity, **When** it arrives, **Then** the original meal is returned and no second meal exists.

---

### User Story 3 - See saved meals (Priority: P1)

The athlete reads their saved meals to choose one.

**Why this priority**: The app needs the list, with enough detail to show and log a meal without further requests.

**Independent Test**: With two saved meals, one logged yesterday and one never logged, read the list and see the logged one first, each with its items and current energy and macros.

**Acceptance Scenarios**:

1. **Given** saved meals, **When** the athlete reads them, **Then** each is returned with its name, items (food, serving, amount, and the item's current energy and macros), the meal's current totals, and when it was last logged, ordered by most recently logged and then most recently saved.
2. **Given** no saved meals, **When** the athlete reads them, **Then** an empty list is returned, never an error.
3. **Given** a food in a meal whose facts changed in the catalog, **When** the list is read, **Then** the meal's totals reflect the current facts.

---

### User Story 4 - Change or remove a saved meal (Priority: P2)

The athlete renames a meal, changes an amount, adds or removes a food, or deletes the meal.

**Why this priority**: Meals drift over time; correcting one must be safe.

**Independent Test**: Log a meal, change one item's amount with the current revision, and see the earlier day unchanged; send a stale revision and see a conflict; delete the meal and see past entries remain.

**Acceptance Scenarios**:

1. **Given** a saved meal, **When** the athlete changes it with the current revision, **Then** the meal updates, its revision advances, and no day already logged changes.
2. **Given** a stale revision, **When** a change or delete arrives, **Then** it is rejected with the documented revision conflict and nothing changes.
3. **Given** a saved meal, **When** it is deleted, **Then** it no longer appears in the list and every entry it ever logged stays on its day.

### Edge Cases

- A meal with no items, more than 20 items, or the same food and serving listed twice is rejected with a field-level validation error. The same food with two different servings is allowed.
- An item that names an unknown food, another athlete's private food, or an archived private food is rejected when a meal is created or changed, with the same answer for all three so nothing is revealed about foods the athlete cannot use.
- A serving that is neither one of the food's servings nor a grams-only serving is rejected, as for a single entry in spec 014.
- A log to a future date is rejected, following spec 012's date rules.
- Logging the same meal twice on one day on purpose (two operation identities) logs it twice; that is the athlete's choice.
- A saved meal's totals are never stored. A food whose current facts fail spec 014's plausibility checks is not returned with wrong numbers; the meal is returned with that item marked unavailable and without totals.
- An older app that knows nothing of saved meals is unaffected: entries a meal logged look like any other food entry.
- Deleting the account removes the athlete's saved meals. Shared catalog foods stay.
- The account data export (spec 025) includes saved meals.

## Non-Goals *(mandatory)*

- No meals supplied, suggested, ranked, or shared by GymPulse; no meal library, templates, or copying another athlete's meal.
- No recipes: no instructions, yields, portions of a cooked dish, photos, or ingredients other than foods.
- No meal plans or schedules: a saved meal has no day, time, or meal slot (breakfast, lunch, dinner), and the service never logs one on its own.
- No "meal" grouping stored on a day, and no link from an entry back to the meal that logged it.
- No quick-add items inside a meal, and no stored energy or macro values on a meal.
- No change to targets, the weekly view, Health import, food search, barcode lookup, or recents.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The service MUST let an athlete create a saved meal with a name of 1 to 60 characters and 1 to 20 ordered items, each a food the athlete can use, a serving valid for that food, and an amount from 0.1 to 50.
- **FR-002**: A saved meal's name MUST be unique among that athlete's saved meals ignoring case, and an athlete MUST NOT hold more than 50 saved meals; both violations MUST return stable, documented errors.
- **FR-003**: The service MUST return the athlete's saved meals with name, ordered items (food identity and display fields, serving, amount, current energy and macros), current meal totals, revision, and last-logged time, ordered by most recently logged and then most recently saved, and an empty list when there are none.
- **FR-004**: The service MUST let the athlete replace a saved meal's name and items and delete a saved meal, both under the documented revision rule; a stale request MUST be rejected with the documented revision conflict.
- **FR-005**: The service MUST log a saved meal to a date in one request, creating one food entry per item with spec 014's entry rules (facts taken at logging time, snapshot stored on the entry), in item order.
- **FR-006**: The log request MAY carry its own item list, validated like a meal's items, that replaces the saved items for that log only and MUST NOT change the saved meal.
- **FR-007**: Logging a meal MUST be atomic: every entry and the day's totals change in one transaction, or nothing changes and the error identifies the item at fault.
- **FR-008**: The day's totals MUST equal the sum of the day's entries after a meal is logged, and typed totals on that date MUST first be converted as spec 014 requires.
- **FR-009**: Creating, changing, deleting, and logging a saved meal MUST follow the documented idempotency rules; a verbatim retry MUST replay the original answer.
- **FR-010**: Entries a meal logged MUST be indistinguishable from other food entries, so every existing entry read, change, and delete behaviour applies to them unchanged.
- **FR-011**: Changing or deleting a saved meal, or a food inside it, MUST NOT change any entry already logged.
- **FR-012**: An archived private food MUST still log from a saved meal that already contains it and MUST be refused as a new item.
- **FR-013**: The service MUST NOT store energy or macro values on a saved meal; values shown for a meal MUST be computed from its foods' current facts.
- **FR-014**: Every saved meal MUST be scoped to the authenticated athlete with the shared not-found behaviour for unknown and foreign meals, and account deletion MUST remove them.
- **FR-015**: The account data export MUST include the athlete's saved meals.
- **FR-016**: The change MUST be additive: no existing request, response, or error changes, and an app without saved meals keeps working.
- **FR-017**: The public contract for every behaviour above MUST be documented in `docs/CONTRACTS.md` in the same change as the implementation.

### Key Entities *(include if feature involves data)*

- **Saved meal**: a named, ordered list of items owned by one athlete. Name, revision, when it was saved, when it was last logged.
- **Saved meal item**: one food in a meal. Position, food reference (catalog or the athlete's own), serving (label and grams), amount.
- **Food entry** (from spec 014): unchanged. Logging a meal creates one per item.
- **Nutrition day** (from spec 012): unchanged. Still the sum of the date's entries.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A saved meal of up to 20 foods is logged with a single request from the app, and the athlete sees the day updated in under one second on a normal connection.
- **SC-002**: In 100% of automated scenarios, a day's totals equal the sum of its entries after a meal is logged, including two devices logging at once.
- **SC-003**: In 100% of automated scenarios, a failed or repeated meal log leaves the day with either all of the meal's entries once or none of them.
- **SC-004**: In 100% of automated scenarios, changing or deleting a saved meal leaves every previously logged day unchanged.
- **SC-005**: No athlete can read, change, log, or infer another athlete's saved meals or private foods, in automated ownership tests.
- **SC-006**: An app built before this feature passes its existing nutrition and food logging scenarios against the new service without change.

## Assumptions

- Spec 014 is deployed wherever this ships; its food, entry, day-total, idempotency, and revision rules are reused as they stand.
- Logging a meal writes the day's totals once for the whole meal, not once per item.
- The migration takes the next free number at implementation time (039 or later; 038 is the newest on `main` as of 2026-11-15).
- The contract change is an additive minor version after 1.7.0.
- The API ships and deploys before the app behaviour in the sibling spec; the app hides saved meals when the server does not offer them.
- The 50-meal and 20-item limits are generous for one athlete's repeated meals and keep a single log request bounded.
