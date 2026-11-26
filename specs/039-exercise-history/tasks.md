# Tasks: Exercise history

**Feature ID**: `gympulse-exercise-history` | **Spec**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md)

- [X] T001 Model: `ExerciseSession`, `ExerciseSessionSet`, `HeaviestSet`, `ExerciseSessionsPage`, `ExerciseSessionsFilter`.
- [X] T002 `logDAO.ExerciseSessions`: both set sources, three identities, date paging, all-time heaviest, one batch (FR-003 to FR-007).
- [X] T003 `logService.ExerciseSessions`: identity, name, `before`, and `limit` validation (FR-002, FR-005).
- [X] T004 Handler with godoc, route `GET /exercises/sessions`.
- [X] T005 Unit tests: service and handler; mocks.
- [X] T006 Integration test `TestExerciseSessions`.
- [X] T007 `docs/CONTRACTS.md` (contract 1.18.0), regenerated swagger, smoke step 8b2.
- [ ] T008 `./scripts/smoke-toggle.sh` against a running stack (CI `smoke` job runs it).

## Evidence

2026-11-25, branch `feat/039-exercise-history`:

| Check | Result |
| --- | --- |
| `go test ./...` | Passed |
| `golangci-lint run` | Passed, 0 issues |
| `go test -p 1 -race -tags=integration ./internal/dao ./internal/dbmigrate` on a scratch PostgreSQL 16 migrated to 051 | Passed |
| `swag init` | Regenerated, committed |
| `./scripts/smoke-toggle.sh` | Not run locally (no dev stack running); step 8b2 is syntax-checked only and runs in CI |

Not verified: the query plan on a production-sized history, and the route on the deployed API.

The requirement-to-test table is in [spec.md](spec.md), Acceptance.
