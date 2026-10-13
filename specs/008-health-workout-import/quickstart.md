# Quickstart: Imported health workouts API

## Implementation order

1. Write migration 022 (up and down), update `docs/DATABASE_DESIGN.md`, and run
   `./scripts/validate-database-design.sh`.
2. Extend the model and acceptance test with `source`, `external_id`, and measurements.
3. Add service validation with table-driven tests: default source, forbidden identity on manual,
   required identity on `apple_health`, length and range bounds, unknown source.
4. Add the DAO duplicate lookup inside the existing transaction and return `created`; test that a
   duplicate writes no activity, participation, or ledger row.
5. Map `created` to `201` or `200` in the handler and add handler tests for both statuses.
6. Update `docs/CONTRACTS.md` under "Sport activities" and add the smoke assertion if the suite
   covers sport routes.
7. Run `gofmt`, `go test ./...`, `golangci-lint run`, `./scripts/smoke-toggle.sh`, and
   `./scripts/validate-supabase-security.sh` against a migrated local database.

## Tests identified before implementation

| Requirement | Evidence |
|---|---|
| FR-001, FR-009 | Model acceptance test: existing JSON without new fields decodes; response includes `source: manual` |
| FR-002, FR-011 | Service tests for identity presence, length, and source enum |
| FR-003 | DAO test: second submit with same `(user, source, external_id)` returns the first row, `created = false`, no participation or ledger insert |
| FR-004 | Service tests for each measurement bound and for absent values staying nil |
| FR-006, FR-007 | Existing date and participation tests reused with `source = apple_health` |
| FR-008 | DAO list test includes the new columns in the scan |
| FR-010 | Handler test: foreign activity id still returns the shared 404 |
| SC-001 | Handler test: same payload three times yields 201 then 200, 200 |

## Manual acceptance

Create an `apple_health` activity with `external_id`, distance, and energy; confirm `201` and the
fields in the body. Repeat with a new `operation_key`; confirm `200`, the same `id`, and unchanged
weekly progress. Repeat with the original key; confirm `201` replay. Send `source: manual` with an
`external_id`; confirm `422` on `external_id`. List the range and confirm `source` on every row,
including rows created before the migration.

## Compatibility and rollback

The change is additive with a column default; no backfill. Deploy this API before releasing the
app. Old apps ignore the new response fields and keep creating manual activities. Roll back the app
first, then run the down migration, which drops the index and columns.
