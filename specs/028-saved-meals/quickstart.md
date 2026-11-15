# Quickstart: Saved meals

Shapes are in [contracts/saved-meals.openapi.yaml](./contracts/saved-meals.openapi.yaml); tables
and transactions are in [data-model.md](./data-model.md).

## Implementation order

1. Fetch `origin/main` and confirm the migration number (039 unless taken). Migration up and down:
   two tables, indexes, checks, RLS, Data API revoke, runtime-role policy and grants;
   `docs/DATABASE_DESIGN.md`; `./scripts/validate-database-design.sh`; update
   `supabase_security_migration_test.go` expectations.
2. Model: `SavedMeal`, `SavedMealItem`, `SaveMealRequest`, `LogMealRequest`, `MealLogged`, outcomes,
   `SavedMealLimitError`; item-value and totals maths reusing `EntryValues`; unit tests.
3. Make `ListFoodsByIDs` delegate to a `querier`-based helper so it runs in a transaction; leave
   `CreateEntry` untouched (a static test pins its conversion block) and mirror the conversion in
   the meal DAO.
4. DAO: list, create, update, delete, log, each with ledger replay; integration tests.
5. Service: validation, hashing, local-date rule, item values, totals, `available`.
6. Handler, routes, `response.go` mapping, wiring in `main.go`, router test.
7. `account_dao.go` deletion order; `export_dao.go` and `model/export.go`.
8. `docs/CONTRACTS.md` (section, codes, export line); smoke step 13k; Swagger regeneration.
9. `gofmt`, `go test ./...`, `golangci-lint run`, `./scripts/smoke-toggle.sh`, integration suite,
   `./scripts/validate-supabase-security.sh` on a migrated database.

## Tests identified before implementation

| Requirement | Evidence |
|---|---|
| FR-001, US2/AC1 | Handler test: create → `201` with items in order; `422` for name of 0 or 61 characters, 0 or 21 items, amount 0.05 and 50.01, bad serving (each with `details.field`, items with `details.index`) |
| FR-002, US2/AC2–AC3 | Integration: same name in different case → `422` on `name`; two concurrent creates of one name → one `201`, one `422`; 51st meal → `409 SAVED_MEAL_LIMIT_REACHED` with `details.limit` 50 and no row added |
| FR-003, US3 | Integration: list ordered by `last_logged_at` then `created_at`; `[]` when none. Service test: item values and totals follow the food's current `per_100g` after it changes |
| FR-004, US4 | Integration: update with current revision → revision + 1 and items replaced; stale update and stale delete → `409 REVISION_CONFLICT` with the meal as `resource`; delete → `204`, then `404` |
| FR-005, US1/AC1 | Integration: log a three-item meal → three `food_entries` in item order with `per_100g × grams × amount / 100` snapshots; day `source: foods` |
| FR-006, US1/AC2 | Integration: log with an override list → those entries only; meal row and items unchanged, revision unchanged |
| FR-007, SC-003, US1/AC4 | Integration: third item names an unknown food → `422` with `details.index` 2, zero entries inserted, day unchanged, `last_logged_at` unchanged |
| FR-008, SC-002, US1/AC5 | Integration: day totals equal the sum after a log; log onto a `manual` day → one `quick_add` under `converted`; two concurrent logs from two connections both land and the sum is exact |
| FR-009, US1/AC3, US2/AC4 | Integration: verbatim replay of create `201`, update `200`, delete `204`, log `201` returns the same body and adds no rows; same key with a different payload → `409 IDEMPOTENCY_CONFLICT` |
| FR-010, SC-006 | Integration: an entry logged by a meal is changed and deleted through the existing entry routes and appears in recents; existing food tests and smoke step 13g pass unchanged |
| FR-011, SC-004 | Integration: after a log, update then delete the meal, and update the private food → the logged day's entries and totals are byte-identical |
| FR-012 | Integration: archive a private food in a meal → the meal still logs it; create, update, and an override list naming it → `422` with the item index |
| FR-013 | Schema test: neither table has an energy or macro column (`validate-database-design.sh` plus the documented column list) |
| FR-014, SC-005 | Handler and integration: another athlete's meal id on update, delete, and log → shared `404`; another athlete's private food as an item → the same `422` as an unknown id; account deletion leaves zero rows in both tables and succeeds with a private food referenced by a meal |
| FR-015 | Export test: `nutrition.saved_meals` lists name and items; `[]` when none |
| FR-016, FR-017 | `./scripts/smoke-toggle.sh` including new step 13k; contract section reviewed against the handler tests |
| Unavailable item (edge case) | Model tests: an implausible food yields `available: false`, no values, and `totals: null`; static DAO test: the log refuses an unavailable item before the day is locked |

## Manual acceptance

Create a private food and find one catalog food. `POST /meals` with both → `201`, `totals` equal
the sum. `GET /meals` → the meal. `POST /days/{today}/meal-logs` → `201` with two entries;
`GET /days/{today}/entries` → both, day totals equal the meal totals. Retry with the same key →
same body, still two entries. Log again with an override list of one item at amount 2 → one more
entry; `GET /meals` shows the meal unchanged. `PUT /meals/{id}` renaming it with the current
revision → `200`, revision 2; with revision 1 → `409`. `DELETE /meals/{id}` → `204`; the day still
lists its entries.

## Compatibility and rollback

Additive routes and tables. Deploy before the app release. Apps built before this feature never
call the routes; entries a meal logged are ordinary entries to them. Rollback: roll back the app,
then run the down migration, which drops `saved_meals` and `saved_meal_items`. Saved meals are lost;
every logged entry and day total stays. Roll-forward is preferred once athletes have saved meals.
