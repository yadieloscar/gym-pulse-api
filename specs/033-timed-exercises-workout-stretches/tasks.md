# Tasks: Timed exercises and chosen stretching routines in saved workouts

**Feature ID**: `gympulse-workout-stretches` | **Spec**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md)

## Phase 1: Schema

- [X] T001 Migration 049 up and down: three columns, three named checks, additive.
- [X] T002 `docs/DATABASE_DESIGN.md`: both tables, the inventory count (480 columns), the history row.

## Phase 2: Timed exercises (US1)

- [X] T003 `Exercise.DurationSeconds`, `CreateExerciseRequest.DurationSeconds`.
- [X] T004 `validateExerciseShape` accepts the timed shape and refuses the mixed ones; `toExercises` copies the field.
- [X] T005 `templateDAO` select and batched insert.
- [X] T006 `logDAO.getTemplate` carries `duration_seconds`.

## Phase 3: Chosen routines (US2)

- [X] T007 `WorkoutTemplate` routine fields; `OptionalText` on the request.
- [X] T008 Service: slug and length check per field; `Update` keeps an absent key.
- [X] T009 `templateDAO` get, insert, update.

## Phase 4: Nothing lost (US3)

- [X] T010 Data export: template routine fields and exercise `duration_seconds`.
- [X] T011 Legacy weekly-plan adoption maps a timed exercise to a timed strength program exercise.

## Phase 5: Contract and gates

- [X] T012 `docs/CONTRACTS.md`: Templates (shape rule, routine fields, PUT rule), Logs wording, export fields; contract 1.13.0.
- [X] T013 Swagger regenerated with swag v1.16.4.
- [X] T014 Tests: `template_svc_shape_test.go`, `model/template_test.go`, `goal_based_training_acceptance_test.go`, `dao/template_timed_routines_integration_test.go`.

## Evidence

- `go test ./...`: pass. `golangci-lint run`: 0 issues. `./scripts/validate-database-design.sh`: pass (44 tables, 480 columns).
- Integration suite, migration round trip, and smoke: run by CI on the pull request. Local Docker was unresponsive on 2026-11-17, so they were not run locally.

### Consistency analysis (2026-11-17, before the pull request)

| # | Finding | Resolution |
| --- | --- | --- |
| A1 | The owner was told programs "already support timed holds". Research showed the schema does and nothing uses it; the Knee Kit hold is a name with one rep. | Recorded under Clarifications; no program content changes; the app sibling owns the player work. |
| A2 | No requirement lacks a task; no task lacks a requirement. | None needed. |
