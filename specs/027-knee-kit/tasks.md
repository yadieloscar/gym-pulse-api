# Tasks: Knee Kit (daily knee checklist and pain log)

**Input**: [spec.md](spec.md), [plan.md](plan.md) | **Feature ID**: `gympulse-knee-kit`

Tests are required by the constitution and by SC-001..SC-003.

## Phase 1: Setup

- [X] T001 Confirm the migration number (037; 034 is the latest on `main`, 035 and 036 are claimed by other open branches) and that golang-migrate and the static migration tests tolerate the gap

## Phase 2: Foundational

- [X] T002 Create `migrations/037_create_knee_kit_days.up.sql` and `.down.sql` (table, checks, unique key, RLS, revocation, runtime-role policy)
- [X] T003 [P] Add migration-contract and JSON-shape tests in `internal/model/knee_kit_acceptance_test.go`
- [X] T004 [P] Add `KneeKitDay`, `SaveKneeKitDayRequest`, `KneeKitRange`, and the outcome in `internal/model/knee_kit.go`
- [X] T005 Add `knee_kit_days` to account deletion in `internal/dao/account_dao.go`
- [X] T006 Document the table, index, FK matrix, traceability, counts, lock, and migration 037 in `docs/DATABASE_DESIGN.md`; bump the public-table count in `internal/dao/supabase_security_migration_test.go`

## Phase 3: User Story 1 - Save a day (P1)

**Independent test**: PUT with revision 0 → 201; same key → the same 201; stored revision → 200.

- [X] T007 [P] [US1] Service tests for validation, local-today, sorting, hashing, and outcomes in `internal/service/knee_kit_svc_test.go`
- [X] T008 [P] [US1] Handler tests for 201/200/409/422/404/400 and header mismatch in `internal/handler/knee_kit_test.go`
- [X] T009 [US1] Implement `SaveDay` in `internal/dao/knee_kit_dao.go` (lock, ledger, row lock, revision, insert or replace, ledger write)
- [X] T010 [US1] Implement validation, sorting, and hashing in `internal/service/knee_kit_svc.go`
- [X] T011 [US1] Implement `SaveDay` in `internal/handler/knee_kit.go`; wire routes in `internal/router/router.go` and `cmd/server/main.go`

## Phase 4: User Story 2 - Read a range (P1)

**Independent test**: save two dates, read a range covering both, oldest first.

- [X] T012 [US2] Implement `ListDays` in the DAO, service, and handler (range ≤ 366 days, `{"days":[]}`)
- [X] T013 [US2] Route registration in `internal/router/training_routes_test.go` and the `router.New` call sites
- [X] T014 [US2] PostgreSQL integration test for create, replay, both conflicts, replacement, concurrent first saves, range, ownership, constraints, and the runtime role in `internal/dao/knee_kit_integration_test.go`

## Phase 5: User Story 3 - Knee Kit never moves consistency (P1)

**Independent test**: stats and participation identical with and without Knee Kit days.

- [X] T015 [US3] Static isolation tests (no other DAO source names `knee_kit_days` except account deletion and the export; the Knee Kit DAO names no other table) in `internal/dao/knee_kit_dao_test.go`
- [X] T016 [US3] Integration regression comparing stats and participation with and without days, plus account-deletion and auth-cascade removal, in `internal/dao/knee_kit_integration_test.go`
- [X] T017 [US3] Additive `knee_kit_days` export section in `internal/model/export.go` and `internal/dao/export_dao.go`, with handler, static, and integration test updates

## Phase 6: Polish

- [X] T018 Document the routes and the export section in `docs/CONTRACTS.md` (contract 1.7.0); regenerate Swagger
- [X] T019 Add Knee Kit step 13j to `scripts/smoke.sh`
- [X] T020 Run the PostgreSQL-backed gates (integration suite, smoke, RLS validator, migration down/up) against a disposable stack and record the results below
- [X] T021 Run the static gates and record the results below

## Dependencies

T001 → T002 → T003..T006 → US1 → US2 → US3 → Polish. US2 and US3 reuse US1's DAO and wiring.

## Evidence

| Requirement | Tasks | Verification |
|---|---|---|
| FR-001 | T009–T011 | service + handler tests, integration, smoke |
| FR-002 | T009–T011, T014 | service + handler + static DAO tests, integration, smoke |
| FR-003 | T012–T014 | service + handler + router tests, integration, smoke |
| FR-004 | T002, T014 | migration-contract test, database-design validator, constraint and runtime-role integration tests, RLS validator |
| FR-005 | T015, T016 | static isolation tests, stats/participation regression |
| FR-006 | T005, T016, T017 | static account and export tests, cascade and export integration tests, smoke |
| FR-007 | T018 | CONTRACTS.md review; Swagger regenerated with no further drift |

## Verification (2026-11-14)

| Check | Result |
|---|---|
| `gofmt -l cmd internal` | Passed (no files) |
| `go build ./...`, `go vet ./...`, `go vet -tags=integration ./internal/dao` | Passed |
| `golangci-lint run` | Passed, 0 issues (with `--build-tags=integration`: 1 pre-existing `predeclared` finding in `planned_sport_sessions_integration_test.go`, untouched) |
| `go test ./...` | Passed |
| Coverage gate (`go test ./internal/<pkg> -coverprofile`, ≥ 90%) | Passed: service 90.6%, middleware 91.4%, handler 90.8%, router 100.0%, config 98.1% |
| `swag init -g cmd/server/main.go --output docs` (v1.16.4) then `git diff --exit-code docs/` | Passed after committing the regenerated files |
| `./scripts/validate-database-design.sh` | Passed: 40 tables, 414 columns, 42 named indexes |
| `govulncheck ./...` | Passed (exit 0): no reachable vulnerabilities |
| Disposable CI-equivalent stack (compose project `kneekit-api`: postgres 16 + API image from this branch, owner migrates, API runs as `gym_pulse_api`) | Migrations 001–037 applied on boot (version 37, not dirty) |
| Migration 037 down then up (files piped to `psql` as the owner) | Passed: table dropped, recreated with RLS, policy, and runtime-role grant |
| `scripts/validate-supabase-security.sh` (run inside the db container) | Passed |
| `API=http://localhost:18137 ./scripts/smoke.sh` | 32 passed, 0 failed, including step 13j (Knee Kit) |
| `go test -race -tags=integration ./internal/dao -count=1` | Passed, including the 6 Knee Kit integration tests (runtime-role test not skipped) and the export tests |
