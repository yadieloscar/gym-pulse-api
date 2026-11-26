# Feature Specification: Exercise notes

**Feature Branch**: `feat/040-exercise-notes`

**Feature ID**: `gympulse-exercise-notes`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/052-exercise-notes/spec.md)

**Created**: 2026-11-25

**Status**: Approved in shape 2026-11-25 (owner picked "exercise notes" from a feature brainstorm
and asked for it to be built; the decisions below are recorded assumptions, open to review on the PR)

**Input**: The athlete wants to keep one short private note per exercise, such as "seat height 4"
or "narrow grip". The note shows on that exercise's screen in the workout player every time the
exercise comes up, in planned and off-plan workouts, and is added or edited there in place. It is
the athlete's own text, private to the account, and it works offline like the rest of the player.

Governing product rules: workspace `docs/PRODUCT.md`, "User ownership", "Privacy by design",
"One way to train", "Progressive disclosure", and "The gym works offline".

## What exists today

Three other "notes" exist and none of them is this feature:

- `exercises.notes`, `program_exercises.notes`, and `scheduled_sets.notes` are **prescription
  notes**: written while building a saved workout or program, copied into one workout, and shown
  read-only in the player. A note written in one saved workout does not follow the exercise into a
  program or another saved workout, and it cannot be edited while training.
- `exercise_overrides.notes` is a legacy note for one exercise on one day.
- `workout_sessions.notes` and `day_logs.session_notes` describe a whole workout.

This feature adds a note that belongs to the **exercise itself**, across every workout.

## Decisions

- **An exercise is identified by its name.** The key is the name trimmed, with inner whitespace
  collapsed to one space, lowercased. The same movement appears as different rows in saved
  workouts, programs, and scheduled workouts, some with a catalog id and some without; the name is
  the one thing all of them carry, and it is how the app already matches "last time" for an
  exercise without a catalog id. Rejected: keying by catalog id with a name fallback, which gives
  one exercise two keys when one copy has a catalog id and another does not. Consequence: renaming
  an exercise starts a fresh note, and two exercises with the same name share one.
- **One row per athlete and exercise key, last write wins.** There is one possible row and the
  whole value is replaced, so the route is idempotent by its natural key and needs no operation
  key, ledger entry, or revision (the choice spec 038 made for weeks off). Consequence, accepted:
  when two devices edit the same note, the save that reaches the server last wins, even if it was
  typed first on a phone that was offline.
- **Clearing is saving an empty note.** One route covers save, edit, and clear, so a queued
  offline change is always the same request. Rejected: `DELETE` with the name in the path or
  query, which puts athlete-chosen text in URLs and access logs.
- **Short on purpose.** A note is 1 to 200 characters. It is a reminder, not a journal.
- **At most 500 notes per athlete**, so the list stays one bounded read.
- **Notes are part of the data export.** They are text the athlete authored.

## User journeys

1. **Add a note.** The athlete saves "seat height 4" for "Leg press". Every later read returns it.
2. **Edit it.** They save "seat height 5". The stored note is replaced.
3. **Clear it.** They save an empty note. The note is gone from the list.
4. **Save it again from an offline queue.** The same save sent twice leaves one note.
5. **Another athlete.** A second athlete with a "Leg press" of their own never sees the first
   athlete's note and cannot change it.

## Requirements

- **FR-001**: `GET /api/v1/exercise-notes` MUST return all of the athlete's notes as
  `{ "exercise_notes": [{ "exercise_key", "exercise_name", "note", "updated_at" }] }`, ordered by
  `exercise_key`, with `[]` when there are none.
- **FR-002**: `PUT /api/v1/exercise-notes` with `{ "exercise_name", "note" }` MUST save the note
  for that exercise, replacing any earlier one, and return it with 200. Repeating the request MUST
  leave exactly one note with the same text.
- **FR-003**: `exercise_name` MUST be 1 to 120 characters after trimming and collapsing
  whitespace, otherwise 422 `VALIDATION_ERROR` with `field: "exercise_name"`. Names that differ
  only by case or spacing MUST address the same note. The stored `exercise_name` is the name last
  saved.
- **FR-004**: `note` is trimmed. More than 200 characters MUST return 422 `VALIDATION_ERROR` with
  `field: "note"`.
- **FR-005**: An empty `note` (after trimming) MUST delete the athlete's note for that exercise
  and return 200 with `"note": ""`, also when there was no note.
- **FR-006**: Saving a note for a new exercise when the athlete already has 500 MUST return 422
  `VALIDATION_ERROR` with `field: "exercise_name"`. Editing or clearing an existing note MUST
  still work.
- **FR-007**: Notes MUST be private to the authenticated athlete: every read and write is scoped
  by the authenticated user id, and the table is removed with the account.
- **FR-008**: `GET /api/v1/export` MUST include `exercise_notes` as
  `[{ "exercise_name", "note", "updated_at" }]`, `[]` when there are none.
- **FR-009**: The change MUST be additive: no existing route, field, or stored row changes.
- **FR-010**: Note text MUST NOT be written to logs.

## Non-goals

- No change to prescription notes, exercise overrides, or workout notes.
- No per-set or per-day notes, no history of earlier note text, no formatting, no photos.
- No note on the shared exercise catalog; a note is never visible to another athlete.
- No merge of two devices' edits.

## Compatibility and rollout

Additive (contract 1.19.0). Deploy the API first; migration 052 creates one empty table. An older
app never calls the routes. A newer app against an older API gets a 404 without the JSON envelope:
it keeps notes on the device and sends them once the routes exist.

## Acceptance

| Requirement | Evidence |
| --- | --- |
| FR-001 | `TestExerciseNoteList`, `TestExerciseNoteHandlerSaveListClear`, `TestExerciseNoteSaveEditClear` |
| FR-002 | `TestExerciseNoteSave`, `TestExerciseNoteEditReplacesAndRepeatsSafely`, `TestExerciseNoteHandlerSaveListClear`, `TestExerciseNoteSaveEditClear` |
| FR-003 | `TestExerciseNoteSave`, `TestExerciseNoteKey`, `TestExerciseNoteHandlerErrors` |
| FR-004 | `TestExerciseNoteSave`, `TestExerciseNoteHandlerErrors`, `TestExerciseNoteChecks` |
| FR-005 | `TestExerciseNoteClear`, `TestExerciseNoteHandlerSaveListClear`, `TestExerciseNoteSaveEditClear` |
| FR-006 | `TestExerciseNoteLimit` (service and DAO) |
| FR-007 | `TestExerciseNotesArePrivateToTheirOwner`, `TestExerciseNoteHandlerScopesToTheAuthenticatedAthlete`, `TestExerciseNoteList`, `account_dao.go` delete list, `supabase_security_migration_test.go` (46 tables), `validate-supabase-security.sh` |
| FR-008 | `TestExportReadsEveryOwnedSectionAndNothingElse`, `TestExportOfNewAthleteIsEmptyArrays`, `TestExportDAOReadsOneOwnedReadOnlySnapshot`, `export_test.go` |
| FR-009 | Full unit and integration suites unchanged apart from the new tests |
| FR-010 | Review: the handler, service, and DAO log nothing; `TestExerciseNoteHandlerErrors` checks an error body never repeats the note |
