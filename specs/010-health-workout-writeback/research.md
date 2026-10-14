# Research: Remember a workout's health export

## Decisions

### A separate ledger, not columns on sessions

- **Decision**: New table `health_exports(user_id, workout_kind, workout_id, source, external_id, exported_at)`.
- **Rationale**: The app finishes workouts in two shapes: a planned workout finalized through
  `POST /scheduled-workouts/{id}/complete` (its `workout_sessions` row is server-internal and the
  client never sees its id) and an off-plan workout stored as a legacy `day_logs` row. One ledger
  keyed by kind and id covers both without touching two tables or two response shapes.
- **Spec amendment**: the spec's "finished workout session" maps to a finalized planned workout
  (status `completed` or `incomplete`) or an off-plan log. Reads use a date-range list rather than
  fields embedded in session reads. Recorded in `spec.md` under Clarifications.
- **Rejected**: Columns on `workout_sessions` (unreachable by id from the client) or on both
  `scheduled_workouts` and `day_logs` (two scans, two contracts).

### Write-once identity per workout and source

- **Decision**: `UNIQUE (user_id, workout_kind, workout_id, source)`. The DAO looks the row up inside
  the transaction: same `external_id` → replay with `200`; different → `ConflictError` mapped to
  `409 HEALTH_EXPORT_CONFLICT` with the stored row as `resource`.
- **Rationale**: The device must never write a second Health workout for one GymPulse workout; the
  stored identity is the proof. The existing idempotency ledger still replays a verbatim retry.
- **Rejected**: Overwriting silently hides a duplicate Health write from the client.

### Finished means an outcome that represents real work

- **Decision**: `scheduled_workout` qualifies when `finalized_at IS NOT NULL` and status is
  `completed` or `incomplete`; `missed` never exports. `day_log` qualifies when owned and `type_id`
  is not `rest`.
- **Rationale**: A missed planned workout or a rest log is not a workout in Health terms.

### Ownership through the referenced workout

- **Decision**: The DAO resolves the workout by `(user_id, id)` in the same transaction and returns
  the shared `NOT_FOUND` for missing, foreign, or not-finished workouts; validation of the body
  happens first in the service.
- **Rationale**: Constitution II: foreign and missing ids stay indistinguishable.

### Source is a constrained column

- **Decision**: `source TEXT CHECK (source IN ('apple_health'))`, mirroring the import feature so a
  Health Connect origin is a constraint change later.

### Route shape

- **Decision**: `PUT /api/v1/health-exports/{workout_kind}/{workout_id}` (idempotent record) and
  `GET /api/v1/health-exports?from&to` (`{ "health_exports": [...] }`, newest first, `[]` when
  empty, range bounded by the existing 366-day rule). The range joins each kind's date.
- **Rejected**: `POST` to a collection would need the client to invent an id for a resource that is
  naturally addressed by its workout.

## Resolved Technical Questions

- `workout_kind`: `scheduled_workout` or `day_log`; anything else is `422` on `workout_kind`.
- `external_id`: trimmed, 1–128. `exported_at`: optional RFC3339, defaults to now; future values are clamped to now.
- Migration number: `023` (main is at `022`).
- Account deletion: cascade through `user_id` plus an explicit delete in `account_dao.go` for parity with other tables.
- No change to stats, participation, records, or volume.
