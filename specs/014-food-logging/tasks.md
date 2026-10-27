---
description: "Implementation tasks for food search, barcode lookup, and food entries"
---

# Tasks: Food search, barcode lookup, and food entries

**Input**: Design documents from `/specs/014-food-logging/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/food-logging.openapi.yaml, quickstart.md

**Tests**: Required by the API constitution; identified before each implementation task. Outside sources are always `httptest` fixtures in tests; no test calls USDA or Open Food Facts.

## Format: `[ID] [P?] [Story] Description`

## Requirement and Verification Evidence

| Requirement / acceptance | Tasks | Verification | Result / limitation |
| --- | --- | --- | --- |
| FR-001, US1/AC1, AC3 | T017, T020, T021 | handler search test: 25 cap, every field present, source set, `{foods:[],incomplete:false}` on no match; `q` under 2 or over 100 → 422 `q` | Planned |
| FR-002, US2/AC1–AC3 | T006, T018, T019 | model tests: 8/12/13/14-digit forms normalise to one GTIN-14, bad check digit rejected; service chain test private → catalog → USDA → OFF → miss with fixtures | Planned |
| FR-003, SC-002, SC-005 | T010, T013, T018 | DAO test: repeat barcode makes no outbound call; search with USDA fixture `503` → local rows and `incomplete:true`; 90-day refresh keeps the old row on failure | Planned |
| FR-004 | T008, T009 | config test: key from env only; adapter test: key only in the query string to the USDA base URL; log scrub test asserts the key never appears in log output | Planned |
| FR-005, SC-003 | T023, T024 | DAO test: entry snapshot unchanged after the catalog row changes; archived private food keeps the entry valid | Planned |
| FR-006, SC-004 | T023, T024 | DAO tests: sums after create, change, delete; delete-last removes the day; two concurrent creates serialise under the `nutrition:` lock and both land in the sum | Planned |
| FR-007 | T023, T024, T027 | DAO tests: verbatim replay of 201/200; payload mismatch → `IDEMPOTENCY_CONFLICT`; stale entry revision → `REVISION_CONFLICT` with the entry as `resource` | Planned |
| FR-008, SC-006 | T029, T030, T031 | `nutrition_dao` tests: manual `PUT` on a `foods` day → 409 `DAY_HAS_FOOD_ENTRIES` with the day; empty `PUT` → same; Health `PUT` → 200 ignored; first entry on a manual day yields one `quick_add` in `converted` | Planned |
| FR-009, US3 | T033, T034 | DAO test: recents ranked by count then recency, last serving and amount from the latest entry, 30-day window, 20 cap, quick adds excluded | Planned |
| FR-010, US5 | T036–T039 | handler tests: create → 201 `source: own`; foreign `GET`/`PUT`/`DELETE` → shared 404; private food wins the barcode chain; archive excludes from search and barcode but not from entries or recents | Planned |
| FR-011 | T011, T012 | adapter tests: items missing protein or energy, negative, energy over 900 or any macro over 100 per 100 g are dropped; the 100 g serving is appended | Planned |
| FR-012 | T021, T014 | router test: 61st search in a minute → 429 `RATE_LIMITED`; OFF budget test: 13th read in a minute → 404 with `details.retry_after_seconds`, no outbound call | Planned |
| FR-013 | T012, T013 | adapter test: OFF food carries `attribution`; USDA food has `attribution: null`; DAO row tagged `open_food_facts` | Planned |
| FR-014, SC-001 | T040, T041 | account deletion test asserts zero rows in `food_entries` and `user_foods`; 50-GTIN fixture list run once against the USDA DEMO_KEY to record the resolve rate | Planned |
| FR-015 | T002, T042, T044 | `docs/CONTRACTS.md` "Food logging" and amended `PUT /days/{date}`; contract tests; `./scripts/smoke-toggle.sh` | Planned |
| Schema document | T003, T005, T043 | `./scripts/validate-database-design.sh` | Planned |
| Supabase security and runtime role | T005, T045 | `supabase_security_migration_test.go`; `./scripts/validate-supabase-security.sh` on a migrated database | Planned |

## Phase 1: Setup

- [ ] T001 Rebase `spec/014-food-logging` on the merged nutrition branch; record `git status --short`; pick the next free migration number against `origin/main` (027 is claimed by both PR #35 and PR #36; this feature follows both) and replace `0NN` in plan.md and data-model.md
- [ ] T002 [P] Draft the "Food logging" section in `docs/CONTRACTS.md` after "Nutrition" from `contracts/food-logging.openapi.yaml` (nine routes, bodies, envelopes, barcode rules, budget 404, rate limit) and amend `PUT /api/v1/nutrition/days/{date}` (`foods` source, `DAY_HAS_FOOD_ENTRIES`)
- [ ] T003 [P] Add `food_catalog`, `user_foods`, `food_entries`, `food_barcode_misses` sections, checks, index rows, and the `nutrition_days.source` change to `docs/DATABASE_DESIGN.md`; update the table and column counts

## Phase 2: Foundational

- [ ] T004 [P] Add `internal/model/food.go`: `FoodSource`, `Serving`, `Per100g`, `Attribution`, `Food`, `RecentFood`, `FoodEntry`, `FoodEntryKind`, `SaveUserFoodRequest`, `CreateFoodEntryRequest`, `UpdateFoodEntryRequest`, envelopes, outcomes (`created`, `replaced`, `removed`, `archived`, `replayed`), `NutritionSourceFoods`
- [ ] T005 Add `migrations/0NN_create_food_logging.{up,down}.sql` per data-model.md: four tables, checks, generated `search_vector` with GIN, partial indexes, `nutrition_days` source check widened, RLS, Data API revoke, runtime-role policy and grants; down asserts no `foods` rows and restores the two-value check; extend `internal/dao/supabase_security_migration_test.go`; run `./scripts/validate-database-design.sh`
- [ ] T006 [P] Add `internal/model/food_acceptance_test.go` and barcode tests: GTIN normalisation (`NormalizeGTIN`) for 8/12/13/14 digits with spaces and hyphens, GS1 check digit, display form (13 when it fits); snapshot math (`EntryValues`) rounding parity cases; JSON shape (nullable fields always present on `Food`)
- [ ] T007 Implement `NormalizeGTIN`, `DisplayGTIN`, `EntryValues`, and `EnsureHundredGramServing` in `internal/model/food.go`
- [ ] T008 [P] Extend `internal/config/config.go`: `USDAFDCAPIKey`, `FoodSourceUserAgent` (default `GymPulse/<version> (contact)`), `USDAFDCBaseURL`, `OpenFoodFactsBaseURL`; staging and production log a warning when the key is empty; config tests
- [ ] T009 [P] Add `internal/service/foodsource.go`: `FoodSource` interface (`Search(ctx, q, limit)`, `LookupBarcode(ctx, gtin)`), `SourceFood` DTO, `ErrSourceUnavailable`, `ErrBudgetSpent`; a `loggingTransport` test helper that asserts the key never reaches logs

## Phase 3: User Story 1 - Find a food and log a serving (P1) 🎯 MVP

- [ ] T010 [P] [US1] Add `internal/service/foodsource_usda_test.go` with `httptest` fixtures: search maps `fdcId`, `description`, `dataType`, `brandOwner`/`brandName`, `gtinUpc`, `servingSize`+`servingSizeUnit`+`householdServingFullText`, nutrients `208/203/205/204`; `429` and `503` → `ErrSourceUnavailable`; 3 s timeout; key only in the query string
- [ ] T011 [US1] Implement `internal/service/foodsource_usda.go`: search (`dataType` Foundation, SR Legacy, Branded; `pageSize`), barcode via Branded search with exact `gtinUpc` match, plausibility filter, 100 g serving, `http.Client{Timeout: 3s}`
- [ ] T012 [P] [US1] Add `internal/dao/food_dao_test.go`: catalog upsert by `(source, source_id)` keeps `id`; `SearchCatalog` uses `websearch_to_tsquery('simple', $1)` ranked by `ts_rank`; `FindCatalogByGTIN`; refresh only when `fetched_at` older than 90 days; `user_id` absent from catalog statements and present in every private-food and entry statement
- [ ] T013 [US1] Implement `internal/dao/food_dao.go`: `UpsertCatalogFoods`, `SearchCatalog`, `FindCatalogByGTIN`, `GetFood` (catalog or own by id for the athlete)
- [ ] T014 [P] [US1] Add `internal/service/food_svc_test.go`: `q` bounds; search merge order (own first, then USDA order, de-duplicated by `(source, source_id)`, 25 cap); `incomplete:true` on `ErrSourceUnavailable`, timeout, or empty key; create-entry validation (exactly one of `food` or `quick_add`; serving must be one of the food's servings or grams-only; amount 0.1–50 two decimals; quick add needs a value; date not after local today; `logged_at` default); hash covers path params plus client body only
- [ ] T015 [US1] Implement `internal/service/food_svc.go` `Search` and `CreateEntry` (profile timezone like `nutrition_svc.go`, snapshot via `EntryValues`, hashing, scopes `food-entries/post`)
- [ ] T016 [P] [US1] Add `internal/dao/food_entry_dao_test.go`: create-entry transaction order markers (lock `nutrition:` → replay → load food → select day for update → convert typed day → insert → recompute sums → upsert day `foods` revision+1 → ledger → commit); sums equal the entries; first entry on an empty date creates the day with revision 1; `user_id` in every statement
- [ ] T017 [US1] Implement `internal/dao/food_entry_dao.go` `CreateEntry` and `ListDayEntries` (`{day, entries}` oldest first, `[]` when none)
- [ ] T018 [US1] Implement `internal/handler/food.go` `Search` (`{foods, incomplete}`), `ListDayEntries`, `CreateEntry` (201, replay status from the ledger); swag godoc
- [ ] T019 [P] [US1] Add `internal/handler/food_test.go`: statuses, validation field surfacing, `IDEMPOTENCY_CONFLICT`, 404 passthrough for no profile and unknown food, header/body key mismatch
- [ ] T020 [US1] Wire adapters, DAOs, service, handler in `cmd/server/main.go`; routes under `/api/v1/nutrition/foods/search`, `/days/{date}/entries` in `internal/router/router.go` inside the `RequireActiveUser` group; add `middleware.FoodLookupLimit = 60` and apply `RateLimitPerUser` to search; extend the router test
- [ ] T021 [US1] Add `DELETE FROM food_entries` and `DELETE FROM user_foods` to `internal/dao/account_dao.go` and its enumeration test

**Checkpoint**: search works with USDA and degrades to the catalog; an entry rolls into the day.

## Phase 4: User Story 2 - Scan a barcode (P1)

- [ ] T022 [P] [US2] Add `internal/service/foodsource_off_test.go` with fixtures: `/api/v2/product/{code}` `status 1` maps `product_name`, `brands`, `nutriments` per 100 g, `serving_quantity` as a serving; `status 0` → not found; missing protein → dropped; `User-Agent` sent; `503` → `ErrSourceUnavailable`; budget window: 13th call within a minute → `ErrBudgetSpent` with seconds remaining and no request
- [ ] T023 [US2] Implement `internal/service/foodsource_off.go` with the mutex-guarded 12-per-minute window, tries 13-digit then the original 8- or 12-digit form, builds `attribution`
- [ ] T024 [P] [US2] Extend `internal/dao/food_dao_test.go`: `RecordBarcodeMiss` upserts `checked_at`; `IsRecentMiss` honours 7 days; `FindUserFoodByGTIN`
- [ ] T025 [US2] Implement misses and GTIN lookups in `internal/dao/food_dao.go`
- [ ] T026 [P] [US2] Extend `internal/service/food_svc_test.go`: `code` validation (`422 code` on length or check digit); chain order private → catalog → USDA → OFF → miss with a call recorder; recent miss short-circuits; `ErrBudgetSpent` → `NotFoundError` with `details.retry_after_seconds`; private food found even when a miss exists
- [ ] T027 [US2] Implement `LookupBarcode` in `internal/service/food_svc.go`; handler `LookupBarcode` (200, shared 404 with optional details) in `internal/handler/food.go`; route `/foods/barcode/{code}` with `FoodLookupLimit`; handler and router tests

**Checkpoint**: a packaged product resolves from either source and repeats come from the catalog.

## Phase 5: User Story 4 - Correct or remove an entry (P2) and the day rule

- [ ] T028 [P] [US4] Extend `internal/dao/food_entry_dao_test.go`: update with current revision recomputes sums and bumps entry and day revisions; stale → `ConflictError` with the entry as `Authoritative`; delete recomputes; delete-last deletes the day row; replay for `food-entries/put` and `food-entries/delete`; two concurrent creates serialise and both land in the sum
- [ ] T029 [US4] Implement `UpdateEntry` and `DeleteEntry` in `internal/dao/food_entry_dao.go`; service validation for `UpdateFoodEntryRequest` and the delete query parameters; handler `UpdateEntry` (200 `{entry, day}`) and `DeleteEntry` (200 `{day}`) with `Idempotency-Key` from the header; routes `/entries/{id}`; tests
- [ ] T030 [P] [US4] Extend `internal/dao/nutrition_dao_test.go`: manual save on a `foods` day → `DayHasFoodEntriesError` carrying the stored day; remove → same; Health save → `ignored`; scan includes the new source value
- [ ] T031 [US4] Add the `foods` branch to `SaveDay` in `internal/dao/nutrition_dao.go`; map `DayHasFoodEntriesError` to `409 DAY_HAS_FOOD_ENTRIES` with `resource` in `internal/handler/response.go`; handler test pins `REVISION_CONFLICT` and the plan-transition `CONFLICT` unchanged
- [ ] T032 [US4] Extend `internal/dao/food_entry_dao_test.go` and the DAO for conversion: first entry on a `manual` or `apple_health` day inserts a `quick_add` entry with the row's values and `logged_at`, returned under `converted`; a later entry on a `foods` day returns `converted: []`

**Checkpoint**: entries are correctable, totals never drift, and typed saves cannot wipe foods.

## Phase 6: User Story 3 - Log again in one tap (P2)

- [ ] T033 [P] [US3] Extend `internal/dao/food_dao_test.go`: `ListRecentFoods` groups the last 30 days by food reference, orders by count then latest `logged_at`, 20 cap, excludes `quick_add`, returns the latest entry's serving and amount, includes archived own foods
- [ ] T034 [US3] Implement `ListRecentFoods` (DAO, service, handler `{foods: []}`), route `/foods/recent`; tests

## Phase 7: User Story 5 - Create a food from a label (P3)

- [ ] T035 [P] [US5] Extend `internal/service/food_svc_test.go`: `SaveUserFoodRequest` validation (name 1–200, brand ≤ 120, serving label 1–60, grams 0–5000 exclusive, per-serving bounds, one decimal, barcode normalisation); per-100 g derivation in the response; `expected_revision` required on `PUT` and `DELETE`
- [ ] T036 [P] [US5] Extend `internal/dao/food_dao_test.go`: create (revision 1); update with revision check; archive sets `archived_at` under revision check; archived excluded from search and barcode; still resolvable by id for existing entries; foreign id → not found
- [ ] T037 [US5] Implement private-food DAO transactions (scopes `user-foods/post|put|delete`, namespace `nutrition:`) in `internal/dao/food_dao.go`
- [ ] T038 [US5] Implement service `CreateUserFood`, `UpdateUserFood`, `ArchiveUserFood`; handler `POST /foods` 201, `PUT /foods/{id}` 200, `DELETE /foods/{id}` 204; routes; handler and router tests
- [ ] T039 [US5] Service test: private foods come first in search and win the barcode chain; `CreateEntry` rejects an archived own food with the shared 404 unless the entry already exists (update path)

**Checkpoint**: all five stories independently testable; contract complete.

## Phase 8: Polish and Verification

- [ ] T040 Run the 50-GTIN fixture list (`testdata/gtin_sample.txt`, 10 Kirkland Signature items) once against the live USDA `DEMO_KEY` from a developer machine, record the resolve rate in the evidence table (SC-001); never in CI
- [ ] T041 Account deletion test asserts zero rows in `food_entries` and `user_foods` and that `food_catalog` rows survive
- [ ] T042 [P] Regenerate `docs/swagger.json` and `docs/swagger.yaml`; confirm only food and nutrition routes changed
- [ ] T043 Run `gofmt -w`, `./scripts/validate-database-design.sh`, `go test ./...` (service and middleware coverage ≥ 90%), `golangci-lint run`
- [ ] T044 Add smoke step 14 "Food logging": create private food 201 → barcode lookup returns it → entry 201 with `converted: []` and day `source: foods` → same-key replay 201 → manual `PUT /days/{today}` 409 `DAY_HAS_FOOD_ENTRIES` → Health `PUT` 200 unchanged → update entry 200 with new sums → stale revision 409 → recents lists the food → delete entry 200 `{day: null}` → second account sees `[]` and 404; search runs only when `USDA_FDC_API_KEY` is set, otherwise asserts `incomplete:true`; run `./scripts/smoke-toggle.sh`
- [ ] T045 Run `./scripts/validate-supabase-security.sh` against the migrated database (four tables RLS on, Data API privileges revoked, runtime-role policy present); record results in the evidence table
- [ ] T046 Update `README.md`/`CLAUDE.md` route and environment inventories (`USDA_FDC_API_KEY`, `FOOD_SOURCE_USER_AGENT`); `git diff --check`

## Dependencies & Execution Order

T001–T003 parallel. T004 → T005 → T006/T007; T008 and T009 parallel with T005. US1: T010/T012/T014/T016/T019 (tests) before T011 → T013 → T015 → T017 → T018 → T020 → T021. US2 (T022–T027) after T013. US4 (T028–T032) after T017; T030/T031 may run parallel with T028/T029. US3 (T033–T034) after T017. US5 (T035–T039) after T013. Phase 8 last. Deploy before the app branch `spec/017-food-logging`.

## Implementation Strategy

MVP is Phases 1–3: search, catalog, and entries that roll into the day, enough for the app's search-and-log story. Phase 4 adds scanning; Phase 5 makes entries safe to correct and protects them from typed saves; Phases 6 and 7 add recents and private foods. Each phase ends with `go test ./...` green and the evidence table updated.
