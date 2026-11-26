# Implementation Plan: Program recap

**Feature ID**: `gympulse-program-recap` | **Spec**: [spec.md](spec.md)

## Constitution check

I contract 1.18.0 in `docs/CONTRACTS.md` in the same change; II every query in the batch is scoped
by the authenticated `user_id`, and a foreign id is the same 404 as a missing one; III read-only and
additive, no migration; IV tests named in the spec's acceptance table, service coverage stays above
90%; V handler → service → DAO, no new dependency, no concurrency.

## Design assessment

| Area | Decision |
| --- | --- |
| Contract | `GET /api/v1/programs/{id}/recap` → 200 `ProgramRecap`. 400 `BAD_REQUEST` for a malformed id (as `GET /programs/{id}`), 404 `NOT_FOUND` for a missing or foreign program. |
| Identity | `middleware.MustGetUserID`. The program row is read with `id=$2 AND user_id=$1`; the other four queries filter by `user_id` and by that `program_id`, so a foreign id reads nothing. |
| Placement | DAO `ProgramRecapDAO.Read` returns raw rows (`dao.ProgramRecapRead`). The service picks "today" in the athlete's timezone and calls the pure `buildProgramRecap`. The handler only parses the id. |
| Round trips | One `pgx.Batch` of five queries: program header with its training weekday count and the profile timezone; the program's session dates and statuses; `participated` dates inside the session span; weeks off inside the span; the heaviest set per lift and day. |
| Queries | Sessions by `(user_id, program_id)`: `idx_scheduled_workouts_user_date` narrows to the athlete, whose rows are a few hundred. Participation by `(user_id, date)` unique key. Sets join `workout_sessions` → `scheduled_workouts` by key. No new index. |
| Compatibility | Additive. No stored row changes. Old clients never call the route. |

Simpler alternative rejected: having the app compute the recap from `/schedule`, `/participation`,
`/weeks-off`, and `/workout-sessions`. That is four ranged reads of up to a year each, and the
schedule read fills and finalizes as a side effect.

Alternative rejected: one SQL statement that returns JSON. The week rules (edge weeks, weeks off,
the unfinished week) are easier to read and to test as a Go function over plain rows.

## Files

`internal/model/program_recap.go`, `internal/dao/program_recap_dao.go`,
`internal/service/program_recap_svc.go`, `internal/handler/program_recap.go`,
`internal/router/router.go`, `cmd/server/main.go`, tests beside each, `docs/CONTRACTS.md`,
generated swagger, `scripts/smoke.sh` step 8e, `bruno/`.

## Verification

`go test ./...`, `golangci-lint run`, the tagged integration suite against a scratch PostgreSQL 16,
and the `smoke` job in CI.
