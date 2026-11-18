# Implementation Plan: Swap or skip an exercise during a workout

**Feature ID**: `gympulse-in-workout-exercise-changes` | **Spec**: [spec.md](spec.md) | **Date**: 2026-11-17

## Summary

Two additive routes on a scheduled workout. Each locks the workout at the expected revision, splits the exercise's sets into performed and open, changes only the open ones, and stores the replay record in the same transaction. No migration: `scheduled_sets` already holds the exercise snapshot per set.

## Workflow and Specialist Scope

API engineering and the PostgreSQL practices skill apply. Cross-repository delivery applies: the app sibling is spec 040, API first. One primary agent; no delegation.

## Design

- `internal/dao/scheduled_exercise_dao.go`: `SwapScheduledExercise`, `SkipScheduledExercise`, the shared `beginExerciseChange` and `finishExerciseChange`, and `shiftExerciseOrders`, which passes rows through the parked range (`startedSetParking`) so `UNIQUE (scheduled_workout_id, exercise_order, set_index)` never collides inside one statement.
- `set_logs_required_extra_check` forbids deleting a `scheduled_sets` row that a required set log points at, which is why only open sets are deleted.
- `PATCH /api/v1/scheduled-workouts/{id}` was not reused: it deletes and reinserts every set, which a started workout cannot survive.
- Service: validation, sport refusal, payload hash. Handler: path parsing and the mutation envelope.

## Risks

- Order renumbering touches every later set of the workout; a workout has tens of rows, inside one transaction under the schedule lock.
- An app holding the earlier revision gets `REVISION_CONFLICT` on its next write and must reload, as with any other workout change.

## Verification

`go test ./...`, the integration suite (`internal/dao/scheduled_exercise_integration_test.go`), `golangci-lint run`, coverage gate, swagger regenerated.
