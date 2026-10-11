# Data Model: Skip a missed workout

## ScheduledWorkout (changed)

Table `scheduled_workouts` (`migrations/012_create_training_domain.up.sql:116`). New columns from
migration `020_add_scheduled_workout_resolution`:

| Column | Type | Rules |
| --- | --- | --- |
| `resolution` | `TEXT NULL` | `NULL`, `'skipped'`, or `'recovered'` |
| `resolved_at` | `TIMESTAMPTZ NULL` | Set exactly when `resolution` is set |

Constraint: `(resolution IS NULL) = (resolved_at IS NULL)`.

JSON (`internal/model/training.go` `ScheduledWorkout`):

| Field | Type | Notes |
| --- | --- | --- |
| `resolution` | `"skipped" \| "recovered"`, omitted when null | Meaningful only when `status` is `missed` |
| `resolved_at` | RFC 3339 timestamp, omitted when null | |

`status`, `finalized_at`, sets, and every derived statistic are unchanged.

## State transitions (missed workouts only)

```text
            skip                 recover (copy planned for a date)
 unresolved ─────► skipped        unresolved ─────────────────► recovered
     ▲               │
     └──── unskip ───┘            recovered: terminal (unskip → 409 RESOLUTION_CONFLICT)
```

| From | Operation | Result | Revision |
| --- | --- | --- | --- |
| unresolved (missed) | skip | skipped | +1 |
| skipped | skip | skipped (no-op) | unchanged |
| skipped | unskip | unresolved | +1 |
| unresolved | unskip | unresolved (no-op) | unchanged |
| recovered | skip or unskip | 409 `RESOLUTION_CONFLICT` | unchanged |
| not missed | skip | 409 `RESOLUTION_CONFLICT` | unchanged |
| unresolved (missed) | recover | source recovered; new planned copy returned | source +1 |
| skipped or recovered | correction makes outcome completed/incomplete | resolution cleared | per correction |

Recovery selection: earliest by `sequence_position NULLS LAST, date` among
`status='missed' AND resolution IS NULL AND date <= :date`.

## Backfill (migration 020 up)

For each `idempotency_records` row `r` with `scope = 'schedule/recover'` and `resource_id` = copy `c`,
the source `m` is the earliest missed row (by `sequence_position NULLS LAST, date`) with the same
`user_id`, `program_workout_id = c.program_workout_id`, `sequence_position IS NOT DISTINCT FROM
c.sequence_position`, `m.date <= c.date`, and `m.finalized_at <= r.created_at`. Mark `m` recovered with
`resolved_at` = the earliest matching `r.created_at`. Later-cycle occurrences of the same program
workout are never matched. Down migration drops both columns.

## Idempotency records

Scopes `scheduled-workouts/skip` and `scheduled-workouts/unskip`, hashing `{workout_id, request}`,
stored in `idempotency_records` like `scheduled-workouts/complete`.
