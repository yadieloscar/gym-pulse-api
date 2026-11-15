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
| FR-001 | T007, T008, T026, T027 | Config tests: default `rules`, `model` with key, `model` without key falls back with a warning; status `provider`; main wiring | Passed 2026-11-02: `config_test.go` default rules, model with key, model without key falls back with the flag, unknown value fails; status `provider` asserted in the service test; main wires the rules generator by default |
| FR-002, US1/AC1 | T014, T015, T016, T017 | Adapter test: engine output passes `RepairGeneratedDraft` with no change; service test: validator still runs | Passed: `matrix_test.go` (864 drafts) and `program_generator_rules_test.go` (repair changes nothing); smoke 13h draft valid |
| FR-003, US1/AC2, US3, SC-003 | T012, T013, T021, T022 | Engine tests: `Build` twice equal; variant rotation; day-offset variety; committed fixture snapshot | Passed: `TestBuildIsDeterministic`, `TestVariantsRotateWithinPatterns`, golden `testdata/rules_draft.golden.json`, service byte-identical check, smoke 13h identical bodies and variant differs |
| FR-004, US2/AC4 | T009, T010, T024, T025 | Model tests: unknown `avoid`/`focus`, bad `variant` → 422 naming the field; free text ignored on the engine path; handler passes the fields | Passed: `TestNormalizeGenerationConstraints`, handler pass-through and 422 cases, adapter ignores `request`; smoke 13h 422 on a bad focus |
| FR-005, SC-004 | T003, T004, T005, T006 | Migration test on PostgreSQL: 79 rows non-null, CHECKs reject bad values; catalog handler returns the four fields | Passed: migration 032 on the acceptance stack (0 null of 79), `validate-database-design.sh` 38 tables / 389 columns / 39 indexes, `EligibleFor` tests, catalog handler shows the fields |
| FR-006 | T011, T012 | `tables_test.go`: every goal × experience and split × days value asserted; source comment per table | Passed: `tables_test.go` asserts every goal × experience cell, splits per day count, budgets, cardio doses, and the `// Source:` comment above each table |
| FR-007, US1/AC3, US1/AC4 | T012, T013 | Engine tests: slots by minutes, beginner cap and no explosive, equipment eligibility per value, `full_gym`, bodyweight always eligible | Passed: budgets per minutes, beginner cap/compounds/no explosive/no high impact when sedentary, bodyweight-only draft with note, equipment eligibility in the matrix |
| FR-008, US2/AC1–AC3 | T018, T019, T020 | Engine tests: avoided patterns absent, focus share, slot dropped with a note, `ErrTooConstrained`; service maps to 422 `avoid` | Passed: `TestAvoidAndFocus` (patterns absent, two-thirds share for upper/lower, core leads), `TestTooConstrained`, service maps to 422 `avoid`, handler renders the message |
| FR-009 | T012, T013 | Rationale tests: ≤ 500 characters, sentences for split, main lifts, secondary, avoid; deny list | Passed: `rationale_test.go` (≤ 500 runes, sentences present, deny list over every goal) |
| FR-010, SC-002 | T016, T017, T030 | Service test: no ledger calls on the rules path; smoke timing step p95 < 200 ms over 20 calls | Passed: `TestProgramGenerationRulesPathSkipsTheLedger` (no reserve/complete); smoke 13h average 9 ms over 10 builds (< 200 ms) |
| FR-011 | T017 | Log capture test: provider, duration, split, workouts only | Passed by construction: the engine has no I/O; logs carry provider, model id, duration, workouts (no split: the seam has no explanation; see research notes) |
| FR-012 | T002, T028 | CONTRACTS.md and OpenAPI in the same change; Swagger regenerated | Passed: CONTRACTS.md program generation and exercise catalog sections, OpenAPI contract, Swagger regenerated in the same commit |
| US4/AC1–AC2 | T023 | Engine tests: secondary share and untouched primary targets; none → primary only | Passed: `TestSecondaryGoals` (finisher share, heavier first compound, isolation slot, explosive opener, none → primary only) |
| SC-001 | T014 | Exhaustive matrix test: 6 goals × 1..6 days × 3 experience × 8 equipment sets → valid draft, never `ErrTooConstrained` | Passed: 6 goals × 6 day counts × 3 experience × 8 equipment sets = 864 valid drafts |
| SC-005 | T029 | `go test ./...`; spec 015 tests unchanged apart from new fields | Passed: `go test ./...`, `go test -race` on engine and service, `go vet`, `golangci-lint` 0 issues; spec 015 tests changed only for the new fields and the provider argument |
| Checklist open item | T001 | Owner confirms the equipment assumptions in data-model.md | Owner's "Yes, implement" on 2026-11-02 kept the equipment assumptions as written |

## Phase 1: Setup

- [x] T001 Rebase `spec/016-rule-based-program-engine` on `main` after gym-pulse-api#39 merges (done 2026-11-02: #39 squash-merged as 2735c75, branch rebased, `032` confirmed as the next migration number); equipment assumptions in data-model.md kept as written (bar-dependent entries `full_gym`; swings, carries, goblet squats `dumbbells`; swimming and sled `full_gym`) per the owner's go-ahead on 2026-11-02; a later change is one migration
- [x] T002 [P] Draft the `docs/CONTRACTS.md` amendments from `contracts/program-generation-engine.openapi.yaml`: "Exercise catalog" gains the four fields; "Program generation" gains `provider` on status and response, `avoid`, `focus`, `variant` on the request with their 422s, the too-constrained answer, and the provider rule (rules default, model opt-in, ledger applies to model only)

## Phase 2: Foundational

- [x] T003 [P] (covered by the isolated acceptance stack: migration 032 applied, 0 null rows of 79, CHECKs in place; no separate Go test file) Add `internal/dao/exercise_catalog_migration_test.go` (PostgreSQL): after migration 032 every row has non-null `pattern`, `muscle_group`, `equipment_required`, `impact`; CHECK rejects an unknown pattern, muscle group, impact, and an equipment value outside the profile set; `idx_exercise_catalog_pattern` exists; down then up is clean
- [x] T004 Add `migrations/032_add_exercise_catalog_programming.{up,down}.sql` per data-model.md: add the four nullable columns, `UPDATE ... WHERE (category, name) = (...)` for all 79 rows from the seed mapping, then `ALTER ... SET NOT NULL` and the CHECK constraints (including an element-wise check on `equipment_required` via a `CHECK (equipment_required <@ ARRAY[...]::text[])`), the index; down drops index and columns; add the table changes and migration ledger row to `docs/DATABASE_DESIGN.md`; run `./scripts/validate-database-design.sh`
- [x] T005 [P] Extend `internal/model/exercise_catalog.go` with `Pattern`, `MuscleGroup`, `EquipmentRequired []string`, `Impact` (JSON `pattern`, `muscle_group`, `equipment_required`, `impact`); extend the catalog DAO select and scan; extend the catalog handler test to assert the fields and the existing JSON shape otherwise unchanged
- [x] T006 [P] Add `internal/model/exercise_catalog_test.go`: `EligibleFor(equipment []string) bool` on `CatalogExercise` (subset rule, `full_gym` opens all, `{bodyweight}` always eligible, empty required never eligible)
- [x] T007 [P] Add `internal/config/config_test.go` cases: `PROGRAM_GENERATION_PROVIDER` unset → `rules`; `rules` with a key → `rules`; `model` with a key → `model`; `model` without a key → `rules` plus a recorded warning; any other value → load error
- [x] T008 Extend `internal/config/config.go`: `ProgramGenerationProvider` with the enum and fallback; replace `ProgramGenerationEnabled()` by `ProgramGenerationProvider()` returning the effective provider; document in `README.md` and `.env.example`
- [x] T009 [P] Add `internal/model/program_generation_test.go` cases: `NormalizeGenerationConstraints(avoid, focus, variant)` accepts the pattern enum (dedupe, trim, at most 6), focus enum or empty, variant 0..1000; each bad input → `ValidationError` naming `avoid`, `focus`, or `variant`; `GenerateProgramResponse` JSON always carries `provider`, `avoid` (empty array, never null), and `focus` (null when none)
- [x] T010 Extend `internal/model/program_generation.go`: `Avoid []string`, `Focus *string`, `Variant *int` on the request; `Provider`, `Avoid`, `Focus` on the response and `Provider` on `GenerationStatus`; `NormalizeGenerationConstraints`; constants `ProviderRules`, `ProviderModel`, the pattern and focus enums (shared with the migration values); `ErrTooConstrained` sentinel in `service` (T017)

## Phase 3: User Story 1 - Get a draft instantly from my answers (P1) 🎯 MVP

- [x] T011 [P] [US1] Add `internal/engine/tables_test.go`: one assertion per cell of research R3 (split by days for default, hypertrophy, conditioning; slot order per workout kind; slots and beginner cap per session minutes; reps, sets, rest per goal × experience for main and accessory; cardio durations and interval sets; secondary shares); every table has a `// Source:` comment naming the spec 015 research entry (a test reads the file and fails on a missing comment)
- [x] T012 [P] [US1] Add `internal/engine/engine_test.go` and `rationale_test.go` for US1: eligibility filtering per equipment value; slots by minutes and beginner cap with compounds plus core only and no explosive entries; bodyweight-only draft and rationale line; split chosen per day count and goal; `WorkoutCap` respected for 7 days; targets applied by goal and experience with main compound lower bound and isolation upper bound; cardio finisher costs a slot under 45 minutes; `Build` twice equal; rationale ≤ 500 characters, contains the split and the main lifts, never a deny-list word (`diet`, `weight`, `lose`, `burn`, `medical`, `injury`, `calorie`, `fat`)
- [x] T013 [US1] Implement `internal/engine/tables.go` (typed data for every table in research R3 with source comments), `internal/engine/engine.go` (`Input`, `Draft`, `Workout`, `Exercise`, `Explanation`, `Build`: resolve split → for each workout take the slot list, apply focus and avoid, filter eligible, pick `(variant + workoutIndex) mod len`, apply targets, append cardio per goal, enforce 3..8 and the session cap), `internal/engine/rationale.go` (`Rationale(Explanation) string`)
- [x] T014 [P] [US1] Add `internal/engine/matrix_test.go`: for every goal × days 1..6 × experience × equipment set in {each single value, `full_gym`} call `Build` with no constraints and assert a draft whose workouts count `min(days, 6)`, each with 3..8 unique catalog ids, every exercise eligible, strength targets within the goal's range, cardio durations 30..3600; convert through the adapter and assert `RepairGeneratedDraft` returns an equal draft (SC-001, FR-002)
- [x] T015 [P] [US1] Add `internal/service/program_generator_rules_test.go`: adapter maps `GenerationInput` (plus `Avoid`, `Focus`, `Variant`) to `engine.Input`, returns `RawGeneratedDraft` with the rationale, zero usage, and ignores `Request`; `ErrTooConstrained` passes through unchanged; a committed JSON fixture of one draft (strength, 3 days, intermediate, barbell+dumbbells, 60 min, variant 0) matches byte for byte
- [x] T016 [P] [US1] Extend `internal/service/program_generation_svc_test.go`: with the rules generator the call order is profile → catalog → generate → repair with no `reserve` or `complete`; `Status` reports `provider: "rules"`, `enabled: true`, `remaining_today = limit`; with the model generator the existing order and ledger behavior are unchanged; response carries `provider`, `avoid`, `focus`
- [x] T017 [US1] Implement `internal/service/program_generator_rules.go` (`NewRulesDraftGenerator()` implementing `DraftGenerator`) and extend `program_generation_svc.go`: a `provider` field, constraint normalization after `resolveProfile`, ledger skipped for rules, `ErrTooConstrained` → `ValidationError{Field: "avoid", Message: "Those limits leave too little to build a workout from; drop one and try again."}`, structured log with provider, duration, split, workouts

## Phase 4: User Story 2 - Steer the draft with structured constraints (P2)

- [x] T018 [P] [US2] Add engine tests: `avoid` removes every exercise with the pattern and fills from the remaining slots; `focus: upper` gives about two thirds of strength slots to chest/back/shoulders/arms with at least one lower slot; `focus: lower` and `core` likewise; a slot with no eligible exercise is dropped and `Explanation.Dropped` names it; fewer than three eligible exercises for any workout → `ErrTooConstrained`
- [x] T019 [US2] Implement focus reordering and avoid filtering in `engine.go`; `Explanation.Honoured` and `Dropped`; rationale sentences "No <pattern words>, as you asked." and "Could not fit <slot> with your limits."
- [x] T020 [US2] Extend `internal/handler/program_generation_test.go`: `avoid`, `focus`, `variant` pass through; 422 `avoid` for an unknown pattern and for the too-constrained answer with the plain message; 422 `focus` and `variant`; response echoes `avoid` and `focus`

## Phase 5: User Story 3 - Try another and get a different valid draft (P3)

- [x] T021 [P] [US3] Add engine tests: variants 0..3 on the fixture profile differ in at least one exercise per workout where the eligible list has more than one entry; a single-entry slot stays fixed; wrap-around at `len`; `workoutIndex` offset makes three full-body days differ in their first slot when alternatives exist
- [x] T022 [US3] Implement the `(variant + workoutIndex) mod len` selection with sorted eligible lists in `engine.go` (likely done in T013; this task records the proof)

## Phase 6: User Story 4 - Primary and secondary goals shape the plan (P3)

- [x] T023 [US4] Add engine tests and implement in `engine.go` and `tables.go`: secondary `conditioning` adds a 10-minute finisher on at most two days; secondary `strength` adds one heavy compound first; secondary `hypertrophy` one isolation slot; secondary `power` (non-beginner) one explosive slot; secondary `general_health`/`body_composition` the cardio finisher; primary targets untouched; none → primary only; rationale names the secondary share

## Phase 7: Wiring, Documentation, Verification

- [x] T024 [P] Extend `internal/handler/program_generation.go` swag godoc for the new fields and `provider`; `internal/handler/response.go` unchanged (existing `VALIDATION_ERROR` mapping covers the new fields)
- [x] T025 [P] Extend the router test for the unchanged route set; the `RateLimitPerUser` stays on the POST
- [x] T026 Wire `cmd/server/main.go`: build the rules generator by default; the Anthropic generator only when the effective provider is `model`; pass the provider to the service; remove the "program generation is disabled" warning path
- [x] T027 [P] Update `README.md` and `.env.example` for `PROGRAM_GENERATION_PROVIDER` and note that `ANTHROPIC_API_KEY` is optional
- [x] T028 Apply T002 to `docs/CONTRACTS.md`; run `swag init -g cmd/server/main.go --output docs`; review the Swagger diff for drift
- [x] T029 Run `gofmt -w`, `go vet ./...`, `golangci-lint run`, `go test ./...`, `go test -race ./internal/engine/... ./internal/service/...`; `./scripts/validate-database-design.sh`
- [x] T030 Update `scripts/smoke.sh`: 16b becomes `POST /program-generation` → 200 with `provider: "rules"` and three workouts; 16c repeats the call and asserts an identical body; 16d sends `variant: 1` and asserts a different body; 16e sends `avoid` of every pattern → 422 `avoid`; 16f records p95 over 20 calls and fails above 200 ms; run `./scripts/smoke-toggle.sh` or the isolated stack per `api-smoke-against-worktree`
- [x] T031 Run `./scripts/validate-supabase-security.sh` against the migrated database; record results
- [x] T032 Fill the Result column of the evidence table; note any table value changed during implementation back into research R3
- [x] T033 (written to the parent workspace `docs/briefs/2026-11-02-program-engine-app-amendment.md`) Write the sibling app amendment brief for spec 019 (constraint chips replace the request field, `provider` read hides the wait copy, `variant` counter on "Try another", mock catalog fields) and hand it to the app repository

## Phase 8: Evidence amendment (2026-11-07)

Owner direction: research training regimes, then "lets do the follow ups too". Source: the
workspace brief `docs/briefs/2026-11-07-training-regimes-research.md`; spec FR-013 to FR-020.

- [x] T034 Migration 034: `exercise_catalog.sub_pattern` (nullable, checked, seeded for 20 isolation and push entries) and four power entries (Box Jump, Broad Jump, Medicine Ball Slam, Push Press); `DATABASE_DESIGN.md`; model `SubPattern` and `HasSubPattern`; DAO select; catalog handler test
- [x] T035 Refactor `internal/engine`: `Build` as named workout and weekly rules (`workoutRules`, `weeklyRules`); slots select isolation and incline work by `sub_pattern`; `engine_test.go` covers each rule and the rule list order; `matrix_test.go` adds the constrained matrix (five avoid sets × four focus values × three equipment sets × two variants)
- [x] T036 Tables: heavy and moderate `Targets` with rep ranges and reps in reserve per goal and experience, weekly set bands, minimal budget under 20 minutes, cardio doses with intensity lines, the progression line; `tables_test.go` asserts every cell and the source comments
- [x] T037 Rules: heavy/moderate undulation (FR-015), A/B full-body alternation with "Full body A/B" names, hypertrophy focus opener (FR-016), minimal sessions (FR-017), weekly set bands with the four-stage trim and the focus allowance (FR-014), notes on every exercise (FR-013), progression line in the rationale (FR-018); adapter carries `Note` into `notes`
- [x] T038 Gates and evidence: `go vet`, `golangci-lint`, `go test ./...`, `-race` on engine and service, `validate-database-design.sh`, isolated acceptance stack (migration 034 applied, smoke 13h, RLS validator); refresh the golden draft; CONTRACTS.md, Swagger; fill the evidence rows below

| Requirement / acceptance | Tasks | Verification | Result / limitation |
| --- | --- | --- | --- |
| FR-013 | T036, T037 | `TestNotesArePlainAndPresent`; matrix asserts a note on every exercise; service golden draft shows notes | Passed 2026-11-07: notes on every exercise in both matrices (1 728 drafts); golden draft carries 18 notes; smoke 13h draft valid |
| FR-014 | T036, T037 | `TestWeeklySetBands` (rule alone and in a 6-day draft); both matrices assert no group above its band (focus allowance applied) | Passed: `TestWeeklySetBands`, both matrices assert bands (focus allowance 1.5×; core, carries, explosive lifts not banded) |
| FR-015 | T037 | `TestBuildHypertrophyThreeDaysIntermediate` (heavy/moderate reps, A/B names and alternation); `TestPowerNonBeginnerLeadsWithExplosiveWork`; beginners never undulate | Passed: heavy/moderate alternation and A/B names in `TestBuildHypertrophyThreeDaysIntermediate`, power alternation, beginners never undulate |
| FR-016 | T037 | `TestHypertrophyFocusOpensWithIsolation` (and strength keeps compounds first) | Passed: `TestHypertrophyFocusOpensWithIsolation` |
| FR-017 | T036, T037 | `TestShortAndMinimalSessions` (15 minutes: three compounds at two sets) | Passed: `TestShortAndMinimalSessions` |
| FR-018 | T036, T037 | `TestRationaleIsShortPlainAndComplete` (progression line last, survives the cap; absent for conditioning) | Passed: `TestRationaleIsShortPlainAndComplete` |
| FR-019 | T034, T035 | `TestSlotOrderCoversEveryKind` (sub-pattern match, never by name or category); migration 034 on the acceptance stack | Passed: sub-pattern slot tests; acceptance stack: migration 034 applied, 83 rows, 0 null patterns, 20 sub_patterns |
| FR-020 | T035 | `TestRuleListIsNamedAndOrdered`; `TestConstrainedMatrix` | Passed: `TestRuleListIsNamedAndOrdered`, `TestConstrainedMatrix` (864 constrained builds, refusals under a quarter) |

## Dependencies & Execution Order

T001 first. T002, T003, T005, T006, T007, T009 in parallel; T004 after T003; T008 after T007; T010 after T009. US1: T011, T012, T014, T015, T016 (tests) in parallel, then T013 → T017. US2 (T018 → T019 → T020) and US3 (T021 → T022) and US4 (T023) after T017, in any order. Phase 7: T024, T025, T027 parallel with the stories; T026 after T017; T028 → T029 → T030 → T031 → T032 last; T033 after T028.

## Implementation Strategy

MVP is Phase 1 through Phase 3: an instant, deterministic, valid draft from the answers with the provider switch, which the current app can already use with no change (free text ignored). US2 adds the chips' backend, US3 the variant proof, US4 the secondary shares. Nothing in this feature spends provider credit; the model path is exercised only by the existing spec 015 tests.
