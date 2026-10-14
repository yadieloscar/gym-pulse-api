---
description: "Implementation tasks for remembering a workout's health export"
---

# Tasks: Remember a workout's health export

**Input**: Design documents from `/specs/010-health-workout-writeback/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/health-exports.openapi.yaml, quickstart.md

**Tests**: Required by the API constitution; identified before each implementation task.

## Format: `[ID] [P?] [Story] Description`

## Requirement and Verification Evidence

| Requirement / acceptance | Tasks | Verification | Result / limitation |
| --- | --- | --- | --- |
| FR-001, FR-005, US2 | T004, T010, T012 | handler list test; `[]` when empty; range rule | Passed 2026-10-14 (`TestHealthExportHandlerList`; service range test) |
| FR-002, US1/AC1–AC3, SC-001 | T006, T008, T011 | DAO existing/conflict tests; handler 201 → 200 → 409 | Passed in unit tests; smoke 13c proved 201, 200, 409, and the list on a live database |
| FR-003, US1/AC4–AC5, SC-003 | T006, T008 | DAO not-finished, missing, foreign → `NotFoundError` | Passed (source predicates asserted; smoke 13c missing day_log → 404) |
| FR-004 | T005, T009 | service validation table | Passed (`TestHealthExportServiceValidation`) |
| FR-006 | T006 | DAO source test: only `SELECT` on workout tables | Passed (`TestHealthExportDAOOnlyReadsWorkoutTables`) |
| FR-007 | T013 | account cleanup test | Passed (`TestAccountCleanupIncludesHealthExports`; smoke step 14) |
| Contract | T002, T014, T015 | `docs/CONTRACTS.md`; smoke step | Passed: section added; smoke 23/23 on the isolated stack |
| Schema document | T003, T014 | `./scripts/validate-database-design.sh` | Passed: 31 tables, 292 columns, 27 indexes |
| Supabase security | T016 | security SQL on a migrated database | Passed on the isolated stack at migration 23 |

## Phase 1: Setup

- [x] T001 Record `git status --short`; confirm `023` is the next migration number
- [x] T002 [P] Draft the "Health exports" section in `docs/CONTRACTS.md` from the OpenAPI contract
- [x] T003 [P] Add the `health_exports` table, checks, and both indexes to `docs/DATABASE_DESIGN.md` (catalog, index inventory, counts)

## Phase 2: Foundational

- [x] T004 [P] Add `internal/model/health_export.go` (`HealthExport`, `RecordHealthExportRequest`, kinds, source, `HealthExportOutcome`) and `health_export_acceptance_test.go` for migration invariants and JSON shape
- [x] T005 Add `migrations/023_create_health_exports.{up,down}.sql` per data-model.md; run `./scripts/validate-database-design.sh`

## Phase 3: User Story 1 - Remember the export once (P1) 🎯 MVP

- [x] T006 [P] [US1] Add `internal/dao/health_export_dao_test.go`: transactional order markers (replay → resolve → existing → insert → ledger → commit); finished predicates for both kinds; existing/conflict branches; only `SELECT` on workout tables
- [x] T007 [P] [US1] Add `internal/service/health_export_svc_test.go`: kind/source enums, identity trim and bounds, `exported_at` default and clamp, outcome passthrough, range validation
- [x] T008 [US1] Implement `internal/dao/health_export_dao.go` (`Record`, `List`) with the transaction from data-model.md
- [x] T009 [US1] Implement `internal/service/health_export_svc.go` (`Record`, `List`)
- [x] T010 [US1] Implement `internal/handler/health_export.go` (`Record` → 201/200, `List`); map `ConflictError` messages containing "health export" to `HEALTH_EXPORT_CONFLICT` in `internal/handler/response.go`
- [x] T011 [P] [US1] Add `internal/handler/health_export_test.go`: statuses per outcome, conflict code and `resource`, validation field surfacing, 404 passthrough
- [x] T012 [US1] Wire DAO/service/handler in `cmd/server/main.go` and routes in `internal/router/router.go`; extend `internal/router/training_routes_test.go`

## Phase 4: User Story 2 - Read which workouts are already in Health (P2)

- [x] T013 [US2] Add `health_exports` to `internal/dao/account_dao.go` deletion and its test; confirm cascade in the migration

## Phase 5: Polish and Verification

- [x] T014 Run `gofmt -w`, `./scripts/validate-database-design.sh`, `go test ./...`, `golangci-lint run`
- [x] T015 Add smoke step 15: record → replay → conflict → list; run `./scripts/smoke-toggle.sh` (or the isolated acceptance stack)
- [x] T016 Run the Supabase security SQL against the migrated database; record results in the table above

## Dependencies & Execution Order

T001–T003 parallel. T004 → T005. US1 tests T006/T007/T011 parallel before T008 → T009 → T010 → T012. T013 after T008. Phase 5 last. Deploy before the app.
