# Tasks: Mobility Sessions (recorded stretching)

**Input**: [spec.md](spec.md), [plan.md](plan.md) | **Feature ID**: `gympulse-stretching-routines`

Tests are required by the constitution and by SC-001..SC-003.

## Phase 1: Setup

- [X] T001 Fetch `origin/main` and confirm the next free migration number (033; 032 is held by open PR #40)

## Phase 2: Foundational

- [X] T002 Create `migrations/033_create_mobility_sessions.up.sql` and `.down.sql` (table, checks, unique key, index, RLS, revocation, runtime-role policy)
- [X] T003 [P] Add migration-contract and JSON-shape tests in `internal/model/mobility_session_acceptance_test.go`
- [X] T004 [P] Add `MobilitySession`, `CreateMobilitySessionRequest`, and context values in `internal/model/mobility_session.go`
- [X] T005 Add `mobility_sessions` to account deletion in `internal/dao/account_dao.go`
- [X] T006 Document the table, indexes, FK matrix, traceability, counts, and migration 033 in `docs/DATABASE_DESIGN.md`

## Phase 3: User Story 1 - Record a finished routine (P1)

**Independent test**: POST twice with one key → 201 then 200 with the same id; changed body → 409.

- [X] T007 [P] [US1] Service tests for validation, date default, hashing, and outcomes in `internal/service/mobility_session_svc_test.go`
- [X] T008 [P] [US1] Handler tests for 201/200/409/422 and header mismatch in `internal/handler/mobility_session_test.go`
- [X] T009 [US1] Implement create in `internal/dao/mobility_session_dao.go` (insert on conflict do nothing, replay by hash)
- [X] T010 [US1] Implement create validation and hashing in `internal/service/mobility_session_svc.go`
- [X] T011 [US1] Implement `Create` in `internal/handler/mobility_session.go`; wire routes in `internal/router/router.go` and `cmd/server/main.go`

## Phase 4: User Story 2 - Read and delete entries (P1)

**Independent test**: create on two dates, list one date, delete it, list again; foreign id → 404.

- [X] T012 [US2] Implement list/get/delete in the DAO, service, and handler (range ≤366 days, shared 404, 204)
- [X] T013 [US2] Route registration in `internal/router/training_routes_test.go`
- [X] T014 [US2] PostgreSQL integration test for create, replay, conflict, concurrent duplicates, range, ownership, delete, and cascade in `internal/dao/mobility_session_integration_test.go`

## Phase 5: User Story 3 - Stretching never moves consistency (P1)

**Independent test**: stats and participation identical with and without mobility entries.

- [X] T015 [US3] Static isolation test (no other DAO source names `mobility_sessions`; the mobility DAO writes no other table) in `internal/dao/mobility_session_dao_test.go`
- [X] T016 [US3] Integration regression comparing stats and participation with and without entries on scheduled and rest dates in `internal/dao/mobility_session_integration_test.go`

## Phase 6: Polish

- [X] T017 Document the routes in `docs/CONTRACTS.md` (contract 1.5.0)
- [X] T018 Add a mobility step to `scripts/smoke.sh`
- [X] T019 Run the gates and record results below

## Dependencies

T001 → T002 → T003..T006 → US1 → US2 → US3 → Polish. US2 and US3 reuse US1's DAO and wiring.

## Evidence

| Requirement | Tasks | Verification |
|---|---|---|
| FR-001 | T009–T011 | service + handler tests, integration, smoke |
| FR-002 | T012–T014 | integration, smoke |
| FR-003 | T002, T012, T014 | integration ownership 404, RLS validator |
| FR-004 | T002, T005, T014 | migration-contract test, cascade integration test |
| FR-005 | T015, T016 | static isolation test, stats/participation regression |
| FR-006 | T017 | CONTRACTS.md review |

## Verification (2026-11-03)

| Check | Result |
|---|---|
| `gofmt -l`, `go build ./...`, `go vet ./...` (and `-tags=integration ./internal/dao`) | Passed |
| `golangci-lint run` | Passed, 0 issues (with `--build-tags=integration`: 1 pre-existing `predeclared` finding in `planned_sport_sessions_integration_test.go`, untouched) |
| `go test ./...` | Passed |
| `swag init` drift | None |
| `./scripts/validate-database-design.sh` | Passed: 39 tables, 398 columns, 40 named indexes |
| Disposable CI-equivalent stack (postgres 16 + API image from this branch, least-privilege runtime role) | Migrations 001–033 applied |
| `scripts/validate-supabase-security.sh` (run inside the db container) | Passed |
| `scripts/smoke.sh` | 29 passed, 0 failed, including step 13i (mobility) |
| `go test -race -tags=integration ./internal/dao -count=1` | Passed, including the 5 mobility integration tests and the stats/participation regression |
| Migration 033 down then up in a rolled-back transaction | Passed |
