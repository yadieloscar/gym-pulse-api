# Implementation Plan: Import Workout History from Strong or Hevy

**Branch**: `spec/026-workout-history-import` | **Spec**: [spec.md](spec.md) | **Research**: [research.md](research.md) | **Feature ID**: `gympulse-workout-history-import`

## Design

- **Transport** (`internal/handler/workout_import.go`): `POST /imports/preview` and `POST /imports`
  read `multipart/form-data` with exactly one `file` part (≤ 10 MiB via `MaxBytesReader`, the avatar
  pattern) and option fields ≤ 256 bytes. Commit requires `Idempotency-Key` equal to
  `operation_key`. `GET /imports` lists; `DELETE /imports/{id}` removes. File errors map to `422`
  with `IMPORT_*` codes through `handleServiceError`.
- **Router**: both POST routes share `RateLimitPerUser(ExportLimit)` (6/min) with export; list and
  delete are unthrottled like other owner reads and deletes.
- **Parser** (`internal/service/import_parse.go`): strips a BOM, picks `;` or `,` from the header
  line, and matches the header as an exact column set against the published layouts (Strong
  seconds, legacy, units, units-old; Hevy × {kg, lb} × {km, miles}). Rows are read with
  `encoding/csv` and a fixed field count. Every field is parsed strictly; one bad row fails the
  file with its line number. Rest-timer and note rows are dropped.
- **Matcher** (`internal/service/import_match.go`): normalized names; alias table → exact catalog
  name → `Equipment Movement` → bare movement. Saved-workout link only when exactly one owned
  exercise has the resolved or source name.
- **Service** (`internal/service/import_svc.go`): groups rows into workouts and dates, skips
  existing and future dates, builds the `keep` and `skip` day sets in one pass, converts weights to
  the athlete's unit, and reads export times as wall-clock in the training-profile timezone.
  Commit checks a stored replay first (its days now exist, so planning would find nothing), then
  plans and calls the DAO. The request hash is `sha256(file)` plus the options.
- **DAO** (`internal/dao/workout_import_dao.go`): one transaction under a per-athlete advisory
  lock: replay check, re-read of existing dates (a log added since the preview is a `409`), import
  row with its response body, then day logs and set logs in chunks of 2,000 rows through
  `INSERT … SELECT FROM unnest(...)`. COPY is refused under RLS for the runtime role.

## Existing behavior touched

- `DayLog` and `DayLogSummary` expose `import_id`; set logs expose the stored name snapshot and a
  nullable `exercise_id`. Set logs are ordered `logged_at, exercise_id, set_index`, which is the
  old order for logs written in one transaction and file order for imports.
- `GET /logs?week=` scanned `DATE` into a string and failed with 500 for any week with logs (since
  the first commit). It now selects `to_char(date, 'YYYY-MM-DD')` like `GetByDate`. Imported days
  depend on this route for the calendar.
- Health export refuses imported logs (`import_id IS NULL` in the finished-workout query).
- Account deletion deletes `workout_imports`.

## Persistence

Migration 034: `workout_imports` (row-level operation key, like 033), `day_logs.import_id` with
`ON DELETE CASCADE` and a partial index, RLS and Data API revocation, runtime-role grant and
policy. Additive; existing logs keep `NULL`.

## Compatibility and rollout

Contract 1.6.0, additive. Old apps never call the import routes and ignore the new fields. The one
visible change for old apps is `exercise_id: null` on imported sets, which only exist after a new
app imports. Deploy the API before the app release (app spec 030). Rollback: the down migration
drops the column and table (imported logs would then lose their batch link, so remove imports
first if rolling back).

## Verification

Unit tests for every layout fixture, matcher rules, units, limits, errors, replay; handler tests
for multipart edges; integration tests for commit/replay/conflict/list/delete, record linking,
health-export refusal, and 25,000 sets across chunks; smoke step 13j as the least-privilege role.
