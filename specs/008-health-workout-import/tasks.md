---
description: "Implementation tasks for imported health workouts API"
---

# Tasks: Imported health workouts API

**Input**: Design documents from `/specs/008-health-workout-import/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/sport-activities-import.openapi.yaml, quickstart.md

**Tests**: Required by the API constitution and listed before each implementation task. Identify the failing assertion first where practical.

**Organization**: Grouped by user story. US1 (keep once) is the MVP; US2 (consistency) and US3 (reads and compatibility) build on the same transaction and model.

## Format: `[ID] [P?] [Story] Description`

## Requirement and Verification Evidence

| Requirement / acceptance | Tasks | Verification | Result / limitation |
| --- | --- | --- | --- |
| FR-001, FR-009, US3/AC2, SC-005 | T004, T005, T018 | `go test ./internal/model/...` acceptance test: legacy JSON decodes, response carries `source: manual` | Planned |
| FR-002, FR-011, US1 edge cases | T008, T011 | `go test ./internal/service/...` table-driven validation | Planned |
| FR-003, US1/AC2, US1/AC3, SC-001 | T007, T010, T012 | DAO duplicate test; handler 201 → 200 → 200 test | Planned |
| FR-004 | T008, T011 | Service range tests; migration `CHECK`s | Planned |
| FR-005, FR-006 | T008, T011 | Existing sport/date validation reused with `source = apple_health` | Planned |
| FR-007, US2/AC1–AC3 | T014, T015 | DAO test: duplicate path writes no participation; first import upserts once | Planned |
| FR-008, US3/AC1 | T017, T018 | DAO list/get scan the new columns | Planned |
| FR-010, US1/AC4 | T009, T012 | Handler foreign-id 404 test; DAO ownership in duplicate lookup | Planned |
| FR-012 | T019 | Account cleanup test still deletes `sport_activities` | Planned |
| SC-004 | T022 | `go test ./...` | Planned |
| Contract | T002, T023 | `./scripts/smoke-toggle.sh`; CONTRACTS.md updated with code | Planned |
| Schema document | T003, T020 | `./scripts/validate-database-design.sh` | Planned |
| Supabase security | T024 | `./scripts/validate-supabase-security.sh` on a migrated local database | Planned |

## Phase 1: Setup

- [ ] T001 Record `git status --short` and confirm `migrations/` has no `021_*` file; keep `021` per research.md (020 is taken by the open 007 branch)
- [ ] T002 [P] Draft the additive "Sport activities" contract changes in `docs/CONTRACTS.md`: `source`, `external_id`, measurements, the `200` existing-activity response, and field-level 422 rules
- [ ] T003 [P] Add the five new columns and `ux_sport_activities_user_source_external` to the `sport_activities` section of `docs/DATABASE_DESIGN.md`, then run `./scripts/validate-database-design.sh` (expected to fail until T006)

## Phase 2: Foundational

- [ ] T004 [P] Extend `internal/model/sport_activity_acceptance_test.go`: legacy payload decodes; response JSON includes `source`; `external_id` and measurements omitted when nil
- [ ] T005 Add `Source`, `ExternalID`, `DistanceMeters`, `EnergyKcal`, `AvgHeartRateBPM` to `SportActivity` and `CreateSportActivityRequest` in `internal/model/sport_activity.go` with `omitempty` pointers; add `ActivitySource` constants `manual` and `apple_health`
- [ ] T006 Add `migrations/021_add_sport_activity_source.up.sql` and `.down.sql` per data-model.md (columns, `CHECK`s, source/identity pairing check, partial unique index); re-run `./scripts/validate-database-design.sh`

**Checkpoint**: model and schema agree; existing tests still pass.

## Phase 3: User Story 1 - Keep an imported workout exactly once (P1) 🎯 MVP

**Goal**: Create an imported activity once and return the existing row with `200` on any re-submit.

**Independent Test**: Submit the same `apple_health` payload three times with different operation keys and observe `201`, `200`, `200` with one `id`.

- [ ] T007 [P] [US1] Add DAO tests in `internal/dao/sport_activity_dao_test.go`: duplicate `(user, source, external_id)` returns the first row with `created=false` and inserts no activity, participation, or ledger row; a different athlete with the same `external_id` creates its own row
- [ ] T008 [P] [US1] Add service tests in `internal/service/sport_activity_svc_test.go`: omitted source defaults to manual; manual with `external_id` → 422 `external_id`; `apple_health` without `external_id` → 422; unknown source → 422 `source`; identity trimmed and bounded 1–128; each measurement below and above its bound → 422; zero rejected; nil preserved
- [ ] T009 [P] [US1] Add handler tests in `internal/handler/sport_activity_test.go`: `created=true` → 201, `created=false` → 200 with the same body; foreign `id` on GET still 404 `NOT_FOUND`
- [ ] T010 [US1] In `internal/dao/sport_activity_dao.go` `Create`: after the idempotency replay check, when `ExternalID` is set select the owned row by `(user_id, source, external_id)` and return it with `false`; insert the new columns; extend `scanSportActivity` and both `SELECT`s
- [ ] T011 [US1] In `internal/service/sport_activity_svc.go`: normalize and validate `source`, `external_id`, and measurements in `validateSportRequest`; carry the fields onto the model; return the DAO `created` flag from `Create`
- [ ] T012 [US1] In `internal/handler/sport_activity.go` `Create`: write `201` when created and `200` otherwise; update the `SportActivityService` interface and its test mocks in `internal/handler/mocks_test.go` or the service mock file
- [ ] T013 [US1] Confirm `internal/router/training_routes_test.go` and `cmd/server/main.go` need no change (routes unchanged); note the result in this file

**Checkpoint**: US1 passes with `go test ./internal/...`.

## Phase 4: User Story 2 - Count imported sport toward consistency (P2)

**Goal**: A first import credits its date once; a re-import credits nothing.

**Independent Test**: Import a past-dated workout on an inactive date; weekly count rises by one; re-import; count unchanged.

- [ ] T014 [P] [US2] Add DAO test: `apple_health` create on a past date upserts `day_participation` once with the profile timezone and preserves `scheduled_opportunity`; the duplicate path leaves `revision` unchanged
- [ ] T015 [US2] Add a service test that a past `date` with `source = apple_health` is accepted and a future one is rejected, reusing the existing timezone fixtures
- [ ] T016 [US2] Confirm `internal/dao/stats_dao.go` weekly aggregation needs no change (dates already unioned); confirm `total_workouts`, distribution, and volume ignore sport rows; record in this file

## Phase 5: User Story 3 - Review imported activities with their facts (P3)

**Goal**: List and get return origin, identity, and measurements; old rows read as manual.

**Independent Test**: List a range containing a pre-migration row and an imported row; the first has `source: manual` and no extras, the second has all supplied fields.

- [ ] T017 [P] [US3] Extend DAO list/get tests with the new columns, including a row whose measurements are null
- [ ] T018 [US3] Verify JSON omission rules end to end in a handler list test (`external_id` absent on manual rows)
- [ ] T019 [US3] Confirm `internal/dao/account_dao.go` deletion covers imported rows (same table) and the test in its `_test.go` still passes; record in this file

## Phase 6: Polish and Verification

- [ ] T020 Run `gofmt -w` on changed Go files and `./scripts/validate-database-design.sh`
- [ ] T021 Finalize `docs/CONTRACTS.md` wording against the implemented behavior; bump the sport-activities contract note to 1.1.0
- [ ] T022 Run `go test ./...` and `golangci-lint run`
- [ ] T023 Run `./scripts/smoke-toggle.sh`; add a wire assertion for `source` in `scripts/smoke.sh` if the smoke suite exercises sport routes
- [ ] T024 Apply migration 021 to a local database and run `./scripts/validate-supabase-security.sh`; record the outcome or the reason it could not run
- [ ] T025 Walk the quickstart manual acceptance against the local API and record results in the evidence table above
- [ ] T026 Update the sibling app plan's dependency note if any contract detail changed during implementation

## Dependencies & Execution Order

- T001–T003 can run together; T003's validation is expected to fail until T006.
- T004 → T005 → T006 are sequential (test, model, migration).
- US1 tests T007–T009 are parallel and precede T010 → T011 → T012.
- US2 and US3 depend on US1's DAO changes; their tests can be written in parallel once T010 lands.
- Phase 6 follows all stories. The API deploys before the app release.

## Implementation Strategy

Ship US1 first: it delivers the whole data path and the duplicate guarantee. US2 is mostly
confirmation that existing participation code behaves on the new path. US3 is read coverage. Keep
one commit per phase on `spec/008-health-workout-import` and open the PR with the contract diff
first in the description.
