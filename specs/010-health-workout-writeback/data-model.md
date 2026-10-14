# Data Model: Remember a workout's health export

## `health_exports` (migration 025)

| Field | Type | Rules |
|---|---|---|
| `id` | UUID | Primary key, application-generated |
| `user_id` | UUID | Not null, `auth.users(id)` cascade delete |
| `workout_kind` | TEXT | `CHECK (workout_kind IN ('scheduled_workout', 'day_log'))` |
| `workout_id` | UUID | Not null; the finalized planned workout or off-plan log |
| `source` | TEXT | `CHECK (source IN ('apple_health'))` |
| `external_id` | TEXT | 1–128; the health store's identity for the exported workout |
| `exported_at` | TIMESTAMPTZ | Not null; when the device wrote it |
| `created_at` | TIMESTAMPTZ | Server generated |

Unique index `ux_health_exports_workout(user_id, workout_kind, workout_id, source)` makes the
identity write-once. Index `idx_health_exports_user_exported(user_id, exported_at DESC, id DESC)`
serves the range list. RLS enabled, no direct-client policies, Data API role privileges revoked.

No foreign key to the workout tables: the two kinds live in different tables, ownership and finished
state are checked in the transaction, and account deletion removes rows by `user_id`.

## Record transaction (DAO)

1. Lock the athlete's export domain (`health-export:` advisory namespace).
2. Replay from `idempotency_records` under `health-exports/record`.
3. Resolve the workout: `scheduled_workouts` row with `user_id`, `finalized_at IS NOT NULL`, status
   in (`completed`, `incomplete`), or `day_logs` row with `user_id` and `type_id <> 'rest'`.
   Absent → `NotFoundError`. Carry the workout's `date` for the response.
4. Select the existing export by `(user_id, workout_kind, workout_id, source)`:
   same `external_id` → return it, outcome `existing`; different → `ConflictError` ("health export
   identity already recorded") with the stored export as `Authoritative`.
5. Insert; write the idempotency record with status 201; commit.

The handler writes `201` for `created` and `200` for `existing`.

## API model

```json
{
  "id": "uuid",
  "workout_kind": "scheduled_workout",
  "workout_id": "uuid",
  "date": "2026-10-13",
  "source": "apple_health",
  "external_id": "B5D7...-...",
  "exported_at": "2026-10-13T18:42:00Z",
  "created_at": "2026-10-13T18:42:01Z"
}
```
