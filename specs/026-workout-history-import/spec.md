# Feature Specification: Import Workout History from Strong or Hevy

**Feature Branch**: `spec/026-workout-history-import`

**Feature ID**: `gympulse-workout-history-import`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/030-workout-history-import/spec.md)

**Created**: 2026-11-03

**Status**: Approved 2026-11-03, implemented (owner direction: follow-up to the 2026-11-03 build-or-switch
review; an athlete switching apps should bring their history)

**Input**: "A user switching apps picks a Strong or Hevy CSV export, sees a preview (date range,
workout count, set count, unmatched exercise names), confirms, and gets their history in GymPulse.
Handle units, set types, and importing the same file twice. Undo removes an import batch."

## Source formats

Neither app documents its columns; the help centers only say where the export lives. The layouts
below come from real exports published in public GitHub repositories. Full notes and paths:
`side-session-notes/.../csv-import-format-research.md` (copied into `research.md` at plan time).

| Source | Where | Header (verbatim) | Evidence |
| --- | --- | --- | --- |
| Strong, layout A (6.x, current) | Settings › Export Strong Data ([help](https://help.strongapp.io/article/235-export-workout-data)) | `"Workout #";"Date";"Workout Name";"Duration (sec)";"Exercise Name";"Set Order";"Weight (kg)";"Reps";"RPE";"Distance (meters)";"Seconds";"Notes";"Workout Notes"` | AbishakeSrithar/fitness-tracker-backend `src/main/resources/strongData.csv`; matias-ceau/biovector `data/user/_temp_raw/strong_app/…csv` |
| Strong, layout B (legacy comma) | same | `Date,Workout Name,Duration,Exercise Name,Set Order,Weight,Reps,Distance,Seconds,Notes,Workout Notes,RPE` | AlexandrosKyriakakis/StrongAppAnalytics `Data/strong.csv`; harrig12/strong `strong.csv` |
| Strong, layout C (unit columns) | same | `Date;Workout Name;Exercise Name;Set Order;Weight;Weight Unit;Reps;RPE;Distance;Distance Unit;Seconds;Notes;Workout Notes;Workout Duration` (older files lack `RPE` and `Workout Duration`) | sitek94/strong-charts `static/example-dataset.csv`; imacek/lifting-with-friends-old `examples/android-strong.csv`; Brandon-Valley/strong_app_export_tools `export.csv` |
| Hevy | Profile › Settings › Export & Import Data › Export Workouts ([help](https://help.hevyapp.com/hc/en-us/articles/43708290987415-Exporting-Your-Data-from-Hevy)) | `"title","start_time","end_time","description","exercise_title","superset_id","exercise_notes","set_index","set_type","weight_kg","reps","distance_km","duration_seconds","rpe"` — `weight_lbs` replaces `weight_kg`, and `distance_miles` replaces `distance_km`, independently | TonyTromp/HevyConnect `workouts.csv`; mouadja02/Hevy-Coach `workout_data.csv`; inkandswitch/patchwork-experiments `strength/workout_data.csv`; sheehanr/fitness-data-visualizer `data/raw/workout_data.csv` |

Field facts used by the parser (same sources):

- **Dates**: Strong `2025-02-24 15:10:32`; Hevy `5 Dec 2025, 11:37` (24-hour, day not padded).
  Neither carries a timezone; both are the workout's local start and repeat on every row.
- **Duration**: Strong A whole seconds; Strong B/C text such as `1h 14m`, `35m`, `38min`; Hevy has
  none (end − start).
- **Units**: Strong A is kg by header; Strong C states `kg`/`lbs` per row; Strong B states nothing;
  Hevy states it in the column name.
- **Set types**: Strong `Set Order` is `1..n`, `W` (warm-up), `D` (drop), `F` (failure), plus
  non-set rows `Rest Timer` and `Note`. Hevy `set_type` is `normal`, `warmup`, `failure`,
  `dropset`; `set_index` is 0-based and counts warm-ups.
- **Cardio**: weight and reps empty (Strong B writes `0`), with seconds and/or distance.
- **Encoding**: Strong A/C semicolon-delimited, B and Hevy comma-delimited; RFC 4180 quoting; LF or
  CRLF; no BOM observed (accept one anyway).

Hevy's own Strong importer is the closest precedent: it requires an unedited English export, makes
unmatched exercises custom, and lets one import be reverted
([Hevy help](https://help.hevyapp.com/hc/en-us/articles/35687878672663-Importing-Strong-Data-into-Hevy)).

## Clarifications

### Session 2026-11-03

Decisions derived from the request, `docs/PRODUCT.md` (trustworthy history, user ownership, no
surprising automation), and the current data model. Items marked **owner** need confirmation.

- Q: Where do imported workouts live? → A: As **day logs** (`/api/v1/logs`), one per calendar date,
  `type_id: "other"`, `subtype_id: "general"`, no template. The calendar, past-workout screen,
  exercise progression, and stats all read day logs; off-plan workout sessions are not shown by the
  app today. Two source workouts on the same date merge into one log, in start-time order.
- Q: Same file twice, or overlapping files? → A: A date that already has any GymPulse day log
  (logged in GymPulse or imported earlier) is **skipped whole**, never merged or overwritten. The
  preview counts these as "already in GymPulse". Importing the same file twice therefore adds
  nothing the second time.
- Q: Units? → A: Weights are converted to the athlete's `settings.weight_unit` (1 lb =
  0.45359237 kg, rounded to 0.01) because stored set weights carry no unit. The source unit comes
  from the header or row. Strong layout B states none, so the request must name it; the preview
  reports `source_weight_unit: null` and the app asks. Distance is not stored in GymPulse sets and
  is dropped; the preview counts sets that lose a distance.
- Q: Set types? → A: Working, drop, and failure sets import as ordinary completed sets in file
  order. **Warm-up sets are skipped** (GymPulse has no warm-up flag; keeping them would inflate
  volume and records) and counted in the preview. `Rest Timer` and `Note` rows are ignored. RPE is
  not stored (no column) and is dropped.
- Q: Exercise matching? → A: Each source name is normalized (case, punctuation, whitespace) and
  matched, in order, to (1) a curated alias table of common Strong/Hevy names, (2) the exercise
  catalog by exact normalized name, (3) the catalog after rewriting `Name (Equipment)` to
  `Equipment Name` (Strong/Hevy style `Bench Press (Barbell)` → `Barbell Bench Press`). A matched
  set takes the catalog name, category, and modality. Names with no match are **unmatched**.
- Q: What happens to unmatched names? → A: The athlete chooses once per import: **keep** them as
  custom exercises (the source name, category `other`, modality `cardio` when every set has time
  or distance but no reps, else `strength`) or **skip** them. The preview lists every unmatched
  name with its set count. Default: keep. GymPulse has no custom-exercise catalog, so "custom"
  means the set keeps its own name snapshot.
- Q: Do imports count toward streaks and PRs? → A: **owner** — proposed: yes for history and
  progress, no for planning. Imported days count toward total workouts, weekly volume, the weekly
  goal and week streak (the athlete did train). They create no participation rows, so planned-day
  streaks and missed-workout outcomes are unchanged. Records ("PRs") are keyed by saved-workout
  exercise ids, so an imported set counts toward records, "last time", and progression only when
  its resolved name matches **exactly one** exercise in the athlete's saved workouts; it is then
  linked to that exercise id. Otherwise it is history only.
- Q: Apple Health? → A: Imported logs are never offered for Apple Health export (the source app
  usually wrote them already). Day logs expose `import_id` so the app can exclude them.
- Q: Undo? → A: `DELETE /api/v1/imports/{id}` removes every day log the import created (with
  their sets), including logs edited since. Other days are untouched.
- Q: Size limits? → A: File ≤ 10 MiB, ≤ 100,000 data rows, ≤ 5,000 resulting days. Larger files
  return 413/422 before any write. Roughly 15+ years of 4×-weekly training fits.
- Q: Where is the file parsed? → A: In the API, for both preview and commit, from the uploaded
  bytes. The file is never stored; the import record keeps only counts, date range, source, and
  options.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Preview an export before importing (Priority: P1)

An athlete uploads a Strong or Hevy CSV and sees what would be imported, without anything being
written.

**Independent Test**: Upload each fixture layout to the preview endpoint; assert detected source,
counts, date range, and unmatched names; assert no rows were written.

**Acceptance Scenarios**:

1. **Given** a Strong layout A file, **When** previewed, **Then** the response names `strong`,
   `source_weight_unit: "kg"`, the file's first and last dates, the workout, day, and set counts for
   both unmatched choices (`keep` and `skip`), the warm-up sets skipped, and each unmatched name
   with its set count.
2. **Given** a Hevy file with `weight_lbs`, **When** previewed, **Then** `source_weight_unit` is
   `lb`.
3. **Given** a Strong layout B file, **When** previewed, **Then** `source_weight_unit` is `null`.
4. **Given** dates that already have GymPulse logs, **When** previewed, **Then** they are counted in
   `days_already_present` and excluded from the importable counts.
5. **Given** a file that is not a known layout, **When** previewed, **Then** 422
   `IMPORT_UNRECOGNIZED` with the expected headers in `details`.

### User Story 2 - Import confirmed history (Priority: P1)

The athlete confirms the preview and the history appears as day logs.

**Acceptance Scenarios**:

1. **Given** a valid file and options, **When** imported, **Then** one day log per new date exists,
   each with its sets, `import_id` set, and the response matches the preview counts.
2. **Given** a file in lb and an athlete in kg, **When** imported, **Then** `100` lb is stored as
   `45.36`.
3. **Given** `unmatched: "skip"`, **When** imported, **Then** no set with an unmatched name exists
   and a date whose sets were all skipped gets no log.
4. **Given** a resolved name equal to exactly one saved-workout exercise, **When** imported,
   **Then** those sets carry that `exercise_id` and appear in `/exercises/records`.
5. **Given** the same file imported again, **When** committed, **Then** it creates nothing and
   reports every day as already present.
6. **Given** the identical request replayed with the same `Idempotency-Key`, **When** committed,
   **Then** the original import returns with 200 and nothing new is written.
7. **Given** any failure mid-import, **When** it aborts, **Then** no day log from that import exists.

### User Story 3 - Remove an import (Priority: P2)

The athlete lists their imports and removes one.

**Acceptance Scenarios**:

1. **Given** an import, **When** deleted, **Then** its day logs and sets are gone and other logs are
   unchanged; a second delete is 404.
2. **Given** another athlete's import id, **When** deleted, **Then** 404 and nothing changes.

### Edge Cases

- Future dates → those days skipped and counted in `days_skipped_future`.
- A row with an unparsable number or date → 422 `IMPORT_INVALID_ROW` with the line number; no
  partial preview or import.
- Set with neither reps nor seconds (e.g. weight only) → skipped and counted.
- Weight above the column maximum (99,999.99) → 422 row error.
- BOM, CRLF, quoted fields with embedded separators or newlines → parsed correctly.
- Hevy older `Jan 15, 2023, 8:30 AM` date style → accepted.
- Rate limit shared with export: 6 requests per minute per user.

## Requirements *(mandatory)*

- **FR-001**: `POST /api/v1/imports/preview` MUST accept `multipart/form-data` with one `file` part
  and optional `source_weight_unit`, and MUST write nothing.
- **FR-002**: `POST /api/v1/imports` MUST accept the same file plus `unmatched` (`keep`|`skip`),
  `source_weight_unit` when the file states none, and `Idempotency-Key` equal to an
  `operation_key` part, and MUST write all its day logs in one transaction.
- **FR-003**: `GET /api/v1/imports` MUST list the athlete's imports newest first; `DELETE
  /api/v1/imports/{id}` MUST remove that import and every day log it created.
- **FR-004**: Source detection MUST use the exact header sets in "Source formats"; unknown headers
  MUST be rejected, never guessed.
- **FR-005**: Dates already holding a day log MUST be skipped whole; the import MUST never modify an
  existing log.
- **FR-006**: Weights MUST be converted to the athlete's weight unit; warm-up, rest-timer, and note
  rows MUST NOT become sets.
- **FR-007**: Imports MUST NOT create participation, scheduled workouts, templates, or health
  export records.
- **FR-008**: `DayLog` and the weekly summary MUST expose `import_id` (null for normal logs); set
  logs MUST expose `exercise_name`, `exercise_category`, `exercise_modality`, and a nullable
  `exercise_id`. All additive.
- **FR-009**: Every query MUST be scoped to the authenticated user and work under the
  least-privilege runtime role.
- **FR-010**: Limits: 10 MiB file (413 `REQUEST_TOO_LARGE`), 100,000 rows and 5,000 days (422).

- **FR-011**: `GET /api/v1/logs?week=` MUST return weeks that contain logs (it failed with 500 since
  the first commit; the calendar shows imported days through it).

## Key Entities

- **Workout import**: id, user, source (`strong`|`hevy`), source weight unit, unmatched choice,
  operation key, counts (workouts, days, sets, skipped), first and last date, created time.
- **Day log** (existing): gains nullable `import_id`, cascading on import delete.

## Success Criteria *(mandatory)*

- **SC-001**: A real Strong or Hevy export of a year of training previews and imports in under
  10 seconds on the local stack.
- **SC-002**: Importing the same file twice never duplicates a day.
- **SC-003**: Removing an import restores the athlete's history to exactly its prior state.
- **SC-004**: Every fixture layout in "Source formats" has a passing parser test.
