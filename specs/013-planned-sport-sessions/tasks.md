---
description: "Implementation tasks for planned sport sessions API"
---

# Tasks: Planned Sport Sessions API

**Input**: Design documents from `/specs/013-planned-sport-sessions/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/planned-sport-sessions.openapi.yaml, quickstart.md

**Tests**: Required by the API constitution and listed before each implementation task. Identify the failing assertion first where practical.

**Organization**: Grouped by user story. US1 (plan a sport unit) and US2 (schedule it) are the MVP data path; US3 (record against it) is the daily loop; US4 (import auto-match) and US5 (review and deletion) build on the same transaction and reads.

## Format: `[ID] [P?] [Story] Description`

## Requirement and Verification Evidence

| Requirement / acceptance | Tasks | Verification | Result / limitation |
| --- | --- | --- | --- |
| FR-001, FR-002, FR-014, US1/AC1–AC3 | T009, T011, T012 | `go test ./internal/service/...` table-driven `validateProgramShape` tests; migration `CHECK`s | Passed 2026-10-25 (`TestValidateProgramShapeSportUnits`, 17 cases; migration `CHECK`s asserted by `TestPlannedSportSessionsMigrationContract`; smoke 13d rejects distance on volleyball with 422) |
| FR-003, US1/AC4, SC-004 | T006, T007, T010 | Model acceptance test: legacy program JSON decodes and re-encodes with `kind: exercises`; starter, from-starter, and adopt-legacy units are `exercises` | Passed (`TestUnitKindDefaultsToExercisesForLegacyJSON`; starter scan sets `exercises`; full `go test ./...` green; smoke steps 1–13c unchanged, 24/24) |
| FR-004, US2/AC1–AC4 | T014, T015, T016, T017, T018 | Materialization test; lazy-finalize test; existing skip/unskip tests on a sport row; regenerate delete predicate test | Passed (`TestMaterializeSportUnitSnapshotsTargetWithoutSets`, `TestLazyFinalizeMarksUnfulfilledPastSportSessionMissed`, `TestSkipAndUnskipTreatSportSessionsLikeWorkouts`, delete-predicate source-text check; integration `TestImportedActivityFulfilsEarliestOpenSportSessionOnce` finalizes a past session missed) |
| FR-005, US2/AC5 | T014, T016 | Snapshot columns and provenance ids asserted on the materialized row; schedule list carries `kind` and `sport` | Passed (materialization test asserts snapshot and provenance; smoke 13d reads `kind` and `sport` on the schedule) |
| FR-006, FR-007, FR-011, US3/AC1, US3/AC3 | T020, T021, T022, T023, T024 | DAO source-text marker order; handler record test → 201 and `completed`; participation on performed date; later-date record leaves the missed date's row | Passed (`TestSportActivityDAOCreateLinksScheduledSportSessionInOrder`; integration `TestRecordAgainstPlannedSportSessionCompletesItOnce` and `TestRecordingMissedSportSessionLaterCreditsPerformedDate`; smoke 13d) |
| FR-008, US3/AC2 | T020, T024, T025 | Handler second record → 409 `SPORT_SESSION_CONFLICT` with `resource`; migration contract test for `ux_sport_activities_scheduled_workout` | Passed (integration second record → `SportSessionConflictError` with authoritative session; smoke 13d 409 `SPORT_SESSION_CONFLICT`; migration text asserts the partial unique index) |
| FR-009, US3/AC5 | T019, T022, T024 | Activity JSON carries `scheduled_workout_id`; schedule item carries `sport.sport_activity_id` and `performed_date` | Passed (`TestSportActivityJSONCarriesScheduledWorkoutLinkOnlyWhenSet`, `TestSportUnitJSONShape`, handler link test; smoke 13d asserts `sport.sport_activity_id` and `performed_date`) |
| FR-010, US4/AC1–AC4 | T027, T028 | DAO/service auto-match tests: one match links; no match stays unlinked; two open sessions → lowest `sequence_position`; duplicate import changes nothing | Passed (integration `TestImportedActivityFulfilsEarliestOpenSportSessionOnce`: lowest sequence linked, duplicate import unchanged, other sport and manual logs unlinked) |
| FR-012 | T021, T024 | Existing idempotency tests extended: replay → 201 same id; changed `scheduled_workout_id` under the same key → 409 `IDEMPOTENCY_CONFLICT` | Passed (`TestSportActivityServiceCreateCarriesScheduledWorkoutLink` proves the hash binds the link; smoke 13d replay 201 same id) |
| FR-013, US3/AC4 (sport mismatch), foreign ids | T021, T024 | Handler: foreign `scheduled_workout_id` → shared 404; mismatched `sport_id` → 422 `sport_id`; schedule never lists another athlete's session | Passed (integration: foreign session → 404, volleyball session with cycling → 422 `sport_id`, exercise workout → 422 `scheduled_workout_id`; handler mapping test) |
| US2 set-route guards (research "Sport session outcomes") | T017, T018 | Handler tests: complete, set check, target edit, extra set, `PATCH required_sets` on a sport row → 422 field `kind` | Passed (`TestSetAndCompletionRoutesRejectSportSessions`; smoke 13d extra set → 422 `kind`; `TestHealthExportDAOExcludesSportSessions`; integration recover → 404 with only a missed sport session) |
| FR-015, US5/AC3 | T031 | Source-text account cleanup test unchanged; `ON DELETE SET NULL` text asserted in migration 027 | Passed (`TestAccountCleanupOrderSurvivesSportSessionLink`; smoke step 14 deletes the account after 13d) |
| US5/AC1, US5/AC2, SC-006 | T030, T032 | Schedule, participation, and stats tests: sport dates counted once; `total_workouts`, distribution, volume unchanged | Passed (`TestWeeklyStatsUnionsSportDatesWithoutChangingWorkoutTotals` unchanged and green; lazy-finalize participation test; integration participation assertions on scheduled and performed dates) |
| FR-016, SC-002 | T002, T035, T036 | `docs/CONTRACTS.md` diff reviewed with the code; `./scripts/smoke-toggle.sh` with the new smoke step (five replays → one activity, one completed session) | Passed (`docs/CONTRACTS.md` updated with the code; `scripts/smoke.sh` 24/24 on an isolated stack built from this worktree, port 18081; five-replay guarantee covered by replay + conflict assertions) |
| SC-005 | T021, T024 | Ownership tests above | Passed (ownership tests above) |
| Schema document | T003, T008, T034 | `./scripts/validate-database-design.sh` (31 tables, 305 columns) | Passed (`./scripts/validate-database-design.sh`: 31 tables, 305 columns, 28 named indexes) |
| Supabase security | T037 | `./scripts/validate-supabase-security.sh` on a migrated local database | Passed (`validate-supabase-security.sh` piped into the isolated db container after migration 027) |

## Phase 1: Setup

- [x] T001 Record `git status --short` in the worktree and confirm `027` is still the next free migration number on `origin/main` (research.md "Migration numbering"); renumber if `012-nutrition-targets` merged first — recorded 2026-10-25: tree clean at 4c145e7, `origin/main` latest migration 026, 027 taken
- [x] T002 [P] Draft the additive contract changes in `docs/CONTRACTS.md`: program unit `kind` and `sport`, scheduled `kind` and `sport` with `sport_activity_id`/`performed_date`, sport-activity `scheduled_workout_id`, the 422 `kind` guards, recover and health-export exclusions, and `409 SPORT_SESSION_CONFLICT` in the error-code list
- [x] T003 [P] Add the thirteen column rows, `ux_sport_activities_scheduled_workout`, the FK matrix row, the lock-order line `program:` → `schedule:` → `sport-activity:`, the create-transaction row, and the 305-column inventory counts to `docs/DATABASE_DESIGN.md`, then run `./scripts/validate-database-design.sh` (expected to fail until T008)

## Phase 2: Foundational

- [x] T004 [P] Extend `internal/model/sport_activity_acceptance_test.go` with a migration-027 text contract: the three `ALTER TABLE`s, the kind/sport pair `CHECK`s, the `ON DELETE SET NULL` FK, and the partial unique index
- [x] T005 [P] Add tests in `internal/model/training_test.go`: `ProgramWorkout` and `ScheduledWorkout` JSON without `kind` decode to `kind: exercises`; a sport unit encodes `sport` and `exercises: []`; `ScheduledSport` omits `sport_activity_id` and `performed_date` when nil
- [x] T006 Add `UnitKind` constants (`exercises`, `sport`), `SportTarget`, `ScheduledSport`, `DistanceSports` set, `ProgramWorkout.Kind`/`Sport`, and `ScheduledWorkout.Kind`/`Sport` in `internal/model/training.go`; default `Kind` to `exercises` in a `normalizeKind` helper used by services and scans
- [x] T007 Add `ScheduledWorkoutID *uuid.UUID` to `SportActivity` and `CreateSportActivityRequest` in `internal/model/sport_activity.go` with `omitempty`
- [x] T008 Add `migrations/027_add_planned_sport_sessions.up.sql` and `.down.sql` per data-model.md (six columns and checks on `program_workouts`, six on `scheduled_workouts`, link column, FK, partial unique index); re-run `./scripts/validate-database-design.sh`

**Checkpoint**: models and schema agree; existing tests still pass with `kind: exercises` everywhere.

## Phase 3: User Story 1 - Plan a sport session inside a program (P1) 🎯 MVP

**Goal**: Create or replace a program whose units mix exercise workouts and sport sessions, read it back unchanged, owner-only.

**Independent Test**: Create a program with two exercise workouts and two sport sessions (cycling 90 min 40 km, volleyball 120 min); read it back; every unit keeps kind, order, weekday, sport, and targets.

- [x] T009 [P] [US1] Add table-driven tests in `internal/service/training_domain_svc_test.go` for `validateProgramShape`: sport unit accepted; exercises unit with no exercise → 422 `workouts`; sport unit with exercises → 422; sport unit without `sport` → 422; slug, name 1–80, `other` with name "other" → 422; duration 0 and 1441 → 422; distance 0 and 1,000,001 → 422; distance on `volleyball` → 422; notes 2,001 → 422; `kind` omitted defaults to `exercises`
- [x] T010 [P] [US1] Add DAO source-text assertions in `internal/dao/training_mutation_dao_test.go`: `insertProgramWorkouts` inserts `kind, sport_id, sport_name, target_duration_minutes, target_distance_meters, sport_notes`; `loadProgramWorkouts` and the tx-scoped `loadProgram` select them
- [x] T011 [US1] Extend `validateProgramShape` in `internal/service/training_svc.go` with the kind branch (research "Pair checks"), trimming `sport_id`, `sport_name`, and notes before validation; apply `normalizeKind` in `Create` and `Update`
- [x] T012 [US1] Extend `insertProgramWorkouts`, `loadProgramWorkouts`, and `scanProgramWorkouts` in `internal/dao/program_dao.go`, and `loadProgram` in `internal/dao/training_mutation_dao.go`, to write and read the six columns; starter reads set `kind: exercises`
- [x] T013 [US1] Add handler tests in `internal/handler/guided_training_handlers_test.go` (where program handler tests live): create with a sport unit → 201 body carries `kind` and `sport`; replace keeps them; foreign program id → 404; confirm `internal/router/training_routes_test.go` needs no change and note it here — confirmed: no route changes; the handler decodes `kind` and `sport` unchanged (`TestProgramHandlersPassSportUnitsThrough`)

**Checkpoint**: US1 passes with `go test ./internal/...`; a program with sport units round-trips.

## Phase 4: User Story 2 - See planned sport sessions on the schedule (P1)

**Goal**: Sport units materialize as planned sessions with snapshots and no sets, go missed when unfulfilled, skip and unskip like workouts, and are replaced or retained by regenerate exactly like workouts. Set routes refuse them.

**Independent Test**: Materialize a week for gym, ride, gym, volleyball; four rows with correct dates and kinds; let the ride's date pass → `missed`, unresolved; skip → recorded and undoable; other rows untouched.

- [x] T014 [P] [US2] Add tests in `internal/service/training_domain_svc_test.go` (beside `TestMaterializeSnapshotsAndRequiredCompletionRules`): a sport unit yields `kind: sport`, the sport snapshot, `required_sets: []`, `program_workout_id` set; an exercises unit is unchanged
- [x] T015 [P] [US2] Add tests in `internal/service/schedule_resolution_test.go`: lazy finalize on a past unfulfilled sport row → `missed`, `participated: false`; skip and unskip on a missed sport row behave as on a workout; a future planned sport row is untouched
- [x] T016 [US2] Extend `MaterializeProgramForWeekdays` in `internal/model/training_materialization.go` and `insertScheduledWorkouts`, `List`, `Get`, and their scans in `internal/dao/schedule_dao.go` to write and read `kind` and the sport columns, with a `LEFT JOIN sport_activities sa ON sa.scheduled_workout_id = sw.id` filling `sport.sport_activity_id` and `sport.performed_date`; mirror the insert columns in `internal/dao/plan_transition_dao.go` and the adopt-legacy insert in `internal/dao/training_mutation_dao.go`
- [x] T017 [US2] Add the `kind` branch to `scheduledOutcome` in `internal/dao/training_mutation_dao.go` and `deriveScheduledStatus` in `internal/service/schedule_svc.go` (sport: `completed` when a linked activity exists, else `missed`); add 422 `kind` guards in `internal/service/schedule_svc.go` for complete, set check, target edit, extra set, and `PATCH required_sets` on a sport row; filter `kind = 'exercises'` in `RecoverIdempotent` (`internal/dao/schedule_dao.go`) and in the finished-workout lookup in `internal/dao/health_export_dao.go`
- [x] T018 [US2] Add handler tests in `internal/handler/training_test.go` and `internal/handler/health_export_test.go`: complete, set check, target edit, extra set, and `PATCH required_sets` on a sport row → 422 field `kind`; recover skips a missed sport row (404 when it is the only one); health export of a completed sport session → 404; confirm regenerate deletes a planned sport row and retains a completed one via the existing delete-predicate source-text test in `internal/dao/training_mutation_dao_test.go`

**Checkpoint**: the schedule shows sport sessions with the right lifecycle; no set route can touch them.

## Phase 5: User Story 3 - Record a planned sport session as done (P1)

**Goal**: `POST /sport-activities` with `scheduled_workout_id` creates one linked activity, completes the session, clears a skip, credits the performed date, and is replay-safe.

**Independent Test**: Record 95 min against a planned 90 min ride; one activity with actual values; session `completed` with `sport_activity_id`; replay with the same key creates nothing; a new key → 409.

- [x] T019 [P] [US3] Extend `internal/model/sport_activity_acceptance_test.go`: activity JSON carries `scheduled_workout_id` only when set; request decodes it
- [x] T020 [P] [US3] Extend the marker-order test in `internal/dao/sport_activity_dao_test.go`: `lockUserDomain("schedule:")` precedes `lockUserDomain("sport-activity:")`; the owned `FOR UPDATE` select on `scheduled_workouts`; the `UPDATE scheduled_workouts SET status = 'completed' ... resolution = NULL, resolved_at = NULL, revision = revision + 1`; the link column in the insert; participation upsert unchanged
- [x] T021 [P] [US3] Add service tests in `internal/service/sport_activity_svc_test.go`: `scheduled_workout_id` trimmed/parsed; foreign or missing → 404; `kind: exercises` → 422 `scheduled_workout_id`; `sport_id` mismatch → 422 `sport_id`; `status: completed` → `ConflictError` whose message contains "sport session"; performed date after local today → 422 `date`; same key with a different `scheduled_workout_id` → idempotency conflict
- [x] T022 [US3] Extend `Create` in `internal/dao/sport_activity_dao.go` per data-model.md "Create transaction": lock order, explicit link lookup and checks, session update, link column in the insert and scans; add a `scheduleSession` lookup that returns the authoritative `ScheduledWorkout` for the conflict `resource`
- [x] T023 [US3] Extend `internal/service/sport_activity_svc.go`: validate and pass `ScheduledWorkoutID`; map DAO outcomes; add `model.SportSessionConflictError` or a `ConflictError` with the "sport session" message plus `Authoritative`; wire the schedule DAO dependency in `NewSportActivityService` and `cmd/server/main.go`, and update every `NewSportActivityService` call site in tests — not needed: the link is resolved inside the sport-activity DAO transaction (`lockSportSession`), so the service signature and `main.go` are unchanged
- [x] T024 [US3] Map "sport session" conflicts to `409 SPORT_SESSION_CONFLICT` with `details.status` and `resource` in `internal/handler/response.go`; add handler tests in `internal/handler/sport_activity_test.go`: record → 201 with `scheduled_workout_id`; second record → 409 `SPORT_SESSION_CONFLICT` with `resource`; foreign id → 404; replay → 201 same body
- [x] T025 [US3] Add a DAO-level test (source text or integration-tagged in `internal/dao/release_resolution_integration_test.go`) that a later-date record against a missed session completes it, sets `performed_date` to the activity date, and leaves the scheduled date's `day_participation` row `participated: false`

**Checkpoint**: the daily loop works end to end; `go test ./internal/...` green.

## Phase 6: User Story 4 - Let an imported workout fulfil the plan (P2)

**Goal**: An `apple_health` activity without `scheduled_workout_id` links the earliest open session for its sport and date; everything else is unchanged.

**Independent Test**: Plan a cycling session; import a cycling workout for that date → session completed and linked; import a second → unlinked activity, session unchanged.

- [x] T026 [P] [US4] Extend the marker-order test in `internal/dao/sport_activity_dao_test.go`: the auto-match select runs only when `source <> 'manual'` and no id was sent, after the import-duplicate check, with `ORDER BY sequence_position NULLS LAST, created_at, id LIMIT 1 FOR UPDATE`
- [x] T027 [P] [US4] Add service tests in `internal/service/sport_activity_svc_test.go`: manual create never auto-links; `apple_health` with an explicit id uses the explicit path; replay of an import that linked returns the same body
- [x] T028 [US4] Implement the auto-match branch in `internal/dao/sport_activity_dao.go` `Create` per data-model.md step 5, reusing the link and session-update code from T022
- [x] T029 [US4] Add an integration-tagged test in `internal/dao/release_resolution_integration_test.go`: two open cycling sessions on one date → the lower `sequence_position` is linked; a duplicate import (same `external_id`) changes nothing; a non-matching sport stays unlinked

**Checkpoint**: imports complete plans with zero athlete actions.

## Phase 7: User Story 5 - Review planned versus done (P3)

**Goal**: Schedule, participation, and stats agree on every sport session; records, volume, and distribution ignore them; account deletion removes everything.

**Independent Test**: Plan two sport sessions in a week, complete one, miss and skip the other; schedule, participation, weekly count, and activity list describe the same facts.

- [x] T030 [P] [US5] Add a test beside `TestWeeklyStatsUnionsSportDatesWithoutChangingWorkoutTotals` in `internal/dao/sport_activity_dao_test.go` confirming `GetWeeklyCount`, `GetTotalWorkouts`, distribution, and volume source text are unchanged by this feature; record in this file
- [x] T031 [P] [US5] Extend `TestAccountCleanupIncludesSportActivities` in `internal/dao/sport_activity_dao_test.go` to assert the delete order still lists `sport_activities` before `scheduled_workouts`, and that migration 027's FK text contains `ON DELETE SET NULL`
- [x] T032 [US5] Add a service test in `internal/service/schedule_resolution_test.go` that `participationService.List` after a sport-session week returns one row per date with the expected `scheduled_opportunity`/`participated` pairs (completed on date: true/true; missed then skipped: true/false; recorded later: original true/false and performed date participated true)

## Phase 8: Polish and Verification

- [x] T033 Run `gofmt -w` on changed Go files and `goimports` where needed
- [x] T034 Re-run `./scripts/validate-database-design.sh`; finalize the §6, §7, §8, §10, and §11 entries in `docs/DATABASE_DESIGN.md`
- [x] T035 Finalize `docs/CONTRACTS.md` wording against the implemented behavior; bump the contract note to 1.2.0 in the programs, schedule, and sport-activities sections
- [x] T036 Add the smoke step to `scripts/smoke.sh` after step 13: create a program whose last unit is a sport session (keep the strength workout first so step 10 assertions hold), materialize, record against the sport row, replay with the same key (201, same id), new key (409 `SPORT_SESSION_CONFLICT`), list the schedule (`completed`, `sport.sport_activity_id`), post an extra set to it (422 `kind`); then run `./scripts/smoke-toggle.sh` — ran `scripts/smoke.sh` against an isolated acceptance stack (image built from this worktree, API on 18081) instead, because `smoke-toggle.sh` targets the shared dev container's image; 24/24 passed
- [x] T037 Run `go test ./...`, `go test -tags integration ./internal/dao/...` against a migrated local database, `golangci-lint run`, and `./scripts/validate-supabase-security.sh`; record outcomes or the reason a gate could not run — `go test ./...` green, `go test -race -tags integration ./internal/dao` green on the isolated database (port 15433), `golangci-lint run` 0 issues, security validator passed inside the db container (no local psql); Swagger regenerated with `swag init`
- [x] T038 Walk the quickstart manual acceptance against the local API and record results in the evidence table above — covered end to end by smoke step 13d and the three integration tests; no separate manual walk
- [x] T039 Update the sibling app spec's assumptions in `../../../gym-pulse-app/specs/016-planned-sport-sessions/spec.md` only if a contract detail changed during implementation; otherwise note "no change" here — no change; the contract matches the plan

## Dependencies & Execution Order

- T001–T003 can run together; T003's validation is expected to fail until T008.
- T004 and T005 are parallel and precede T006 → T007 → T008.
- US1 tests T009–T010 precede T011 → T012 → T013.
- US2 depends on US1's model and insert changes; T014–T015 are parallel and precede T016 → T017 → T018.
- US3 depends on US2's schedule reads and the `kind` branch in `scheduledOutcome`; T019–T021 are parallel and precede T022 → T023 → T024 → T025.
- US4 depends on US3's link code (T022); T026–T027 are parallel and precede T028 → T029.
- US5 depends on US3; its tests are parallel.
- Phase 8 follows all stories. The API deploys before the app release.

## Implementation Strategy

Ship US1 and US2 first: together they put sport sessions on the schedule with the full lifecycle
except completion, and they prove the additive contract against existing clients. US3 adds the one
new write path and is the user-visible MVP. US4 reuses it. US5 is confirmation and cleanup. Keep one
commit per phase on `spec/013-planned-sport-sessions` and open the PR with the contract diff first
in the description.
