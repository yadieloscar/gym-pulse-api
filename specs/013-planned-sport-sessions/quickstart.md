# Quickstart: Planned Sport Sessions API

## Implementation order

1. Write migration 027 (up and down), update `docs/DATABASE_DESIGN.md` (thirteen column rows, one
   index, the FK matrix, the lock order `program:` → `schedule:` → `sport-activity:`, the
   transaction row, the 305-column inventory line), and run `./scripts/validate-database-design.sh`.
2. Extend the models: `ProgramWorkout.Kind`/`Sport`, `ScheduledWorkout.Kind`/`Sport`,
   `SportTarget`, `ScheduledSport`, `SportActivity.ScheduledWorkoutID`,
   `CreateSportActivityRequest.ScheduledWorkoutID`, `DistanceSports`, and the acceptance tests for
   migration text and JSON shape.
3. Extend `validateProgramShape` with the kind branch and table-driven tests (empty exercises per
   kind, sport bounds, distance-sport rule, custom "other" name).
4. Extend program insert/read (`insertProgramWorkouts`, `loadProgramWorkouts`, the tx-scoped
   `loadProgram`) and materialization (`MaterializeProgramForWeekdays`: no sets for sport,
   snapshot sport fields), then `insertScheduledWorkouts`, the schedule `List`/`Get` scans with the
   activity join, and the plan-transition and adopt-legacy inserts. Add tests to
   `training_domain_svc_test.go`.
5. Extend finalization: `scheduledOutcome` and `deriveScheduledStatus` kind branch; guards on
   complete, set check, target edit, extra set, and `PATCH required_sets` (422 field `kind`);
   recover and health-export filters. Add tests in `schedule_resolution_test.go`,
   `training_mutation_dao_test.go` (source text), and `health_export_dao_test.go`.
6. Extend the sport-activity service and DAO: lock order, explicit link, auto-match, session
   update, `SPORT_SESSION_CONFLICT` mapping in `response.go`, wiring in `cmd/server/main.go`.
   Update `sport_activity_dao_test.go` (marker order), `sport_activity_svc_test.go`,
   `sport_activity_test.go`, and the `NewSportActivityService` call sites in tests.
7. Update `docs/CONTRACTS.md` (programs, schedule, sport activities, error codes) and add the smoke
   step: create a program whose last unit is a sport session, materialize, record against it,
   replay (201, same id), record again with a new key (409 `SPORT_SESSION_CONFLICT`), list the
   schedule (status `completed`, `sport.sport_activity_id` set), and post an extra set to it (422).
8. Run `gofmt`, `go test ./...`, `golangci-lint run`, `./scripts/smoke-toggle.sh`,
   `./scripts/validate-database-design.sh`, and `./scripts/validate-supabase-security.sh` against a
   migrated local database.

## Tests identified before implementation

| Requirement | Evidence |
|---|---|
| FR-001, FR-002, FR-014 | Service table tests: sport unit accepted; exercises unit with no exercise rejected; sport unit with exercises rejected; duration, distance, notes, slug, custom-name bounds; distance on volleyball rejected (field `workouts`) |
| FR-003 | Model acceptance test: existing program JSON without `kind` decodes and re-encodes with `kind: exercises`; starter and legacy adoption units are `exercises` |
| FR-004, FR-005 | Materialization test: sport unit yields a planned row with snapshot, `required_sets: []`, provenance ids; lazy finalize marks a past unfulfilled sport row `missed`; skip/unskip tests reused on a sport row; regenerate deletes a planned sport row and retains a completed one |
| FR-006, FR-007, FR-011 | DAO source-text test for marker order (`schedule:` lock, link select, session update); handler test: record against planned session → 201, schedule shows `completed` and `sport_activity_id`; performed date participation upserted; later-date record keeps the scheduled date's missed participation |
| FR-008 | Handler test: second record → 409 `SPORT_SESSION_CONFLICT` with `resource`; DAO test: partial unique index text present in migration 027 |
| FR-009 | Handler tests: activity body carries `scheduled_workout_id`; schedule item carries `sport.sport_activity_id` and `performed_date` |
| FR-010 | DAO/service test: `apple_health` import on a date with one open cycling session links it; second import on the same date stays unlinked; replay of the first import (duplicate path) changes nothing; two open sessions → lowest `sequence_position` linked |
| FR-012 | Existing idempotency tests extended with the new field: same key same payload → 201 replay; same key changed `scheduled_workout_id` → 409 `IDEMPOTENCY_CONFLICT` |
| FR-013 | Handler test: foreign `scheduled_workout_id` → shared 404; foreign session never appears in another athlete's schedule |
| FR-015 | Source-text test: account deletion order unchanged and the `SET NULL` FK text present in migration 027 |
| FR-016 | Smoke step above plus `docs/CONTRACTS.md` diff reviewed with the code |
| SC-002 | Smoke: five replays yield one activity, one completed session, one participation row |
| SC-004 | Full `go test ./...` and the existing smoke steps 10 and 13 unchanged |

## Manual acceptance

Create a custom program with a strength workout at position 1 and a cycling session (90 min,
40 km) at position 2; confirm the program returns `kind` on both and `sport` on the second.
Materialize a week; confirm the cycling row has `kind: sport`, `status: planned`, and empty
`required_sets`. Post a sport activity with its `scheduled_workout_id`, 95 minutes, 42,000 m;
confirm 201, the activity's `scheduled_workout_id`, the session `completed` with
`sport.sport_activity_id` and `performed_date`, and participation for the date. Repeat with the
original key (201, same id) and with a new key (409 `SPORT_SESSION_CONFLICT`). Let a second sport
row's date pass; confirm `missed`, skip it, undo, then post an activity for it dated today; confirm
`completed`, `performed_date` today, and the original date still `participated: false`. Post an
`apple_health` cycling import for a date with an open cycling session and confirm the link.

## Compatibility and rollback

Additive with column defaults; no backfill. Deploy this API before releasing the app. Old apps
ignore `kind` and `sport`, read `exercises: []` and `required_sets: []` on sport rows without
error, and keep creating manual activities. To roll back, roll back the app first, replace or
delete programs that hold sport units, then run the down migration, which drops the index, the link
column, and the kind/sport columns.

## Residual risk carried from existing code

Materialization ignores `preferred_weekday`, restarts the sequence on every call, and regenerate
can duplicate retained dates. Sport sessions behave the same as workouts here. A separate
specification owns that fix.
