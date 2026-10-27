# Quickstart: Daily nutrition totals and a chosen target

## Implementation order

1. Migration 029 (up and down) with both tables, RLS, Data API revoke, and runtime-role policy and grant;
   `docs/DATABASE_DESIGN.md` sections and index rows; `./scripts/validate-database-design.sh`.
2. Model, enums, request types, outcomes, and the migration acceptance test; update
   `supabase_security_migration_test.go` expectations for the new tables.
3. Service validation with table-driven tests (bounds, one-decimal rounding, at-least-one-value, source
   and revision rules by source, future date, range, target protein > 0).
4. DAO transactions with source-order tests: replay → select for update → branch → ledger → commit, for
   both the day and the target.
5. Handler: statuses by outcome, routes under `/api/v1/nutrition`, wiring in `main.go`, router test.
6. `docs/CONTRACTS.md` section "Nutrition" after "Body weight"; smoke step; Swagger regeneration.
7. `gofmt`, `go test ./...`, `golangci-lint run`, `./scripts/smoke-toggle.sh`, security SQL on a migrated database.

## Tests identified before implementation

| Requirement | Evidence |
|---|---|
| FR-001, FR-006 | Handler list test: `{days:[],targets:[]}` when empty; days oldest first; targets include the one in effect on `from`; range reuses the 366-day rule and rejects `to < from` |
| FR-002, SC-001 | Service test: future date → 422 `date`; local today computed from the profile timezone across a UTC midnight boundary |
| FR-003, SC-003 | DAO test: `expected_revision` 0 with existing row → 409 with `actual_revision`; stale on replace → 409 with stored day as `resource`; correct → revision+1 |
| FR-004, SC-002 | DAO test: verbatim retry replays 201/200/204; same key different payload → `IDEMPOTENCY_CONFLICT` |
| FR-005 | DAO test: all-absent save deletes and returns `removed`; all-absent with no row returns `removed` and writes nothing |
| FR-007, SC-004 | DAO test: Health save over manual row → `ignored`, row unchanged, ledger written; Health save over Health row → replaced, source still `apple_health` |
| FR-008 | DAO test: manual save over Health row with current revision → source `manual`, revision+1; a later Health save → `ignored` |
| FR-009 | DAO test: same `effective_from` replaces; new date inserts and the old row remains; identical values → `unchanged` |
| FR-010 | Handler test: `GET /nutrition/target` → 404 when none; current is latest `effective_from <= today`, not a future-dated row |
| FR-011 | Service tests: no code path mutates targets or days outside the two PUTs (grep-style DAO test on statements) |
| FR-012 | Handler foreign-user test; DAO queries carry `user_id`; account cleanup test includes both tables |
| FR-013 | Service tests: negative, >bound, 2-decimal → 422 naming the field; target `protein_g` 0 → 422 |
| FR-014, SC-006 | `./scripts/smoke-toggle.sh`; existing smoke steps unchanged; contract tests for the new section |
| SC-005 | Account deletion test asserts zero rows in both tables |

## Manual acceptance

Save a target (`PUT /nutrition/target`, revision 0) and see `201`; save it again with the returned
revision and the same values, see `200` unchanged. Save today's totals (revision 0) → `201`; retry
with the same key → `201` same body; send a Health save for today → `200` with the manual values
unchanged. Save yesterday from Health (no revision) → `201` with `source: apple_health`; save yesterday
manually with revision 1 → `200`, `source: manual`, revision 2; send yesterday from Health again →
`200` unchanged. Clear today (all values absent, revision 2) → `204`. Read the range and see yesterday
only, plus one target. Try tomorrow → `422 date`. Read with another account → empty collections.

## Compatibility and rollback

Additive: two tables and four routes; no existing field, route, or error changes. Deploy before the
app release; an old app never calls these routes. Roll back the app first, then run the down
migration, which drops both tables. Mixed versions are safe in both directions.
