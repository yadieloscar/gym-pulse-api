# Tasks: Import Workout History from Strong or Hevy

**Spec**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md)

- [x] T001 Migration 034 `workout_imports` + `day_logs.import_id` — FR-003, FR-009
- [x] T002 Import model types and file error codes (`internal/model/workout_import.go`) — FR-001, FR-010
- [x] T003 Strict layout detection and row parsing (`internal/service/import_parse.go`) — FR-004, FR-006
- [x] T004 Exercise matcher and saved-exercise link (`internal/service/import_match.go`) — clarifications
- [x] T005 Planning, units, dedupe, replay-first commit (`internal/service/import_svc.go`) — FR-002, FR-005, FR-006
- [x] T006 Transactional DAO with unnest inserts (`internal/dao/workout_import_dao.go`) — FR-002, FR-003, FR-007
- [x] T007 Handlers, routes, rate limit, wiring — FR-001, FR-002, FR-003, FR-010
- [x] T008 Additive `import_id` and set name snapshot; weekly list date fix — FR-008
- [x] T009 Health export refuses imported logs; account deletion — FR-007, FR-009
- [x] T010 Tests: fixtures, service, handler, integration, smoke 13j — all FRs, SC-002, SC-003, SC-004
- [x] T011 `docs/CONTRACTS.md` (1.6.0), `docs/DATABASE_DESIGN.md`, Swagger

## Evidence (2026-11-03)

| Check | Result |
| --- | --- |
| `go test ./...` | passed; `internal/service` 90.9% |
| `golangci-lint run ./...` | 0 issues |
| `go test -race -tags=integration ./internal/dao -count=1` on a disposable migrated PostgreSQL 16 (CI-equivalent compose) | passed, whole suite |
| 25,000 imported sets in one commit | 0.8 s (SC-001) |
| `scripts/smoke.sh` against the CI-equivalent stack, requests as `gym_pulse_api` | 32 passed, 0 failed (found and fixed COPY-under-RLS) |
| `scripts/validate-supabase-security.sh` | passed |
| `scripts/validate-database-design.sh` | passed: 40 tables, 418 columns, 44 indexes |
| `swag init` | additions; `exercise_id` no longer required on set logs |
| `./scripts/smoke-toggle.sh` | not run: it recreates the shared dev API container; the equivalent stack above was used |
| Real Strong/Hevy export from a device | not available; fixtures use the published headers and public sample rows |
| Production deploy | not done (owner) |
