# Implementation Plan: A planned week off

**Feature ID**: `gympulse-planned-week-off` | **Spec**: [spec.md](spec.md)

## Constitution check

I contract 1.17.0 in `docs/CONTRACTS.md` in the same change; II every query is scoped by the
authenticated `user_id`; III additive migration 051, idempotent by natural key (recorded as a
decision, since the routes carry no operation key); IV tests named in the spec's acceptance table,
service coverage stays above 90%; V handler → service → DAO, no new dependency, no concurrency.

## Design assessment

| Area | Decision |
| --- | --- |
| Contract | `GET /weeks-off`, `PUT /weeks-off/{week_start}` (200), `DELETE /weeks-off/{week_start}` (204). 422 `VALIDATION_ERROR` for the range and for `week_start`. |
| Identity | `middleware.MustGetUserID`; the table's key starts with `user_id`. There is no id to guess, so there is no foreign-resource case. |
| Invariants | Monday: database check and service. Current week and 8-week limit: service, because they need the athlete's timezone. |
| Transaction | `Put` takes the `schedule:` advisory lock, upserts the week, and skips the week's unresolved misses in one transaction. The lock is the one `FinalizeScheduledWorkout` takes, so a finalize and a `Put` for the same week cannot both miss each other. |
| Finalize | `FinalizeScheduledWorkout` checks `weeks_off` for the workout's week only when the outcome is missed with nothing performed. Then it sets the resolution in the same `UPDATE` and skips the participation upsert. |
| Schema | `weeks_off (user_id, week_start, created_at)`, PK `(user_id, week_start)`, Monday check, cascade from `auth.users`, `secure_table`. No secondary index. |
| Compatibility | Additive. Old clients never call the routes; rows exist only after an athlete takes a week off. |

Simpler alternative rejected: a `weeks_off DATE[]` column on `training_profiles`. It would put the
writes behind the profile's revision and idempotency rules and would not exist for an athlete
without a profile.

Legacy path: `finalizeLegacy` runs only when the service has no mutation DAO, which production
never does. It is unchanged.

## Files

`migrations/051_weeks_off.{up,down}.sql`, `internal/model/week_off.go`,
`internal/dao/week_off_dao.go`, `internal/dao/training_mutation_dao.go`,
`internal/dao/account_dao.go`, `internal/service/week_off_svc.go`, `internal/handler/week_off.go`,
`internal/router/router.go`, `cmd/server/main.go`, tests beside each, `docs/CONTRACTS.md`,
`docs/DATABASE_DESIGN.md`, generated swagger, `scripts/smoke.sh` step 8d.

## Verification

`go test ./...`, `golangci-lint run`, `./scripts/validate-database-design.sh`, the tagged
integration suite and the migration round trip against a scratch PostgreSQL 16,
`validate-supabase-security.sh` against the same database, and the `smoke` job in CI.
