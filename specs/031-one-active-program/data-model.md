# Data Model: One Active Program on Fixed Weekdays

**Spec**: [spec.md](spec.md) · **Research**: [research.md](research.md)

One additive migration pair, `045_program_weekdays_and_rolling_schedule` (number re-checked at
implementation, R12). No new table, no data rewrite, no backfill.

## Changed tables

### `program_workouts`

| Column | Type | Rule |
| --- | --- | --- |
| `weekdays` (new) | `SMALLINT[]`, nullable | `NULL` = unplaced. Otherwise `weekdays <@ ARRAY[1,2,3,4,5,6,7]::smallint[]` and `cardinality(weekdays) BETWEEN 1 AND 7` (constraint `program_workouts_weekdays_check`). Element uniqueness and ascending order are enforced in the model before write. |
| `preferred_weekday` (existing) | `SMALLINT`, nullable | Unchanged. Written as `weekdays[1]` whenever a unit is placed. |

Rows are now updated in place by `Replace` when the request names their `id` (R2).

### `programs`

| Column | Type | Rule |
| --- | --- | --- |
| `scheduled_through` (new) | `DATE`, nullable | Last date the fill covered for this program. `NULL` until the first fill. Only ever moves forward. |

`idx_programs_one_active_per_user` (migration 014) already enforces one active program.

### `scheduled_workouts`

| Column | Type | Rule |
| --- | --- | --- |
| `slot_date` (new) | `DATE`, nullable | The layout date this session was generated for. Set by the fill only; never updated, including by a move. `NULL` on existing rows, recover copies, and off-plan sessions. |

New indexes:

| Index | Definition | Serves |
| --- | --- | --- |
| `uq_scheduled_workouts_unit_slot` | `UNIQUE (program_workout_id, slot_date) WHERE program_workout_id IS NOT NULL AND slot_date IS NOT NULL` | FR-004, FR-013: one session per unit per layout date; `ON CONFLICT DO NOTHING` target. |
| `idx_scheduled_workouts_program_planned` | `(program_id, date) WHERE status = 'planned'` | FR-022: start, edit, and removal delete or update a program's planned sessions by date. |

Existing `idx_scheduled_workouts_user_date` keeps serving reads and the start delete.

## Existing data

- Every existing unit has `weekdays = NULL` (unplaced). It is placed by `PlaceUnits` the first time
  an operation would create a session for its program (R5).
- Every existing session has `slot_date = NULL` and is never rewritten.
- An existing active program has `scheduled_through = NULL`; its first fill starts after its latest
  planned session (R4).

## Down migration

Drops the two indexes, the constraint, and the three columns with `IF EXISTS`. It removes only what
the up migration added. Sessions created by the fill remain as ordinary planned sessions.

## Derived values (not stored)

| Value | Derivation |
| --- | --- |
| Placed program | Every unit has non-null `weekdays`. A program is all placed or all unplaced; a mixed request is 422 `workouts`. |
| Off-availability unit | A placed unit with a weekday not in the proposed (or current) `available_days`. |
| Start date | `max(from, today)` in the training-profile timezone. |
| Fill range | `max(scheduled_through, start - 1) + 1` through `today + 56`. |

## State rules

### Program

```text
saved (active=false) --apply--> active (placed, scheduled_through set)
active --apply of another program--> saved
saved --DELETE--> gone          active --DELETE--> 409 CONFLICT
```

### Session, as touched by this feature

| Operation | Touches | Never touches |
| --- | --- | --- |
| Fill | Inserts `planned` rows with `slot_date` after `scheduled_through` | Any existing row |
| Start (apply) | Deletes `planned` rows with no active or completed workout session, dated on or after the start date | `in_progress`, `completed`, `incomplete`, `missed`, past dates |
| Save active program | Deletes, inserts, or re-snapshots `planned` unstarted rows dated today or later, per unit (R7, Amendment A3) | The same set as above, plus rows of unchanged units |
| Move | Updates `date` and `revision` of one `planned` unstarted row dated today or later | `slot_date`, every other row |
| Remove program | Deletes that program's `planned` unstarted rows | Finished rows (their `program_id` becomes `NULL` by the existing foreign key) |

## Documentation

`docs/DATABASE_DESIGN.md` gains the three columns, the constraint, and the two indexes in the same
change; `./scripts/validate-database-design.sh` checks it.
