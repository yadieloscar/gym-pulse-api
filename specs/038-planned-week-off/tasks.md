# Tasks: A planned week off

**Feature ID**: `gympulse-planned-week-off` | **Spec**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md)

- [X] T001 Migration 051 (up and down): `weeks_off` with the Monday check and `secure_table`.
- [X] T002 Model `WeekOff`, `WeeksOffResponse`, `MaxWeekOffAheadWeeks`.
- [X] T003 `WeekOffDAO`: `List`, `Timezone`, `Put` (lock, upsert, skip the week's misses), `Delete`.
- [X] T004 `FinalizeScheduledWorkout`: skipped and no participation row for an untouched workout in a week off (FR-006).
- [X] T005 `WeekOffService`: Monday, current-week, and 8-week validation in the athlete's timezone.
- [X] T006 `WeekOffHandler` with godoc, routes, wiring in `cmd/server/main.go`.
- [X] T007 Account deletion deletes `weeks_off`; public table count is 45.
- [X] T008 Unit tests: service and handler; route registration.
- [X] T009 Integration tests: DAO round trip, finalize in and out of a week off, late take.
- [X] T010 `docs/CONTRACTS.md` (contract 1.17.0), `docs/DATABASE_DESIGN.md`, regenerated swagger, smoke step 8d.
- [ ] T011 `./scripts/smoke-toggle.sh` against a running stack (CI `smoke` job runs it).

## Evidence

2026-11-20, branch `feat/038-planned-week-off`:

| Check | Result |
| --- | --- |
| `go test ./...` | Passed |
| `golangci-lint run` | Passed, 0 issues |
| Service and middleware coverage | 91.4% and 93.2%; `week_off_svc.go` 100% |
| `./scripts/validate-database-design.sh` | Passed: 46 tables, 489 columns |
| `go test -p 1 -race -tags=integration ./internal/dao ./internal/dbmigrate` on a scratch PostgreSQL 16 migrated to 051 | Passed |
| `scripts/validate-supabase-security.sh` on the same database | Passed |
| `swag init` | Regenerated, committed |
| `./scripts/smoke-toggle.sh` | Not run locally (no dev stack running); step 8d is syntax-checked only and runs in CI |

The requirement-to-test table is in [spec.md](spec.md), Acceptance.
