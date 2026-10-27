# Research: Planned Sport Sessions API

## Decisions

### Sport sessions are program units, not a new table

- **Decision**: Add `kind TEXT NOT NULL DEFAULT 'exercises' CHECK (kind IN ('exercises','sport'))`
  plus nullable `sport_id`, `sport_name`, `target_duration_minutes`, `target_distance_meters`, and
  `sport_notes` to `program_workouts`, and the same six columns to `scheduled_workouts`.
- **Rationale**: The spec puts both kinds in one ordered sequence with one weekday preference, one
  materialization, one missed rule, and one skip rule. The existing `UNIQUE (program_id,
  sequence_position)`, the `LEFT JOIN program_exercises` read, and every schedule query already
  tolerate a workout with no exercises. Old rows take `kind = 'exercises'` through the default.
- **Rejected**: A `program_sport_sessions` child table needs a second join on every program and
  schedule read, a second insert path in three materializers, and a second delete predicate in
  regenerate and plan-transition apply.

### Pair checks keep kinds honest in the database

- **Decision**: `CHECK ((kind = 'sport') = (sport_id IS NOT NULL))` and the same for `sport_name`
  and `target_duration_minutes`; `target_distance_meters` and `sport_notes` require `kind = 'sport'`.
  Service validation repeats the rules for 422s and adds "exercise units need one exercise, sport
  units need none" plus the distance-sport rule, which the database cannot express cheaply.
- **Rationale**: The constitution puts invariants in the database when it can enforce them. The
  sport id, name, duration, distance, and notes bounds copy the `sport_activities` checks verbatim.

### Recording reuses `POST /api/v1/sport-activities`

- **Decision**: Add optional `scheduled_workout_id` to the create request. When present, the one
  existing create transaction locks the owned open session, inserts the activity with the link,
  marks the session `completed`, clears any skip, and credits participation to the activity date.
- **Rationale**: The app's prefilled form posts the same payload it posts today plus one field; the
  idempotency ledger, the import-duplicate path, participation, and validation stay in one place.
  A second "record" route would duplicate all of it and still have to create a sport activity.
- **Rejected**: `POST /scheduled-workouts/{id}/record-sport` (duplicate transaction);
  `POST /scheduled-workouts/{id}/complete` for sport (nothing to derive an outcome from, and it
  would skip the activity's own validation).

### The link lives on the activity; the session exposes it by join

- **Decision**: `sport_activities.scheduled_workout_id UUID REFERENCES scheduled_workouts(id)
  ON DELETE SET NULL` with partial unique index `ux_sport_activities_scheduled_workout
  (scheduled_workout_id) WHERE scheduled_workout_id IS NOT NULL`. Schedule reads `LEFT JOIN` the
  activity to fill `sport.sport_activity_id` and `sport.performed_date`.
- **Rationale**: One foreign key, one uniqueness rule ("one activity per session"), no two-column
  consistency to maintain. Account deletion removes `sport_activities` before `scheduled_workouts`;
  `SET NULL` makes that order valid without reordering the delete list. The join is on the primary
  key, so the schedule read stays one query per table as today.
- **Rejected**: A column on both tables (two writers, drift); a link table (a third write for a
  one-to-one).

### Sport session outcomes: planned, completed, missed

- **Decision**: `scheduledOutcome` and `deriveScheduledStatus` branch on `kind`: a sport session is
  `completed` when a linked activity exists, otherwise `missed`. `in_progress` and `incomplete`
  never apply. Lazy finalization, `/complete`, skip, unskip, and the regenerate delete predicate
  stay as they are; `/complete`, `PATCH` with `required_sets`, set check, target edit, and extra-set
  routes answer `422 VALIDATION_ERROR` with `details.field = "kind"` on a sport session.
- **Rationale**: Today a finalized workout with zero required sets is `missed`, which is exactly
  the unfulfilled sport case, so only the fulfilled branch is new. Set routes cannot act on a row
  that has no sets; a field-level 422 is the existing shape for "this request does not fit this
  resource".

### Recording a missed session later credits the performed date

- **Decision**: The session's `status` becomes `completed`, `finalized_at` is set, `resolution` and
  `resolved_at` are cleared, and `day_participation` is upserted for the activity's date (the
  existing sport upsert). The scheduled date's participation row is left as lazy finalization
  wrote it (`scheduled_opportunity = true, participated = false`) when it was already missed, and
  is never written when the session is completed before its date.
- **Rationale**: Matches the spec (US3 scenario 3) and the contract rule that a finalized day's
  participation is immutable. Writing a finalized future date would corrupt the day streak.

### Imports auto-fulfil the earliest open session

- **Decision**: In the create transaction, when `source <> 'manual'` and no
  `scheduled_workout_id` was sent, after the import-duplicate check select one owned row with
  `kind = 'sport' AND status IN ('planned','missed') AND date = $date AND sport_id = $sport_id`
  and no linked activity, `ORDER BY sequence_position NULLS LAST, created_at, id LIMIT 1 FOR
  UPDATE`, and link it exactly as an explicit record would. Manual creates never auto-link.
- **Rationale**: The spec requires it (FR-010) and the duplicate check already returns before this
  step, so re-imports change nothing. Explicit beats implicit for typed logs, where the app sends
  the id.

### Lock order

- **Decision**: The sport-activity transaction takes `schedule:` and then `sport-activity:` on
  every create, before any row lock; it never takes `program:`. Documented order becomes
  `program:` → `schedule:` → `sport-activity:` → row locks.
- **Rationale**: Linking updates `scheduled_workouts`, which every schedule mutation guards with
  `schedule:`. Taking it unconditionally is simpler than branching on the payload and cannot
  deadlock against any existing path, none of which takes `sport-activity:`.
- **Rejected**: Taking `schedule:` only when linking (two orders to reason about for one row).

### Recover ignores sport sessions

- **Decision**: `POST /schedule/recover` selects only `kind = 'exercises'` rows. A missed sport
  session is resolved by recording an activity against it with a later date, or by skipping.
- **Rationale**: Recover exists to re-plan a set-based workout into the player. A sport session has
  no sets to copy and the spec's "Log it today" is a record with today's date. Copying sport rows
  would create a second open session to reconcile.

### Health export excludes sport sessions

- **Decision**: `health_export_dao` treats a scheduled workout as finished only when
  `kind = 'exercises'`; a completed sport session answers the shared 404.
- **Rationale**: Spec non-goal; the activity that fulfilled it is the health-store record, and the
  device already imported it or logged it.

### The new conflict

- **Decision**: Recording against a session that is already `completed` returns
  `409 SPORT_SESSION_CONFLICT` with `details.status` and the authoritative `ScheduledWorkout` as
  `resource`. The `ConflictError` message contains "sport session" and `response.go` maps it.
- **Rationale**: `RESOLUTION_CONFLICT` is about skip and recover; `REVISION_CONFLICT` needs a
  revision the request does not carry. One new code keeps the client logic explicit.

### Migration numbering

- **Decision**: `027_add_planned_sport_sessions`. The latest on `origin/main` is 026.
- **Rationale**: `012-nutrition-targets` is in flight in another worktree; whichever merges second
  renumbers, as feature 008 did.

### Known materialization defects are out of scope

- `MaterializeProgramForWeekdays` ignores `preferred_weekday`, restarts the sequence on every
  call, and regenerate can duplicate retained dates. Sport sessions inherit these as-is. They are
  recorded as residual risk in `quickstart.md`, not fixed here, because they change exercise
  workout behavior and need their own spec.

### Research amendment (2026-10-26)

Source: `docs/briefs/sport-session-targets-research-2026-10.md` (parent workspace). The owner
asked for recommendations A1–A6; the API carries A1, A3, and A5.

- **A1, links are repairable**: the normative "never re-linked" becomes "never changed
  automatically". The same-day false-positive risk is recorded in `quickstart.md`.
- **A3, link and unlink live on the activity**: `POST /api/v1/sport-activities/{id}/link` with
  `scheduled_workout_id` and `POST /api/v1/sport-activities/{id}/unlink`, both with the session's
  `expected_revision` and an `operation_key`, both answering `{sport_activity, scheduled_workout}`.
  Placing them on the activity keeps the sport-activity handler → service → DAO path and its
  lock order (`schedule:` → `sport-activity:`), and avoids changing the schedule service signature.
  No migration: the nullable link column and one-per-session index already exist.
- **A3, unlink status**: date ≥ local today → `planned`, `finalized_at` cleared; earlier →
  `missed`, `finalized_at` kept, and `day_participation` for the scheduled date is upserted with
  `scheduled_opportunity = true` without lowering `participated`, so a session completed before
  lazy finalization still counts as a missed opportunity.
- **A3, auto-match**: `ORDER BY abs(target_duration_minutes - $duration), sequence_position NULLS
  LAST, created_at, id` and `AND target_duration_minutes <= 3 * $duration`. With link-existing
  available, the guard's false negatives are repairable.
- **A5, perceived effort**: migration `028_add_sport_activity_effort` adds
  `perceived_effort SMALLINT CHECK (1..10)`. CR-10-style whole-session rating; 0 ("rest") is not
  a session. Display only.
- **Unknown request keys**: `decodeJSON` does not reject unknown fields, so newer clients that
  echo future `sport` keys back on a program replace are tolerated (A2 verification for the app).

## Resolved Technical Questions

- Distance sports: `cycling`, `running`, `swimming`, `other`; `target_distance_meters` is rejected
  with `details.field = "workouts"` for any other `sport_id`. Activities keep their existing
  unrestricted `distance_meters`.
- `kind` is always present on program workouts, starter workouts (`exercises`), and scheduled
  workouts. `sport` is present only when `kind = 'sport'`; `exercises` and `required_sets` are `[]`
  for sport units.
- Request `kind` is optional and defaults to `exercises`; a sport unit sends `kind: "sport"` and a
  `sport` object and omits or sends an empty `exercises` array.
- Recording validation order: ownership (404), kind (422 `scheduled_workout_id`), sport match
  (422 `sport_id`), already completed (409), then the activity's own rules.
- `PATCH /scheduled-workouts/{id}` on a sport session accepts a `name` change and rejects a
  non-empty `required_sets` with 422 `kind`; renaming keeps the sport snapshot. (Analysis finding
  U1, 2026-10-25.)
- Weekly planned-versus-done has no API endpoint; the app derives it from the schedule, which now
  carries sport sessions. `/stats` is unchanged. (Analysis finding U2, 2026-10-25.)
- A program `PUT` still re-creates `program_workouts`; scheduled sport sessions keep their snapshot
  and lose `program_workout_id`, exactly as exercise workouts do today.
- Starter programs, `from-starter`, and `adopt-legacy` produce `kind = 'exercises'` only.
- `GetWeeklyCount` already counts `sport_activities` dates and `GetTotalWorkouts` already excludes
  them; both are unchanged. Distribution and volume read set logs only.
- Rollback: the down migration drops the index, the link column, and the twelve kind/sport columns.
  Sport-session rows become exercise workouts with no sets, so the app is rolled back first and
  any remaining sport units are removed by the athlete or by a scripted program replace before the
  down migration runs.
