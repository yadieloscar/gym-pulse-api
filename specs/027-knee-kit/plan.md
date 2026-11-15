# Implementation Plan: Knee Kit (daily knee checklist and pain log)

**Branch**: `spec/027-knee-kit` | **Date**: 2026-11-14 | **Spec**: [spec.md](spec.md)

**Feature ID**: `gympulse-knee-kit` | **Sibling**: `gym-pulse-app/specs/031-knee-kit`

## Summary

Add an owned, revisioned `knee_kit_days` resource: one row per athlete and local date, written by
an idempotent whole-record `PUT` and read by a bounded range `GET`. The API stores facts and
computes nothing from them. Nothing else in the API reads the table except account deletion and an
additive `knee_kit_days` data-export section. Additive contract 1.7.0; new migration 037 (035 and
036 are claimed by other open branches).

## Workflow and Specialist Scope

New non-trivial capability, full cycle on an owner-approved spec. `gym-pulse-api-engineering` and
`google-go-style` govern design, code, and review; `supabase-postgres-best-practices` informs the
table, constraint, and index choices. The app sibling (spec 031) consumes the contract; the API
ships and deploys first.

## Technical Context

**Language/Version**: Go 1.26, chi, pgx/pgxpool | **Storage**: PostgreSQL (Supabase), golang-migrate

**Testing**: `go test ./...`; `-tags=integration` DAO suite against a disposable migrated database;
`scripts/smoke.sh`; `scripts/validate-database-design.sh`; `scripts/validate-supabase-security.sh`

**Project Type**: web service | **Scale**: at most one row per athlete per day

## Constitution Check

| Principle | Status |
|---|---|
| I. Contract first | `docs/CONTRACTS.md` gains a Knee Kit days section and the export field (contract 1.7.0) in the same change. Collections return `[]`. |
| II. Owned authenticated data | Every query filters `user_id` from the JWT subject; there is no id-addressed route, so no foreign-id case exists. |
| III. Idempotent evolution | `Idempotency-Key` must equal `operation_key`; an identical retry replays; a changed payload is `409 IDEMPOTENCY_CONFLICT`; a stale revision is `409 REVISION_CONFLICT`. Forward-only additive migration; down drops only the new table. |
| IV. Executable evidence | Model, service, handler, router, migration-contract, isolation, and PostgreSQL integration tests; smoke step. |
| V. Simple Go boundaries | Handler → service → DAO; no new dependency, abstraction, or concurrency. |

Post-design re-check: pass, no exceptions.

## Design Decisions

- **Own table, mirroring nutrition days** (spec 012): one row per `(user_id, date)`, whole-record
  `PUT` with `expected_revision`, range `GET`. Rejected: columns on an existing per-day table,
  because every existing per-day table feeds a derived read model.
- **Idempotency through the shared ledger**, as nutrition days: the save takes a
  `knee-kit:<user>` advisory lock, checks `idempotency_records` (scope `knee-kit-days/put`), locks
  the row, checks the revision, writes, and records the response in one transaction. The ledger
  check comes first, so a retry replays its original result even after the day changed. The lock
  serializes two first saves for the same date, so one creates and the other gets a revision
  conflict instead of a unique-index error.
- **`completed_items` is a set, stored sorted.** The service sorts a copy before hashing and
  storing, so the same ticks in a different order are the same payload and the response is
  deterministic. Stored as `text[]`. Rejected: request order, which would make a reordered retry an
  idempotency conflict for no benefit.
- **No delete-by-empty** (spec clarification): unlike nutrition days there is no `has_value`
  CHECK and no 204 branch; an empty day is a row.
- **`expected_revision` is required.** An absent value is 422 rather than being read as 0, so a
  client bug cannot silently assert "no record exists".
- **Date bound and profile rule** reuse `localTodayFromProfile` and the nutrition date validator:
  the athlete's training-profile timezone decides "today"; no profile is the shared 404 on `PUT`.
  The range read reuses the sport range rule (inclusive, ≤ 366 days) and needs no profile.
- **Validation** lives in the service with `details.field`; database CHECKs mirror each bound
  (pain 0–10, at most 12 items, slug shape and length on the comma-joined array, no null or empty
  item, `revision > 0`). Duplicate items are service-only because a CHECK cannot compare array
  elements.
- **No route-specific rate limit**, matching the comparable nutrition-day and mobility routes. The
  body is bounded by the 1 MiB limit and 12 items, and each date holds one row.
- **No reads elsewhere** (FR-005): a static test fails if any DAO source other than the Knee Kit
  DAO, account deletion, and the export names `knee_kit_days`; an integration test compares stats
  and participation with and without days.
- **Export**: an additive top-level `knee_kit_days` array read in the existing snapshot
  transaction; `schema_version` stays 1.
- **Exercise catalog entries are not added.** The requested Decline Squat, Spanish Squat Hold, and
  Wall Sit would need honest metadata (`squat` or `isolation`, `quads`). The engine selects by
  `pattern`, `sub_pattern`, and `isolationOf(muscle_group)`, sizes each candidate list by count,
  and has no exclusion field, so any such row changes existing drafts and lets isometric holds be
  picked; the provider-backed generator also receives the whole catalog. Keeping them out would
  require an engine change, which is out of scope. The checklist items stay app-bundled ids.

## Data Model

`knee_kit_days(id uuid PK, user_id uuid FK auth.users ON DELETE CASCADE, date date,
completed_items text[] NOT NULL DEFAULT '{}', morning_pain smallint, activity_pain smallint,
sharp_pain boolean NOT NULL DEFAULT false, limping boolean NOT NULL DEFAULT false,
revision bigint NOT NULL DEFAULT 1, created_at, updated_at)`;
`ux_knee_kit_days_user_date(user_id, date)` unique is the identity and serves the oldest-first
range read. RLS enabled (not forced), Data API roles revoked, `gym_pulse_api` grant and permissive
policy as in 033. Account deletion deletes the rows explicitly in `AccountDAO` and through the FK
cascade.

## Migration and Rollout

Migration 037 creates one new table; no existing table or data changes, so mixed-version deploys
are safe and the down migration drops only `knee_kit_days`. golang-migrate (file source, `Up`)
applies ascending versions and tolerates the 035–036 gap; the repository's static tests glob
`migrations/*.up.sql` and do not require contiguous numbers. It does not go back to a lower
version once 037 is applied, so 035 and 036 must reach a database before this branch does, or
renumber past 037. Deploy the API before the app version that records Knee Kit days.

## Project Structure

```text
migrations/037_create_knee_kit_days.{up,down}.sql
internal/model/knee_kit.go (+ acceptance test), export.go
internal/dao/knee_kit_dao.go (+ static test, integration test), account_dao.go, export_dao.go
internal/service/knee_kit_svc.go (+ test)
internal/handler/knee_kit.go (+ test)
internal/router/router.go, cmd/server/main.go
docs/CONTRACTS.md, docs/DATABASE_DESIGN.md, docs/{docs.go,swagger.json,swagger.yaml}, scripts/smoke.sh
```
