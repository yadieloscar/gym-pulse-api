# Quickstart: Food search, barcode lookup, and food entries

## Implementation order

1. Rebase on the merged nutrition branch and pick the migration number. Migration (up and down):
   four tables, the `nutrition_days.source` check, RLS, Data API revoke, runtime-role policy and
   grants; `docs/DATABASE_DESIGN.md`; `./scripts/validate-database-design.sh`; update
   `supabase_security_migration_test.go` expectations.
2. Model: `Food`, `UserFood`, `FoodEntry`, `RecentFood`, requests, outcomes, GTIN normalisation and
   check digit, snapshot math; acceptance tests.
3. Source adapters behind `FoodSource`: USDA search and barcode match, OFF product read with the
   budget window and `User-Agent`; `httptest` fixtures for found, not found, implausible, `429`,
   `503`, timeout.
4. Config: `USDA_FDC_API_KEY`, `FOOD_SOURCE_USER_AGENT`, base URL overrides; startup warning when
   the key is empty outside development and test.
5. DAOs with source-order tests: catalog upsert and search, barcode misses, private foods, entry
   transactions (convert, rollup, delete-last, conflicts, replay), recents.
6. Service: validation (q length, barcode shape, serving and amount bounds, quick-add has a value,
   dates), search merge and `incomplete`, barcode chain, plausibility filter, hashing.
7. `nutrition_dao.SaveDay`: `foods` branch → `DAY_HAS_FOOD_ENTRIES`; `response.go` mapping.
8. Handler, routes, `FoodLookupLimit`, wiring in `main.go`, router test; `account_dao.go` cleanup.
9. `docs/CONTRACTS.md` ("Food logging" section; amended `PUT /days/{date}`); smoke step 14;
   Swagger regeneration.
10. `gofmt`, `go test ./...`, `golangci-lint run`, `./scripts/smoke-toggle.sh`, security SQL on a
    migrated database.

## Tests identified before implementation

| Requirement | Evidence |
|---|---|
| FR-001, US1/AC1, AC3 | Handler search test: 25 cap, fields present, source set, `[]` and `incomplete:false` on no match; `422` on `q` shorter than 2 or longer than 100 |
| FR-002, US2/AC1–AC3 | Model tests: 8/12/13/14-digit forms normalise to one GTIN-14; bad check digit → `422 code`; service chain test: private → catalog → USDA → OFF → miss, each step asserted with fixtures |
| FR-003, SC-002, SC-005 | DAO test: second barcode lookup makes no outbound call; search test with USDA fixture returning `503` → local rows and `incomplete:true` |
| FR-004 | Config test: key read from env; adapter test: key only in the query string to the USDA host; log scrub test |
| FR-005, SC-003 | DAO test: entry snapshot unchanged after the catalog row's values change; archived private food keeps the entry valid |
| FR-006, SC-004 | DAO tests: sums after create, change, delete; delete-last removes the day; two concurrent creates serialise under the lock and both land in the sum |
| FR-007 | DAO tests: verbatim replay of `201/200`; payload mismatch → `IDEMPOTENCY_CONFLICT`; stale entry revision → `REVISION_CONFLICT` with the entry as `resource` |
| FR-008, SC-006 | `nutrition_dao` test: manual `PUT` on a `foods` day → `409 DAY_HAS_FOOD_ENTRIES` with the day; empty `PUT` → same; Health `PUT` → `200` ignored; conversion test: first entry on a manual day yields a `quick_add` in `converted` |
| FR-009, US3 | DAO test: recents ranked by count then recency, last serving and amount from the latest entry, 30-day window, 20 cap, quick adds excluded |
| FR-010, US5 | Handler tests: create → `201` with `source: own`; another athlete's `GET` → shared 404; private food wins the barcode chain; archive excludes from search but not from existing entries |
| FR-011 | Adapter tests: items missing protein or energy, negative, energy over 900 or macro over 100 per 100 g are dropped |
| FR-012 | Router test: 61st search in a minute → `429 RATE_LIMITED`; OFF budget test: 13th read in a minute → `404` with `retry_after_seconds`, no outbound call |
| FR-013 | Adapter test: OFF food carries `attribution`; USDA food has `attribution: null`; DAO row tagged `open_food_facts` |
| FR-014, SC-001 | Account deletion test asserts zero rows in `food_entries` and `user_foods`; fixture list of 50 GTINs run against the USDA DEMO_KEY once at implementation to record the resolve rate in `tasks.md` |
| FR-015 | Contract tests for the new section; `./scripts/smoke-toggle.sh` |

## Manual acceptance

Set `USDA_FDC_API_KEY`. Search "chicken breast" and see USDA foods with per-100 g values and
servings; search again with the key removed and see the same rows with `incomplete: true`. Look up a
Kirkland Signature UPC → `200` with `gtinUpc` matched; look it up again and see no outbound call in
the log. Look up a code USDA lacks and OFF has → `source: open_food_facts` with attribution. Log 1.5
servings of a food today → `201`, day `source: foods`, totals equal 1.5 × per-serving. Retry with
the same key → same body. `PUT /days/{today}` manually → `409 DAY_HAS_FOOD_ENTRIES`. Send a Health
save for today → `200` unchanged. Change the entry to 2 servings with its revision → totals change;
with a stale revision → `409`. Create a private food with a barcode, scan it → private food first.
Delete the entry → `{ day: null }`. Read recents → the food with its last serving.

## Compatibility and rollback

Additive routes and tables; one behaviour change on `PUT /days/{date}`, which only affects days that
have entries and so cannot exist before this release. Deploy before the app release. Spec 015 apps
keep working: they never see entries, and a typed save on a foods day is refused rather than
accepted. Rollback: roll back the app, run the down migration (it refuses while `foods` days
exist, so delete entries first or accept the data loss explicitly), which drops the four tables and
restores the two-value check.
