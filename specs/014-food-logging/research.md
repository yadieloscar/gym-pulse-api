# Research: Food search, barcode lookup, and food entries

## Sources

### USDA FoodData Central (first source)

- **Facts**: `GET https://api.nal.usda.gov/fdc/v1/foods/search?api_key=…` with `query`,
  `dataType` (`Foundation`, `SR Legacy`, `Branded`), `pageSize` (25); `GET /fdc/v1/food/{fdcId}`.
  Results carry `fdcId`, `description`, `dataType`, `brandOwner`, `brandName`, `gtinUpc`,
  `servingSize`, `servingSizeUnit`, `householdServingFullText`, and `foodNutrients[]` with
  `nutrientNumber` `208` (energy kcal), `203` (protein), `205` (carbohydrate), `204` (fat), values
  per 100 g. Public domain; key free; 1,000 requests per hour per key, `429` beyond, headers
  `X-RateLimit-Remaining`. Confirmed on 2026-10-26 with a `DEMO_KEY` search for "kirkland
  signature" (5,548 Branded items with `gtinUpc`). Exact field spelling is re-checked against the
  OpenAPI document at implementation (`fdc.nal.usda.gov/api-spec`).
- **Decision**: Text search queries `dataType=Foundation,SR Legacy,Branded`, `pageSize=25`.
  Barcode lookup queries `Branded` with the barcode as `query` and accepts only an item whose
  normalised `gtinUpc` equals the requested code; USDA has no barcode endpoint.
- **Rejected**: Bulk download of the Branded CSV (hundreds of MB, needs a refresh job; lazy fill
  gives the same answers for the foods athletes actually scan).

### Open Food Facts (barcode fallback)

- **Facts**: `GET https://world.openfoodfacts.org/api/v2/product/{barcode}?fields=code,product_name,brands,serving_size,serving_quantity,nutriments`;
  `status` `1` found, `0` not; `nutriments["energy-kcal_100g"]`, `proteins_100g`,
  `carbohydrates_100g`, `fat_100g`, optional `_serving` variants; `serving_quantity` in grams
  when parseable. Requires `User-Agent: AppName/Version (contact)`. Limits per IP: 15 product
  reads per minute, 10 searches per minute; `503` on global limits; IP bans for abuse. Staging
  `world.openfoodfacts.net` (basic auth `off`/`off`). Licence ODbL; contents DbCL.
- **Decision**: Barcode fallback only, never text search. The service keeps one in-process budget of
  12 product reads per minute (under the 15 cap); when the budget is spent, the lookup answers
  "not found, retry later" (`404` with `details.retry_after_seconds`) instead of calling. Foods from
  this source carry `source: open_food_facts` and an `attribution` block (`"Open Food Facts"`,
  `https://world.openfoodfacts.org/product/{code}`, `ODbL`). `food_catalog` rows are tagged by
  source so the ODbL subset can be exported on request.
- **Rejected**: OFF text search (10 per minute shared by every athlete); mirroring the OFF dump.

### Not used in this release

FatSecret and Nutritionix (paid or attribution-heavy; decision recorded in `docs/PRODUCT.md`).

## Decisions

### The catalog is shared, filled lazily, and never edited by clients

- **Decision**: `food_catalog` has no `user_id`. Every food the API returns from an outside source
  is upserted by `(source, source_id)` with values per 100 g, servings, barcode, and `fetched_at`.
  Repeat lookups hit the catalog first. Rows refresh when older than 90 days and the source is
  reachable; a failed refresh keeps the old row.
- **Rationale**: FR-003 and SC-002; one copy per product instead of one per athlete.
- **Rejected**: Caching in process (lost on restart, per instance); storing per-athlete copies.

### Values are stored per 100 g; servings are a JSON list

- **Decision**: `energy_kcal_100g`, `protein_g_100g`, `carbs_g_100g`, `fat_g_100g` as
  `NUMERIC(6,1)`. `servings JSONB` is a list of `{ "label": "1 cup (245 g)", "grams": 245 }`; the
  service always appends `{ "label": "100 g", "grams": 100 }` when absent. An entry's values are
  `per_100g × grams × amount / 100`, energy rounded to a whole kcal, macros to one decimal.
- **Rationale**: Both sources publish per 100 g; one formula covers every serving and grams entry.
  A serving child table would add a join to every read for a list that is read whole.
- **Rejected**: Storing per-serving values only (cannot log by weight); a `food_servings` table.

### Private foods are typed per serving

- **Decision**: `user_foods` stores the label as the athlete reads it: `serving_label`, `serving_g`,
  and the four values per serving. The service derives per 100 g for the response so the app uses
  one `Food` shape with `source: own`. Archiving (`DELETE`) sets `archived_at`; entries keep their
  snapshot and the foreign key stays valid.
- **Rejected**: Hard delete (breaks FR-005's snapshot linkage); editing in place without revision.

### Entries snapshot their values and roll up in one transaction

- **Decision**: `food_entries` stores the computed `energy_kcal`, `protein_g`, `carbs_g`, `fat_g`
  beside the food reference, serving, and amount. Every create, change, or delete runs under the
  `nutrition:` advisory lock for the athlete, replays through the ledger, selects the day `FOR
  UPDATE`, applies the change, recomputes the four sums over the day's entries, and upserts
  `nutrition_days` with `source = 'foods'`, bumping the day's revision. Deleting the last entry
  deletes the day row.
- **Rationale**: FR-006 and SC-004; the sum is never derived on read and never drifts.
- **Rejected**: A view or trigger (hides the rule from the Go code the tests cover); client-side
  rollup (two writers, two truths).

### A day's first entry absorbs typed totals

- **Decision**: When the first entry lands on a day whose row is `manual` or `apple_health`, the
  transaction converts that row's values into one `quick_add` entry with the row's `logged_at`, then
  proceeds. The response lists both entries.
- **Rationale**: Spec clarification "typed totals become a quick add entry"; nothing the athlete
  saved disappears when they start logging foods.

### `PUT /days/{date}` on a foods day

- **Decision**: An athlete save (manual or remove) on a `foods` day is `409 DAY_HAS_FOOD_ENTRIES`
  with the stored day as `resource`. A Health save on a `foods` day is `200` ignored (the existing
  non-Health branch already returns the stored row). The day's `revision` keeps advancing on every
  entry change, so spec 015 apps see a normal conflict story.
- **Rejected**: Letting typed totals wipe entries (data loss); a new boolean column (the `source`
  value says the same thing).

### `nutrition_days.source` gains `foods`

- **Decision**: The migration replaces the check constraint with
  `source IN ('manual', 'apple_health', 'foods')`. The down migration asserts no `foods` rows exist
  and restores the two-value check.
- **Rationale**: One column already names who owns the day's values. Spec 015 apps only compare
  against `apple_health`, so an unknown value renders as an athlete day.

### Barcode normalisation and lookup chain

- **Decision**: Accept 8, 12, 13, or 14 digits after stripping spaces and hyphens; verify the GS1
  check digit; store and compare as a 14-digit zero-padded GTIN. Chain: private food for this
  athlete → catalog by GTIN → USDA Branded search → Open Food Facts (tries the 13-digit form, then
  the 8- or 12-digit original) → miss. A miss writes `food_barcode_misses(gtin, checked_at)` and
  is honoured for 7 days, except that creating a private food with that barcode serves at once.
- **Rejected**: Trusting the raw string (the same product reads as 12 or 13 digits on different
  scanners); skipping the check digit (misreads would become permanent misses).

### Search merge and degraded mode

- **Decision**: `GET /foods/search?q=` runs two local queries (private foods by prefix on name and
  brand; catalog by `websearch_to_tsquery('simple', q)` ranked by `ts_rank`) and one USDA search
  with a 3 s timeout, upserts the USDA items, and returns private foods first, then USDA order,
  de-duplicated by `(source, source_id)`, capped at 25. On USDA error, timeout, `429`, or a missing
  key, the response is the local rows with `incomplete: true`.
- **Rejected**: Streaming or paging (the app shows one short list); OFF search.

### Plausibility filter

- **Decision**: Drop a source item when energy or protein is missing, any value is negative,
  energy per 100 g exceeds 900 kcal, or any macro per 100 g exceeds 100 g.

### Recent foods

- **Decision**: `GET /foods/recent` groups the last 30 days of `food_entries` by food reference,
  orders by count then latest `logged_at`, and returns up to 20 with the latest entry's serving and
  amount; quick-add entries are excluded.

### Rate limits and budgets

- **Decision**: `middleware.FoodLookupLimit = 60` per user per minute on search and barcode routes
  via `RateLimitPerUser`. The OFF budget is a mutex-guarded sliding window in the service.
- **Rejected**: `golang.org/x/time/rate` (a dependency for twenty lines).

### Configuration

- **Decision**: `USDA_FDC_API_KEY` (optional in development and test; staging and production warn at
  startup when empty and run in degraded search), `FOOD_SOURCE_USER_AGENT` (default
  `GymPulse/<version> (support@gympulse.app)` once the contact address is confirmed),
  `USDA_FDC_BASE_URL` and `OPEN_FOOD_FACTS_BASE_URL` overrides for tests and staging. Keys never
  appear in logs or responses.

### Route shape and codes

- `GET /api/v1/nutrition/foods/search?q=&limit=` → `200 { foods: [], incomplete }`; `422` on `q`.
- `GET /api/v1/nutrition/foods/barcode/{code}` → `200 Food`, `404 NOT_FOUND` (shared shape, with
  `details.retry_after_seconds` when the budget was spent), `422` on `code`.
- `GET /api/v1/nutrition/foods/recent` → `200 { foods: [RecentFood] }`.
- `POST /api/v1/nutrition/foods` → `201 Food` (own); `PUT /foods/{id}` → `200`; `DELETE /foods/{id}`
  → `204` archived; all with `Idempotency-Key`, the latter two with `expected_revision`.
- `GET /api/v1/nutrition/days/{date}/entries` → `200 { day, entries: [] }` (`day` null when none).
- `POST /api/v1/nutrition/days/{date}/entries` → `201 { entry, day, converted: [] }`.
- `PUT /api/v1/nutrition/entries/{id}` → `200 { entry, day }`; `DELETE /api/v1/nutrition/entries/{id}`
  with `Idempotency-Key` and `?expected_revision=` → `200 { day }` (`day` null when removed).
- Conflicts reuse `REVISION_CONFLICT` and `IDEMPOTENCY_CONFLICT`; `DAY_HAS_FOOD_ENTRIES` is the one
  new code; `RATE_LIMITED` is the existing limiter body.

## Resolved Technical Questions

- Idempotency scopes: `food-entries/post`, `food-entries/put`, `food-entries/delete`,
  `user-foods/post`, `user-foods/put`, `user-foods/delete`; advisory namespace `nutrition:` so
  entry writes and day saves never interleave for one athlete.
- Hashing: hash the request as sent plus the path parameters, not server-computed snapshot values.
- Dates: local today from the training profile as in `nutrition_svc.go`; future dates `422`.
- Migration number: the next free number when this branch is rebased. `spec/012` holds 027 and PR
  #36 holds 027–028; whichever merges second renumbers, and this feature follows both.
- `DATABASE_DESIGN.md`: four table sections and index rows; `scripts/validate-database-design.sh`.
- Account deletion: `DELETE FROM food_entries` and `DELETE FROM user_foods` join `account_dao.go`;
  `food_catalog` and `food_barcode_misses` hold no personal data.
- Swagger: swag godoc on the new handlers; `docs/swagger.*` regenerated.
