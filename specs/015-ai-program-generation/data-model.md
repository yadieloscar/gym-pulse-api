# Data Model: Generate a Program Draft

## `program_generations` (migration 031)

| Field | Type | Rules |
|---|---|---|
| `id` | UUID | Primary key, application-generated |
| `user_id` | UUID | Not null, `auth.users(id)` cascade delete |
| `local_date` | DATE | Not null; the athlete's civil date in the profile timezone when reserved |
| `status` | TEXT | `CHECK (status IN ('pending', 'succeeded', 'failed'))`, default `pending` |
| `model` | TEXT | Not null; the configured model id at request time |
| `created_at` | TIMESTAMPTZ | Not null, `now()` |
| `completed_at` | TIMESTAMPTZ | Null until the provider answers |

Index `idx_program_generations_user_date(user_id, local_date, status)` serves the daily count.
RLS enabled, no direct-client policies, Data API role privileges revoked, runtime role granted
DML with the `gym_pulse_api_access` policy (the post-026 pattern from migrations 029 and 030).

No draft content is stored (spec non-goal); the row exists only for the daily limit and operational
counts of failures.

## Reservation transaction (DAO)

1. `pg_advisory_xact_lock(hashtext('program-generation:' || user_id))`.
2. Count rows for `(user_id, local_date)` where `status = 'succeeded'` or
   (`status = 'pending'` and `created_at > now() - interval '2 minutes'`).
3. If the count is at or above the limit, roll back and return `GenerationLimitError` carrying
   `resets_at` (the next local midnight in the profile timezone).
4. Otherwise insert a `pending` row and commit; return its id and `remaining = limit - count - 1`.

After the provider call the service calls `Complete(ctx, id, status)` which sets `status` and
`completed_at`. A `failed` row never counts; a `pending` row stops counting two minutes after
creation.

## Draft (response only)

`ProgramDraft` is the custom-program create shape without `operation_key`:

```json
{
  "name": "Three-day upper lower",
  "primary_goal": "hypertrophy",
  "roadmap": { "origin": "generated", "model": "claude-opus-5-5", "request": null },
  "workouts": [{
    "kind": "exercises", "name": "Upper A", "preferred_weekday": 1, "sequence_position": 1,
    "exercises": [{
      "catalog_id": "<uuid>", "name": "Barbell Bench Press", "category": "push",
      "modality": "strength", "exercise_order": 1, "target_sets": 4, "target_reps": 8,
      "target_weight": null, "target_duration_seconds": null, "rest_seconds": 120, "notes": null
    }]
  }]
}
```

Invariants enforced by `RepairGeneratedDraft` (spec FR-003, FR-004): 1 ≤ workouts ≤
`min(len(available_days), 6)`; 3 ≤ exercises ≤ 8 per workout; every `catalog_id` resolves and is
unique within a workout; `name`, `category`, `modality` copied from the catalog; strength has reps
1–50 and no duration, cardio has duration 30–3600 s and no reps; sets 1–10; rest 0–600 s;
`target_weight` null; `primary_goal` equals the profile's; `name` 1–80 runes.

## Generation input (provider request)

Only these leave the API: `primary_goal`, `available_days`, `usual_activity`, `experience`,
`equipment`, `session_duration_minutes`, the catalog rows (`id`, `name`, `category`, `modality`,
`mechanic`), the trimmed `request` text, and the JSON schema. No user id, timezone, email,
history, or other tables.
