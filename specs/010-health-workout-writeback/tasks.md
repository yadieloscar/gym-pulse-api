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
| FR-001, FR-005, US2 | T004, T010, T012 | handler list test; `[]` when empty; range rule | Planned |
| FR-002, US1/AC1–AC3, SC-001 | T006, T008, T011 | DAO existing/conflict tests; handler 201 → 200 → 409 | Planned |
| FR-003, US1/AC4–AC5, SC-003 | T006, T008 | DAO not-finished, missing, foreign → `NotFoundError` | Planned |
| FR-004 | T005, T009 | service validation table | Planned |
| FR-006 | T006 | DAO source test: only `SELECT` on workout tables | Planned |
| FR-007 | T013 | account cleanup test | Planned |
| Contract | T002, T014, T015 | `docs/CONTRACTS.md`; smoke step | Planned |
| Schema document | T003, T014 | `./scripts/validate-database-design.sh` | Planned |
| Supabase security | T016 | security SQL on a migrated database | Planned |

## Phase 1: Setup

- [ ] T001 Record `git status --short`; confirm `023` is the next migration number
- [ ] T002 [P] Draft the "Health exports" section in `docs/CONTRACTS.md` from the OpenAPI contract
- [ ] T003 [P] Add the `health_exports` table, checks, and both indexes to `docs/DATABASE_DESIGN.md` (catalog, index inventory, counts)

## Phase 2: Foundational

- [ ] T004 [P] Add `internal/model/health_export.go` (`HealthExport`, `RecordHealthExportRequest`, kinds, source, `HealthExportOutcome`) and `health_export_acceptance_test.go` for migration invariants and JSON shape
- [ ] T005 Add `migrations/023_create_health_exports.{up,down}.sql` per data-model.md; run `./scripts/validate-database-design.sh`

## Phase 3: User Story 1 - Remember the export once (P1) 🎯 MVP

- [ ] T006 [P] [US1] Add `internal/dao/health_export_dao_test.go`: transactional order markers (replay → resolve → existing → insert → ledger → commit); finished predicates for both kinds; existing/conflict branches; only `SELECT` on workout tables
- [ ] T007 [P] [US1] Add `internal/service/health_export_svc_test.go`: kind/source enums, identity trim and bounds, `exported_at` default and clamp, outcome passthrough, range validation
- [ ] T008 [US1] Implement `internal/dao/health_export_dao.go` (`Record`, `List`) with the transaction from data-model.md
- [ ] T009 [US1] Implement `internal/service/health_export_svc.go` (`Record`, `List`)
- [ ] T010 [US1] Implement `internal/handler/health_export.go` (`Record` → 201/200, `List`); map `ConflictError` messages containing "health export" to `HEALTH_EXPORT_CONFLICT` in `internal/handler/response.go`
- [ ] T011 [P] [US1] Add `internal/handler/health_export_test.go`: statuses per outcome, conflict code and `resource`, validation field surfacing, 404 passthrough
- [ ] T012 [US1] Wire DAO/service/handler in `cmd/server/main.go` and routes in `internal/router/router.go`; extend `internal/router/training_routes_test.go`

## Phase 4: User Story 2 - Read which workouts are already in Health (P2)

- [ ] T013 [US2] Add `health_exports` to `internal/dao/account_dao.go` deletion and its test; confirm cascade in the migration

## Phase 5: Polish and Verification

- [ ] T014 Run `gofmt -w`, `./scripts/validate-database-design.sh`, `go test ./...`, `golangci-lint run`
- [ ] T015 Add smoke step 15: record → replay → conflict → list; run `./scripts/smoke-toggle.sh` (or the isolated acceptance stack)
- [ ] T016 Run the Supabase security SQL against the migrated database; record results in the table above

## Dependencies & Execution Order

T001–T003 parallel. T004 → T005. US1 tests T006/T007/T011 parallel before T008 → T009 → T010 → T012. T013 after T008. Phase 5 last. Deploy before the app.
