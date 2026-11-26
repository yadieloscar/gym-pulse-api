# Tasks: Program recap

**Feature ID**: `gympulse-program-recap` | **Spec**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md)

- [X] T001 Model `ProgramRecap`, `ProgramRecapWeeks`, `ProgramRecapLift`, `ProgramRecapSet`.
- [X] T002 `ProgramRecapDAO.Read`: one batch of five owned queries (FR-007, FR-009).
- [X] T003 `buildProgramRecap`: dates, weeks, workouts done, main lifts (FR-002 to FR-006).
- [X] T004 `ProgramRecapService.Get`: today in the athlete's timezone, 404 for a missing program.
- [X] T005 `ProgramRecapHandler` with godoc, route, wiring in `cmd/server/main.go`.
- [X] T006 Unit tests: full program, partly done, week off, no logged lifts, unfinished week, edge weeks, lift ranking; handler and route.
- [X] T007 Integration test: the batch against PostgreSQL, ownership, discarded and incomplete sets.
- [X] T008 `docs/CONTRACTS.md` (contract 1.20.0), regenerated swagger, smoke step 8e, Bruno request.
- [ ] T009 `./scripts/smoke-toggle.sh` against a running stack (CI `smoke` job runs it).

## Evidence

2026-11-25, branch `feat/039-program-recap`:

| Check | Result |
| --- | --- |
| `go test ./...` | Passed |
| `golangci-lint run` | Passed, 0 issues |
| Service and middleware coverage | 91.6% and 93.2%; `program_recap_svc.go` `Get` and `recapLifts` 100%, `buildProgramRecap` 97.7% |
| `go test -p 1 -race -tags=integration ./internal/dao ./internal/dbmigrate` on a scratch PostgreSQL 16 migrated to 051 | Passed, including the three `TestProgramRecapRead*` tests |
| `swag init` (v1.16.4) | Regenerated, committed |
| `bash -n scripts/smoke.sh` | Passed |
| `./scripts/smoke-toggle.sh` | Not run locally (no dev stack running); step 8e runs in the CI `smoke` job |

The branch also carries commit `fix: bump OpenTelemetry modules to 1.47.0 for GO-2026-6974`, picked
unchanged from PR #87, because `govulncheck` in the `ci` job fails on every open pull request without
it and the `smoke` job is skipped when `ci` fails.

Not verified: the route against production data, and its timing across regions (one round trip by
design; not measured). The Postman collection was not updated; it has not been maintained since the
first commit.

The requirement-to-test table is in [spec.md](spec.md), Acceptance.
