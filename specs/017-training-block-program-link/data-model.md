# Data Model: Training Blocks Linked to Real Training

**Spec**: [spec.md](spec.md) · **Research**: [research.md](research.md)

All changes are additive to migration 019 (`criteria_training_*`) and migration 012
(`scheduled_workouts`). No existing row is rewritten.

## Schema changes

### `criteria_training_stages` (changed)

| Column | Type | Rule |
| --- | --- | --- |
| `program_id` | `UUID NULL REFERENCES programs(id) ON DELETE SET NULL` | Owned program this stage runs. Ownership checked in the DAO. |
| `program_linked_at` | `TIMESTAMPTZ NOT NULL DEFAULT now()` | When the stage's effective link last changed. Existing rows get migration time (R2). |

### `criteria_training_exposures` (changed)

| Column | Type | Rule |
| --- | --- | --- |
| `scheduled_workout_id` | `UUID NULL REFERENCES scheduled_workouts(id) ON DELETE SET NULL` | Workout this exposure records. |

Index: unique partial `(block_id, scheduled_workout_id) WHERE scheduled_workout_id IS NOT NULL`.

### `criteria_training_suggestion_dismissals` (new)

| Column | Type | Rule |
| --- | --- | --- |
| `block_id` | `UUID NOT NULL REFERENCES criteria_training_blocks(id) ON DELETE CASCADE` | |
| `scheduled_workout_id` | `UUID NOT NULL REFERENCES scheduled_workouts(id) ON DELETE CASCADE` | |
| `created_at` | `TIMESTAMPTZ NOT NULL DEFAULT now()` | |

Primary key `(block_id, scheduled_workout_id)`. RLS enabled, no client-role privileges (as 019).

### `scheduled_workouts` (index only)

Partial index `(user_id, program_id, finalized_at) WHERE kind = 'exercises' AND status IN
('completed','incomplete')`.

### Down migration

Drops the dismissal table, the two indexes, and the three added columns. Exposures recorded through a
suggestion keep their facts; only the reference is lost.

## Derived values

| Value | Rule |
| --- | --- |
| `effective_program_id` | `stage.program_id`, else `block.program_id`, else null. |
| Stage window start | `GREATEST(stage.program_linked_at, COALESCE(max(transition.created_at) WHERE to_stage_id = current stage, block.created_at))`. |
| Suggestion | Owned `scheduled_workouts` row with `kind = 'exercises'`, `status IN ('completed','incomplete')`, `program_id = effective_program_id`, `finalized_at > window start`, and no exposure or dismissal in this block for it. Current stage of an active block only. |
| Prefill | `performed_on` = `date`; `activity_label` = `name` (≤120); `load_level` = stage load level; `session_outcome` = `completed_as_planned` or `modified`. |
| Due check | Exposure in an active block with null `next_morning_response` and `performed_on` < today in the training-profile timezone. |

## Model changes (`internal/model/training_block.go`)

- `TrainingStage`: `ProgramID *uuid.UUID`, `EffectiveProgramID *uuid.UUID`, `ProgramLinkedAt time.Time`.
- `TrainingExposure`: `ScheduledWorkoutID *uuid.UUID` (`omitempty`).
- `TrainingBlockSummary`: `PendingNextMorning []TrainingPendingCheck` (always `[]`).
- `TrainingBlock`: `ExposureSuggestions []TrainingExposureSuggestion` (always `[]`).
- New `TrainingExposureSuggestion`: `ScheduledWorkoutID`, `Date`, `Name`, `Status`, `FinalizedAt`,
  `Prefill` (`performed_on`, `activity_label`, `load_level`, `session_outcome`).
- New `TrainingPendingCheck`: `ExposureID`, `PerformedOn`, `ActivityLabel`, `StageName`.
- New `TrainingBlocksDue`: `DueChecks []TrainingDueCheck`, `Suggestions []TrainingDueSuggestion`, each
  entry adding `BlockID`, `BlockName`, `BlockRevision`, `StageName` to the shapes above.
- Requests: `CreateTrainingStageRequest.ProgramID *uuid.UUID`;
  `CreateTrainingExposureRequest.ScheduledWorkoutID *uuid.UUID`; new `SetStageProgramRequest`
  (`ProgramID *uuid.UUID`, `ExpectedRevision`, `OperationKey`) and `DismissSuggestionRequest`
  (`ExpectedRevision`, `OperationKey`).

## State rules

| Mutation | Allowed when | Otherwise |
| --- | --- | --- |
| Set stage program | Block active; stage order ≥ current stage order | 409 `CONFLICT` with `resource` |
| Create exposure with reference | Workout is a current suggestion | 422 `scheduled_workout_id`; 404 missing/foreign; 409 `EXPOSURE_SOURCE_ALREADY_RECORDED` if already referenced |
| Dismiss | Workout is a current suggestion | 422 `scheduled_workout_id`; 404 missing/foreign |

Every mutation: `Idempotency-Key` = `operation_key`, `expected_revision` checked first, block row
locked `FOR UPDATE`, revision incremented once, full aggregate returned, replay stored in the same
transaction.
