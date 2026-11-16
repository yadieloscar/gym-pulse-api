# Research: One Active Program on Fixed Weekdays

**Spec**: [spec.md](spec.md) · **Plan**: [plan.md](plan.md)

Evidence is from `origin/main` at `cafdb2a` (2026-11-16).

## R1. Where weekdays live

- **Decision**: A nullable `weekdays SMALLINT[]` column on `program_workouts`. `NULL` means the unit
  is **unplaced**. A placed unit has 1 to 7 unique ISO weekdays. `preferred_weekday` stays and is
  written as the first weekday of a placed unit.
- **Rationale**: One row per unit already exists; an array keeps the read a single query and the
  check simple (`weekdays <@ ARRAY[1..7]` and cardinality 1 to 7). Uniqueness of the elements is
  validated in the model, where the request error is produced.
- **Alternatives considered**: A `program_workout_weekdays` child table (a join on every program
  read and a second insert loop, for at most 7 values); reusing `preferred_weekday` as the weekday
  (cannot hold two days, and a released app sends it without meaning it).

## R2. Units need a stable identity

- **Finding**: `programDAO.Replace` deletes every `program_workouts` row and inserts new ones
  (`internal/dao/program_dao.go`). Each save therefore changes every unit ID and sets
  `scheduled_workouts.program_workout_id` to `NULL` on every existing session.
- **Decision**: `PUT /programs/{id}` accepts an optional `id` on each unit. A unit whose `id` belongs
  to the program is updated in place; a unit without an `id` is inserted; an existing unit absent
  from the request is deleted. A foreign or unknown `id` is 422 `workouts`.
- **Rationale**: FR-015 and FR-016 need to know which unit changed. Without stable IDs every save
  would look like "all units removed, all units added" and every one-off move would be lost.
- **Compatibility**: A released app sends no unit `id`, so its saves behave as today (all units
  replaced). It has no one-off moves to lose.
- **Detail**: `UNIQUE (program_id, sequence_position)` is violated by in-place renumbering. The
  update runs in two passes inside the transaction: matched units first move to
  `sequence_position + 1000`, then to their final value. A program holds at most 14 units.
- **Alternatives considered**: Matching units by name or position (renames and reorders break it);
  making the constraint deferrable (a constraint rewrite for a problem two statements solve).

## R3. How sessions are placed, and how a session is never created twice

- **Decision**: Add `scheduled_workouts.slot_date DATE NULL`: the layout date a session was
  generated for. It is set only by the program fill and never changes, including when the session
  is moved. A partial unique index on `(program_workout_id, slot_date)` where both are non-null
  enforces one session per unit per layout date. The fill inserts with `ON CONFLICT DO NOTHING`.
- **Rationale**: This is the database rule FR-004 asks for. It makes fills idempotent and safe under
  concurrent reads (FR-013), and a moved session still occupies its original slot, so the slot is
  never refilled. Recover copies and off-plan sessions have no `slot_date` and are unaffected, so
  recovering a missed Push onto a day that already holds Push still works.
- **Alternatives considered**: Unique on `(program_workout_id, date)` where planned (breaks recover
  and same-day moves); advisory locks alone (no protection for a future code path that forgets the
  lock).

## R4. The rolling schedule without a background worker

- **Decision**: Add `programs.scheduled_through DATE NULL`, the last date the fill has covered. One
  function, `fillActiveProgram(ctx, tx, userID, today)`, inserts sessions for dates after
  `max(scheduled_through, today - 1)` through `today + 56` and advances `scheduled_through`. It is
  called from the schedule read, and from every operation that starts or edits the active program.
- **Rationale**: The schedule read already writes: `scheduleService.List` lazily finalizes past
  sessions. Filling there needs no scheduler, no new dependency, and no goroutine (Constitution V).
  Because the fill never looks behind `scheduled_through`, a session the athlete skipped, moved, or
  deleted is never re-created (FR-013), and dates that passed while the athlete was away longer
  than the filled range stay empty (User Story 4, scenario 4).
- **Cost**: The read first checks `scheduled_through >= today + 56` with one indexed row lookup. The
  fill transaction runs at most once per athlete per day.
- **First run for an existing program**: `scheduled_through` is `NULL`. The fill starts after the
  latest existing planned session of that program, so no existing session is rewritten.
- **Alternatives considered**: A cron or worker (new concurrency and deployment surface); computing
  future sessions virtually at read time (sessions need IDs to be started, moved, or skipped).

## R5. Placing an unplaced program

- **Decision**: One pure model function, `PlaceUnits(units, availableDays, start)`. It walks dates
  from `start`, takes `max(len(units), len(availableDays))` available-day slots, and gives slot `k`
  to unit `k mod len(units)`. A unit's weekdays are the weekdays of its slots.
- **Rationale**: For the first cycle this is exactly what `MaterializeProgramForWeekdays` does
  today, so a released app's flows (the knee starter rotates `sequence_position` to line units up
  with weekdays from tomorrow) get the same first week. When there are more units than days, units
  share weekdays and nothing is dropped (spec edge case).
- **When it runs**: The first time any operation would create a session for the program
  (plan-transition apply, regenerate, materialize, or the fill on read), using the available days in
  effect for that operation. `POST /programs` alone never places. The result is stored (FR-002).
- **Spec correction**: FR-021 said the migration gives existing units weekdays. Placement in SQL
  would duplicate this rule in a second language. The migration now adds columns only, and existing
  programs are placed by this function on first use. FR-001 now states that an unplaced unit returns
  `weekdays: []`. Both edits are made to the spec in this change.

## R6. One start operation

- **Decision**: Keep `POST /plan-transitions/preview` and `/apply`. `proposed_profile`, `to`, and
  `expected_profile_revision` become optional. Target resolution: named program, else named starter,
  else the active program, else the top recommended starter.
- **Apply rules**, in one transaction under the existing program and schedule locks:
  1. Save the profile only when `proposed_profile` is present.
  2. If the target is the active program and is already placed: change no session (FR-009).
  3. Otherwise: place the target if unplaced, make it the only active program, delete every planned
     unstarted session dated on or after the start date (FR-007), and fill from the start date.
- **Start date**: `max(from, today)` in the profile timezone, so no session is created in the past
  and a device a day behind the profile timezone is not rejected.
- **Active session**: Apply never refuses for an active session. The delete predicate already skips
  sessions with an active or completed workout session (FR-008). Regenerate keeps its existing
  409 for released apps.
- **Alternatives considered**: A new `/programs/{id}/start` route (a second way to start, and app
  spec 030 already uses the transition contract for stage switches).

## R7. Saving the active program

- **Decision**: `Replace` computes a per-unit diff inside its transaction and, when the program is
  active, applies it to planned unstarted sessions dated after today:
  - unit removed: delete its sessions;
  - unit weekdays changed: delete its sessions and insert the new slots through
    `scheduled_through`;
  - unit content changed, weekdays the same: replace the snapshot (name, sets or sport target) of
    its sessions in place;
  - unit added: insert its slots through `scheduled_through`;
  - unit unchanged: nothing, so one-off moves and deletions survive (FR-016).
- **Locks**: `Replace` takes the schedule lock after the program lock, the same order as
  plan-transition apply and regenerate.
- **Released app**: Its save carries no unit IDs or weekdays, so every unit is "removed" and the new
  units are unplaced. Its follow-up regenerate places them (R5) and fills its range; if that call
  never arrives, the next schedule read does. Either way the result is the same schedule.

## R8. Moving one session

- **Decision**: `POST /api/v1/scheduled-workouts/{id}/move` with `date`, `expected_revision`, and
  `operation_key`. Allowed only for a `planned` session with no workout session, dated today or
  later, to a date from today through `today + 56`. It updates `date`, bumps `revision`, recomputes
  participation for both dates as `Delete` does for one, and stores an idempotency record under
  scope `scheduled-workouts/move`.
- **Errors**: state conflicts reuse 409 `CONFLICT` with the session in `resource`; a bad date is 422
  `VALIDATION_ERROR` with `details.field = "date"`; foreign and missing IDs share 404.
- **Alternatives considered**: Extending `PATCH /scheduled-workouts/{id}` with `date` (that route
  replaces `required_sets` and rejects sport sessions; a move must work for both kinds).

## R9. Removing a saved program

- **Decision**: `DELETE /api/v1/programs/{id}` returns 204. The active program is 409 `CONFLICT`.
  The transaction deletes the program's planned unstarted sessions, then the program.
- **Evidence**: `scheduled_workouts.program_id`, `scheduled_workouts.program_workout_id`,
  `scheduled_sets.program_exercise_id`, and `criteria_training_blocks.program_id` are all
  `ON DELETE SET NULL`, so finished sessions and blocks survive. `legacy_adoptions.program_id` is
  `ON DELETE CASCADE`; removing an adopted program lets `adopt-legacy` create a new one, which is
  the existing behavior for an athlete with no adoption.
- **Idempotency**: A repeated delete of a removed program is 404, as for scheduled workouts.

## R10. How the app detects support

- **Decision**: Every program unit in every program response carries `weekdays` as an array (never
  omitted or `null`; `[]` when unplaced). Its presence is the signal. The transition preview always
  carries `off_availability` as an array.
- **Rationale**: Same rule as contract 1.6.0 detection. The API does not reject unknown request
  fields (no `DisallowUnknownFields` in `internal/`), so an app that sent `weekdays` to an older API
  would be silently ignored; the app must check before relying on it.

## R11. Legacy materialize and regenerate

- **Decision**: Both keep their request and response shapes. Internally they place the program if
  unplaced (R5), build sessions from the weekday layout instead of dealing, set `slot_date`, insert
  with `ON CONFLICT DO NOTHING`, and raise `scheduled_through` to at least their `to`.
- **Rationale**: This removes audit finding 6 (duplicates from materialize) and keeps a released
  app's follow-up calls harmless (spec edge case).

## R12. Numbers and versions

- **Decision**: Migration `045_program_weekdays_and_rolling_schedule` and contract `1.10.0`, both
  re-checked against `origin/main` in the first task. `main` holds migration 044 and contract 1.9.0
  on 2026-11-16. The horizon is one constant, `ScheduleHorizonDays = 56`.
