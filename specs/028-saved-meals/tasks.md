# Tasks: Saved meals

**Input**: [spec.md](spec.md), [plan.md](plan.md), [data-model.md](data-model.md), [research.md](research.md), [quickstart.md](quickstart.md), [contract](contracts/saved-meals.openapi.yaml) | **Feature ID**: `gympulse-saved-meals`

Tests are required by the constitution (Principle IV) and by SC-002..SC-006.

## Phase 1: Setup

- [X] T001 (renumbered 2026-11-15: PR #55 merged first and took 039, so these migrations are 040 and 041; the file names and references below were updated, the recorded runs were made under the old numbers and repeated after the renumbering, see "Renumbering" at the end) Fetch `origin/main`, confirm migration number 039 is free, and record a green baseline for `go test ./...`

## Phase 2: Foundational

- [X] T002 Create `migrations/040_create_saved_meals.up.sql` and `.down.sql` (two tables, checks, unique and partial indexes, RLS, Data API revocation, runtime-role policy and grants)
- [X] T003 Document both tables, indexes, FK matrix rows, counts, and migration 040 in `docs/DATABASE_DESIGN.md`; update expectations in `internal/dao/supabase_security_migration_test.go`; run `./scripts/validate-database-design.sh`
- [X] T004 [P] Add `SavedMeal`, `SavedMealItem`, `MealTotals`, `MealItemInput`, `SaveMealRequest`, `LogMealRequest`, `MealLogged`, outcomes, `SavedMealLimitError`, and item-value/total maths in `internal/model/saved_meal.go` with tests in `internal/model/saved_meal_test.go`
- [X] T005 Make `ListFoodsByIDs` delegate to a `querier`-based `listFoodsByIDs` in `internal/dao/food_dao.go` so it runs in a transaction. *Amended*: `CreateEntry` is left untouched because `TestFoodEntryDAOCreateKeepsAtomicOrder` pins its conversion block; the meal DAO mirrors it in `convertTypedTotals`
- [X] T006 Map `SavedMealLimitError` to `409 SAVED_MEAL_LIMIT_REACHED` (`details.limit`) and carry `details.index` on item validation errors in `internal/handler/response.go` and `internal/model`

## Phase 3: User Story 2 - Save a meal (P1)

**Independent test**: create a meal with two foods → `201` with items in order and totals; same name in another case → `422` on `name`; 51st → `409`.

- [X] T007 [P] [US2] Service tests for name, item count, amount, duplicate food-and-serving, operation key, and hashing in `internal/service/saved_meal_svc_test.go`
- [X] T008 [P] [US2] Handler tests for `201`, `422` shapes with `details.field` and `details.index`, `409` limit, and header mismatch in `internal/handler/saved_meal_test.go`
- [X] T009 [US2] Implement `CreateMeal` in `internal/dao/saved_meal_dao.go` (lock, ledger replay, count, food resolution with item index, insert, name-collision mapping, ledger write)
- [X] T010 [US2] Implement validation, hashing, and response assembly (item values, totals, `available`) in `internal/service/saved_meal_svc.go`
- [X] T011 [US2] Implement `Create` in `internal/handler/saved_meal.go`; wire the route in `internal/router/router.go` and the DAO, service, and handler in `cmd/server/main.go` and router call sites

## Phase 4: User Story 3 - See saved meals (P1)

**Independent test**: two meals, one logged → the logged one first; none → `{"meals":[]}`.

- [X] T012 [US3] Implement `ListMeals` in the DAO (two queries, ordering), service (current-fact values), and handler; route and router test

## Phase 5: User Story 1 - Log a saved meal in one request (P1)

**Independent test**: log a three-item meal → three entries in order, day totals equal the sum; retry → same body, still three; bad third item → `422` index 2 and nothing logged.

- [X] T013 [P] [US1] Service tests for the date rule, override-list validation, and hashing; handler tests for `201`, `404`, `422` in the existing saved-meal test files
- [X] T014 [US1] Implement `LogMeal` in `internal/dao/saved_meal_dao.go` (lock, replay, meal row lock, item resolution, archived rule, snapshots, day lock, conversion, ordered multi-row insert, one rollup, `last_logged_at`, ledger)
- [X] T015 [US1] Implement `LogMeal` in the service and handler; route `POST /nutrition/days/{date}/meal-logs`; router test

## Phase 6: User Story 4 - Change or remove a saved meal (P2)

**Independent test**: update with the current revision → revision + 1; stale → `409` with the meal; delete → `204`; logged days unchanged throughout.

- [X] T016 [P] [US4] Service and handler tests for update and delete (revision, `404`, replay statuses)
- [X] T017 [US4] Implement `UpdateMeal` and `DeleteMeal` in the DAO, service, and handler; routes and router test

## Phase 7: Cross-cutting

- [X] T018 Delete `saved_meals` before `user_foods` in `internal/dao/account_dao.go` with its static test
- [X] T019 Add `nutrition.saved_meals` to `internal/model/export.go` and `internal/dao/export_dao.go` with test updates
- [X] T020 PostgreSQL integration tests in `internal/dao/saved_meal_integration_test.go`: create, replay, name race, limit, list order, update, stale, delete, log (order, snapshots, override, atomic failure, conversion, concurrent logs, replay), archived rule, ownership, history unchanged after meal and food changes, account deletion, export
- [X] T021 Document "Saved meals (contract 1.8.0)", the new code, and the export line in `docs/CONTRACTS.md`; regenerate Swagger if the repository generates it
- [X] T022 Add step 13k to `scripts/smoke.sh`: save → list → log → replay → override log → edit → stale → delete → day keeps entries

## Phase 8: Verification

- [X] T023 Run static gates (`gofmt`, `go test ./...`, `golangci-lint run`, `./scripts/validate-database-design.sh`) and record results below
- [X] T024 Run PostgreSQL-backed gates (integration suite, `./scripts/smoke-toggle.sh`, `./scripts/validate-supabase-security.sh`, migration down/up) against a disposable stack and record results below
- [X] T025 Independent read-only review of the final diff against the plan's Design Assessment; record findings below
- [X] T026 Fix review findings 1–3: match a saved serving by label on read and on a log as saved (`internal/model/saved_meal.go`, `internal/dao/saved_meal_dao.go`); bound and de-duplicate weights as stored (`internal/service/saved_meal_svc.go`); map the item unique index to the item error; tests; FR-018 in `spec.md` and `docs/CONTRACTS.md`

## Phase 9: Amendment A1 - day headings (FR-019..FR-024)

- [X] T027 Create `migrations/041_add_meal_sections.up.sql` and `.down.sql`; document the two columns, counts, and migration 041 in `docs/DATABASE_DESIGN.md`; migration-contract test in `internal/model/saved_meal_test.go`
- [X] T028 [P] Add `MealSection` and the `section` fields on `FoodEntry`, `CreateFoodEntryRequest`, `MoveFoodEntryRequest`, `SavedMeal`, `SaveMealRequest`, `LogMealRequest` in `internal/model/food.go` and `internal/model/saved_meal.go` with tests
- [X] T029 [US5] Store and read the heading on entries and add `MoveEntry` in `internal/dao/food_entry_dao.go`; static test that a move writes only heading, revision, and timestamp
- [X] T030 [US5] Validate `section` on create and add `MoveEntry` in `internal/service/food_svc.go`; handler `MoveEntry` in `internal/handler/food.go`; route `PUT /nutrition/entries/{id}/section`; service, handler, and router tests
- [X] T031 [US5] Saved meals remember a heading and the log applies it (`internal/dao/saved_meal_dao.go`, `internal/service/saved_meal_svc.go`); tests
- [X] T032 Add `section` to exported entries and saved meals (`internal/model/export.go`, `internal/dao/export_dao.go`)
- [X] T033 PostgreSQL integration tests for headings in `internal/dao/saved_meal_integration_test.go`
- [X] T034 Document headings in `docs/CONTRACTS.md`; regenerate Swagger; extend smoke step 13k
- [X] T035 Rerun every gate from Phase 8 and record the results below

## Dependencies

T001 → T002..T006 → US2 (create) → US3 (list) → US1 (log) → US4 → cross-cutting → verification.
Logging needs a meal to exist, so US2 precedes US1 although both are P1. `[P]` tasks touch
different files from their neighbours.

## Evidence

| Requirement | Tasks | Verification |
|---|---|---|
| FR-001 | T004, T007–T011 | service + handler tests, integration, smoke |
| FR-002 | T002, T006, T009, T020 | unique index, integration (case, race, limit), handler test |
| FR-003 | T012, T020 | service test (current facts), integration (order), smoke |
| FR-004 | T016, T017, T020 | service + handler tests, integration |
| FR-005 | T014, T015, T020 | integration (order, snapshots), smoke |
| FR-006 | T013–T015, T020 | integration (override leaves the meal unchanged) |
| FR-007 | T014, T020 | integration (failed item logs nothing) |
| FR-008 | T005, T014, T020 | integration (sum, conversion, concurrent logs) |
| FR-009 | T009, T014, T017, T020 | integration replays, idempotency conflict |
| FR-010 | T014, T020, T022 | integration (entry routes on meal-logged entries), existing food tests, smoke 13g |
| FR-011 | T020 | integration (history unchanged) |
| FR-012 | T009, T014, T020 | integration (archived rule) |
| FR-013 | T002, T003 | database-design validator, documented columns |
| FR-014 | T009, T017, T018, T020 | handler + integration ownership tests, account deletion |
| FR-015 | T019, T020 | export tests |
| FR-016 | T022, T024 | smoke including unchanged step 13g |
| FR-017 | T021 | CONTRACTS.md reviewed against handler tests |
| FR-018 | T026 | model test, integration (rounded weight, re-weighed and relabelled serving), smoke |
| FR-019 | T027–T030, T033 | service tests, database check, integration, smoke |
| FR-020 | T029, T030, T033 | static DAO test, integration (values and day byte-identical), smoke |
| FR-021 | T029, T033 | integration |
| FR-022 | T031, T033 | integration, smoke |
| FR-023 | T033 | integration; unchanged nutrition and recents tests |
| FR-024 | T032, T033 | export integration |

## Verification (2026-11-15)

Run against a disposable stack built from this branch (`gpmeals`: PostgreSQL 16 on 15452, API on
18090, runtime role `gym_pulse_api`, migrations applied by the API on boot).

| Check | Result |
|---|---|
| `gofmt -l cmd internal` | no files |
| `go vet ./...` | passed |
| `go test ./...` | passed (8 packages) |
| `golangci-lint run` | 0 issues |
| `./scripts/validate-database-design.sh` | passed: 43 tables, 445 columns, 49 named indexes |
| Service and middleware coverage | 91.0% and 91.4% (floor 90%) |
| `go test -race -tags=integration ./internal/dao -count=1` (whole suite, no `-run`) | passed, including 11 saved-meal tests |
| `scripts/smoke.sh` against the branch image | 35 passed, 0 failed (new step 13k; step 13g unchanged) |
| `scripts/validate-supabase-security.sh` (run inside the db container) | passed |
| Migration 040 down, then up, on the migrated database | both clean; 2 tables and 2 policies restored |
| `swag init -g cmd/server/main.go --output docs` | regenerated and committed |
| `./scripts/smoke-toggle.sh` | not run: it targets the shared `gym-pulse-api-1` dev container, which is not running; `smoke.sh` was run directly against the branch image instead |

### Found and fixed during verification

- **Identity removal failed** when a saved meal held an own food: `ON DELETE RESTRICT` (and a
  non-deferred `NO ACTION`) on `saved_meal_items.user_food_id` is checked inside the cascaded
  `user_foods` delete, before the sibling cascade removes the meal. The key is now `NO ACTION
  DEFERRABLE INITIALLY DEFERRED`. Covered by `TestSavedMealAccountDeletionCascadeAndExport`. Shipped
  food logging was probed for the same failure and is not affected.
- **List read consistency**: `ListMeals` now reads meals, items, and foods in one read-only
  repeatable-read transaction.

### Independent review (fresh-context, read-only, 2026-11-15)

No authorization, ownership, transaction, idempotency, or lock-order defect found. Findings:

| # | Severity | Finding | Outcome |
|---|---|---|---|
| 1 | High | A source serving with more than one decimal (28.35 g) is stored as 28.4 g; the log as saved re-validated the stored weight and refused the meal with `422` | Fixed (T026, FR-018) |
| 2 | Medium/High | Re-weighing or relabelling an own food's serving made every log as saved fail while the list still showed the meal as available | Fixed (T026, FR-018) |
| 3 | Medium | 0.04 g, and 100 g with 100.04 g of one food, passed validation and failed a database check or unique index as a server error | Fixed (T026): bounds and duplicates use the weight as stored; index backstop mapped to the item error |
| 4 | Low | The down migration drops both tables without a guard (030 guards; 037 and 038 do not) | Accepted: the data loss is stated in `quickstart.md`; entries and day totals are unaffected |
| 5 | Low | No index on the referencing side of `catalog_food_id` | Accepted: catalog rows are never deleted, and the index would only add write cost |
| 6 | Low | Static DAO tests inspect source text; behaviour is proven by the integration suite, which `go test ./...` does not run | Accepted as the repository's convention; the missing behavioural cases (findings 1–3, replay after delete) now have integration tests |

The same one-decimal bound hole exists on the shipped `POST /days/{date}/entries` (a grams-only
serving of 0.04 g); it is outside this feature and is not changed here.

### Amendment A1 rerun (2026-11-15, after day headings)

| Check | Result |
|---|---|
| `gofmt`, `go vet ./...`, `go test ./...`, `golangci-lint run` | clean; 8 packages passed; 0 issues |
| `./scripts/validate-database-design.sh` | passed: 43 tables, 447 columns, 49 named indexes |
| Service and middleware coverage | 91.1% and 91.4% |
| Whole integration suite with `-race` | passed, including `TestDayHeadingsAreLabelsOnly` |
| `scripts/smoke.sh` against the branch image | 35 passed, 0 failed; step 13k now covers headings; step 13g (no headings) unchanged |
| `scripts/validate-supabase-security.sh` | passed |
| Migration 041 down, then up | both clean |
| Swagger | regenerated |

The amendment was not given a second independent review; the first review covered commits up to
the saved-meal log, not headings.

### Not verified

- SC-001's one-second figure was not measured; a meal log is one transaction of at most 22
  statements.
- No run against a production-like Supabase project; nothing is deployed.

## Renumbering (2026-11-15)

PR #55 merged to `main` first and took migration 039 (`039_narrow_runtime_auth_users_grant`). After merging `main` into this branch the two migrations here became `040_create_saved_meals` and `041_add_meal_sections`; no SQL changed. One conflict (`scripts/smoke.sh`, both sides added a step) was resolved by keeping both steps, and a router test added on `main` gained the new handler argument.

| Check after the merge and renumbering | Result |
|---|---|
| `gofmt -l .`, `go vet ./...`, `go test ./...`, `golangci-lint run` | clean, 0 issues |
| `./scripts/validate-database-design.sh` | passed: 43 tables, 447 columns, 49 indexes |
| `swag init -g cmd/server/main.go --output docs` | no change |
| Fresh scratch database migrated by the API image | `schema_migrations` at 41, not dirty |
| `./scripts/smoke.sh` against that image (both test users seeded) | 36 passed, 0 failed, including 13k and the cross-user step from `main` |
| `go test -race -tags=integration ./internal/dao -count=1` | passed |
| Security validator SQL on that database | passed |

Not checked: the cross-user smoke step from `main` does not exercise the saved-meal routes; ownership of those routes is covered by the DAO integration tests and handler tests in this branch.
