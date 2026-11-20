# Implementation Plan: Fewer database round trips behind Today

**Spec**: [spec.md](spec.md) · **Branch**: `perf/today-round-trips` · **Date**: 2026-11-20

## Summary

Send each read's statements as one `pgx.Batch` (one round trip), stop the participation read from
redoing the schedule read, and keep enough pool connections open for Today's burst. No contract,
schema, or migration change. The identity-check cache is held (spec, "Held").

## Design

### Batched reads (`internal/dao/schedule_read.go`)

- `scheduleDAO.ReadSchedule(from, to)` queues five statements: the training profile, the active
  program's raw `scheduled_through`, the range's workouts, their required sets, and their extra
  sets. The set reads select by `user_id` and date range instead of by the workout IDs, which is
  what lets them share the round trip; their SELECT and ORDER BY text is shared with `List`
  (`listRequiredSetsSelect`, `listExtraSetsSelect`). A set row whose workout is not in the
  workout read is skipped (each statement takes its own snapshot under READ COMMITTED).
- `participationDAO.ReadParticipation(from, to)` queues three: the profile, the range's
  unfinalized sessions (ID, date, revision only), and the stored participation days.
- Both are optional interfaces (`dao.ScheduleReader`, `dao.ParticipationReader`) reached by type
  assertion, as `RollingScheduleDAO` is; DAOs and test doubles without them keep the old path.
- `selectTrainingProfile`/`scanTrainingProfile` and `selectParticipation`/`scanParticipation` are
  shared with the single reads so the batch cannot drift from them.

### Schedule read (`scheduleService.List`)

1. `readSchedule` → one batch.
2. `needsFill`: rolling DAO, an active program, `to` ≥ the athlete's today, and the watermark empty
   or before today + `ScheduleHorizonDays`. This is the condition `EnsureFilled` tests in SQL,
   evaluated on the watermark the batch returned. Only then: `EnsureFilled` and a `List`.
3. `finalizePastSessions` (extracted unchanged from the old loop) finalizes sessions before today.
4. `List` again only after a finalize.

### Participation read (`participationService.List`)

It called `schedule.List` only for its finalize side effect. The fill never writes participation
rows (it inserts sessions from today on), so the read needs only the unfinalized sessions in the
range: `ReadParticipation`, `finalizePastSessions` on them, and a participation `List` only after a
finalize. The path applies when the schedule service finalizes through the mutation DAO (always in
the server); otherwise the old path runs.

### Identity cache (held, branch `perf/active-user-cache`)

`ActiveUserCache` would wrap the `AuthUserDAO` for `RequireActiveUser` only: positive answers kept
30 seconds, a generation counter so a lookup in flight across a `Forget` is not stored, and
`ForgetActiveUser` on `DELETE /api/v1/account`. Held because it changes documented behaviour.

### Pool (`internal/config/config.go`)

`DATABASE_MIN_CONNS` defaults to `min(10, DATABASE_MAX_CONNS)`. pgx keeps the floor open through
the idle timeout, so `DATABASE_MAX_CONN_IDLE_TIME` no longer matters below the floor and stays 5m.
Measured after the other fixes: a burst right after a restart took ~700 ms with a floor of 1 and
~545 ms with 10.

## Verification

Unit tests for every branch above; integration tests that compare each batch read with the
separate reads on PostgreSQL; the smoke suite and security validator on a scratch stack; the
baseline method re-run. See [tasks.md](tasks.md).

## Rollout

API only, no migration. After deploy the owner may remove `DATABASE_MIN_CONNS` from Railway if it
is set to 1, or set it to 10. Supabase connection limits: 10 per API instance plus the advisory
lock pool (at most 4); a deploy overlap briefly doubles that.
