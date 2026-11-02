---
description: "Implementation tasks for daily nutrition totals and a chosen target"
---

# Tasks: Daily nutrition totals and a chosen target

**Input**: Design documents from `/specs/012-nutrition-targets/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/nutrition.openapi.yaml, quickstart.md

**Tests**: Required by the API constitution; identified before each implementation task.

## Format: `[ID] [P?] [Story] Description`

## Requirement and Verification Evidence

| Requirement / acceptance | Tasks | Verification | Result / limitation |
| --- | --- | --- | --- |
| FR-001, FR-006, US1/AC6 | T006, T010, T013 | handler list test: `{days:[],targets:[]}` when empty; oldest first; targets include the one in effect on `from`; 366-day and `to<from` rules | Passed 2026-10-25 (`TestNutritionHandlerListDays`, `TestNutritionServiceListDaysValidatesRange`) |
| FR-002, US1/AC5 | T007, T011 | service test: future local date → 422 `date`; local today from profile timezone across UTC midnight | Passed (`TestNutritionServiceSaveDayLocalTodayFollowsProfileTimezone`: New York vs Auckland at 23:30 UTC) |
| FR-003, US1/AC2–AC3, SC-003 | T006, T010 | DAO test: `expected_revision` 0 with existing row → 409 `actual_revision`; stale replace → 409 with stored day as `resource`; correct → revision+1 | Passed in unit tests (`TestCheckNutritionRevision`, handler stale/missing-row 409); smoke 13d (now 13f after the rebase) passed 2026-10-25 (24/24 on the worktree image) |
| FR-004, US1/AC4, SC-002 | T006, T010 | DAO test: verbatim retry replays 201/200/204; same key different payload → `IDEMPOTENCY_CONFLICT` | Passed (`TestNutritionServiceSaveDayHashCoversDateAndBody`; DAO order test); smoke 13d (now 13f after the rebase) passed 2026-10-25 (24/24 on the worktree image) |
| FR-005 | T006, T010, T012 | DAO test: all-absent save deletes → `removed`; with no row → `removed`, nothing written; handler 204 | Passed (`TestNutritionServiceSaveDayRemoveAndHealthRules`, handler 204 without body, `nutritionDayStatus`) |
| FR-007, US3/AC1–AC3, SC-004 | T014, T015 | DAO test: Health save over manual row → `ignored`, row unchanged, ledger written; over Health row → replaced, source unchanged | Passed by source assertion (`TestNutritionDAOOwnershipAndRevisionRules`: Health branch has no revision check and the manual arm writes nothing); smoke 13d (now 13f after the rebase) passed 2026-10-25 (24/24 on the worktree image) |
| FR-008, US3/AC4 | T014, T015 | DAO test: manual save over Health row with current revision → source `manual`, revision+1; later Health save → `ignored` | Passed by source assertion (manual arm sets `source = manual`, `revision+1`); smoke 13d (now 13f after the rebase) passed 2026-10-25 (24/24 on the worktree image) |
| FR-009, US2/AC2–AC5 | T017, T018 | DAO test: same `effective_from` replaces; new date inserts and old row remains; identical values → `unchanged` | Passed by source order (`TestNutritionDAOSaveTargetKeepsAtomicOrder`, `TestSameNutritionTargetValues`); smoke 13d (now 13f after the rebase) passed 2026-10-25 (24/24 on the worktree image) |
| FR-010, US2/AC1 | T017, T019 | handler test: `GET /nutrition/target` 404 when none; current is latest `effective_from <= today`, never a future-dated row | Passed (`TestNutritionHandlerTarget`, `TestNutritionServiceGetTargetUsesLocalToday`; `effective_from <= today` query) |
| FR-011 | T006, T017 | DAO source tests: no UPDATE/DELETE outside the two save transactions | Passed (`TestNutritionDAOOnlyTouchesNutritionTables`: no target delete, no foreign-table mutation) |
| FR-012, US1/AC7, SC-005 | T013, T021 | handler foreign-user test; DAO queries carry `user_id`; account cleanup test includes both tables | Passed (`user_id` in every statement asserted; `TestAccountCleanupIncludesNutrition`; handler 404 passthrough) |
| FR-013 | T007, T018 | service tables: negative, over-bound, two decimals → 422 naming the field; target `protein_g` 0 → 422 | Passed (`TestNutritionServiceSaveDayValidation` 12 cases, `TestNutritionServiceSaveTargetValidation` 9 cases) |
| FR-014, SC-006 | T002, T023, T024 | `docs/CONTRACTS.md` "Nutrition"; smoke step; existing smoke steps unchanged | Contract section written; `go test ./...` green across 7 packages; smoke 24/24 with 13d (now 13f) added and every earlier step unchanged 2026-10-25 (24/24 on the worktree image) |
| Schema document | T003, T005, T023 | `./scripts/validate-database-design.sh` | Passed: `./scripts/validate-database-design.sh` → 33 tables, 314 columns, 29 named indexes |
| Supabase security and runtime role | T005, T025 | `supabase_security_migration_test.go`; security SQL on a migrated database | `TestSupabaseSecurityMigrationProtectsEveryPublicApplicationTable` passes at 32 tables; `validate-supabase-security.sh` passed inside the db container at migration 027 (renumbered 029 on 2026-10-27) |
| SC-001 | T024 | smoke timing: save and range read each one round trip | Smoke 13d: each save and the range read completed in one round trip on the local stack |

## Phase 1: Setup

- [x] T001 Record `git status --short` in the worktree; confirm `027` is the next migration number against `origin/main` — renumbered to `029` on 2026-10-27 after gym-pulse-api#36 merged 027 and 028
- [x] T002 [P] Draft the "Nutrition" section in `docs/CONTRACTS.md` after "Body weight" from `contracts/nutrition.openapi.yaml` (four routes, bodies, statuses, revision rules by source, bounds, error codes)
- [x] T003 [P] Add `nutrition_days` and `nutrition_targets` sections, checks, and index rows to `docs/DATABASE_DESIGN.md` (catalog, index inventory, counts)

## Phase 2: Foundational

- [x] T004 [P] Add `internal/model/nutrition.go` (`NutritionDay`, `NutritionTarget`, `NutritionSource`, `SaveNutritionDayRequest`, `SaveNutritionTargetRequest`, `NutritionRange`, `NutritionDayOutcome` created/replaced/ignored/removed/replayed, `NutritionTargetOutcome` created/replaced/unchanged/replayed) and `nutrition_acceptance_test.go` for migration invariants and JSON shape (pointer `omitempty` on day values; target optional values always present)
- [x] T005 Add `migrations/029_create_nutrition.{up,down}.sql` (was 027) per data-model.md: both tables, checks, unique indexes, RLS, Data API revoke block from 025, and the `gym_pulse_api_access` policy plus DML grant guarded by `IF EXISTS gym_pulse_api`; extend `internal/dao/supabase_security_migration_test.go` expectations; run `./scripts/validate-database-design.sh`

## Phase 3: User Story 1 - Record and read a day's totals (P1) 🎯 MVP

- [x] T006 [P] [US1] Add `internal/dao/nutrition_dao_test.go`: save-day transaction order markers (lock → replay → select for update → branch → ledger → commit); create with revision 0; replace with current revision; stale → `ConflictError{Expected, Actual, Authoritative}`; remove branch; list ordering and `[]`; `user_id` in every statement
- [x] T007 [P] [US1] Add `internal/service/nutrition_svc_test.go`: bounds and one-decimal rounding per field; at-least-one-value; `source` default manual; `expected_revision` required for manual and ignored for `apple_health`; future date vs profile timezone (fixed clock across UTC midnight); range validation; payload hash covers path date plus client body only
- [x] T008 [US1] Implement `internal/dao/nutrition_dao.go` `SaveDay` (manual and remove branches) and `ListDays` (days plus targets in effect) with the transaction from data-model.md, scope `nutrition-days/put`, namespace `nutrition:`
- [x] T009 [US1] Implement `internal/service/nutrition_svc.go` `SaveDay` and `ListDays` (profile timezone like `sport_activity_svc.go`, normalization, validation, hashing)
- [x] T010 [US1] Implement `internal/handler/nutrition.go` `SaveDay` (201 created, 200 replaced/ignored, 204 removed, replay status from ledger) and `ListDays`; swag godoc for both
- [x] T011 [P] [US1] Add `internal/handler/nutrition_test.go`: statuses per outcome; `REVISION_CONFLICT` details and `resource`; `IDEMPOTENCY_CONFLICT`; validation field surfacing; 404 passthrough when no profile; header/body key mismatch
- [x] T012 [US1] Map a revision conflict against a missing row (`Actual` 0, message "revision conflict") to `REVISION_CONFLICT` in `internal/handler/response.go`; handler test covers it and the 204 path
- [x] T013 [US1] Wire DAO/service/handler in `cmd/server/main.go` and routes under `/api/v1/nutrition` in `internal/router/router.go` (inside the `RequireActiveUser` group); extend `internal/router/training_routes_test.go`

**Checkpoint**: an athlete can save, correct, remove, and read days; conflicts and retries behave.

## Phase 4: User Story 3 - Bring in totals the phone read from Health (P2)

- [x] T014 [P] [US3] Extend `internal/dao/nutrition_dao_test.go`: Health save creates `apple_health`; Health over Health replaces and keeps source; Health over manual → `ignored` with stored row and ledger write only; manual over Health with current revision → `manual`, revision+1; subsequent Health → `ignored`
- [x] T015 [US3] Implement the `apple_health` branch in `SaveDay` in `internal/dao/nutrition_dao.go`; handler maps `ignored` to 200
- [x] T016 [US3] Add service tests for `source=apple_health` without `expected_revision` and with a stray `expected_revision` (ignored) in `internal/service/nutrition_svc_test.go`

## Phase 5: User Story 2 - Keep the athlete's chosen target (P2)

- [x] T017 [P] [US2] Extend `internal/dao/nutrition_dao_test.go`: save-target order markers; create with revision 0; stale → conflict with current target as `Authoritative`; same `effective_from` replaces; new date inserts and old remains; identical → `unchanged`; `GetCurrentTarget` picks latest `effective_from <= today` and ignores future rows; `ListDays` returns the target in effect on `from` plus in-range targets oldest first
- [x] T018 [P] [US2] Extend `internal/service/nutrition_svc_test.go`: `protein_g` required and > 0; optional bounds; `effective_from` default local today and future → 422; hash excludes defaulted `effective_from`
- [x] T019 [US2] Implement `SaveTarget` and `GetCurrentTarget` in `internal/dao/nutrition_dao.go` (scope `nutrition-target/put`) and in `internal/service/nutrition_svc.go`
- [x] T020 [US2] Implement handler `GetTarget` (200 or shared 404) and `SaveTarget` (201/200) in `internal/handler/nutrition.go`; routes in `internal/router/router.go`; handler and router tests
- [x] T021 [US2] Add `DELETE FROM nutrition_days` and `DELETE FROM nutrition_targets` to `internal/dao/account_dao.go` and its enumeration test; confirm cascade in the migration

**Checkpoint**: all three stories independently testable; contract complete.

## Phase 6: Polish and Verification

- [x] T022 [P] Regenerate `docs/swagger.json` and `docs/swagger.yaml`; confirm only the nutrition routes changed
- [x] T023 Run `gofmt -w`, `./scripts/validate-database-design.sh`, `go test ./...` (service and middleware coverage ≥ 90%), `golangci-lint run`
- [x] T024 Add smoke step "Nutrition": target 201 → unchanged 200; day 201 → same-key replay 201 → Health over manual 200 unchanged → Health day 201 → manual over Health 200 source manual → Health again 200 unchanged → clear 204 → range read → tomorrow 422 → second account sees `[]`; run `./scripts/smoke-toggle.sh` or the isolated acceptance stack
- [x] T025 Run `./scripts/validate-supabase-security.sh` against the migrated database (both tables RLS on, Data API privileges revoked, runtime role policy present); record results in the table above
- [x] T026 Update `README.md`/`CLAUDE.md` route inventory if they list domains; `git diff --check`

## Corrections

Independent read-only review of the uncommitted diff on 2026-10-25 returned two medium and five low
findings; all are fixed and re-verified (`go test ./...`, `golangci-lint` 0 issues, design validator,
Swagger regenerated, smoke 24/24 on the rebuilt image, security SQL).

- [x] C001 `response.go` mapped any "revision conflict" message to `REVISION_CONFLICT`, which would have changed the plan-transition profile race from `CONFLICT`. Scoped to the exact nutrition message; `TestNutritionHandlerSaveDayErrors` pins the other domain's code.
- [x] C002 A missing-row revision conflict carried `"resource": null` (typed-nil pointer in an interface). `authoritativeDay` returns a true nil; handler and smoke assert `resource` is absent.
- [x] C003 A traceability row had landed in the schema-count table of `docs/DATABASE_DESIGN.md`; moved to the API-to-persistence table.
- [x] C004 Smoke 13d now also covers Health-over-Health replace, remove replay and remove with no row, the gone-row conflict, inserting an older target under the current revision, the target in effect on `from`, and a same-day range returning one target.
- [x] C005 `docs/CONTRACTS.md` scoped the training-profile 404 to the three profile-dependent routes; the range read needs none.
- [x] C006 OpenAPI `Error.required` no longer lists `details`, which `NOT_FOUND` and `IDEMPOTENCY_CONFLICT` omit.
- [x] C007 Added swag godoc to the four handlers and regenerated `docs/swagger.*` (nutrition-only diff) so CI's drift check passes.

Noted, not changed: a target saved as "today" in a far-east timezone becomes future-dated if the athlete later moves their profile timezone west; it stays in range reads but drops out of the current-target lookup until that date arrives. Unspecified edge; candidate for a spec edge case if it matters.

## Dependencies & Execution Order

T001–T003 parallel. T004 → T005. US1 tests T006/T007/T011 before T008 → T009 → T010 → T012 → T013. US3 (T014–T016) after T008. US2 (T017–T021) after T013; T017/T018 parallel. Phase 6 last. Deploy before the app branch `spec/015-nutrition-targets`.

## Implementation Strategy

MVP is Phase 1–3: manual days only, enough for the app's log sheet and Progress week. Phase 4 unlocks the Health switch; Phase 5 unlocks the target card and the on-target count. Each phase ends with `go test ./...` green and the evidence table updated.
