# Data Model: Planned Sport Sessions API

## `program_workouts` (altered by migration 027)

| Field | Type | Rules |
|---|---|---|
| `kind` | TEXT | Not null, default `'exercises'`, `CHECK (kind IN ('exercises', 'sport'))` |
| `sport_id` | TEXT nullable | Slug 1–64 (`^[a-z0-9]+(-[a-z0-9]+)*$`); `CHECK ((kind = 'sport') = (sport_id IS NOT NULL))` |
| `sport_name` | TEXT nullable | 1–80; `CHECK ((kind = 'sport') = (sport_name IS NOT NULL))` |
| `target_duration_minutes` | INTEGER nullable | 1–1,440; `CHECK ((kind = 'sport') = (target_duration_minutes IS NOT NULL))` |
| `target_distance_meters` | INTEGER nullable | 1–1,000,000; `CHECK (target_distance_meters IS NULL OR kind = 'sport')` |
| `sport_notes` | TEXT nullable | 1–2,000; `CHECK (sport_notes IS NULL OR kind = 'sport')` |

Existing columns and `UNIQUE (program_id, sequence_position)` are unchanged. Existing rows take
`kind = 'exercises'` through the default and need no backfill. Service validation adds: an
`exercises` unit has at least one exercise; a `sport` unit has none; `target_distance_meters` is
accepted only for `cycling`, `running`, `swimming`, and `other`; `sport_id = 'other'` requires a
custom `sport_name`.

## `scheduled_workouts` (altered by migration 027)

The same six columns with the same checks, snapshotted at materialization time. `status` keeps its
existing check; a sport row only ever holds `planned`, `completed`, or `missed`. `required_sets`
stays `[]` for a sport row because no `scheduled_sets` are inserted.

## `sport_activities` (altered by migration 027)

| Field | Type | Rules |
|---|---|---|
| `scheduled_workout_id` | UUID nullable | `REFERENCES scheduled_workouts(id) ON DELETE SET NULL` |

Partial unique index `ux_sport_activities_scheduled_workout (scheduled_workout_id) WHERE
scheduled_workout_id IS NOT NULL`: one activity per session. Existing indexes are unchanged; the
link lookup from the schedule side is by primary key.

No new table: RLS, policy, Data API revokes, and the `publicTables` count in
`supabase_security_migration_test.go` are unchanged. `docs/DATABASE_DESIGN.md` gains thirteen
column rows (292 → 305 supported columns), one index, one FK row, the new lock-order line, and the
transaction row below.

## Program unit (API)

```json
{
  "id": "<uuid>", "kind": "sport", "name": "Long ride",
  "preferred_weekday": 6, "sequence_position": 3,
  "sport": {
    "sport_id": "cycling", "sport_name": "Cycling",
    "target_duration_minutes": 90, "target_distance_meters": 40000, "notes": null
  },
  "exercises": []
}
```

`kind` is always present (`exercises` for every existing and starter unit). `sport` is present only
when `kind = 'sport'`. Create and replace requests accept the same shape; `kind` defaults to
`exercises` when omitted.

## Scheduled sport session (API)

```json
{
  "id": "<uuid>", "kind": "sport", "program_id": "<uuid>", "program_workout_id": "<uuid|null>",
  "date": "2026-10-31", "name": "Long ride", "sequence_position": 3,
  "status": "completed", "finalized_at": "<RFC3339>", "revision": 2,
  "sport": {
    "sport_id": "cycling", "sport_name": "Cycling",
    "target_duration_minutes": 90, "target_distance_meters": 40000, "notes": null,
    "sport_activity_id": "<uuid>", "performed_date": "2026-10-31"
  },
  "required_sets": [], "extra_sets": [],
  "created_at": "<RFC3339>", "updated_at": "<RFC3339>"
}
```

`sport_activity_id` and `performed_date` come from the `LEFT JOIN sport_activities` on
`scheduled_workout_id` and are omitted while the session is open. `resolution` and `resolved_at`
behave exactly as for workouts.

## Sport activity (API)

Unchanged shape plus `scheduled_workout_id` (`omitempty`), present when the activity fulfilled a
session, whether by explicit record or by import auto-match.

## State transitions (sport session)

| From | Event | To |
|---|---|---|
| (none) | materialize / regenerate / plan-transition apply | `planned` |
| `planned` | local date passes, lazy finalize, no linked activity | `missed` |
| `planned` or `missed` | activity recorded with `scheduled_workout_id`, or import auto-match | `completed` (resolution cleared) |
| `missed` | skip | `missed` + `skipped` |
| `missed` + `skipped` | unskip | `missed` |
| `completed` | record again | 409 `SPORT_SESSION_CONFLICT` |
| any | set, target, extra-set, complete, or `required_sets` patch | 422 `VALIDATION_ERROR` field `kind` |

Regenerate and plan-transition apply delete a `planned` sport row in range exactly as they delete
a planned workout; `completed` and `missed` rows are retained.

## Create transaction (sport-activity DAO, extended)

1. Lock `schedule:` then `sport-activity:` (new order; see `docs/DATABASE_DESIGN.md` §11).
2. Replay from `idempotency_records` under `sport-activities/create` (existing).
3. Import duplicate by `(user_id, source, external_id)` → return existing, write nothing (existing).
4. **New, explicit link**: when `scheduled_workout_id` is set, `SELECT ... FROM scheduled_workouts
   WHERE id = $1 AND user_id = $2 FOR UPDATE`. Missing → 404. `kind <> 'sport'` → 422
   `scheduled_workout_id`. `sport_id` mismatch → 422 `sport_id`. `status = 'completed'` → 409
   `SPORT_SESSION_CONFLICT` with the authoritative row.
5. **New, auto-match**: when no id was sent and `source <> 'manual'`, select the earliest open
   sport session for `(user_id, date, sport_id)` `FOR UPDATE`; none → no link.
6. Insert the activity with `scheduled_workout_id` (existing insert plus one column).
7. **New**: when linked, `UPDATE scheduled_workouts SET status = 'completed', finalized_at = now(),
   resolution = NULL, resolved_at = NULL, revision = revision + 1, updated_at = now()`.
8. Upsert `day_participation` for the activity date (existing statement; `scheduled_opportunity`
   is ORed with the existing row, so a same-day planned session keeps its opportunity flag).
9. Insert the idempotency record with status 201 (existing).

The handler is unchanged: `201` for created or replayed, `200` for an import duplicate.

## Finalization (training mutation DAO, extended)

`scheduledOutcome` gains a `kind` branch: for `sport`, `completed` when `sport_activities` has a
row with this `scheduled_workout_id`, otherwise `missed`; `participated` is true only in the
completed case. Lazy finalization therefore never changes a session an activity already completed
(it is already `finalized_at IS NOT NULL`) and marks an unfulfilled past session `missed` with
`scheduled_opportunity = true, participated = false` for its date, as today.
