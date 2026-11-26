# Feature Specification: Program recap

**Feature Branch**: `feat/039-program-recap` (named before the directory moved to 041; 039 and 040 went to features opened the same day)

**Feature ID**: `gympulse-program-recap`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/054-program-recap/spec.md)

**Created**: 2026-11-25

**Status**: Draft for owner review (the owner picked the feature from a brainstorm on 2026-11-25 and
asked for it to be built end to end; the decisions below are the drafting defaults)

**Input**: "When the athlete finishes a program, one page sums it up with the weeks on target out of
the program's weeks, the workouts done, and how each main lift's heaviest set moved from the start
of the program to the end." Planned weeks off must not count against the athlete, and a program left
early or with little logged work needs a calm, honest state.

Governing product rules: workspace `docs/PRODUCT.md`, "Progress answers two questions first",
"Consistency is weekly and repairable", "Records are sets the athlete actually lifted, never
estimates", and "Do not use shame, guilt, or fabricated urgency".

## What the code does today

Verified on `main` (254ac04) on 2026-11-25:

| # | Finding | Evidence |
| --- | --- | --- |
| 1 | A program never ends by itself. Its schedule rolls forward with no end date. | spec 031 D5, `internal/dao/schedule_fill.go` |
| 2 | A program stops only when the athlete starts another one: `plan-transitions/apply` sets `active=false` on it and removes its unstarted sessions from the start date on. Nothing else happens. | `internal/dao/plan_transition_dao.go` `activateTransitionTarget` |
| 3 | No start date, end date, or history of activations is stored. `programs` has `active`, `created_at`, `updated_at`. | `migrations/012`, `045` |
| 4 | `roadmap.weeks` is free-form: a number on seeded starters, text ("Weeks 1 to 4") on series phases, absent on custom programs. Nothing reads it on the server. | `migrations/013`, app `utils/starterProgramSeries.ts` |
| 5 | Each scheduled session keeps its `program_id`, date, and status; a set logged in a program session keeps the exercise name, not an exercise id. | `migrations/012`, `internal/dao/training_log_dao.go` `PutRequired` |
| 6 | Weeks on target is computed in the app from `GET /participation` and `GET /weeks-off`. The server has no such number. | app `utils/weeklyConsistency.ts` |

So there is no recap, no "program finished" event, and nothing to migrate: everything a recap needs
is already stored on the program's scheduled sessions.

## Decisions

- **"Finished" means the athlete started another program.** That is the only way a program ends in
  the code (finding 2). Rejected: ending a program when it reaches `roadmap.weeks`, because that
  field is free-form, absent on custom programs, and programs deliberately roll on (spec 031 D5).
- **The recap is computed, never stored.** One read derives it from the program's sessions,
  participation, weeks off, and logged sets. No migration, no write, nothing to backfill. A recap
  can also be read while the program is still active; it then covers the program so far.
- **The program's dates come from its sessions**: the first and the last session dated today or
  earlier. Rejected: a new `started_at` column, which would be empty for every existing program.
- **A week is judged the way the app already judges it**: Monday to Sunday, a trained day is a
  `participated` day, and the target is the number of weekdays the program trains on. Two additions
  make a program's edge weeks fair: the target of a week is never more than the days the program had
  a session in that week (a program started on a Thursday is not held to Monday), and only days from
  the program's first session to its last are counted.
- **Weeks that are not the athlete's fault are left out, not failed**: a week off that fell short, a
  week the program had no session in, and a week that is not over yet and is not on target yet.
  A week off that reached the target counts. The number of weeks off is never returned.
- **A lift is named, not identified**: program sets carry an exercise name only (finding 5), so
  lifts are grouped by name, ignoring case and outer spaces.
- **The main lifts are the up to five lifted on the most days** in the program. Each carries the
  heaviest set of the first day it was lifted and of the last day. Rejected: every lift, which would
  make a long page, and estimated maxes, which the product contract forbids.
- **Only "done" is counted for workouts.** The response has no missed or planned total, so no screen
  can show a shortfall.
- **One database round trip.** The API and the database are in different regions; the read is one
  pipelined batch.

## User journeys

1. **A full program.** The athlete followed a program for eight weeks and started the next one. The
   recap says how many of those weeks were on target, how many workouts were done, and where each
   main lift started and ended.
2. **A program left early.** The athlete started another program after a week and a half. The recap
   returns the small true numbers; the app shows them plainly.
3. **A week off inside the program.** The week off is not one of the program's weeks unless the
   athlete reached the target in it anyway.
4. **Nothing lifted.** A program of sport sessions, or one with no weighted sets logged, returns an
   empty `lifts` list and still returns weeks and workouts.
5. **Nothing done at all.** A program started and replaced before any workout returns zeros and no
   dates.

## Requirements

- **FR-001**: `GET /api/v1/programs/{id}/recap` MUST return the `ProgramRecap` of a program the
  authenticated athlete owns, active or not. A missing, foreign, or deleted program MUST be 404
  `NOT_FOUND`; a malformed id MUST be handled as the other `/programs/{id}` routes handle it.
- **FR-002**: `first_date` and `last_date` MUST be the dates of the program's first and last
  scheduled session dated today or earlier, or both `null` when it has none. "Today" is judged in
  the training profile's timezone, or UTC without one or when it cannot be loaded.
- **FR-003**: `weeks.total` MUST count each Monday-to-Sunday week from `first_date` to `last_date`
  in which the program had at least one session, except (a) a week off that is not on target and
  (b) a week whose Sunday is after today and that is not on target.
- **FR-004**: A week is on target when its trained days are at least its target. Trained days are
  the athlete's `participated` days in that week that fall on or between the program's first and
  last session dates. The target is the number of distinct weekdays the program's units sit on,
  capped at the number of distinct dates the program had a session in that week; a program with no
  weekdays uses that number of dates. `weeks.on_target` MUST count the on-target weeks among
  `weeks.total`.
- **FR-005**: `workouts_done` MUST count the program's sessions whose status is `completed` or
  `incomplete`, exercise workouts and sport sessions alike. Missed, skipped, and planned sessions
  are not counted and not returned.
- **FR-006**: `lifts` MUST hold at most five lifts: those with a completed set with a weight above
  zero and positive reps in a non-discarded workout session of this program, ranked by the number of
  days lifted, then by name. Each has `name`, `days`, and `first` and `last`: the heaviest such set
  (then more reps) on the first and on the last day it was lifted, each with `weight`, `reps`, and
  `date`. A lift done on one day has equal `first` and `last`. The list is `[]`, never `null`.
- **FR-007**: The route MUST be read-only (no idempotency key, no revision, no write) and MUST use
  one database round trip.
- **FR-008**: The change MUST be additive (contract 1.20.0) and need no migration. An older server
  answers 404 without the JSON error envelope, which clients read as "not available yet".
- **FR-009**: Another athlete's sessions, participation, weeks off, and sets MUST never be read.

## Non-goals

- No stored recap, no "program finished" state, no end date on a program, and no change to how a
  program is started, replaced, or deleted. A deleted program has no recap.
- No comparison between programs, no volume, no estimated max, no body weight, no nutrition.
- No recap for training blocks or for a multi-phase series as a whole; each phase is a program.
- Sets logged off-plan while the program was active are not part of its lifts.

## Compatibility and rollout

Additive. Deploy the API first. An older app never calls the route. A newer app against an older API
gets a 404 without the JSON envelope and hides the recap.

## Acceptance

| Requirement | Evidence |
| --- | --- |
| FR-001 | `TestProgramRecapHandler`, `TestProgramRecapHandlerErrors`, `TestProgramRecapNotFound`, `TestProgramRecapReadIsOwned` |
| FR-002 | `TestBuildProgramRecap/no_sessions`, `TestBuildProgramRecap/only_sessions_still_to_come`, `TestProgramRecapTodayFollowsTheSavedTimezone` |
| FR-003, FR-004 | `TestBuildProgramRecap` (full program, partly done, week off, unfinished week, edge weeks) |
| FR-005 | `TestBuildProgramRecap/full_program`, `TestProgramRecapRead` |
| FR-006 | `TestBuildProgramRecap/no_logged_lifts`, `TestBuildProgramRecapMainLifts`, `TestProgramRecapRead` |
| FR-007 | `program_recap_dao.go` (one `SendBatch`), `TestProgramRecapRoundTrips` |
| FR-008 | `training_routes_test.go`, `docs/CONTRACTS.md` 1.20.0, smoke step 8f |
| FR-009 | `TestProgramRecapReadIsOwned` |
