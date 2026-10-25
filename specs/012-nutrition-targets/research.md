# Research: Daily nutrition totals and a chosen target

## Decisions

### Days are addressed by date, not by id

- **Decision**: `PUT /api/v1/nutrition/days/{date}` replaces the whole entry for one athlete and date.
  `UNIQUE (user_id, date)` is the identity. There is no `POST`, no `DELETE` route, and no id in the URL.
- **Rationale**: The spec allows exactly one entry per day and the client always knows the date it is
  editing. Addressing by date removes the create-versus-update split and the lookup a client would
  otherwise need after reinstall. The existing `PUT /api/v1/plan/overrides/{date}` and
  `/api/v1/logs/{date}` already set this precedent.
- **Spec amendment**: FR-002's "default an omitted date to local today" becomes "the date is always
  explicit in the address; the athlete's local today is used only to reject future dates". Recorded
  in `spec.md` under Clarifications.
- **Rejected**: `POST /nutrition/days` with an optional date (needs a second route to edit) and a UUID
  per day (an identity the client never needs).

### Removing a day is an empty save

- **Decision**: A `PUT` whose four values are all absent deletes the row when it exists and returns
  `204`; when nothing exists it returns `204` and writes nothing. The ledger records the `204`.
- **Rationale**: Keeps one route and one idempotency scope per day; the app's "clear all fields and
  save" maps onto it directly.
- **Rejected**: A separate `DELETE` route, which would need its own revision handling and ledger scope.

### Revision rules differ by source

- **Decision**: An athlete save (`source` omitted or `manual`) carries `expected_revision`: `0` when
  no entry exists, otherwise the current revision. A stale value is `409 REVISION_CONFLICT` with the
  stored day as `resource`. A Health save (`source: apple_health`) carries no `expected_revision`; it
  creates or replaces only a row whose `source` is `apple_health`, and when the row is `manual` it
  returns the stored row with `200` and writes nothing.
- **Rationale**: The device cannot know a server revision for a day it has never shown the athlete;
  requiring one would turn every import into a read-then-write. Imports are idempotent facts about a
  day, and the one invariant that matters ("the athlete's entry wins") is enforced by the source
  check inside the transaction.
- **Rejected**: Latest-write-wins for imports over manual rows (silently discards a correction);
  requiring revisions for imports (doubles the device's requests for no safety gain).

### A manual save over a Health row takes ownership

- **Decision**: When the stored row is `apple_health` and an athlete save arrives, it must still carry
  the current revision; on success the row becomes `manual` and the revision advances.
- **Rationale**: The app shows the imported values before the athlete edits them, so it has the
  revision; requiring it protects against a second device editing the same day.

### Targets keep history by effective date

- **Decision**: `nutrition_targets` rows are unique on `(user_id, effective_from)`. `PUT
  /api/v1/nutrition/target` with an `effective_from` (default local today) replaces the row for that
  date or inserts a new one; the current target is the latest `effective_from` on or before local
  today. `expected_revision` is the current target's revision, `0` when none exists. A save identical
  to the current target returns it unchanged with `200`.
- **Rationale**: Progress for past weeks must be judged against the target that applied then (FR-009,
  FR-010). A history table costs one index and avoids a target-change log.
- **Rejected**: A single mutable target row (loses history); a target column on `training_profiles`
  (couples two revision streams and two features).

### Range reads return days and the targets that applied

- **Decision**: `GET /api/v1/nutrition/days?from&to` returns `{ "days": [...], "targets": [...] }`.
  `targets` holds the target in effect on `from` (the latest `effective_from <= from`, if any) plus
  every target with `effective_from` inside the range, oldest first. The client resolves each day's
  target by picking the latest `effective_from <= date`.
- **Rationale**: One round trip for the Progress section; the target-per-day join is trivial on the
  client and avoids repeating the same target on every day row.
- **Rejected**: Embedding the target on each day (payload bloat, two sources of truth); a separate
  targets range route (an extra request for every Progress load).

### Numbers, bounds, and units

- **Decision**: `energy_kcal` is an integer 0–20,000. `protein_g`, `carbs_g`, `fat_g` are
  `NUMERIC(6,1)` 0–2,000 and exchanged as JSON numbers with at most one decimal. Zero is allowed for a
  day (an athlete can record a true zero); the target's `protein_g` must be greater than zero. At least
  one of the four day values must be present, enforced by a table check as well as validation.
- **Rationale**: 20,000 kcal reuses the sport-activity bound; 2,000 g covers any real intake with
  room; one decimal matches how trackers export. Grams only; the device converts kilojoules.

### Timezone and future dates

- **Decision**: The service loads the training profile and computes local today exactly as
  `sport_activity_svc.go` does; a date after local today is `422` on `date` (or `effective_from`).
  A missing profile surfaces the profile DAO's `NotFoundError`, as it does for sport activities.
- **Rejected**: `model.UTCToday()` (the legacy body-weight rule) because it mis-dates evening saves.

### Migration 027 provisions the runtime role

- **Decision**: The up migration creates both tables, enables RLS, revokes Data API role privileges
  (copying the DO block from 025), and, when the `gym_pulse_api` role exists, grants DML and creates
  the `gym_pulse_api_access` policy on both tables, as `internal/dao/supabase_security_migration_test.go`
  requires of every table created after 026.

### Route shape and codes

- **Decision**: `PUT /api/v1/nutrition/days/{date}` → `201` created, `200` replaced or Health-save-ignored,
  `204` removed; `GET /api/v1/nutrition/days?from&to`; `GET /api/v1/nutrition/target` → `200` or shared
  `404`; `PUT /api/v1/nutrition/target` → `201` created, `200` replaced or unchanged. Conflicts reuse
  `REVISION_CONFLICT` and `IDEMPOTENCY_CONFLICT`; no new code is introduced.

## Resolved Technical Questions

- Idempotency scopes: `nutrition-days/put` and `nutrition-target/put`; advisory lock namespace
  `nutrition:` per athlete.
- Hashing: hash the request as sent plus the path date, not server-defaulted fields, so a verbatim
  retry replays (lesson recorded in `health_export_svc.go`).
- `DATABASE_DESIGN.md`: two table sections and index rows; `scripts/validate-database-design.sh`
  must pass.
- Account deletion: `DELETE FROM nutrition_days` and `DELETE FROM nutrition_targets` join the list in
  `account_dao.go`; the DAO test that enumerates tables is updated.
- Swagger: swag godoc on the four handlers; `docs/swagger.*` regenerated so CI's drift check passes. `docs/CONTRACTS.md` remains the client contract.
- Migration number: `027`; `026` is the runtime role on `main`.
