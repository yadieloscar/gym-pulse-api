# Quickstart: Remember a workout's health export

## Implementation order

1. Migration 023 (up and down), `docs/DATABASE_DESIGN.md` table and index rows, `./scripts/validate-database-design.sh`.
2. Model and migration acceptance test.
3. Service validation with table-driven tests (kind, source, identity bounds, exported_at clamp, range).
4. DAO transaction with source-order tests: replay → resolve workout → existing lookup → insert → ledger → commit.
5. Handler: 201 / 200 by outcome, conflict code mapping in `response.go`, routes, wiring, router test.
6. `docs/CONTRACTS.md` section "Health exports"; smoke step.
7. `gofmt`, `go test ./...`, `golangci-lint run`, `./scripts/smoke-toggle.sh`, security SQL on a migrated database.

## Tests identified before implementation

| Requirement | Evidence |
|---|---|
| FR-001, FR-005 | Handler list test: `[]` when empty; rows include every field; range validation reuses the 366-day rule |
| FR-002, SC-001 | DAO test: same identity returns existing with `existing`; different identity returns `ConflictError`; handler maps 201 → 200 → 409 |
| FR-003 | DAO test: unfinished (`planned`, `missed`, no `finalized_at`), `rest` log, missing, and foreign ids all surface `NotFoundError` |
| FR-004 | Service tests: blank, whitespace, 129-character identity → 422 `external_id`; unknown kind/source → 422 |
| FR-006 | DAO test: no statement touches `scheduled_workouts`, `day_logs`, participation, or stats tables beyond `SELECT` |
| FR-007 | Account cleanup test includes `health_exports` |
| SC-002, SC-004 | `go test ./...`; smoke suite unchanged for other routes |
| SC-003 | Handler foreign-id test; DAO queries carry `user_id` |

## Manual acceptance

Finish a planned workout in the app (or call `/complete` in smoke), `PUT` an export with a UUID, see
`201`; repeat with a new operation key and the same UUID, see `200` and the same `id`; send a
different UUID, see `409 HEALTH_EXPORT_CONFLICT` with the stored record. List the range and see the
row with the workout's date. Try a missed planned workout and a rest log: `404`.

## Compatibility and rollback

Additive: a new table and two new routes. Deploy before the app release. Roll back the app first,
then run the down migration, which drops the table.
