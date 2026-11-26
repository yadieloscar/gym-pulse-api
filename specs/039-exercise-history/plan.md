# Implementation Plan: Exercise history

**Feature ID**: `gympulse-exercise-history` | **Spec**: [spec.md](spec.md)

## Constitution check

I contract 1.18.0 in `docs/CONTRACTS.md` in the same change; II every query is scoped by the
authenticated `user_id`; III no migration, no write, so no idempotency or revision; IV tests named
in the spec's acceptance table; V handler → service → DAO, no new dependency, no concurrency.

## Design assessment

| Area | Decision |
| --- | --- |
| Contract | `GET /exercises/sessions?exercise_id&catalog_id&name&before&limit` → 200 page. 422 `VALIDATION_ERROR` naming `exercise_id`, `catalog_id`, `name`, `before`, or `limit`. |
| Identity | `middleware.MustGetUserID`. Both set sources are joined to their owner (`day_logs.user_id`, `workout_sessions.user_id`), so a foreign id matches nothing and is indistinguishable from an unknown one. |
| Placement | Query parsing and limits in `logService.ExerciseSessions`; SQL in `logDAO.ExerciseSessions`; the handler only passes query values. It sits beside the three existing per-exercise reads. |
| Query | One `UNION ALL` over the two set sources, the shape `RecordSets` already uses, with `LEFT JOIN exercises` and `LEFT JOIN scheduled_sets` for the catalog match. Page: distinct dates before the cursor, `LIMIT limit + 1`; the extra day only answers "is there another page". Heaviest: the same source, ordered and limited to one row. |
| Round trips | Both statements go in one `pgx.Batch`, so the route costs one database round trip. The API and database are in different regions. |
| Transaction | None. Two read statements in one batch; a set logged between them can at worst show a heaviest set one request early. |
| Schema | None. The driving filters are `day_logs(user_id)` and `workout_sessions(user_id)`, as for records. A name match cannot use `idx_setlogs_exercise`, but it runs inside one athlete's rows. |
| Compatibility | Additive. Older clients never call the route. |

Simpler alternative rejected: let the app page through `GET /logs/{date}`. It costs a request per
trained day, cannot see planned workouts, and cannot know the all-time heaviest set without reading
everything.

## Files

`internal/model/set_log.go`, `internal/dao/log_dao.go`, `internal/service/log_svc.go`,
`internal/handler/logs.go`, `internal/router/router.go`, mocks and tests beside each,
`internal/dao/exercise_sessions_integration_test.go`, `docs/CONTRACTS.md`, generated swagger,
`scripts/smoke.sh` step 8b2.

## Verification

`go test ./...`, `golangci-lint run`, the tagged integration suite against a scratch PostgreSQL 16,
`swag init`, and the `smoke` job in CI. No migration, so the database-design and Supabase security
validators have nothing new to check.
