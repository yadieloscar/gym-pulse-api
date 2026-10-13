# Tasks: Push/Pull/Legs Starter Program

Design is a single additive seed migration; no plan.md is needed beyond the notes in spec.md.

- [X] T001 Add `migrations/020_seed_push_pull_legs_starter.up.sql` (FR-001..FR-004)
- [X] T002 Add `migrations/020_seed_push_pull_legs_starter.down.sql` (FR-004)
- [X] T003 Add `TestPushPullLegsStarterSeed` in `internal/model/goal_based_training_acceptance_test.go` covering markers, 15 catalog-resolvable exercises, and the down migration (SC-002)
- [X] T004 Update `docs/DATABASE_DESIGN.md` migration sources for `starter_programs` (FR-005)
- [X] T005 Run `go test ./...`, `golangci-lint run`, `./scripts/validate-database-design.sh`; apply migrations to a local database and verify SC-001/SC-002 (evidence below)

## Evidence

| Check | Result |
|---|---|
| `go test ./internal/model/ -run TestPushPullLegsStarterSeed` | passed |
| `go test ./...` | passed |
| `golangci-lint run` | passed (0 issues) |
| `./scripts/validate-database-design.sh` | passed (30 tables, 277 columns, 24 indexes) |
| Live migration + catalog query | passed on postgres:16-alpine: 7 starters, PPL has 3 workouts x 5 exercises all catalog-linked, re-apply adds nothing, down removes all PPL rows (2026-10-12) |
