# Implementation Plan: Macro calculator and meal template

**Branch**: `feat/032-macro-calculator` | **Date**: 2026-11-16 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/032-macro-calculator/spec.md`

## Summary

A pure calculation package (`internal/nutritionplan`) turns an athlete's numbers into targets, an
optional training/rest split, and a four-meal template built from a fixed list of USDA foods, with
swaps. Four additive routes under `/api/v1/nutrition`: read options, calculate (stateless), save
the profile (one row per athlete, revisioned, idempotent), read the saved plan. One new table;
contract 1.11.0.

## Workflow and Specialist Scope

- **Feature**: `gympulse-macro-calculator`, new scope; sibling
  [app spec](../../../gym-pulse-app/specs/038-macro-calculator/spec.md). This repository ships first.
- **gym-pulse-api-engineering**: route shape, validation placement, transaction design, migration.
- **gym-pulse-cross-repo-delivery**: one calculation owner (this service); the app renders options
  and results and never re-derives a target. Capability probe is the options route.
- **supabase-postgres-best-practices**: one owner-keyed table, checks in the database for closed
  sets and ranges, `secure_table`, no secondary index (all access by primary key).
- **gym-pulse-engineering-lead**: owns the two-repo sequence. Single primary agent; one read-only
  survey of the app ran in parallel and an independent review runs on the final diff.

## Technical Context

**Language/Version**: Go 1.26+

**Primary Dependencies**: chi, pgx/pgxpool, google/uuid (no additions)

**Storage**: PostgreSQL; new `nutrition_profiles`; no change to existing tables

**Testing**: table-driven unit tests and a property sweep for `internal/nutritionplan`; service and
handler tests with stub DAOs; DAO test behind the `integration` tag; `go test ./...`,
`golangci-lint run`, `./scripts/smoke-toggle.sh`, `./scripts/validate-database-design.sh`

**Project Type**: Go web service

**Performance Goals**: a calculation is in-memory arithmetic over at most 3 templates × 4 meals ×
3 unknowns; no I/O. Save is one transaction with one row lock; read is one primary-key query.

**Constraints**: additive only; owner-scoped; the client sends no computed number; saved targets
never change without a save; foods and facts only from the fixed USDA list

**Scale/Scope**: one package, one migration, one model file, one DAO, one service, one handler,
four routes, export and account-deletion additions, contract and schema documentation, smoke step

## Constitution Check

*GATE: Passed before Phase 0 and re-checked after Phase 1 design.*

- [x] **Contract first**: `contracts/macro-calculator.md` lands in `docs/CONTRACTS.md` as "Macro
  calculator (contract 1.11.0)" with the export line, in the same change as the code.
- [x] **Owned authenticated data**: the saved plan is keyed by the authenticated `user_id`; there is
  no client-supplied owner or id, so a foreign plan cannot be addressed. Calculate and options hold
  no athlete data. Biometrics are never logged.
- [x] **Idempotent evolution**: `PUT /nutrition/profile` uses `Idempotency-Key` = `operation_key`,
  ledger replay, and `expected_revision` (0 when none) with `REVISION_CONFLICT`. Forward migration
  046 creates one table; down drops it; no existing data is touched.
- [x] **Executable evidence**: tests named in `tasks.md` precede each implementation task; service
  coverage stays ≥ 90%; smoke step added.
- [x] **Simple Go boundaries**: handler → service → DAO; the arithmetic lives in a dependency-free
  package the service calls. No new dependency, no concurrency.

## Design

### Package `internal/nutritionplan` (pure, no I/O)

- `calc.go`: `Calculate(Input) (Targets, []Notice)`; BMR, activity suggestion, goal ranges, macro
  maths, carb cycling split.
- `catalog.go`: the 18 foods (USDA per-100 g facts, FDC id, unit, grams per unit, step, tags,
  meal affinity) and the option lists.
- `meals.go`: per-meal targets (largest-remainder split), food selection, the portion solver, and
  rounding to each food's step.
- `plan.go`: `Build(Input, Targets) Plan`, used by calculate (fresh targets) and by the saved-plan
  read (stored targets).

**Portion solver**: the candidates are the meal's protein food, carbohydrate food, fat source, and
the protein food's lean companion (eggs → egg whites) when allowed. Every combination of one to
three candidates is fitted by energy-weighted least squares to the meal's protein, carbohydrate,
and fat; combinations needing a negative portion are discarded and the closest fit wins, with ties
going to the athlete's own foods before the companion and to more foods before fewer. Three foods
and three targets usually fit exactly. Amounts are then rounded to the food's step and the meal
reports what the rounded amounts provide. Rejected: a general optimizer (at most 14 small systems
per meal do not need one), and the first draft's fixed fallback to protein plus carbohydrate, which
the independent review showed could triple a meal's fat.

**Selection**: for each meal and role, in order: a preferred, allowed food suited to the meal and
not yet used that day; a preferred allowed food suited to the meal; any preferred allowed food; an
allowed food suited to the meal; any allowed food. Meal 3's carbohydrate prefers foods flagged
fast-digesting. A `meal_choices` entry overrides selection for its role.

**Cycling**: `rest = C × (1 − 0.175)` rounded to the nearest whole gram for which
`7C − rest_days × rest` divides evenly by `training_days` (one always exists within
`training_days` consecutive integers because 7 is prime); `training` is that quotient.

### Layers

- `internal/model/nutrition_plan.go`: wire types (`CalculatorInput`, `SaveNutritionProfileRequest`,
  `NutritionPlan`, `SavedNutritionPlan`, `CalculatorOptions`, `NutritionProfile` row, outcomes).
- `internal/service/nutrition_plan_svc.go`: validation with field names, default resolution,
  mapping to and from the package, hashing, save, read.
- `internal/dao/nutrition_profile_dao.go`: `Get`, `Save` (lock `nutrition:`, ledger replay, row
  lock, revision check, upsert, ledger write).
- `internal/handler/nutrition_plan.go` and routes in `internal/router/router.go`.
- `internal/dao/export_dao.go`, `internal/model/export.go`, `internal/dao/account_dao.go`.

### Compatibility and rollout

Additive. Started on `feat/031-one-active-program` (API #67) because migrations must be contiguous
and #67 held 045; #67 merged on 2026-11-16 and main is merged in, so this is 046 on main. Older apps never call the routes. The app probes
the options route and hides the calculator on `404`.

## Complexity Tracking

| Addition | Why | Simpler alternative rejected |
| --- | --- | --- |
| Fourth route `GET /nutrition/calculator/options` | The app must show foods, ranges, and defaults | Hard-coding them in the app duplicates business rules across repositories |
| Foods as Go data, not a table | 18 fixed published records, changed only by a release | A table adds a migration, seed, and read path for data athletes cannot change |
