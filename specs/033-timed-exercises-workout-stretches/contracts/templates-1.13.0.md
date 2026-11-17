# Contract draft: templates, 1.13.0 (additive)

The published contract is `docs/CONTRACTS.md`, section Templates. This file records the delta.

```json
{
  "name": "Legs", "type_id": "legs", "subtype_id": "general",
  "warmup_routine_id": "warmup-lower",
  "cooldown_routine_id": null,
  "exercises": [
    { "name": "Back squat", "sets": 3, "reps": 5 },
    { "name": "Couch stretch", "sets": 2, "duration_seconds": 30 }
  ]
}
```

| Field | Rule |
| --- | --- |
| `exercises[].duration_seconds` | 1..3600, seconds per set; requires `sets`; never with `reps` or `duration_minutes` |
| `warmup_routine_id`, `cooldown_routine_id` | `null` or a 1..64 slug; always present in responses |
| `PUT` | an absent routine key keeps the stored value; `null` or `""` clears it |

Errors: 422 `VALIDATION_ERROR`, `details.field` = `exercises`, `warmup_routine_id`, or `cooldown_routine_id`.
