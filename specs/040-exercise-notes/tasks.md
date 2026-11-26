# Tasks: Exercise notes

**Feature ID**: `gympulse-exercise-notes` | **Spec**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md)

- [X] T001 Migration 052 (up and down): `exercise_notes` with the length checks and `secure_table`.
- [X] T002 Model `ExerciseNote`, `ExerciseNotesResponse`, `SaveExerciseNoteRequest`, `ExerciseNoteKey`, limits.
- [X] T003 `ExerciseNoteDAO`: `List`, `Save` (upsert under the 500 limit), `Delete`.
- [X] T004 `ExerciseNoteService`: name and note validation, key derivation, empty note clears.
- [X] T005 `ExerciseNoteHandler` with godoc, routes, wiring in `cmd/server/main.go`.
- [X] T006 Account deletion deletes `exercise_notes`; public table count is 46.
- [X] T007 Data export carries `exercise_notes`.
- [X] T008 Unit tests: model key, service, handler; route registration.
- [X] T009 Integration tests: save, edit, clear, limit, checks, another athlete, export.
- [X] T010 `docs/CONTRACTS.md` (contract 1.19.0), `docs/DATABASE_DESIGN.md`, regenerated swagger, smoke step 8e.
- [ ] T011 `./scripts/smoke-toggle.sh` against a running stack (CI `smoke` job runs it).

## Evidence

2026-11-25, branch `feat/040-exercise-notes`:

| Check | Result |
| --- | --- |
| `go test ./...` | Passed |
| `golangci-lint run` | Passed, 0 issues |
| Service and middleware coverage | 91.5% and 93.2% |
| `./scripts/validate-database-design.sh` | Passed: 47 tables, 495 columns |
| `go test -p 1 -race -tags=integration ./internal/dao ./internal/dbmigrate` on a scratch PostgreSQL 16 migrated to 052 | Passed |
| `scripts/validate-supabase-security.sh` on the same database | Passed |
| `swag init` (v1.16.4) | Regenerated, committed |
| `./scripts/smoke-toggle.sh` | Not run locally (no dev stack running); step 8e is syntax-checked only and runs in CI |

The requirement-to-test table is in [spec.md](spec.md), Acceptance.
