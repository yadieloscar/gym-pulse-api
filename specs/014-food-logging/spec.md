# Feature Specification: Food search, barcode lookup, and food entries

**Feature Branch**: `spec/014-food-logging`

**Feature ID**: `gympulse-food-logging`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/017-food-logging/spec.md)

**Depends on**: API spec 012 `gympulse-nutrition-targets` (daily totals, targets). This branch is stacked on `spec/012-nutrition-targets`.

**Created**: 2026-10-27

**Status**: Approved (defaults confirmed 2026-10-27)

**Input**: User description: "Can we get nutritional facts for those products? That's the only way our app will become actually useful if we can combine all." Then: "Let's spec this", after a comparison of the food data competitors use.

Governing product rules: workspace `docs/PRODUCT.md`, section "Nutrition" (amended 2026-10-27): foods add up to the day; trustworthy food data from published sources (USDA first, an open product database as fallback); athlete-created foods are private; no crowdsourcing, meal plans, recipes, or prescribed diets; logging stays fast.

## Clarifications

### Session 2026-10-27

- Q: Where do food facts come from? → A: Generic foods (for example "chicken breast, cooked") and branded products come from USDA FoodData Central. When a barcode is not found there, the service asks Open Food Facts. A food the athlete creates from a label is private to that athlete. GymPulse never edits or estimates a food's facts.
- Q: Does a logged food change if the source later changes? → A: No. Each entry keeps the facts it was logged with, so history never moves on its own (product invariant "Trustworthy history"). The catalog may refresh; past entries do not.
- Q: How do foods relate to the daily totals from spec 012? → A: A day's totals are the sum of its food entries. A "quick add" entry carries raw totals with no food, so the existing "type your totals" flow becomes one quick-add entry. A Health import never replaces a day that has entries, matching the rule that the athlete's own data wins.
- Q: Meals (breakfast, lunch, dinner)? → A: Not in this release. Entries are listed in the order they were logged. Meal grouping can follow if athletes ask for it.
- Q: Open Food Facts licence? → A: Its database licence asks that a database derived from it be shared under the same licence. Foods that came from Open Food Facts are kept apart from USDA foods, credited in the app, and that subset is offered back under the same licence on request. Nothing else in GymPulse is affected.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Find a food and log a serving (Priority: P1)

An athlete searches "greek yogurt", sees matching foods with their protein per serving and where the facts come from, picks one, chooses a serving amount, and logs it to today. The day's totals update to include it.

**Why this priority**: Logging real foods is the capability the athlete asked for; everything else builds on finding a food and recording a serving.

**Independent Test**: Search "chicken breast", log 1.5 servings of a USDA result to today, read today's day and see one entry and totals equal to 1.5 times the food's per-serving values.

**Acceptance Scenarios**:

1. **Given** an authenticated athlete, **When** they search with at least two characters, **Then** they receive up to 25 matching foods, each with a name, brand when branded, at least one serving with its gram weight, energy and the three macros per serving, and its source.
2. **Given** a search result, **When** the athlete logs it with a serving and an amount between 0.1 and 50, **Then** an entry is stored for that date with the food's facts at that moment, and the day's totals become the sum of its entries.
3. **Given** a search with no matches, **When** it completes, **Then** an empty list is returned, never an error.
4. **Given** the food source is unavailable, **When** the athlete searches, **Then** previously looked-up foods that match are still returned and the response says results may be incomplete.
5. **Given** a retry of the same log with the same operation identity, **When** it arrives, **Then** the original entry is returned and nothing is logged twice.

---

### User Story 2 - Scan a barcode (Priority: P1)

An athlete scans the barcode on a package. The service finds the product by its barcode and returns its facts, and the athlete logs a serving.

**Why this priority**: Scanning is the fastest way to log packaged food and the main reason athletes asked for product data, including store brands such as Kirkland Signature.

**Independent Test**: Look up the barcode of a Kirkland Signature product present in USDA FoodData Central, see the product with its serving and macros, log it, and see the day's totals change.

**Acceptance Scenarios**:

1. **Given** a barcode in any common retail format (12-digit UPC, 13-digit EAN, with or without leading zeros), **When** it is looked up, **Then** the same product is found regardless of format.
2. **Given** a barcode USDA does not have, **When** Open Food Facts has it with energy and the three macros, **Then** that product is returned with its source marked as Open Food Facts.
3. **Given** a barcode neither source has, or one missing protein or energy, **When** it is looked up, **Then** the response is the shared not-found answer, so the app can offer to create the food.
4. **Given** a barcode was found once, **When** it is looked up again, **Then** the answer comes from the service's own copy without asking the outside source.

---

### User Story 3 - Log again in one tap (Priority: P2)

The athlete sees the foods they log most and most recently, with the serving they used last, and can log one to today in a single action.

**Why this priority**: Most people eat the same foods often; product rule "Logging stays fast" requires a logged food to be one tap away.

**Independent Test**: Log oats twice this week, read recent foods, see oats first with the last serving and amount, log it again with one request.

**Acceptance Scenarios**:

1. **Given** an athlete with entries in the last 30 days, **When** they read recent foods, **Then** they receive up to 20 foods ordered by how often and how recently they were logged, each with the serving and amount last used.
2. **Given** a recent food whose catalog facts changed since, **When** it is logged again, **Then** the new entry uses the current facts and older entries keep theirs.

---

### User Story 4 - Correct or remove an entry (Priority: P2)

The athlete changes the amount of a logged food, or removes it, and the day's totals follow.

**Why this priority**: Logging mistakes are common; trustworthy history needs safe correction.

**Independent Test**: Change an entry from 1 to 2 servings with its current revision, see totals change; delete it and see totals drop; send a stale revision and see a conflict.

**Acceptance Scenarios**:

1. **Given** an entry, **When** the athlete changes its amount or serving with the current revision, **Then** the entry and the day's totals update and the revision advances.
2. **Given** a stale revision, **When** a change arrives, **Then** it is rejected with the documented revision conflict and nothing changes.
3. **Given** an entry, **When** it is deleted, **Then** the day's totals drop by its values; deleting the last entry removes the day's totals.

---

### User Story 5 - Create a food from a label (Priority: P3)

When a product is not found, the athlete types the facts from its label (name, serving size in grams, energy, protein, carbs, fat, optional barcode) and logs it. The food is private to them and appears in their searches and scans.

**Why this priority**: Covers gaps in both sources without opening a crowdsourced database.

**Independent Test**: Create a food with a barcode, scan that barcode, and get the private food back; another athlete scanning the same barcode does not.

**Acceptance Scenarios**:

1. **Given** valid label values, **When** the athlete creates a food, **Then** it is stored as theirs, marked as created by them, and returned first for its barcode and name in their lookups.
2. **Given** another athlete, **When** they search or scan, **Then** they never see it.
3. **Given** a private food used in past entries, **When** the athlete edits or archives it, **Then** past entries keep the facts they were logged with.

### Edge Cases

- A search shorter than two characters, or longer than 100, is rejected with a field-level validation error.
- A barcode that is not 8, 12, 13, or 14 digits, or fails its check digit, is rejected with a field-level validation error.
- A food with no gram weight for any serving still has a 100 g serving so it can be logged by weight.
- Values from a source that are missing, negative, or implausible (energy above 9 kcal per gram, macros above 100 g per 100 g) cause that food to be skipped, not shown with wrong numbers.
- Search and lookups are limited per athlete (60 per minute) so one account cannot exhaust the outside source's quota; the limit returns the standard rate-limit response.
- A quick-add entry with all values absent is rejected; a quick-add entry is otherwise validated like spec 012's day values.
- The spec 012 day save on a date that has food entries is rejected with a stable conflict (`DAY_HAS_FOOD_ENTRIES`) so an older app cannot replace foods with typed totals.
- A Health import on a date with any entry is acknowledged and ignored, as for a manual day.
- Dates follow spec 012: local calendar dates in the training-profile timezone; future dates are rejected.
- Deleting the account removes the athlete's entries and private foods; shared catalog foods stay.

## Non-Goals *(mandatory)*

- No grocery shopping, retailer accounts, carts, prices, or store availability. Shopping handoff (for example Instacart) is a separate later specification.
- No meal slots, recipes, meal plans, or diets; no food scores or "healthy" labels.
- No crowdsourced or shared user foods; no edits to USDA or Open Food Facts data; no contributions back to Open Food Facts in this release.
- No micronutrients, water, fibre, sugar, sodium, or alcohol; the four values from spec 012 only.
- No photo, voice, or AI-estimated logging.
- No restaurant menus or paid data providers (for example Nutritionix) in this release.
- No change to targets, the weekly view, or the Health read except as stated above.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The service MUST offer food search by text returning up to 25 foods with name, brand when branded, one or more servings with gram weight, energy and the three macros per serving, and source (USDA, Open Food Facts, or the athlete's own).
- **FR-002**: The service MUST offer barcode lookup accepting 8, 12, 13, and 14 digit codes, normalizing them so equivalent formats match, checking USDA first and Open Food Facts second, and returning the shared not-found answer when neither has a usable product.
- **FR-003**: The service MUST keep its own copy of every food it returns, keyed by source and source identity, and answer repeat lookups from that copy; it MUST still answer search from its copy when an outside source is unavailable and say so.
- **FR-004**: All calls to outside food sources MUST be made by the service with credentials held by the service; the app MUST never call them directly.
- **FR-005**: The service MUST store food entries per athlete and date, each with either a food and serving and amount, or quick-add values, plus a snapshot of energy and macros at logging time, a revision, and when it was logged.
- **FR-006**: A day's totals (spec 012) MUST equal the sum of its entries whenever it has entries, MUST be maintained by the service in the same transaction as the entry change, and MUST be removed when the last entry is removed.
- **FR-007**: Creating, changing, and deleting entries MUST follow the documented idempotency and revision rules; a stale change MUST be rejected with the documented revision conflict.
- **FR-008**: The spec 012 day save MUST be rejected with `DAY_HAS_FOOD_ENTRIES` on a date that has entries, and a Health import on such a date MUST be acknowledged and ignored.
- **FR-009**: The service MUST return the athlete's recent foods (up to 20, last 30 days) ranked by frequency and recency with the last serving and amount used.
- **FR-010**: The service MUST let an athlete create, edit, and archive private foods with name, serving gram weight, energy, three macros, and optional barcode; private foods MUST be visible only to their owner and preferred in that owner's lookups.
- **FR-011**: The service MUST skip outside foods with missing, negative, or implausible values rather than return them.
- **FR-012**: The service MUST limit search and lookup per athlete and return the standard rate-limit response beyond it.
- **FR-013**: Foods from Open Food Facts MUST be stored apart from other foods and carry their source so the app can credit it.
- **FR-014**: Every athlete-owned resource MUST be scoped to the authenticated athlete with the shared not-found behaviour, and account deletion MUST remove entries and private foods.
- **FR-015**: The public contract for every behaviour above MUST be documented in `docs/CONTRACTS.md` in the same change as the implementation.

### Key Entities *(include if feature involves data)*

- **Catalog food**: a food from an outside source (USDA or Open Food Facts) the service has looked up. Source, source identity, barcode when known, name, brand, servings, values per 100 g, when it was fetched. Shared across athletes; not personal data.
- **Private food**: a food an athlete created from a label. Owner, name, serving gram weight, values, optional barcode, archived flag, revision.
- **Food entry**: one logged item on an athlete's day. Date, food reference (catalog or private) with serving and amount, or quick-add values; snapshot of energy and macros; revision; logged time.
- **Nutrition day** (from spec 012): now the sum of a date's entries when it has entries.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: At least 90% of a fixed test list of 50 common US packaged groceries (including 10 Kirkland Signature products) resolve by barcode to a product with energy and the three macros.
- **SC-002**: A repeat barcode lookup answers in under 300 ms at the 95th percentile because it comes from the service's own copy.
- **SC-003**: No logged entry's values ever change after logging unless the athlete edits it, in 100% of automated scenarios including a catalog refresh.
- **SC-004**: A day's totals equal the sum of its entries in 100% of automated scenarios, including concurrent changes from two devices.
- **SC-005**: Search keeps working from the service's copy when the outside source is down, in automated tests.
- **SC-006**: An app from spec 015 keeps working: days without entries behave exactly as before, and typed totals on a day with entries are refused rather than silently replacing foods.

## Assumptions

- A USDA FoodData Central API key is issued to the service (free; USDA data is public domain). Open Food Facts requires an identifying user agent and attribution; no key.
- The four values and spec 012's bounds apply unchanged; catalog values are stored per 100 g and per serving.
- Spec 012 (PR #35) merges and deploys first. The migration for this feature takes the next free number at implementation time; API PR #36 (planned sport sessions) also claims migration 027, so numbering is settled when those merge.
- Rate limiting uses the existing per-user middleware.
- Shopping and grocery handoff are a separate later feature; this one makes product facts available so that feature can build on them.
