# Implementation Plan: Export My Data

**Branch**: `spec/025-data-export` | **Spec**: [spec.md](spec.md) | **Feature ID**: `gympulse-data-export`

## Design

- **Transport** (`internal/handler/export.go`): `GET /api/v1/export` writes `model.DataExport` as
  JSON; `GET /api/v1/export/workout-sets` renders the CSV into a buffer first so a failure still
  answers the standard JSON error. Both set `Content-Disposition: attachment` with a dated name.
- **Router**: both routes share `RateLimitPerUser(ExportLimit)` (6/min) inside the existing
  authenticated, active-user group.
- **Service** (`internal/service/export_svc.go`): stamps `exported_at` (UTC, whole seconds), merges
  sessions and legacy logs into one oldest-first history, and owns the CSV rendering, including
  formula neutralization (CWE-1236).
- **DAO** (`internal/dao/export_dao.go`): one `BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY`
  transaction; one query per section plus one per child table (sets, overrides, template exercises),
  grouped in Go. No N+1. Every query binds the user id; child tables join to an owned parent.
- **Model** (`internal/model/export.go`): dedicated export types so the export shape is stable and
  independent of endpoint response types. `schema_version` 1.

## Ownership and privilege

The runtime role `gym_pulse_api` already has `SELECT` on every read table and one permissive policy
per table; ownership is enforced by `WHERE user_id = $1` as in every other DAO. An integration test
runs the export after `SET ROLE gym_pulse_api` to prove no new grant is needed.

## Persistence

No migration. Reads use existing indexes (`user_id`/`user_id, date` on every root table and parent
indexes on child tables). An athlete's lifetime history is bounded (thousands of sets), so the
response is built in memory; streaming is a later optimization if it ever matters.

## Compatibility and rollout

Additive (contract 1.4.0). Deploy the API before the app release that shows the Export action. Old
apps ignore the new routes. Rollback is removing the routes; nothing is stored.

## Verification

- Unit: service ordering, user scoping, UTC stamp, error wrapping; CSV header, rows, escaping, empty
  workout, rest skipping, unit only on weighted rows.
- Handler: JSON headers and top-level keys, `[]` collections, CSV headers, error path without
  attachment or leaked cause.
- Router: routes registered and share the export limit.
- DAO static: read-only snapshot, ownership predicates, no writes, no pool queries.
- DAO integration (`-tags=integration`): two-athlete isolation, discarded sessions excluded, empty
  athlete, runtime role.
