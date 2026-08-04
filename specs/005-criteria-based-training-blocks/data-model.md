# Data Model: Criteria-Based Training Blocks

## `criteria_training_blocks`

| Field | Type | Rules |
|---|---|---|
| `id` | UUID | primary key |
| `user_id` | UUID | required, `auth.users(id) ON DELETE CASCADE` |
| `program_id` | UUID | optional, `programs(id) ON DELETE SET NULL` |
| `name` | VARCHAR(120) | trimmed, nonblank |
| `purpose` | VARCHAR(500) | optional |
| `status` | VARCHAR(16) | `active`, `completed`, `archived` |
| `current_stage_order` | SMALLINT | 1–12 |
| `revision` | BIGINT | starts at 1, increments once per mutation |
| timestamps | TIMESTAMPTZ | created/updated |

Indexes cover `(user_id, status, updated_at DESC, id DESC)` and optional program lookup.

## `criteria_training_stages`

| Field | Type | Rules |
|---|---|---|
| `id` | UUID | primary key |
| `block_id` | UUID | required, block cascade |
| `stage_order` | SMALLINT | 1–12, unique per block |
| `name` | VARCHAR(120) | trimmed, nonblank |
| `instructions` | VARCHAR(1000) | optional |
| `load_level` | VARCHAR(16) | `easy`, `demanding` |
| `target_count` | INTEGER | optional, 1–10,000 |
| `target_duration_minutes` | SMALLINT | optional, 1–1,440 |
| `target_intensity_percent` | SMALLINT | optional, 1–100 |
| `required_qualifying_exposures` | SMALLINT | 1–20 |

Stage order is contiguous and definitions are immutable after aggregate creation.

## `criteria_training_exposures`

| Field | Type | Rules |
|---|---|---|
| `id` | UUID | primary key |
| `block_id`, `stage_id` | UUID | required, cascade |
| `performed_on` | DATE | not future in profile timezone |
| `activity_label` | VARCHAR(120) | trimmed, nonblank |
| `load_level` | VARCHAR(16) | `easy`, `demanding` |
| performed targets | INTEGER/SMALLINT | same optional bounds as stages |
| `session_outcome` | VARCHAR(24) | `completed_as_planned`, `modified`, `stopped` |
| `next_morning_response` | VARCHAR(24) | optional, `baseline`, `above_baseline` |
| `notes` | VARCHAR(1000) | optional |
| timestamps | TIMESTAMPTZ | created and response update |

`qualifies` is derived as completed-as-planned plus baseline; it is not stored or client writable. A response changes from null exactly once.

## `criteria_training_transitions`

| Field | Type | Rules |
|---|---|---|
| `id` | UUID | primary key |
| `block_id` | UUID | required, cascade |
| `action` | VARCHAR(16) | `advance`, `regress`, `complete`, `archive` |
| `from_stage_id` | UUID | optional only where state has no stage |
| `to_stage_id` | UUID | advance/regress destination, otherwise null |
| `reason` | VARCHAR(500) | required for regress, optional otherwise |
| `created_at` | TIMESTAMPTZ | append-only event time |

## Aggregate invariants

- One block has 2–12 contiguous stages and exactly one current stage while active or completed.
- Exposures may be added only to the current stage of an active block.
- Advance targets only the immediate next stage and requires sufficient current-stage qualifications.
- Regress targets any earlier stage and requires a reason.
- Complete requires the final stage and sufficient qualifications. Archive preserves current stage and history.
- All nested reads derive ownership from the block; missing and foreign identifiers share the same not-found behavior.
- Account deletion cascades all four tables. Program deletion only clears the optional association.
