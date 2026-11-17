# Feature Specification: Macro calculator and meal template

**Feature Branch**: `feat/032-macro-calculator`

**Feature ID**: `gympulse-macro-calculator`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/038-macro-calculator/spec.md)

**Created**: 2026-11-16

**Status**: Approved (requested in full by the owner on 2026-11-16; drafting defaults are listed under Clarifications)

**Input**: User description: "Implement an Interactive Macro Calculator & Personalized Meal Plan Generator: a guided flow through biometrics, weekly training, goal, and food preferences that calculates daily targets (Mifflin-St Jeor, activity multiplier, goal adjustment, protein and fat per pound, carbohydrate from the remainder, optional carb cycling) and generates an editable 4-meal daily template with scaled portions and 1-for-1 swaps. `POST /api/nutrition/calculate`, `PUT /api/nutrition/profile`, `GET /api/nutrition/plan`."

Governing product rules: workspace `docs/PRODUCT.md`, section "Nutrition". This feature **changes product defaults explicitly** (amended in the same delivery, 2026-11-16):

- "GymPulse never prescribes a calorie or macro target" and "does not estimate energy expenditure" become: GymPulse may run a calculator **when the athlete asks for it**, from numbers the athlete enters, and show the result as an estimate. Nothing is calculated, saved, or changed without an athlete action.
- "No meal plans" and "GymPulse never supplies or suggests meals" become: the calculator may show a four-meal **template** built from foods the athlete picked, as a starting point they can change. It is not scheduled, scored, reminded, or logged for them.
- "No deficit, surplus, or weight-outcome language" becomes: the calculator may name the calorie adjustment the athlete chose. It still makes no promise about weight or health outcomes.

Rules that stay: the athlete's daily target is whatever they save; targets never change on their own; food facts come from USDA and show their source; no adaptive coaching, streaks, warnings, or reminders.

## Clarifications

### Session 2026-11-16 (drafting defaults; the owner's request fixes everything else)

- Q: Do the routes sit under `/api/nutrition/...` as written? → A: They follow the existing prefix: `/api/v1/nutrition/calculate`, `/api/v1/nutrition/profile`, `/api/v1/nutrition/plan`. One read is added, `/api/v1/nutrition/calculator/options`, so the app shows the same foods, ranges, and defaults the service uses instead of keeping its own copy.
- Q: Which units travel on the wire? → A: The athlete's own: a unit system plus weight and height in that system (pounds and total inches, or kilograms and centimetres). The service stores what was entered and converts for the maths.
- Q: Which number inside a goal's range is the default? → A: The middle: fat loss −400 kcal (range −500 to −300), recomposition −275 (−300 to −250), maintenance 0, muscle gain +300 (+200 to +400). The athlete may pick any whole number inside the range.
- Q: How many weekly sessions map to each activity level? → A: Strength days plus cardio or sport days: up to 3 is lightly active (1.375), 4 or 5 is moderately active (1.55), 6 or more is very active (1.725). The athlete may override the suggestion with any of the three.
- Q: Which days are "training days" for carb cycling? → A: Strength days plus cardio or sport days, at most 7. Cycling needs at least one training day and one rest day; otherwise it is not applied and the response says so.
- Q: How much carbohydrate moves? → A: Rest days carry about 17.5% less carbohydrate than the daily figure (the middle of 15–20%); the grams removed from rest days are added to training days. Whole-gram values are chosen so the seven-day total is exactly seven times the daily figure, so the weekly calorie average is identical.
- Q: Does saving the calculator's result change the daily target the day is judged against? → A: No. The saved plan is its own record. The app offers "use as my daily target", which is the existing athlete-confirmed target save.
- Q: Where do the template foods and their facts come from? → A: A fixed list of 18 foods, each a copy of a USDA FoodData Central record, identified by its FDC id and shown with that source. The list is part of the service, not athlete data.
- Q: Who may use the calculator? → A: Adults. Ages 18 to 100 are accepted; the equation was derived from adults aged 19 to 78 and the response is an estimate.

### Amendment A1, 2026-11-17: template meals can be logged and saved

- Q (owner, after using the shipped calculator): "You created the meal template but we are not able to log those easily." → A (owner chose "Both"): each template meal can be logged to today in one tap and saved as a saved meal. This replaces the Non-Goal "Saving a plan never ... logs food" only in this sense: logging and saving happen when the athlete taps, never on their own.
- Q: Does the service need a new write route? → A: No. Food entries and saved meals already exist (specs 014 and 028). The only missing piece is identity: a template food had no id in the food catalog. Each template item now carries its catalog food id and the serving that logs exactly its portion; the app uses the existing entry and saved-meal routes.
- Q: Where do the catalog rows come from? → A: The 18 template foods are written to the shared food catalog under their FDC ids, the same rows a USDA food search stores, the first time a process serves a plan. If that write fails the plan is still served without ids.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Calculate targets from my numbers (Priority: P1)

An athlete sends their weight, height, age, sex, weekly training days, and goal and receives their estimated resting energy, maintenance energy, target calories, and protein, carbohydrate, and fat targets in grams, calories, and share of calories. Nothing is stored.

**Why this priority**: every other part of the feature is derived from these numbers.

**Independent Test**: send one imperial and one metric description of the same person and receive the same targets; change the goal and see only the calorie adjustment and the numbers that depend on it change.

**Acceptance Scenarios**:

1. **Given** a 180 lb, 70 in, 30-year-old male with 4 strength days and 1 cardio day, goal maintenance, **When** he calculates, **Then** resting energy is `4.536×180 + 15.875×70 − 5×30 + 5` rounded to a whole kilocalorie, the suggested level is moderately active, maintenance energy is resting energy × 1.55, the target equals maintenance, protein is `0.85 × 180` g, fat is `0.33 × 180` g, and carbohydrate is the remaining calories ÷ 4, all in whole grams.
2. **Given** the same person described in kilograms and centimetres, **When** he calculates, **Then** every target is within one unit of the imperial result.
3. **Given** a female athlete, **When** she calculates, **Then** the constant is −161 instead of +5.
4. **Given** each goal, **When** no adjustment is sent, **Then** the default adjustment for that goal is applied; **When** an adjustment inside the goal's range is sent, **Then** it is used; outside the range it is rejected naming the field.
5. **Given** an activity level in the request, **When** it differs from the suggestion, **Then** the override is used and the response still reports what was suggested.
6. **Given** protein and fat rates inside their ranges, **When** sent, **Then** they replace the defaults (0.95 g/lb for fat loss and recomposition, 0.85 g/lb for maintenance and gain; fat 0.33 g/lb).
7. **Given** protein and fat that already exceed the target calories, **When** calculated, **Then** carbohydrate is 0 and the response carries a notice instead of a negative number.

---

### User Story 2 - Carb cycling (Priority: P2)

With cycling on, the athlete receives a training-day and a rest-day set of numbers. Protein and fat are the same on both; carbohydrate is higher on training days and lower on rest days; the seven-day calorie total is unchanged.

**Independent Test**: for every split from 1 to 6 training days, training-day and rest-day grams sum over the week to exactly seven times the daily carbohydrate figure.

**Acceptance Scenarios**:

1. **Given** cycling on and 4 training days, **When** calculated, **Then** rest-day carbohydrate is 15–20% below the daily figure for ordinary intakes, training-day carbohydrate is above it, protein and fat match the daily figures, and `4 × training + 3 × rest` equals `7 × daily` in grams.
2. **Given** 0 or 7 training days, **When** cycling is requested, **Then** one standard day is returned with a notice that cycling was not applied.

---

### User Story 3 - A four-meal template from my foods (Priority: P2)

The response includes a template of four meals (Breakfast, Lunch, Pre/Post-Workout Fuel, Dinner) for each day type, each with a protein food, a carbohydrate food, and when needed a fat source, in everyday units, scaled to that meal's share of the day.

**Independent Test**: for a 200 g protein day, each meal's protein target is 50 g; the meals' targets add up to the day; no excluded food appears.

**Acceptance Scenarios**:

1. **Given** daily targets, **When** a template is built, **Then** protein is split evenly across four meals, carbohydrate 25% / 20% / 35% / 20% with the largest share at Meal 3, fat 30% / 30% / 10% / 30%, in whole grams that sum exactly to the day.
2. **Given** preferred proteins and carbohydrate staples, **When** a template is built, **Then** each meal uses a preferred food suited to that meal where one exists, and Meal 3 uses a fast-digesting carbohydrate when one is allowed.
3. **Given** an exclusion (rice, dairy, nuts, gluten, eggs, red meat), **When** a template is built, **Then** no food carrying that tag appears in any meal, including as a fat source or a companion food.
4. **Given** a meal's foods, **When** portions are scaled, **Then** each amount is in the food's unit (oz, g, cup, or count) at a practical step, and the meal reports both its target and what the rounded portions actually provide.
5. **Given** whole eggs as a meal's protein, **When** eggs alone would exceed the meal's fat, **Then** egg whites make up the remaining protein (unless eggs are excluded, in which case neither appears).

---

### User Story 4 - Swap a food and keep the meal's macros (Priority: P2)

The athlete names a different protein, carbohydrate, or fat food for one meal. The meal is rebuilt with the new food and portions recalculated for the same targets.

**Independent Test**: swap potatoes for tortillas in Meal 2; the meal's target is unchanged, the tortilla portion provides the same carbohydrate within the rounding step, and other meals are untouched.

**Acceptance Scenarios**:

1. **Given** a swap for one meal, **When** calculated, **Then** only that meal's foods change and its target is identical.
2. **Given** a swap naming an unknown or excluded food, or the wrong kind of food for the role, **When** calculated, **Then** the request is rejected naming the field.

---

### User Story 5 - Save and read my plan (Priority: P1)

The athlete saves their inputs, preferences, and swaps. The service recomputes the targets itself, stores the inputs with the resulting targets, and returns the full plan. Reading the plan later returns the same targets and a template built from them.

**Independent Test**: save, read, and get identical targets; save again with a stale revision and receive a conflict; another account reads and gets not-found.

**Acceptance Scenarios**:

1. **Given** no saved plan, **When** read, **Then** the shared not-found response is returned.
2. **Given** valid inputs and revision 0, **When** saved, **Then** the plan is created at revision 1 and returned in full.
3. **Given** a saved plan, **When** saved again with the current revision, **Then** it is replaced and the revision advances; with a stale revision the documented conflict is returned with the stored plan.
4. **Given** a save that was applied, **When** retried with the same operation key and body, **Then** the original response is replayed; a different body under the same key is the documented idempotency conflict.
5. **Given** a saved plan, **When** time passes, **Then** its targets do not change until the athlete saves again.
6. **Given** an account deletion or a data export, **Then** the saved plan is removed, or included, with the rest of the athlete's nutrition data.

---

### User Story 6 - Options the app can render (Priority: P3)

The app reads the foods (with role, tags, unit, and source), exclusion tags, goals with their ranges and defaults, activity levels with multipliers, and the protein and fat ranges, so it never hard-codes a number the service owns.

**Acceptance Scenarios**:

1. **Given** an authenticated athlete, **When** options are read, **Then** every list is present and non-null and each food names its USDA source record.

### User Story 7 - Log or save a template meal (Priority: P1, amendment A1)

Every item of a template meal names its food in the shared catalog and the serving that logs its exact portion, so the app can add the meal to a day or save it as a saved meal with the routes that already exist.

**Independent Test**: calculate, take an item's catalog food id and serving, log it to today with amount 1, and see an entry whose values equal the item's.

**Acceptance Scenarios**:

1. **Given** a plan, **Then** every item carries a catalog food id and a grams-only serving, and logging that food with that serving and amount 1 yields the item's energy and macros.
2. **Given** the template foods are already in the catalog (from an earlier plan or a food search), **When** another plan is served, **Then** the same ids are returned.
3. **Given** the catalog cannot be written, **When** a plan is requested, **Then** it is served with null catalog ids and the next request tries again.
4. **Given** a template meal's items, **When** they are sent to the existing saved-meal create route, **Then** a saved meal with those foods and portions is created.

### Edge Cases

- Weight outside 66–660 lb (30–300 kg), height outside 48–96 in (122–244 cm), age outside 18–100, or days outside 0–7: rejected naming the field.
- Unknown unit system, sex, goal, activity level, food, or exclusion tag: rejected naming the field. Duplicate list entries are collapsed.
- A preferred food that is also excluded is ignored; the exclusion wins.
- Every preferred food excluded for a role: the template falls back to allowed foods that suit the meal.
- Target calories below resting energy: allowed, with a notice.
- A portion that rounds to zero is left out of the meal; the meal still reports what it provides.
- More than one swap for the same meal and role: rejected.

## Non-Goals *(mandatory)*

- No change to the daily target, daily totals, food entries, or saved meals by the service on its own. Saving a plan never writes a target or logs food; since A1 the athlete may log or save a template meal themselves through the existing food routes.
- No recalculation from weight trends, activity, or time. No reminders, schedules, scores, or streaks.
- No athlete-defined foods, recipes, grocery lists, or foods beyond the fixed USDA list in this release.
- No sedentary or "extra active" level; three levels as requested.
- No medical or clinical use; no users under 18.
- No write to the training profile or body-weight log from the calculator.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The service MUST compute resting energy with the Mifflin-St Jeor equation: imperial `4.536 × lb + 15.875 × in − 5 × age + s`, metric `10 × kg + 6.25 × cm − 5 × age + s`, with `s = +5` for male and `−161` for female.
- **FR-002**: Maintenance energy MUST be resting energy × the activity multiplier (1.375, 1.55, 1.725). The service MUST suggest a level from weekly sessions (≤3, 4–5, ≥6) and MUST honour an override.
- **FR-003**: Target calories MUST be maintenance energy plus the goal's adjustment, which defaults per goal and MUST lie in the goal's range.
- **FR-004**: Protein MUST be body weight in pounds × a rate of 0.70–1.10 g/lb (default 0.95 for fat loss and recomposition, 0.85 otherwise); fat MUST be body weight × 0.25–0.45 g/lb (default 0.33); carbohydrate MUST be `(target − 4 × protein − 9 × fat) ÷ 4`, never below 0. Energy is counted at 4, 4, and 9 kcal per gram.
- **FR-005**: The response MUST give grams, calories, and percentage share for each macro, and whole-number values throughout.
- **FR-006**: With cycling on and both day types present, the service MUST return training-day and rest-day values with unchanged protein and fat and a seven-day gram total equal to seven times the daily figure.
- **FR-007**: The service MUST build one four-meal template per day type with per-meal targets that sum exactly to the day (protein evenly; carbohydrate 25/20/35/20; fat 30/30/10/30).
- **FR-008**: Template foods MUST come only from the fixed USDA-sourced list, MUST honour exclusions without exception, and SHOULD use the athlete's preferred foods.
- **FR-009**: Portions MUST be scaled to the meal's targets, expressed in the food's unit at its step, and each meal MUST report target and provided values.
- **FR-010**: A swap MUST replace one food in one meal with another food of the same role and recalculate portions for the same meal target.
- **FR-011**: Saving MUST recompute targets from the inputs, store inputs, preferences, swaps, and the resulting targets as one record per athlete, require the current revision, and honour the documented idempotency behaviour.
- **FR-012**: Reading the plan MUST return the stored inputs and targets unchanged and a template built from them; no saved plan MUST be the shared not-found.
- **FR-013**: Calculating MUST store nothing about the athlete. (A1: it may write the fixed template foods to the shared food catalog.)
- **FR-014**: Every route MUST be scoped to the authenticated athlete; account deletion MUST remove the saved plan and the data export MUST include it.
- **FR-015**: Validation failures MUST name the offending field. Notices (carbohydrate floor, below resting energy, cycling not applied) MUST be machine-readable.
- **FR-017** (A1): Every template item MUST carry the food's id in the shared food catalog and a serving that, logged with amount 1 through the existing entry route, yields the item's values. The id MUST be null, and the plan still served, when the catalog is unavailable.
- **FR-018** (A1): Template foods MUST be stored in the shared catalog as their USDA records (source, FDC id, description, per-100 g facts), never as athlete data.
- **FR-016**: The change MUST be additive: no existing route, field, or error changes. The contract MUST be documented in `docs/CONTRACTS.md` in the same change.

### Key Entities

- **Calculator input**: unit system, weight, height, age, sex, strength days, cardio days, optional activity override, goal, optional adjustment, optional protein and fat rates, cycling flag, preferred proteins, preferred carbohydrates, exclusions, swaps.
- **Targets**: resting energy, maintenance energy, adjustment, target calories, protein, carbohydrate, fat.
- **Meal template**: day type; four meals, each with a target, foods with portions, and provided totals.
- **Saved plan**: one per athlete: the input, the targets computed when saved, revision, timestamps.
- **Template food**: fixed list entry: id, name, role (protein, carbohydrate, fat), exclusion tags, unit and step, per-100 g facts, USDA record id.

## Success Criteria *(mandatory)*

- **SC-001**: The six worked examples in the automated tests (imperial male and female, metric, each goal, cycling, floor) match hand-computed values exactly.
- **SC-002**: For every valid input in the automated sweep, per-meal targets sum to the daily targets and the cycled week sums to seven standard days, with zero exceptions.
- **SC-003**: No automated case with an exclusion produces a meal containing a food with that tag.
- **SC-004**: A swap changes no other meal and leaves the swapped meal's target identical in 100% of automated cases.
- **SC-005**: A retried save never creates a second record or advances the revision twice.
- **SC-006**: A calculation answers in one round trip without touching stored data.
- **SC-007**: Clients that predate this feature keep working unchanged.

## Assumptions

- The activity multipliers and the per-pound protein and fat rates are the owner's stated values; `research.md` records how they compare with published ranges. Results are estimates and are labelled so.
- Strength and cardio days are counted as separate days up to seven.
- Meats and potatoes are measured cooked; the food name says so.
- The existing idempotency ledger, revision conflict shape, per-user lock, row-level security helper, and account-deletion pattern are reused.
- The API ships and is deployed before the app surface depends on it.
