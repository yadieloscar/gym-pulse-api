# Implementation Plan: Mobility Sessions (recorded stretching)

**Branch**: `spec/018-mobility-sessions` | **Date**: 2026-11-03 | **Spec**: [spec.md](spec.md)

**Feature ID**: `gympulse-stretching-routines` | **Sibling**: `gym-pulse-app/specs/026-stretching-routines`

## Summary

Add an owned, write-once `mobility_sessions` resource with create (idempotent), range list, get, and
delete routes. Nothing else in the API reads it, so stats, participation, records, history, volume,
plan, schedule, sport activities, and health exports are unchanged. Additive contract 1.5.0; new
migration 033 (032 is claimed by open PR #40; renumber at merge if needed).

## Workflow and Specialist Scope

New non-trivial capability, full cycle on an owner-approved spec. `gym-pulse-api-engineering` and
`google-go-style` govern design, code, and review; `supabase-postgres-best-practices` informs the
table, constraint, and index choices. The app sibling (spec 026) consumes the contract; the API ships
and deploys first.

## Technical Context

**Language/Version**: Go 1.26, chi, pgx/pgxpool | **Storage**: PostgreSQL (Supabase), golang-migrate

**Testing**: `go test ./...`; `-tags=integration` DAO suite against a disposable migrated database;
`scripts/smoke.sh`; `scripts/validate-database-design.sh`; `scripts/validate-supabase-security.sh`

**Project Type**: web service | **Scale**: at most a few entries per athlete per day

## Constitution Check

| Principle | Status |
|---|---|
| I. Contract first | `docs/CONTRACTS.md` gains a Mobility sessions section (contract 1.5.0) in the same change. Collections return `[]`. |
| II. Owned authenticated data | Every query filters `user_id` from the JWT subject; foreign and missing ids share `404 NOT_FOUND`. |
| III. Idempotent evolution | `Idempotency-Key` must equal `operation_key`; replay returns 200 with the original; a changed body is `409 IDEMPOTENCY_CONFLICT`. Forward-only additive migration; down drops only the new table. |
| IV. Executable evidence | Service, handler, router, migration-contract, isolation, and PostgreSQL integration tests; smoke step. |
| V. Simple Go boundaries | Handler → service → DAO; no new dependency, abstraction, or concurrency. |

Post-design re-check: pass, no exceptions.

## Design Decisions

- **Own table, not sport activities or workout sessions** (spec clarifications). Rejected: a
  `sport_id = 'stretching'` sport activity, because sports credit participation and fulfil sessions.
- **Idempotency on the row, not the shared ledger.** The table stores `operation_key` and a
  `request_hash` with `UNIQUE (user_id, operation_key)` (FR-004). Create is
  `INSERT … ON CONFLICT (user_id, operation_key) DO NOTHING`, then reads the existing row and compares
  hashes: equal → 200 replay, different → `409 IDEMPOTENCY_CONFLICT`. Concurrent duplicates resolve
  through the unique index without an advisory lock. Rejected: `idempotency_records`, which would keep
  replaying a deleted entry and adds a second write per create. A create replayed after its entry was
  deleted creates a new entry; the app only retries pending creates, so this is acceptable and
  documented.
- **Hash covers the request as sent** (trimmed, `date` before defaulting), so a retry with an omitted
  date that crosses local midnight still replays instead of conflicting.
- **Date default and bound** use the training-profile timezone exactly as sport activities do; an
  athlete without a training profile receives the shared 404 on create (same as sports).
- **Validation**: `routine_id` uses the existing sport slug pattern `^[a-z0-9]+(-[a-z0-9]+)*$`, 1–64;
  `routine_name` trimmed 1–80 characters; `context` enum; `duration_seconds` 1–7200; `moves_completed`
  1–100; `moves_total` 1–100 and ≥ `moves_completed`; `content_version` trimmed 1–32; `operation_key`
  trimmed 1–128. Database CHECKs mirror each bound. The range read reuses the sport range rule
  (inclusive, ≤366 days).
- **Response** includes `operation_key` so the app can reconcile a pending local entry; it omits
  `request_hash` and `user_id`.
- **No reads elsewhere** (FR-005): a static test fails if any non-mobility DAO source names
  `mobility_sessions`, and an integration test compares stats and participation with and without
  entries.

## Data Model

`mobility_sessions(id uuid PK, user_id uuid FK auth.users ON DELETE CASCADE, date date, routine_id,
routine_name, context, duration_seconds, moves_completed, moves_total, content_version, operation_key,
request_hash, created_at)`; `ux_mobility_sessions_user_operation(user_id, operation_key)` unique;
`idx_mobility_sessions_user_date(user_id, date DESC, created_at DESC, id DESC)` serves the list order.
RLS enabled (not forced), Data API roles revoked, `gym_pulse_api` grant and permissive policy as in
030/031. Account deletion deletes the rows explicitly in `AccountDAO` and through the FK cascade.

## Migration and Rollout

Migration 033 creates one new table; no existing table or data changes, so mixed-version deploys are
safe and the down migration drops only `mobility_sessions`. Deploy the API before the app version that
records stretching. Recheck the migration number against `origin/main` before merge.

## Project Structure

```text
migrations/033_create_mobility_sessions.{up,down}.sql
internal/model/mobility_session.go (+ acceptance test)
internal/dao/mobility_session_dao.go (+ static test, integration test), account_dao.go
internal/service/mobility_session_svc.go (+ test)
internal/handler/mobility_session.go (+ test)
internal/router/router.go, cmd/server/main.go
docs/CONTRACTS.md, docs/DATABASE_DESIGN.md, scripts/smoke.sh
```
