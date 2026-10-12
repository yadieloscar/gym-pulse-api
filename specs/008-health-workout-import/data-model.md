# Data Model: Imported health workouts API

## `sport_activities` (altered by migration 021)

| Field | Type | Rules |
|---|---|---|
| `source` | TEXT | Not null, default `'manual'`, `CHECK (source IN ('manual', 'apple_health'))` |
| `external_id` | TEXT nullable | 1–128 when present; `CHECK ((source = 'manual') = (external_id IS NULL))` |
| `distance_meters` | INTEGER nullable | 1–1,000,000 when present |
| `energy_kcal` | INTEGER nullable | 1–20,000 when present |
| `avg_heart_rate_bpm` | SMALLINT nullable | 30–250 when present |

Existing columns are unchanged. Existing rows take `source = 'manual'` through the default and
need no backfill.

Index `ux_sport_activities_user_source_external` is `UNIQUE (user_id, source, external_id)
WHERE external_id IS NOT NULL`. The existing `idx_sport_activities_user_date` still serves listing.

RLS stays enabled with no direct-client policies; Data API role privileges stay revoked. No grant
changes are needed because the table already exists.

## Create transaction (DAO)

1. Lock the athlete's sport domain (existing advisory lock).
2. Replay from `idempotency_records` under `sport-activities/create` (existing).
3. **New**: if `external_id` is set, select the owned row by `(user_id, source, external_id)`.
   If found, return it with `created = false` and commit nothing.
4. Insert the activity with the new columns.
5. Upsert `day_participation` (existing).
6. Insert the idempotency record with status 201 (existing).

The handler writes `201` when `created` is true and `200` otherwise.

## API model

```json
{
  "id": "uuid",
  "date": "2026-10-11",
  "sport_id": "running",
  "sport_name": "Running",
  "duration_minutes": 42,
  "notes": null,
  "source": "apple_health",
  "external_id": "9B3C2E1A-6F0D-4A51-9C7E-2B1F0D8E4A77",
  "distance_meters": 8120,
  "energy_kcal": 512,
  "created_at": "2026-10-11T13:05:00Z",
  "updated_at": "2026-10-11T13:05:00Z"
}
```

`source` is always present. `external_id`, `distance_meters`, `energy_kcal`, and
`avg_heart_rate_bpm` are omitted when null. The API never returns `user_id`, operation keys, or
request hashes.
