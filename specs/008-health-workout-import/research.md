# Research: Imported health workouts API

## Decisions

### Extend `sport_activities` instead of a new table

- **Decision**: Add `source`, `external_id`, and three measurement columns to `sport_activities`.
- **Rationale**: The spec defines an imported workout as a sport activity with an origin. One table
  keeps history reads, participation, account deletion, and the client list unchanged.
- **Rejected**: A separate `health_workouts` table would need a second list route, a second
  participation path, and client-side merging for every history surface.

### Origin is a constrained text column, not a boolean

- **Decision**: `source TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'apple_health'))`.
- **Rationale**: The spec reserves room for a later Health Connect origin. Adding a value is a
  constraint change, not a schema redesign. The default backfills existing rows for free.
- **Rejected**: `imported BOOLEAN` cannot distinguish future origins; a lookup table is overkill for
  two values.

### Database-enforced uniqueness per athlete, origin, and identity

- **Decision**: Partial unique index on `(user_id, source, external_id) WHERE external_id IS NOT NULL`
  plus `CHECK ((source = 'manual') = (external_id IS NULL))`.
- **Rationale**: The constitution puts invariants in the database when it can enforce them. Two
  devices importing concurrently are serialized by the existing per-athlete advisory lock, and the
  index is the backstop if that lock is ever bypassed.
- **Rejected**: Application-only checks lose to concurrent requests.

### Re-import returns the existing activity with `200`

- **Decision**: Inside the create transaction, after the idempotency replay check, look up
  `(user_id, source, external_id)`. If it exists, return it with `200 OK` and write nothing.
  A verbatim retry of the original creating request still replays `201` from the ledger.
- **Rationale**: The spec requires the response to make clear nothing new was recorded, and `200`
  versus `201` does that without a new error code. Participation is untouched because the transaction
  returns before the upsert.
- **Rejected**: `409` would make the client treat a normal re-import as a failure; silently returning
  `201` hides the duplicate from the client's sync bookkeeping.

### Measurements are validated, optional, and inert

- **Decision**: `distance_meters` 1–1,000,000, `energy_kcal` 1–20,000, `avg_heart_rate_bpm` 30–250,
  all nullable with matching `CHECK`s. Zero is rejected; absent means absent.
- **Rationale**: Bounds catch unit mistakes (meters sent as kilometers) without rejecting real
  ultra-distance days. Stats, records, and volume queries do not read these columns.
- **Rejected**: Storing raw unit strings pushes conversion to every reader.

### Migration numbering

- **Decision**: Use `021`, not `020`.
- **Rationale**: The open `spec/007-skip-missed-workout` branch already carries
  `020_add_scheduled_workout_resolution`. Skipping 020 avoids a collision when both merge. If 007 is
  abandoned, renumbering to 020 before merge is a one-line rename.

### Validation placement

- **Decision**: Service validates source enum, identity presence and length, and measurement ranges;
  the database repeats them as `CHECK`s. The handler only maps the DAO's `created` flag to the status.
- **Rationale**: Matches the existing sport-activity layering and gives clients field-level 422s.

## Resolved Technical Questions

- `source` accepted values: `manual` (default when omitted) and `apple_health`.
- `external_id`: trimmed, 1–128 characters, required when `source` is not `manual`, forbidden when it is.
- Date validation is unchanged: athlete-local civil date from the saved training-profile timezone,
  future dates rejected, past dates accepted. The device converts workout start time to that date.
- List and get responses always include `source`; `external_id` and measurements use `omitempty`.
- Rollback: the down migration drops the index and the five columns. Imported rows survive as manual
  rows only if the down migration is edited to keep them; the documented down drops them with the
  columns' data, which is acceptable because the app is rolled back first.
- No change to `GetWeeklyCount`, distribution, volume, or records.
