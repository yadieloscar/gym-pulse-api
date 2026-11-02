# Implementation Plan: Food search, barcode lookup, and food entries

**Branch**: `spec/014-food-logging` | **Date**: 2026-10-27 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/014-food-logging/spec.md`

## Summary

Add a shared food catalog filled lazily from USDA FoodData Central and Open Food Facts, private
athlete foods, and per-day food entries whose sum is written into the existing `nutrition_days` row
in the same transaction. Search and barcode lookup are served by the API with server-held
credentials; repeat lookups come from the catalog. New routes live under `/api/v1/nutrition/foods`
and `/api/v1/nutrition/days/{date}/entries` plus `/api/v1/nutrition/entries/{id}`. The spec 012
`PUT /days/{date}` gains one rule: a day with entries refuses typed totals with `409
DAY_HAS_FOOD_ENTRIES`, and a Health save over it is ignored as it already is over a manual day.

## Workflow and Specialist Scope

- **Feature**: `gympulse-food-logging`, new scope stacked on `gympulse-nutrition-targets`; sibling
  [app plan](../../../gym-pulse-app/specs/017-food-logging/plan.md). This repository ships first.
- **gym-pulse-api-engineering**: routes, validation placement, source adapters, transactions,
  migration (this plan, `research.md`, `data-model.md`, `contracts/`).
- **gym-pulse-cross-repo-delivery**: additive rollout, mixed-version window, the one behaviour
  change on `PUT /days/{date}` (`quickstart.md`).
- **supabase-postgres-best-practices**: shared-catalog table without `user_id`, generated `tsvector`
  column with GIN index, partial indexes, check constraints, RLS and runtime-role policy on all
  new tables.
- **gym-pulse-engineering-lead**: owns the two-repo sequence and the migration renumbering against
  PR #36; single-agent execution, one lane.

## Technical Context

**Language/Version**: Go 1.26+

**Primary Dependencies**: chi, pgx/pgxpool, google/uuid, net/http client (no additions; the outbound
budget limiter is a small in-process counter, see `research.md`)

**Storage**: PostgreSQL; new `food_catalog`, `user_foods`, `food_entries`, `food_barcode_misses`;
`nutrition_days.source` gains the value `foods`

**Testing**: Go unit, handler, DAO, and adapter tests with `httptest` servers standing in for USDA
and Open Food Facts; migration acceptance test; `go test ./...`, `./scripts/smoke-toggle.sh`,
`./scripts/validate-database-design.sh`, security SQL on a migrated database

**Target Platform**: Linux HTTP service and Supabase-hosted PostgreSQL; outbound HTTPS to
`api.nal.usda.gov` and `world.openfoodfacts.org`

**Project Type**: Go web service

**Performance Goals**: Catalog hit on barcode in one indexed query (SC-002 under 300 ms p95);
search is one local query plus at most one USDA call bounded by a 3 s timeout; entry writes are one
transaction touching `food_entries` and `nutrition_days`

**Constraints**: Additive except the documented `409` on `PUT /days/{date}`; owner-scoped
entries and private foods; the catalog is shared and never user-editable; keys stay server-side;
Open Food Facts allows 15 product reads per minute per IP, so the server holds a global budget and
caches misses; USDA allows 1,000 requests per hour per key

**Scale/Scope**: One migration (four tables, one constraint change), two source adapters, one
model file, two DAOs, one service, one handler, ten routes, contract and schema documentation, one
smoke step

## Constitution Check

*GATE: Passed before Phase 0 and re-checked after Phase 1 design.*

- [x] Contract changes are identified in `contracts/food-logging.openapi.yaml` and land in
  `docs/CONTRACTS.md` as a new "Food logging" section after "Nutrition", plus the amended
  `PUT /api/v1/nutrition/days/{date}` entry (`foods` source, `DAY_HAS_FOOD_ENTRIES`).
- [x] Authentication, ownership, idempotency, revision, and error behaviour are specified: entries
  and private foods are owner-scoped with the shared 404; the catalog is read-only to clients;
  entry create, change, and delete use `Idempotency-Key` plus `expected_revision`; stale →
  `REVISION_CONFLICT` with the entry as `resource`; new stable code `DAY_HAS_FOOD_ENTRIES`.
- [x] Tests are identified before implementation in `quickstart.md`, including adapter fixtures,
  contract smoke, and schema-document validation.
- [x] The migration creates four tables with RLS, Data API revoke, and the runtime-role policy, and
  widens the `nutrition_days.source` check; existing rows are untouched; the down migration drops the
  tables and restores the two-value check after asserting no `foods` rows remain.
- [x] New dependency: none. New layer: `internal/service/foodsource_*.go` adapters behind one
  interface, justified because two outside HTTP sources must be swappable and fixture-tested. New
  concurrency: none beyond the request; the outbound budget counter is mutex-guarded.

## Project Structure

### Documentation (this feature)

```text
specs/014-food-logging/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/food-logging.openapi.yaml
└── tasks.md
```

### Source Code (repository root)

```text
cmd/server/main.go                                   # wire config, adapters, DAOs, service, handler
docs/CONTRACTS.md                                    # "Food logging" section; amended PUT /days/{date}
docs/DATABASE_DESIGN.md                              # four table sections, index rows, counts
migrations/030_create_food_logging.{up,down}.sql     # nutrition renumbers to 029 after PR #36 took 027–028
internal/
├── config/config.go                                 # USDA_FDC_API_KEY, FOOD_SOURCE_USER_AGENT, base URL overrides
├── dao/{food_dao.go,food_dao_test.go}               # catalog, private foods, barcode misses, recents
├── dao/{food_entry_dao.go,food_entry_dao_test.go}   # entry transactions that also write nutrition_days
├── dao/nutrition_dao.go                             # SaveDay: foods-day branch
├── dao/account_dao.go                               # delete food_entries and user_foods
├── handler/{food.go,food_test.go}                   # foods and entries routes
├── handler/response.go                              # DAY_HAS_FOOD_ENTRIES mapping
├── middleware/rate_limit.go                         # FoodLookupLimit = 60
├── model/{food.go,food_acceptance_test.go}          # Food, UserFood, FoodEntry, requests, outcomes, barcode
├── router/router.go                                 # routes under /api/v1/nutrition
└── service/{food_svc.go,food_svc_test.go}           # search merge, barcode chain, validation, day rollup rules
    service/{foodsource.go,foodsource_usda.go,foodsource_off.go,foodsource_test.go}
internal/service/testdata/{usda_search.json,usda_barcode.json,off_product.json,gtin_sample.txt}
scripts/smoke.sh                                     # step 14: private food → entry → day → refused PUT
```

**Structure Decision**: Mirror the nutrition feature: handler → service → DAO, with the DAO owning
each transaction. Outside sources sit in the service package like `avatar_storage.go` does, behind a
`FoodSource` interface so tests inject `httptest` fixtures. The entry DAO writes `nutrition_days`
itself rather than calling the nutrition DAO, because the rollup must be in the same transaction
and under the same `nutrition:` advisory lock.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Fourth table `food_barcode_misses` | Open Food Facts allows 15 product reads per minute per IP and the API is one IP; a scanned unknown barcode would otherwise be re-asked on every retry | An in-memory set empties on restart and is per instance; a catalog row with no values would corrupt the catalog's meaning |
| Generated `tsvector` column on `food_catalog` | Search must keep answering from the catalog when USDA is down (FR-003) | `ILIKE '%term%'` cannot use an index and misses word order; a separate search service is far outside scope |
