# Research: Training Blocks Linked to Real Training

**Feature**: `gympulse-training-block-program-link` · **Spec**: [spec.md](spec.md) · **Date**: 2026-11-14

Every decision below was checked against `main` at planning time (merge `5c46fd5`, migrations
through 034, contract 1.5.0).

## R1 — Feature detection (spec amendment)

- **Decision**: The app detects support from response fields, not from a status code. On a
  supporting API every block summary carries `pending_next_morning` (always an array) and every
  aggregate carries `exposure_suggestions` (always an array). The app calls the due read only after
  it has seen one of those fields. An athlete with no blocks has nothing to show, so no probe is
  needed.
- **Rationale**: The spec said an older API returns a plain 404 for the due read. It does not.
  `GET /api/v1/training-blocks/due` on an older API matches `/{block_id}`, and `pathUUID`
  (`internal/handler/training.go:604`) answers `400 BAD_REQUEST "invalid block_id"`. Field presence
  is unambiguous because constitution I requires promised collections to be `[]`, never omitted.
- **Alternatives**: a top-level route such as `/api/v1/training-block-attention`, which an older API
  would answer with a plain chi 404 (rejected: it splits one resource across two prefixes); treating
  `400` with `block_id` as "unsupported" (rejected: it couples the app to an error message).
- **Spec change**: FR-015 and the 2026-11-07 detection clarification are corrected in this plan
  change; the sibling app spec FR-011 needs the same correction (see plan, Cross-repo follow-ups).

## R2 — Stage window without a backfill

- **Decision**: Add `criteria_training_stages.program_linked_at TIMESTAMPTZ NOT NULL DEFAULT now()`.
  Derive the window start at read time as
  `GREATEST(stage.program_linked_at, COALESCE(latest transition into the current stage, block.created_at))`,
  where "latest transition into the current stage" is `max(created_at)` of transitions whose
  `to_stage_id` is the current stage. Compare it with `scheduled_workouts.finalized_at`.
- **Rationale**: Existing stages receive the migration time as their link time, which is exactly
  the owner's rule "only workouts finished after the link took effect": sessions recorded by hand
  before rollout never resurface. New stages get creation time. No data rewrite; transitions already
  store `created_at`. Advance, regress, and re-advance all resolve to the latest entry.
- **Alternatives**: a `current_stage_started_at` column on blocks (rejected: needs a backfill from
  transitions and one more write path to keep consistent); date comparison (rejected by the spec).

## R3 — Deleting a referenced workout

- **Decision**: Foreign keys do the work. `criteria_training_exposures.scheduled_workout_id`
  references `scheduled_workouts(id) ON DELETE SET NULL`; the dismissal table references it
  `ON DELETE CASCADE`. `scheduleDAO.Delete` (`internal/dao/schedule_dao.go:629`) needs no code
  change; an integration test proves FR-013.
- **Race**: an exposure insert that references a workout deleted in a concurrent transaction fails
  with `23503`; the DAO maps it to the shared 404.

## R4 — Error codes

- **Decision**: State conflicts reuse 409 `CONFLICT` with `resource` via the existing
  `model.ConflictError{Authoritative: block}` path (the DAO already does this for read-only blocks).
  A duplicate reference returns 409 `EXPOSURE_SOURCE_ALREADY_RECORDED` with `resource`, raised from
  an explicit check under the block row lock and, as a backstop, from unique-violation `23505` on
  the partial unique index.
- **Rationale**: one new code only where the app needs a distinct message ("Already counted").

## R5 — One eligibility predicate

- **Decision**: One DAO query builder produces suggestions. The list reads it for the current stage;
  create-exposure and dismiss call it with a workout-ID filter inside the locked transaction and
  answer 422 `VALIDATION_ERROR` (`details.field = "scheduled_workout_id"`) when the workout is owned
  but not suggestible, 404 when it is missing or foreign.
- **Rationale**: suggestion list and mutation validation cannot drift.

## R6 — Indexes

- **Decision**:
  - `idx_scheduled_workouts_block_suggestions ON scheduled_workouts (user_id, program_id, finalized_at)
    WHERE kind = 'exercises' AND status IN ('completed','incomplete')`.
  - Unique partial `ux_criteria_training_exposures_block_source ON criteria_training_exposures
    (block_id, scheduled_workout_id) WHERE scheduled_workout_id IS NOT NULL`.
  - Dismissal primary key `(block_id, scheduled_workout_id)`.
- **Rationale**: every suggestion read is owner + program + time range; anti-joins hit the unique
  index and the dismissal key. `CREATE INDEX` without `CONCURRENTLY` matches the migration runner,
  which applies each file in a transaction; `scheduled_workouts` is per-athlete small.

## R7 — Due read shape and cost

- **Decision**: `GET /api/v1/training-blocks/due` returns `{ "due_checks": [...], "suggestions": [...] }`,
  each at most 20, oldest first, active blocks only. Two SQL statements: one join for due checks;
  one `LATERAL` over the athlete's active blocks for suggestions using the R5 predicate.
- **Rationale**: one request for Today (SC-005); bounded; no N+1 round trips.

## R8 — Idempotency scopes

- **Decision**: new scopes `training_block_stage_program` and `training_block_dismiss`; the request
  hash includes the path identifiers (block, stage or workout) so the same key on a different target
  is an `IDEMPOTENCY_CONFLICT`. Create-exposure keeps its scope; `scheduled_workout_id` joins its hash.

## R9 — Prefill

- **Decision**: The model derives prefill from the workout and stage: `performed_on` = scheduled
  date, `activity_label` = workout name truncated to 120 characters, `load_level` = stage load level,
  `session_outcome` = `completed_as_planned` for `completed` and `modified` for `incomplete`.

## R10 — Numbers

- **Decision**: Migration uses the next free number when implementation starts (main holds 034 and
  an open workout-imports branch also uses 034, so expect 035 or 036). Contract version is the next
  free minor (1.6.0 unless workout imports claims it first). Both are confirmed in task T001.

## R11 — Stage program on summaries

- **Decision**: `current_stage` in list summaries gains `program_id` and `effective_program_id`, so
  the app can compute a switch offer from the list without loading every aggregate.
