# Quickstart: validate skip and recovery resolution

## Prerequisites

- Go toolchain and Docker; local stack from the workspace root (`docker compose up`).
- For integration tests: `GYMPULSE_TEST_DATABASE_URL` pointing at a disposable PostgreSQL.

## Automated checks

```bash
go test ./...
go test -race -tags=integration ./internal/dao -run '^TestRelease'
./scripts/validate-database-design.sh
swag init -g cmd/server/main.go --output docs && git diff --exit-code docs/
./scripts/smoke-toggle.sh
```

Expected: all pass; swagger regeneration produces no diff after the committed update.

## Scenarios (map to spec acceptance)

| # | Steps | Expected | Spec |
| --- | --- | --- | --- |
| 1 | Seed a finalized missed workout; `POST /scheduled-workouts/{id}/skip` | 200, `resolution: "skipped"`, revision +1, `status` still `missed` | US1-1, FR-001–003 |
| 2 | After 1, `POST /schedule/recover` for today | 404 when no other unresolved miss | US1-2, FR-004 |
| 3 | Repeat 1 with the same key and body | Identical stored response, no further revision change | US1-3, FR-007 |
| 4 | Skip a `completed` workout | 409 `RESOLUTION_CONFLICT` | US1-4, FR-006 |
| 5 | Recover an unresolved miss, then read the schedule | Source has `resolution: "recovered"`; a second recover with a new key returns 404 | US1-5, FR-004 |
| 6 | Skip, then `POST .../unskip` | 200, no `resolution`; recover now selects it | US2-1, FR-005 |
| 7 | Unskip a recovered workout | 409 `RESOLUTION_CONFLICT` | US2-2 |
| 8 | Skip with another user's token | 404 `NOT_FOUND` | FR-008 |
| 9 | Skip, then regenerate and apply a plan transition | `resolution` unchanged | FR-009 |
| 10 | Compare `/stats`, participation, and records before and after skip/unskip | Identical | FR-002, SC-002 |
| 11 | Migrate a database with a pre-020 recovery record and a later-cycle occurrence of the same workout | Only the recovered source is marked; the later occurrence stays unresolved | FR-012 |
| 12 | Migrate down then up | Columns dropped and recreated; backfill reapplied | Rollback |
| 13 | Skip a miss, then log a set on it as a correction | Outcome becomes incomplete; `resolution` cleared | FR-013 |
