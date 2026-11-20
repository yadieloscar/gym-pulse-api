# Feature Specification: Fewer database round trips behind Today

**Feature Branch**: `perf/today-round-trips`

**Feature ID**: `gympulse-today-round-trips` (API only; the app is unchanged)

**Created**: 2026-11-20

**Status**: Approved 2026-11-20 (owner request; no product choice open)

**Input**: Owner, 2026-11-20: the app feels slow. The region move (API in US East, database in
us-west-1, about 65 ms per round trip) is left for later; cut round trips instead.

**Related**: the earlier round-trip audit is recorded in
[031 tasks, "Round-trip audit"](../031-one-active-program/tasks.md) (API PR #73). That audit cut
the per-session reads inside `GET /schedule` and named the identity check as a security rule
it left alone. This spec is new because it owns the participation read, the pool floor, and the
held decision on that identity check, none of which 031 covers.

## What the athlete notices

Opening Today is faster. Nothing else changes: every response keeps its shape, values, status
codes, and errors. `docs/CONTRACTS.md` does not change.

## Baseline (measured 2026-11-20, main `e2448f0`)

Method: Postgres 16 with `log_statement=all` in a scratch container, the API built from the
branch, a TCP proxy that delays each chunk 32.5 ms each way (about 65 ms per round trip), one
athlete with a training profile, a starter program (`POST /programs/from-starter`), and
`POST /schedule/materialize` over today−14 to today+21. Today's nine GETs are fired at once,
as the app does.

| Route | Statements | Time at 65 ms/trip |
| --- | --- | --- |
| `GET /schedule` (5 weeks) | 6, one after another | ~460 ms |
| `GET /participation` (this week) | 7, one after another | ~490 ms |
| each of the other 7 GETs | 2 or 3 | 140–220 ms |
| **All nine** | **28** | **Today ready ~650 ms warm, ~1.25 s after a restart** |

Why:

- `GET /participation` ran the whole schedule read (profile, fill watermark, workouts, sets,
  extra sets) only so past sessions were finalized, then read participation.
- `GET /schedule` ran its six reads one after another.
- Every authenticated request first ran `SELECT gym_pulse_private.auth_user_exists($1)` (9 of
  the 28 statements).
- The pool kept one connection (`DATABASE_MIN_CONNS=1`, idle 5 minutes), so a burst of nine
  opened new cross-region connections.

## Target

- `GET /participation` and `GET /schedule` each read their data in one round trip (after the
  identity check) when nothing needs filling or finalizing.
- Today ready in about half the baseline, warm and after a restart, at 65 ms per trip.

## Requirements

- **FR-001** `GET /participation` returns exactly what it returned before: the same days, and the
  same finalization of sessions dated before the athlete's today, with the same errors (an
  invalid range; no training profile while the range holds an unfinalized session; an invalid
  saved timezone). It no longer fills the schedule ahead or lists sessions with their sets.
- **FR-002** `GET /schedule` returns exactly what it returned before. It reads the profile, the
  active program's fill watermark, the range's sessions, and their sets in one round trip. It
  fills the schedule only when the watermark is short of the horizon (the same test the fill
  makes), and reads back only after a fill or a finalize.
- **FR-003** With `DATABASE_MIN_CONNS` unset, the pool keeps 10 connections open (one per request
  of Today's burst), or `DATABASE_MAX_CONNS` if that is lower. An explicit value still wins.

## Held: caching the identity check (needs the owner)

Every authenticated request still runs `auth_user_exists` (one round trip each, run in parallel
across Today's burst). A 30-second in-process cache of positive answers, forgotten on
`DELETE /api/v1/account`, was built and measured (Today warm ~157 ms instead of ~354 ms) but is
**not** in this change: `docs/CONTRACTS.md` ("A deleted identity is rejected even while its
previously issued access token remains unexpired") and `docs/RELEASE_RUNBOOK.md` ("delete them
in Supabase Auth; the next request is `401`") promise zero staleness for an identity deleted in
Supabase. Adopting it means the owner accepts up to 30 seconds of access after a dashboard
deletion and both documents change. The code is on branch `perf/active-user-cache`.

## Edge cases

- With an invalid saved timezone, `GET /participation` used to fail when the fill had just added
  a session to an otherwise finalized range; it now fails only when the range already holds an
  unfinalized session. A saved timezone is validated on save, so this is not reachable through
  the API.
- A session created between the batched statements can appear in the set read but not the
  workout read; its sets are skipped, as a separate later read would not have seen it.

## Non-goals

- Moving the API or the database to another region.
- The other seven Today GETs (one read each after the identity check; `/programs` two).
- `pgxpool` pinging a connection idle for more than one second before use (one trip after idle).
  It guards against dead connections and is kept.

## Success criteria

- **SC-001** Statement counts and Today ready times are re-measured by the baseline method and
  recorded in `tasks.md` and the PR.
- **SC-002** Unit, integration, lint, smoke, and security-validator gates pass.
- **SC-003** `docs/CONTRACTS.md` is unchanged.
