# Research: Skip a missed workout

All decisions trace to [spec.md](spec.md). Code references were mapped on 2026-10-11.

## R1. How a missed workout records its resolution

- **Decision**: Add `resolution TEXT NULL CHECK (resolution IN ('skipped','recovered'))` and
  `resolved_at TIMESTAMPTZ NULL` to `scheduled_workouts`, with a check that both are null or both are
  set.
- **Rationale**: One field covers both resolutions the spec defines (FR-003) and lets clients exclude
  resolved workouts with a single null check. It keeps `status` derivation untouched (FR-002), since
  `deriveScheduledStatus` (`internal/service/schedule_svc.go:491`), `scheduledOutcome`
  (`internal/dao/training_mutation_dao.go:990`), and `updateScheduledStatus`
  (`training_mutation_dao.go:781`) never read the new columns.
- **Alternatives considered**:
  - `skipped_at` only: cannot fix the recovery gap (R2), because recovered sources would still look
    unresolved.
  - `recovered_from_id` on the copy: links copies to sources but needs a self-join to answer
    "is this missed workout resolved", and still needs a skip marker.
  - A new `skipped` status: changes outcome semantics, participation, and streaks, which FR-002
    forbids.

## R2. Recovery selects the same missed workout repeatedly

- **Finding**: `RecoverIdempotent` selects
  `WHERE user_id=$1 AND date <= $2 AND status='missed' ORDER BY sequence_position NULLS LAST, date LIMIT 1 FOR UPDATE`
  (`internal/dao/schedule_dao.go:449`). Nothing marks the source after a copy is made, so a second
  recovery with a new operation key copies the same workout again. The non-atomic fallback filters
  the same way in Go (`schedule_svc.go:306-318`).
- **Decision**: Add `AND resolution IS NULL` to the selection and, in the same
  transaction, set the source's `resolution='recovered'`, `resolved_at=now()`,
  `revision=revision+1`. The response stays the new copy (contract unchanged apart from the
  documented side effect).
- **Fallback path**: `RecoverToday`'s non-atomic fallback runs only when the DAO lacks
  `IdempotentScheduleDAO`; production always has it. The fallback gets the unresolved filter only and is
  documented as legacy. Service tests use a fake that implements `RecoverIdempotent` (analysis I1).
- **Rationale**: Satisfies FR-004 and the existing contract wording "earliest unresolved missed
  sequence item" (`docs/CONTRACTS.md:423`) that the code did not implement.

## R3. Backfill for workouts recovered before this change (FR-012)

- **Rejected first design (analysis C1)**: matching on `program_workout_id` + `sequence_position` +
  later date. `sequence_position` is the program workout's own position and repeats every cycle
  (`internal/model/training_materialization.go:44-48`), so later weeks of the same workout would match
  and most historical misses would be wrongly and irreversibly marked recovered.
- **Decision**: Use recovery provenance. Each recovery stores an `idempotency_records` row with
  `scope='schedule/recover'` and `resource_id` = the copy (`schedule_svc.go:292`, `:342`); records are
  removed only on account deletion (`account_dao.go:46`). For each such record, the source is the
  earliest missed row (by `sequence_position NULLS LAST, date`) of the same user with the copy's
  `program_workout_id` and `sequence_position`, `date <= copy.date`, and
  `finalized_at <= record.created_at`, which is exactly the row the old selection returned. Mark it
  `recovered` with `resolved_at` = the earliest such record's `created_at`.
- **Risk**: Recoveries whose record or copy no longer exists (a planned copy deleted by regenerate or a
  plan transition) cannot be traced; those misses stay unresolved and remain skippable. Null
  `program_workout_id` values (deleted program workouts) are matched null-safely.
- **Alternatives considered**: No backfill (users skip old misses by hand); rejected by the product
  owner.

## R4. Endpoint shape

- **Decision**: `POST /api/v1/scheduled-workouts/{id}/skip` and
  `POST /api/v1/scheduled-workouts/{id}/unskip`, body `RevisionRequest`
  (`operation_key`, `expected_revision`, `internal/model/training.go:476`), `Idempotency-Key` header
  equal to `operation_key`. Response 200 is the full `ScheduledWorkout`.
- **Rationale**: Mirrors `POST /scheduled-workouts/{id}/complete` (`router.go:90`,
  `handler/training.go:402`, `schedule_svc.go:425`, `training_mutation_dao.go:1010`), which already
  provides the advisory lock, idempotency replay, ownership via `user_id`, and revision checks
  (FR-007, FR-008).
- **Alternatives considered**: `PATCH` with a `resolution` field (overloads the existing full-replace
  PATCH that rejects historical workouts at `schedule_svc.go:230`); `DELETE /skip` for undo (no body
  for revision and idempotency).

## R5. Error behavior

- **Decision**:
  - Skip on a workout whose outcome is not missed, or that is recovered → 409
    `RESOLUTION_CONFLICT` with the authoritative workout as `resource`.
  - Skip on an already skipped workout → 200, unchanged, no revision bump.
  - Unskip on a recovered workout → 409 `RESOLUTION_CONFLICT`.
  - Unskip on an unresolved workout → 200, unchanged.
  - Foreign or unknown id → 404 `NOT_FOUND`; stale revision → 409 `REVISION_CONFLICT`; reused key
    with a different body → 409 `IDEMPOTENCY_CONFLICT`; header mismatch → 422 `VALIDATION_ERROR`.
- **Rationale**: Reuses the existing mapping in `handleServiceError` (`internal/handler/response.go`)
  and adds one stable code (FR-006). Idempotent no-ops match the spec edge case for repeated skips.
- **Implementation note**: Add a typed `ResolutionConflictError` rather than matching on message text.

## R5a. Resolution after corrections (FR-013)

- **Finding**: `PutRequiredSet` accepts sets on finalized workouts and `updateScheduledStatus`
  re-derives a finalized row's outcome (`training_mutation_dao.go:781-798`, `:809-888`).
- **Decision**: In the same statement that re-derives status, clear `resolution` and `resolved_at`
  when the new status is not `missed`.

## R5b. Concurrency

- Skip, unskip, and recover all take the per-user schedule advisory lock
  (`training_mutation_dao.go:474`, `schedule_dao.go:439`), so they serialize. Recover bumps the source
  revision; a later skip with the old revision gets `REVISION_CONFLICT`, and with the new revision gets
  `RESOLUTION_CONFLICT`.

## R6. Preservation across plan changes (FR-009)

- **Finding**: Regenerate (`schedule_dao.go:417`), `DeleteUnstartedRange` (`schedule_dao.go:552`),
  and transition apply (`plan_transition_dao.go:86`) delete only
  `status='planned' AND finalized_at IS NULL` rows. Missed rows are finalized, so they and their
  resolution are never deleted or updated.
- **Decision**: No code change; add an integration test that proves a skip survives regenerate and
  transition apply.

## R7. Compatibility and rollout (FR-010, FR-011)

- **Decision**: Additive JSON fields `resolution` and `resolved_at` (`omitempty` like `finalized_at`)
  on every `ScheduledWorkout`. Older clients ignore them. The only behavior change for older clients
  is that recovery no longer re-copies a resolved workout, which is the documented contract.
- **Rollout**: Deploy API (migration runs on startup, `cmd/server/main.go:251`) before the app
  release that calls skip/unskip. The down migration drops both columns.

## R8. Required artifacts that CI enforces

- `docs/CONTRACTS.md`: shape, recover side effect, new routes, error code (contract drift gate in
  `.github/workflows/ci.yml`).
- `docs/DATABASE_DESIGN.md:700-725`: new columns (`scripts/validate-database-design.sh`).
- Swagger: regenerate `docs/docs.go`, `docs/swagger.json`, `docs/swagger.yaml` with swag v1.16.4.
- Coverage ≥ 90% for service, handler, router, and middleware packages.
