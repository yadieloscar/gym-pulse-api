# Tasks: Macro calculator and meal template

**Input**: [spec.md](spec.md), [plan.md](plan.md), [data-model.md](data-model.md), [research.md](research.md), [quickstart.md](quickstart.md), [contract](contracts/macro-calculator.md) | **Feature ID**: `gympulse-macro-calculator`

Tests are required by the constitution (Principle IV) and by SC-001..SC-005.

## Phase 1: Setup

- [X] T001 (renumbered 2026-11-17: API #69 merged first and took 046, so this migration is 047; no SQL change, gates re-run) Fetch origin, confirm 046 is the next migration after `feat/031-one-active-program`, record a green `go test ./...` baseline

## Phase 2: Calculation package (US1, US2)

- [X] T002 [US1] Tests in `internal/nutritionplan/calc_test.go`: imperial male and female worked examples, metric equivalence, each goal default and range, activity suggestion boundaries and override, protein and fat defaults and ranges, carbohydrate floor notice, below-resting notice
- [X] T003 [US1] Implement `internal/nutritionplan/calc.go` (FR-001..FR-005)
- [X] T004 [US2] Tests for cycling: every split 1–6 sums to seven standard days, protein and fat unchanged, 0 and 7 training days not applied
- [X] T005 [US2] Implement the cycling split (FR-006)

## Phase 3: Foods and template (US3, US4, US6)

- [X] T006 [US6] `internal/nutritionplan/catalog.go`: 18 USDA foods with FDC ids recorded in `research.md`; test that every food has facts, a source id, a unit, and that energy agrees with 4/4/9 within the published rounding
- [X] T007 [US3] Tests in `internal/nutritionplan/meals_test.go`: per-meal targets sum to the day for a sweep of targets; split percentages; preferred foods used; Meal 3 fast carbohydrate; every exclusion honoured across the sweep; egg-white companion; units and steps; provided vs target within the rounding step
- [X] T008 [US3] Implement selection, solver, rounding in `internal/nutritionplan/meals.go` and `plan.go` (FR-007..FR-009)
- [X] T009 [US4] Tests and implementation for swaps: only the named meal changes, target identical, wrong role or excluded food rejected (FR-010)

## Phase 4: Service, handler, routes (US1, US6)

- [X] T010 [P] Wire types in `internal/model/nutrition_plan.go`
- [X] T011 [US1] Service tests in `internal/service/nutrition_plan_svc_test.go`: every validation rule names its field; defaults resolved in the echoed input; options lists non-null
- [X] T012 [US1] Implement `Options` and `Calculate` in `internal/service/nutrition_plan_svc.go`
- [X] T013 [US1] Handler tests and implementation in `internal/handler/nutrition_plan.go`; routes in `internal/router/router.go`; route list test; wiring in `cmd/server/main.go`

## Phase 5: Save and read (US5)

- [X] T014 [US5] `migrations/047_create_nutrition_profiles.up.sql` and `.down.sql`; `docs/DATABASE_DESIGN.md`; `./scripts/validate-database-design.sh`
- [X] T015 [US5] Service and handler tests for save (201/200, stale revision, key mismatch, replay) and read (200, 404)
- [X] T016 [US5] Implement `internal/dao/nutrition_profile_dao.go` and the service and handler methods (FR-011, FR-012)
- [X] T017 [US5] Integration test `internal/dao/nutrition_profile_integration_test.go`: create, replace, stale revision, replay, idempotency conflict, cascade on identity deletion
- [X] T018 [US5] Account deletion statement and data export `nutrition.calculator_profile` with tests (FR-014)

## Phase 6: Contract and gates

- [X] T019 `docs/CONTRACTS.md` "Macro calculator (contract 1.11.0)" and the export line; regenerate Swagger docs if the repository's generator is available
- [X] T020 Smoke step in `scripts/smoke.sh`: options, calculate, save, read, stale save
- [X] T021 Run `gofmt`, `go test ./...`, `golangci-lint run`, `./scripts/validate-database-design.sh`, `./scripts/smoke-toggle.sh`, and the integration suite; record results below
- [X] T022 Independent read-only review of the final diff; fix or record findings. Findings (2026-11-16): no blockers. Fixed: the solver's fallback always kept protein + carbohydrate and could abandon the egg-white companion or drop the fat source (replaced by a best-fit search with regression and sweep tests); `per_100g` example; `strength_days`/`cardio_days` documented as defaulting to 0; conflict `resource` wording; zero-weight guard in `buildMeal`; a note on the cycling share at small intakes. Added in the same pass: `limits` in the options so the app holds no copy of the ranges

## Phase 7: Amendment A1 (loggable template items, US7)

- [X] T023 [US7] `USDAName` on every template food with a catalog test
- [X] T024 [US7] Service tests: catalog rows written once, ids stable, serving loggable and equal to the item's values, failure served without ids and retried, saved plan carries ids
- [X] T025 [US7] `TemplateFoodCatalog`, `catalog_food_id` and `serving` on items, `catalog_food_id` on option foods; wiring in `cmd/server/main.go`
- [X] T026 [US7] `docs/CONTRACTS.md` (1.12.0), Swagger, smoke step 13m extended: log a template item and save a template meal through the existing routes
- [X] T027 [US7] Gates re-run (see Evidence, "A1")

## Evidence

| Check | Result |
| --- | --- |
| `go test ./...` | Passed 2026-11-16 (service coverage 90.9%, `internal/nutritionplan` 96.5%, middleware 92.2%) |
| `golangci-lint run` | Passed, 0 issues |
| `./scripts/validate-database-design.sh` | Passed: 44 tables, 477 columns |
| `swag init` | Regenerated; `docs/` committed |
| Integration `-p 1 -race -tags=integration ./internal/dao ./internal/dbmigrate` | Passed against an isolated migrated database (version 47 after the renumbering, runtime role present) |
| `scripts/smoke.sh` | 41 passed, 0 failed against an image built from this branch (isolated stack, API as `gym_pulse_api`, migrate job first); step 13m is new |
| `scripts/validate-supabase-security.sh` | Passed on the same database |
| `./scripts/smoke-toggle.sh` | Not run: it tests the shared dev stack's image, not this branch. The same `smoke.sh` ran against this branch's image instead |
| `govulncheck` | Not run: no dependency change |
| A1 (2026-11-17): `go test ./...`, `golangci-lint run`, schema validation | Passed; service coverage 91.0% |
| A1: integration suite | Passed on an isolated migrated database (version 48) |
| A1: `scripts/smoke.sh` | 41 passed, 0 failed on an image built from this branch. The first run failed, usefully: the catalog keeps per-100 g facts to one decimal, so a logged portion differed from the template by 0.1 g. Item values are now computed the way a food entry's are, and a sweep over weights, goals, cycling, and exclusions pins equality |
