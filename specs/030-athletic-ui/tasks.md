# Tasks: Athletic UI palettes

The design is one additive check-and-default migration plus two validator tags and one default value; the design notes in `spec.md` stand in for a separate plan.

- [X] T001 Add `migrations/042_add_settings_palette_iris.up.sql` and `.down.sql` (FR-003, FR-004, FR-005, FR-006)
- [X] T002 Add `irisViolet` to both palette validators and default to `voltLime` in `internal/model/types.go` (FR-001, FR-002, FR-004)
- [X] T003 Add `internal/model/settings_test.go`: default value, all seven palettes on both request types, migration text in step with the validator and default (FR-001, FR-003, FR-004, FR-006)
- [X] T004 Extend `internal/service/settings_svc_test.go`: `irisViolet` accepted and passed to the DAO, unknown name rejected before the DAO (FR-001)
- [X] T005 Extend `internal/handler/settings_test.go`: handler over the real service returns 200 for all seven palettes, `voltLime` for an account with no settings, 422 for unknown names (SC-001, SC-002, SC-003)
- [X] T006 Add `internal/dao/settings_integration_test.go`: no-row read, first partial write, column default, all seven palettes stored, check rejects an unknown name (FR-003, FR-004)
- [X] T007 Add smoke step 2b to `scripts/smoke.sh` (SC-001, SC-002)
- [X] T008 Update `docs/CONTRACTS.md` (contract 1.9.0), `docs/DATABASE_DESIGN.md`, and regenerate Swagger (FR-007)
- [X] T009 Run the repository gates (SC-004, evidence below)

## Evidence

Run on 2026-11-15 against a disposable `postgres:16-alpine` stack built from this branch with the CI compose settings (restricted `gym_pulse_api` runtime role).

| Check | Result |
|---|---|
| `go test ./...` | passed |
| `golangci-lint run` | passed (0 issues) |
| `./scripts/validate-database-design.sh` | passed (43 tables, 447 columns, 49 indexes) |
| `swag init -g cmd/server/main.go --output docs` (v1.16.4) | regenerated; only the two palette enums changed |
| `./scripts/smoke.sh` against the branch image | passed (38 passed, 0 failed), including step 2b |
| `go test -race -tags=integration ./internal/dao -count=1` | passed |
| `./scripts/validate-supabase-security.sh` | passed |
| Fresh account `GET /api/v1/settings` on the live stack | `{"weight_unit":"lb","weekly_goal":5,"palette":"voltLime"}` |
| Migration 042 down then up on a row holding `irisViolet` | passed: down moves the row to `voltLime`, restores the six-value check and `obsidianEmber` default; up restores seven values and `voltLime` default |
| `./scripts/smoke-toggle.sh` | not run: it recreates the shared `gym-pulse-api-1` dev container from the main checkout's image. `smoke.sh` was run directly against this branch's image instead, as CI does |

## Rollout

Deploy before the app change (`gym-pulse-app` spec `034-athletic-ui`). Not merged and not deployed by this task.
