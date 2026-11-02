---
description: "Implementation tasks for the rule-based program engine"
---

# Tasks: Rule-Based Program Engine

**Input**: Design documents from `/specs/016-rule-based-program-engine/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/program-generation-engine.openapi.yaml, quickstart.md; gym-pulse-api#39 (spec 015) merged and this branch rebased on it

**Tests**: Required by the API constitution; identified before each implementation task. The engine is pure, so every engine test runs without a database or network; migration and DAO tests use PostgreSQL.

**Organization**: Grouped by user story. US1 (instant draft) is the MVP and carries the engine; US2 (constraints), US3 (variants), and US4 (secondary goals) extend it.

## Format: `[ID] [P?] [Story] Description`

## Requirement and Verification Evidence

| Requirement / acceptance | Tasks | Verification | Result / limitation |
| --- | --- | --- | --- |
| FR-001 | T007, T008, T026, T027 | Config tests: default `rules`, `model` with key, `model` without key falls back with a warning; status `provider`; main wiring | Planned |
| FR-002, US1/AC1 | T014, T015, T016, T017 | Adapter test: engine output passes `RepairGeneratedDraft` with no change; service test: validator still runs | Planned |
| FR-003, US1/AC2, US3, SC-003 | T012, T013, T021, T022 | Engine tests: `Build` twice equal; variant rotation; day-offset variety; committed fixture snapshot | Planned |
| FR-004, US2/AC4 | T009, T010, T024, T025 | Model tests: unknown `avoid`/`focus`, bad `variant` → 422 naming the field; free text ignored on the engine path; handler passes the fields | Planned |
| FR-005, SC-004 | T003, T004, T005, T006 | Migration test on PostgreSQL: 79 rows non-null, CHECKs reject bad values; catalog handler returns the four fields | Planned |
| FR-006 | T011, T012 | `tables_test.go`: every goal × experience and split × days value asserted; source comment per table | Planned |
| FR-007, US1/AC3, US1/AC4 | T012, T013 | Engine tests: slots by minutes, beginner cap and no explosive, equipment eligibility per value, `full_gym`, bodyweight always eligible | Planned |
| FR-008, US2/AC1–AC3 | T018, T019, T020 | Engine tests: avoided patterns absent, focus share, slot dropped with a note, `ErrTooConstrained`; service maps to 422 `avoid` | Planned |
| FR-009 | T012, T013 | Rationale tests: ≤ 500 characters, sentences for split, main lifts, secondary, avoid; deny list | Planned |
| FR-010, SC-002 | T016, T017, T030 | Service test: no ledger calls on the rules path; smoke timing step p95 < 200 ms over 20 calls | Planned |
| FR-011 | T017 | Log capture test: provider, duration, split, workouts only | Planned |
| FR-012 | T002, T028 | CONTRACTS.md and OpenAPI in the same change; Swagger regenerated | Planned |
| US4/AC1–AC2 | T023 | Engine tests: secondary share and untouched primary targets; none → primary only | Planned |
| SC-001 | T014 | Exhaustive matrix test: 6 goals × 1..6 days × 3 experience × 8 equipment sets → valid draft, never `ErrTooConstrained` | Planned |
| SC-005 | T029 | `go test ./...`; spec 015 tests unchanged apart from new fields | Planned |
| Checklist open item | T001 | Owner confirms the equipment assumptions in data-model.md | Unverified |

## Phase 1: Setup

- [ ] T001 Rebase `spec/016-rule-based-program-engine` on `main` after gym-pulse-api#39 merges; confirm `032` is the next migration number; confirm with the owner the equipment assumptions in data-model.md (bar-dependent entries `full_gym`; swings, carries, goblet squats `dumbbells`; swimming and sled `full_gym`) and update the seed mapping if they change
- [ ] T002 [P] Draft the `docs/CONTRACTS.md` amendments from `contracts/program-generation-engine.openapi.yaml`: "Exercise catalog" gains the four fields; "Program generation" gains `provider` on status and response, `avoid`, `focus`, `variant` on the request with their 422s, the too-constrained answer, and the provider rule (rules default, model opt-in, ledger applies to model only)

## Phase 2: Foundational

- [ ] T003 [P] Add `internal/dao/exercise_catalog_migration_test.go` (PostgreSQL): after migration 032 every row has non-null `pattern`, `muscle_group`, `equipment_required`, `impact`; CHECK rejects an unknown pattern, muscle group, impact, and an equipment value outside the profile set; `idx_exercise_catalog_pattern` exists; down then up is clean
- [ ] T004 Add `migrations/032_add_exercise_catalog_programming.{up,down}.sql` per data-model.md: add the four nullable columns, `UPDATE ... WHERE (category, name) = (...)` for all 79 rows from the seed mapping, then `ALTER ... SET NOT NULL` and the CHECK constraints (including an element-wise check on `equipment_required` via a `CHECK (equipment_required <@ ARRAY[...]::text[])`), the index; down drops index and columns; add the table changes and migration ledger row to `docs/DATABASE_DESIGN.md`; run `./scripts/validate-database-design.sh`
- [ ] T005 [P] Extend `internal/model/exercise_catalog.go` with `Pattern`, `MuscleGroup`, `EquipmentRequired []string`, `Impact` (JSON `pattern`, `muscle_group`, `equipment_required`, `impact`); extend the catalog DAO select and scan; extend the catalog handler test to assert the fields and the existing JSON shape otherwise unchanged
- [ ] T006 [P] Add `internal/model/exercise_catalog_test.go`: `EligibleFor(equipment []string) bool` on `CatalogExercise` (subset rule, `full_gym` opens all, `{bodyweight}` always eligible, empty required never eligible)
- [ ] T007 [P] Add `internal/config/config_test.go` cases: `PROGRAM_GENERATION_PROVIDER` unset → `rules`; `rules` with a key → `rules`; `model` with a key → `model`; `model` without a key → `rules` plus a recorded warning; any other value → load error
- [ ] T008 Extend `internal/config/config.go`: `ProgramGenerationProvider` with the enum and fallback; replace `ProgramGenerationEnabled()` by `ProgramGenerationProvider()` returning the effective provider; document in `README.md` and `.env.example`
- [ ] T009 [P] Add `internal/model/program_generation_test.go` cases: `NormalizeGenerationConstraints(avoid, focus, variant)` accepts the pattern enum (dedupe, trim, at most 6), focus enum or empty, variant 0..1000; each bad input → `ValidationError` naming `avoid`, `focus`, or `variant`; `GenerateProgramResponse` JSON always carries `provider`, `avoid` (empty array, never null), and `focus` (null when none)
- [ ] T010 Extend `internal/model/program_generation.go`: `Avoid []string`, `Focus *string`, `Variant *int` on the request; `Provider`, `Avoid`, `Focus` on the response and `Provider` on `GenerationStatus`; `NormalizeGenerationConstraints`; constants `ProviderRules`, `ProviderModel`, the pattern and focus enums (shared with the migration values); `ErrTooConstrained` sentinel in `service` (T017)

## Phase 3: User Story 1 - Get a draft instantly from my answers (P1) 🎯 MVP

- [ ] T011 [P] [US1] Add `internal/engine/tables_test.go`: one assertion per cell of research R3 (split by days for default, hypertrophy, conditioning; slot order per workout kind; slots and beginner cap per session minutes; reps, sets, rest per goal × experience for main and accessory; cardio durations and interval sets; secondary shares); every table has a `// Source:` comment naming the spec 015 research entry (a test reads the file and fails on a missing comment)
- [ ] T012 [P] [US1] Add `internal/engine/engine_test.go` and `rationale_test.go` for US1: eligibility filtering per equipment value; slots by minutes and beginner cap with compounds plus core only and no explosive entries; bodyweight-only draft and rationale line; split chosen per day count and goal; `WorkoutCap` respected for 7 days; targets applied by goal and experience with main compound lower bound and isolation upper bound; cardio finisher costs a slot under 45 minutes; `Build` twice equal; rationale ≤ 500 characters, contains the split and the main lifts, never a deny-list word (`diet`, `weight`, `lose`, `burn`, `medical`, `injury`, `calorie`, `fat`)
- [ ] T013 [US1] Implement `internal/engine/tables.go` (typed data for every table in research R3 with source comments), `internal/engine/engine.go` (`Input`, `Draft`, `Workout`, `Exercise`, `Explanation`, `Build`: resolve split → for each workout take the slot list, apply focus and avoid, filter eligible, pick `(variant + workoutIndex) mod len`, apply targets, append cardio per goal, enforce 3..8 and the session cap), `internal/engine/rationale.go` (`Rationale(Explanation) string`)
- [ ] T014 [P] [US1] Add `internal/engine/matrix_test.go`: for every goal × days 1..6 × experience × equipment set in {each single value, `full_gym`} call `Build` with no constraints and assert a draft whose workouts count `min(days, 6)`, each with 3..8 unique catalog ids, every exercise eligible, strength targets within the goal's range, cardio durations 30..3600; convert through the adapter and assert `RepairGeneratedDraft` returns an equal draft (SC-001, FR-002)
- [ ] T015 [P] [US1] Add `internal/service/program_generator_rules_test.go`: adapter maps `GenerationInput` (plus `Avoid`, `Focus`, `Variant`) to `engine.Input`, returns `RawGeneratedDraft` with the rationale, zero usage, and ignores `Request`; `ErrTooConstrained` passes through unchanged; a committed JSON fixture of one draft (strength, 3 days, intermediate, barbell+dumbbells, 60 min, variant 0) matches byte for byte
- [ ] T016 [P] [US1] Extend `internal/service/program_generation_svc_test.go`: with the rules generator the call order is profile → catalog → generate → repair with no `reserve` or `complete`; `Status` reports `provider: "rules"`, `enabled: true`, `remaining_today = limit`; with the model generator the existing order and ledger behavior are unchanged; response carries `provider`, `avoid`, `focus`
- [ ] T017 [US1] Implement `internal/service/program_generator_rules.go` (`NewRulesDraftGenerator()` implementing `DraftGenerator`) and extend `program_generation_svc.go`: a `provider` field, constraint normalization after `resolveProfile`, ledger skipped for rules, `ErrTooConstrained` → `ValidationError{Field: "avoid", Message: "Those limits leave too little to build a workout from; drop one and try again."}`, structured log with provider, duration, split, workouts

## Phase 4: User Story 2 - Steer the draft with structured constraints (P2)

- [ ] T018 [P] [US2] Add engine tests: `avoid` removes every exercise with the pattern and fills from the remaining slots; `focus: upper` gives about two thirds of strength slots to chest/back/shoulders/arms with at least one lower slot; `focus: lower` and `core` likewise; a slot with no eligible exercise is dropped and `Explanation.Dropped` names it; fewer than three eligible exercises for any workout → `ErrTooConstrained`
- [ ] T019 [US2] Implement focus reordering and avoid filtering in `engine.go`; `Explanation.Honoured` and `Dropped`; rationale sentences "No <pattern words>, as you asked." and "Could not fit <slot> with your limits."
- [ ] T020 [US2] Extend `internal/handler/program_generation_test.go`: `avoid`, `focus`, `variant` pass through; 422 `avoid` for an unknown pattern and for the too-constrained answer with the plain message; 422 `focus` and `variant`; response echoes `avoid` and `focus`

## Phase 5: User Story 3 - Try another and get a different valid draft (P3)

- [ ] T021 [P] [US3] Add engine tests: variants 0..3 on the fixture profile differ in at least one exercise per workout where the eligible list has more than one entry; a single-entry slot stays fixed; wrap-around at `len`; `workoutIndex` offset makes three full-body days differ in their first slot when alternatives exist
- [ ] T022 [US3] Implement the `(variant + workoutIndex) mod len` selection with sorted eligible lists in `engine.go` (likely done in T013; this task records the proof)

## Phase 6: User Story 4 - Primary and secondary goals shape the plan (P3)

- [ ] T023 [US4] Add engine tests and implement in `engine.go` and `tables.go`: secondary `conditioning` adds a 10-minute finisher on at most two days; secondary `strength` adds one heavy compound first; secondary `hypertrophy` one isolation slot; secondary `power` (non-beginner) one explosive slot; secondary `general_health`/`body_composition` the cardio finisher; primary targets untouched; none → primary only; rationale names the secondary share

## Phase 7: Wiring, Documentation, Verification

- [ ] T024 [P] Extend `internal/handler/program_generation.go` swag godoc for the new fields and `provider`; `internal/handler/response.go` unchanged (existing `VALIDATION_ERROR` mapping covers the new fields)
- [ ] T025 [P] Extend the router test for the unchanged route set; the `RateLimitPerUser` stays on the POST
- [ ] T026 Wire `cmd/server/main.go`: build the rules generator by default; the Anthropic generator only when the effective provider is `model`; pass the provider to the service; remove the "program generation is disabled" warning path
- [ ] T027 [P] Update `README.md` and `.env.example` for `PROGRAM_GENERATION_PROVIDER` and note that `ANTHROPIC_API_KEY` is optional
- [ ] T028 Apply T002 to `docs/CONTRACTS.md`; run `swag init -g cmd/server/main.go --output docs`; review the Swagger diff for drift
- [ ] T029 Run `gofmt -w`, `go vet ./...`, `golangci-lint run`, `go test ./...`, `go test -race ./internal/engine/... ./internal/service/...`; `./scripts/validate-database-design.sh`
- [ ] T030 Update `scripts/smoke.sh`: 16b becomes `POST /program-generation` → 200 with `provider: "rules"` and three workouts; 16c repeats the call and asserts an identical body; 16d sends `variant: 1` and asserts a different body; 16e sends `avoid` of every pattern → 422 `avoid`; 16f records p95 over 20 calls and fails above 200 ms; run `./scripts/smoke-toggle.sh` or the isolated stack per `api-smoke-against-worktree`
- [ ] T031 Run `./scripts/validate-supabase-security.sh` against the migrated database; record results
- [ ] T032 Fill the Result column of the evidence table; note any table value changed during implementation back into research R3
- [ ] T033 Write the sibling app amendment brief for spec 019 (constraint chips replace the request field, `provider` read hides the wait copy, `variant` counter on "Try another", mock catalog fields) and hand it to the app repository

## Dependencies & Execution Order

T001 first. T002, T003, T005, T006, T007, T009 in parallel; T004 after T003; T008 after T007; T010 after T009. US1: T011, T012, T014, T015, T016 (tests) in parallel, then T013 → T017. US2 (T018 → T019 → T020) and US3 (T021 → T022) and US4 (T023) after T017, in any order. Phase 7: T024, T025, T027 parallel with the stories; T026 after T017; T028 → T029 → T030 → T031 → T032 last; T033 after T028.

## Implementation Strategy

MVP is Phase 1 through Phase 3: an instant, deterministic, valid draft from the answers with the provider switch, which the current app can already use with no change (free text ignored). US2 adds the chips' backend, US3 the variant proof, US4 the secondary shares. Nothing in this feature spends provider credit; the model path is exercised only by the existing spec 015 tests.
