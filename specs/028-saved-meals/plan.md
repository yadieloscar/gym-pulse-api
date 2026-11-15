# Implementation Plan: Saved meals

**Branch**: `spec/028-saved-meals` | **Date**: 2026-11-15 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/028-saved-meals/spec.md`

## Summary

Let an athlete store named lists of foods (food, serving, amount) and log one to a day in a single
request. Two new owner-scoped tables hold the meal and its items; no values are stored on a meal.
Logging runs in one transaction under the existing `nutrition:` lock and inserts ordinary
`food_entries` rows, reusing spec 014's snapshot maths, typed-totals conversion, and day rollup, so
existing entry, recents, and export behaviour is untouched. Five additive routes under
`/api/v1/nutrition`; contract 1.8.0.

## Workflow and Specialist Scope

- **Feature**: `gympulse-saved-meals`, new scope built on shipped spec 014; sibling
  [app spec](../../../gym-pulse-app/specs/033-saved-meals/spec.md). This repository ships and
  deploys first.
- **gym-pulse-api-engineering**: route shape, validation placement, transaction design, error
  codes, migration (this plan, `research.md`, `data-model.md`, `contracts/`).
- **gym-pulse-cross-repo-delivery**: additive rollout and mixed-version behaviour; the read shape
  carries full `Food` objects because the app logs offline (`research.md`, `quickstart.md`).
- **supabase-postgres-best-practices**: unique expression indexes for the name and duplicate-item
  rules, partial index on the private-food reference, check constraints, RLS and runtime-role
  policy on both tables (`data-model.md`).
- **gym-pulse-engineering-lead**: owns the two-repo sequence. Single-agent execution: one lane,
  sequential design, overlapping files.

## Technical Context

**Language/Version**: Go 1.26+

**Primary Dependencies**: chi, pgx/pgxpool, google/uuid (no additions)

**Storage**: PostgreSQL; new `saved_meals`, `saved_meal_items`; no change to existing tables

**Testing**: Go unit and handler tests with stub DAOs; DAO tests behind the `integration` tag
against a migrated PostgreSQL; `go test ./...`, `golangci-lint run`, `./scripts/smoke-toggle.sh`,
`./scripts/validate-database-design.sh`, `./scripts/validate-supabase-security.sh`

**Target Platform**: Linux HTTP service and Supabase-hosted PostgreSQL

**Project Type**: Go web service

**Performance Goals**: A meal log is one transaction with one food read, at most 20 entry inserts,
and one day rollup regardless of item count (SC-001). The list is two queries for up to 50 meals × 20
items.

**Constraints**: Additive only (FR-016); owner-scoped with the shared 404; no values stored on a
meal (FR-013); all-or-nothing log (FR-007); entries carry no link to a meal

**Scale/Scope**: One migration (two tables), one model file, one DAO, one service, one handler,
five routes, export and account-deletion additions, contract and schema documentation, one smoke
step

## Constitution Check

*GATE: Passed before Phase 0 and re-checked after Phase 1 design.*

- [x] Contract changes are identified in `contracts/saved-meals.openapi.yaml` and land in
  `docs/CONTRACTS.md` as a "Saved meals (contract 1.8.0)" section after "Food logging", plus one
  line in "Data export" for `nutrition.saved_meals`.
- [x] Authentication, ownership, idempotency, revision, and error behaviour are specified: meals
  are read and locked by `(user_id, id)` with the shared 404; a private food on an item is resolved
  through the owner-filtered food read; create, change, delete, and log use `Idempotency-Key` equal
  to `operation_key` with ledger replay; change and delete take `expected_revision`, stale →
  `REVISION_CONFLICT` with the meal as `resource`; one new stable code `SAVED_MEAL_LIMIT_REACHED`;
  item faults are `VALIDATION_ERROR` on `items` with `details.index`.
- [x] Tests are identified before implementation in `quickstart.md`, including contract smoke,
  schema-document validation, and the PostgreSQL integration suite.
- [x] The migration creates two tables with RLS, Data API revoke, and the runtime-role policy and
  touches no existing table or row. The down migration drops both tables; saved meals are lost,
  logged entries are not.
- [x] New dependency: none. New layer: none (handler → service → DAO as for foods). New
  concurrency: none; writes serialise under the existing per-athlete `nutrition:` advisory lock.

## Project Structure

### Documentation (this feature)

```text
specs/028-saved-meals/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/saved-meals.openapi.yaml
└── tasks.md                                          # next stage
```

### Source Code (repository root)

```text
cmd/server/main.go                                    # wire DAO, service, handler
docs/CONTRACTS.md                                     # "Saved meals" section; export line; codes
docs/DATABASE_DESIGN.md                               # two table sections, index rows, counts, FK matrix
migrations/039_create_saved_meals.{up,down}.sql
internal/
├── dao/{saved_meal_dao.go,saved_meal_dao_test.go}    # list, create, update, delete, log transactions
├── dao/saved_meal_integration_test.go                # //go:build integration
├── dao/food_dao.go                                   # listFoodsByIDs takes a querier so it runs in a transaction
├── dao/account_dao.go                                # delete saved_meals before user_foods
├── dao/export_dao.go                                 # readExportSavedMeals
├── dao/supabase_security_migration_test.go           # expect the two tables
├── handler/{saved_meal.go,saved_meal_test.go}        # five routes
├── handler/response.go                               # SAVED_MEAL_LIMIT_REACHED mapping; details.index
├── model/{saved_meal.go,saved_meal_test.go}          # SavedMeal, items, requests, outcomes, limit error, item values and totals
├── model/types.go                                    # ValidationError.Index
├── model/export.go                                   # ExportSavedMeal under ExportNutrition
├── router/router.go                                  # routes under /api/v1/nutrition
└── service/{saved_meal_svc.go,saved_meal_svc_test.go} # request validation, local-date rule, hashing
scripts/smoke.sh                                      # step 13k: save → list → log → replay → edit → delete
```

**Structure Decision**: A separate `SavedMeal` DAO, service, and handler beside the food ones,
rather than growing `food_svc.go` (481 lines) and `food_entry_dao.go` (418 lines). The meal DAO
shares the package-level helpers the entry DAO already exposes inside `internal/dao`
(`lockUserDomain`, `findIdempotency`, `insertIdempotency`, `getFood`, `lockNutritionDay`,
`insertFoodEntry`, `recomputeNutritionDay`), so the day rollup has one implementation.

*Amended during implementation (2026-11-15)*: the plan first proposed extracting the "typed totals
become one quick add" block out of `CreateEntry`. The existing static test
`TestFoodEntryDAOCreateKeepsAtomicOrder` pins that block inside `CreateEntry`, and weakening a test
to make room for a refactor is not allowed, so `CreateEntry` is untouched and the meal DAO has its
own ten-line `convertTypedTotals` that mirrors it. The only edit to shipped food code is that
`ListFoodsByIDs` now delegates to `listFoodsByIDs(ctx, querier, …)` so the same read runs inside a
transaction. Item values and totals are pure functions in `internal/model`
(`NewSavedMealItem`, `SumMealItems`) called by the DAO, because the response is stored in the
idempotency ledger inside the transaction.

## Design Assessment

| Concern | Decision |
|---|---|
| Contract | `GET/POST /nutrition/meals`, `PUT/DELETE /nutrition/meals/{id}`, `POST /nutrition/days/{date}/meal-logs`; statuses 200/201/204/404/409/422; collections `[]` never `null` |
| Identity and ownership | `userID` from the auth middleware through handler → service → DAO; every meal query filters `user_id`; private foods resolved through the owner-filtered read |
| Invariant placement | Database: name uniqueness, duplicate item, position range, exactly-one food reference, value bounds. Transaction: 50-meal count, archived rule, all-or-nothing log. Service: request shape, local-date rule, hashing |
| Transaction and races | One transaction per mutation under `nutrition:`; lock order is advisory lock → meal row → day row, the same order entry writes use for lock → day |
| Compatibility | Additive; older apps never call the routes and see meal-logged entries as ordinary entries |
| Evidence | See `quickstart.md` |

## Complexity Tracking

No constitution violations. The second table is the normal form for an ordered list with foreign
keys; the JSONB alternative is recorded in `research.md`.
