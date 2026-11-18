# Implementation Plan: Weekly heaviest set per exercise

**Feature ID**: `gympulse-progress-payoff` | **Spec**: [spec.md](spec.md) | **Date**: 2026-11-18

## Summary

One additive read route, `GET /api/v1/exercises/progression?ids=&weeks=`, answered by one query. The query reads the performed sets `/exercises/records` reads, limited to the window, and keeps the heaviest set per exercise and Monday-start week. No migration, no write. Contract 1.15.0.

## Workflow and Specialist Scope

API engineering, Google Go style, and the PostgreSQL practices apply. Cross-repository delivery applies: the app sibling is spec 042, API first. One primary agent; no delegation.

## Design

- `internal/model/set_log.go`: `ExerciseProgression` and `ProgressionWeek`.
- `internal/dao/log_dao.go`: `WeeklyHeaviestSets(ctx, userID, exerciseIDs, since)`. The inner select is the `RecordSets` source (day logs, plus sessions with `status <> 'discarded'`; `completed`, weight not null, reps > 0) with `date >= since` and `user_id = $1` in both branches. `DISTINCT ON (exercise_id, week_start) ... ORDER BY exercise_id, week_start, actual_weight DESC, actual_reps DESC, date ASC` does the reduce, so the tie-breaks live in one place. `week_start` is `date_trunc('week', date)`, the bucketing `GetWeeklyVolume` uses.
- `internal/service/log_svc.go`: `ExerciseProgression` validates `ids` with `parseExerciseIDs` and `weeks` (default 12, 1 to 52, else 422 field `weeks`), returns `[]` for empty ids without a query, and puts the rows in requested order. A repeated id is answered once.
- Week boundary, mirrored from `GetVolume`: `since = model.MondayOfWeek(time.Now()) - 7 × (weeks-1)` days. The current week is decided on the server clock (UTC in production), not the athlete's timezone. Set dates are the calendar dates stored on the log or session.
- One deliberate difference from `/stats/volume`: that route caps `weeks` above 52; this one refuses it, as FR-003 says.
- Handler `LogHandler.ExerciseProgression` with godoc; route registered next to `/exercises/records`.

## Risks

- The window read uses `idx_logs_user_date` and `idx_workout_sessions_user_date`, then `idx_setlogs_log` or the session set index: at most 52 weeks of one athlete's sets. No new index.
- A server-clock week can differ from the athlete's week for a few hours around Monday. `/stats/volume` has the same behavior; changing it is out of scope.
- An app on a server older than 1.15.0 gets 404 without the JSON envelope and must treat the trend as not available.

## Verification

`go build ./...`, `go vet ./...`, `go test ./...`, `golangci-lint run`, service coverage gate, swagger regenerated, the integration suite (`internal/dao/exercise_progression_integration_test.go`), and `scripts/smoke.sh` step 8b.
