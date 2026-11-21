# Feature Specification: A planned week off

**Feature Branch**: `feat/038-planned-week-off`

**Feature ID**: `gympulse-planned-week-off`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/047-planned-week-off/spec.md)

**Created**: 2026-11-20

**Status**: Approved 2026-11-20 (owner: "Yes, week off", after seeing the design)

**Input**: The owner asked to make the weeks-on-target streak rewarding and reviewed how Duolingo
does it. Duolingo's Streak Freeze gives a learner slack so one missed day does not end a long
streak, and it raised daily activity. GymPulse's streak is weekly and repairable, but a holiday, a
deload, or an illness still ends it, and that week's planned workouts turn into misses that each
ask for a choice. The owner approved a week the athlete can mark as off.

Governing product rules: workspace `docs/PRODUCT.md`, "Consistency is weekly and repairable",
"Missed work waits for a choice", and "Do not use shame, guilt, or fabricated urgency". This feature
adds to them: a week the athlete took off neither counts toward nor ends the streak, and its
untouched workouts do not wait for a choice.

## Decisions

- **A week off is a stored fact, one row per athlete and Monday.** Rejected: deleting or moving the
  week's scheduled rows. The schedule fill never recreates a slot, the athlete may still train that
  week, and history must stay explainable.
- **Untouched workouts in the week become skipped, not moved.** Programs repeat on weekdays and have
  no stored position, so there is nothing to shift. A skipped workout is the existing "the athlete
  decided not to do this" state, and `unskip` already undoes it.
- **No participation row for an untouched day in the week.** The day is neutral, like a rest day,
  so nothing downstream treats it as a missed opportunity.
- **Idempotent by the week's Monday.** There is one possible row per week and no payload, so the
  routes need no operation key, ledger entry, or revision. Rejected: a POST with an operation key,
  which would add a ledger scope for no behaviour.
- **`GET /stats` is unchanged.** The app does not read its streak fields. Teaching the stats streak
  about weeks off is left out until a client needs it.
- **The streak rule lives in the app**, where the weeks-on-target streak is already computed from
  participation. The API only supplies the weeks.

## User journeys

1. **Take next week off.** The athlete marks next week off. Its planned workouts stay on the
   schedule. When each day passes untouched it is recorded as skipped, and no miss waits for a
   choice the week after.
2. **Take this week off, late.** On Thursday the athlete marks the current week off. Monday's
   unresolved miss becomes skipped at once.
3. **Train anyway.** The athlete does a workout during the week off. It is finalized and counted
   like any other workout.
4. **End it early.** The athlete ends the week off. The rest of the week is ordinary again; days
   already skipped stay skipped and can be unskipped one by one.

## Requirements

- **FR-001**: `GET /api/v1/weeks-off?from&to` MUST return the athlete's weeks off whose Monday is in
  the range, oldest first, as `{ "weeks_off": [{ "week_start", "created_at" }] }`, with `[]` when
  there are none. The range is at most 731 inclusive days.
- **FR-002**: `PUT /api/v1/weeks-off/{week_start}` MUST mark the week off and return it with 200. A
  week already off MUST return 200 unchanged.
- **FR-003**: `week_start` MUST be a `YYYY-MM-DD` Monday, not before the athlete's current week,
  and at most 8 weeks after it. Otherwise 422 `VALIDATION_ERROR` with `field: "week_start"`. The
  current week is judged in the training profile's timezone, or UTC without a profile.
- **FR-004**: Taking a week off MUST, in the same transaction, set `resolution: "skipped"` on that
  week's `missed` workouts that have no resolution, and leave every other workout alone.
- **FR-005**: `DELETE /api/v1/weeks-off/{week_start}` MUST end the week off with 204, also when the
  week was not off. A week before the current week MUST be refused with 422. Workouts already
  skipped stay skipped.
- **FR-006**: When a scheduled workout in a week off is finalized with nothing performed, it MUST
  become `status: "missed"` with `resolution: "skipped"` and `resolved_at` set, and MUST NOT write a
  participation row. A workout with anything performed MUST be finalized exactly as before.
- **FR-007**: Weeks off MUST be private to the authenticated athlete and removed with the account.
- **FR-008**: The change MUST be additive: no existing route, field, or stored row changes for an
  athlete who never takes a week off.

## Non-goals

- No reason for the week (holiday, deload, illness), no partial weeks, and no recurring weeks off.
- No change to `GET /stats`, the schedule fill, program content, or training blocks. A training
  block's dated stages keep their dates through a week off.
- No server-side reminders exist, so there is nothing to cancel on the server.

## Compatibility and rollout

Additive (contract 1.17.0). Deploy the API first; migration 051 creates one empty table. An older
app never calls the routes. A newer app against an older API gets a 404 without the JSON envelope
and hides the feature.

## Acceptance

| Requirement | Evidence |
| --- | --- |
| FR-001 | `TestWeekOffList`, `TestWeekOffHandlerTakeListEnd`, `TestWeekOffPutListDelete` |
| FR-002 | `TestWeekOffTakeIsIdempotent`, `TestWeekOffHandlerTakeListEnd`, `TestWeekOffPutListDelete` |
| FR-003 | `TestWeekOffTake`, `TestWeekOffCurrentWeekFollowsTheSavedTimezone`, `TestWeekOffHandlerErrors`, `TestWeekOffPutListDelete` (the Monday check) |
| FR-004 | `TestWeekOffPutSkipsTheWeeksUnresolvedMisses` |
| FR-005 | `TestWeekOffEnd`, `TestWeekOffHandlerTakeListEnd`, `TestWeekOffHandlerErrors` |
| FR-006 | `TestWeekOffFinalizesUntouchedWorkoutsAsSkipped` |
| FR-007 | `account_dao.go` delete list, `supabase_security_migration_test.go` (45 tables), `validate-supabase-security.sh` |
| FR-008 | Full unit and integration suites unchanged apart from the new tests |
