# Contract delta: missed-workout resolution

Proposed additions to `docs/CONTRACTS.md`. Conventions (auth, `Idempotency-Key`, `operation_key`,
`expected_revision`, error shape) are unchanged from `docs/CONTRACTS.md:128-155`.

## `ScheduledWorkout` shape (additive)

```json
{
  "id": "<uuid>",
  "status": "missed",
  "finalized_at": "2026-10-05T23:59:00Z",
  "resolution": "skipped",
  "resolved_at": "2026-10-06T07:12:00Z",
  "revision": 4
}
```

- `resolution`: `"skipped"` or `"recovered"`, omitted when the workout is unresolved. Meaningful only
  when `status` is `missed`. It never changes `status`, participation, streaks, records, volume, or
  history.
- `resolved_at`: when the resolution was recorded; omitted when unresolved.
- Clients treat a missed workout as *unresolved* when `resolution` is absent.

## `POST /api/v1/scheduled-workouts/{id}/skip`

```json
{ "operation_key": "skip-<id>-2026-10-06", "expected_revision": 3 }
```

Marks the athlete's own missed workout as skipped. Response 200: full `ScheduledWorkout` with
`resolution: "skipped"` and `revision` incremented. Skipping an already skipped workout returns 200
with the unchanged workout.

| Case | Status | Code |
| --- | --- | --- |
| Unknown id or another athlete's workout | 404 | `NOT_FOUND` |
| Outcome is not `missed`, or `resolution` is `recovered` | 409 | `RESOLUTION_CONFLICT` (body includes `resource`) |
| Stale `expected_revision` | 409 | `REVISION_CONFLICT` |
| Same key, different body | 409 | `IDEMPOTENCY_CONFLICT` |
| Header and body key differ or missing | 422 | `VALIDATION_ERROR` |

## `POST /api/v1/scheduled-workouts/{id}/unskip`

Same body. Clears a skip, making the workout unresolved. Response 200: full `ScheduledWorkout`
without `resolution`, `revision` incremented. On an unresolved workout, returns 200 unchanged.
A `recovered` workout returns 409 `RESOLUTION_CONFLICT`. Other errors as for skip.

## `POST /api/v1/schedule/recover` (behavior clarified)

- Selects the earliest missed workout with no `resolution`, ordered by `sequence_position` then
  `date`, on or before `date`.
- In the same transaction, marks that source workout `resolution: "recovered"` and increments its
  revision. The response is unchanged: 201 with the new planned copy.
- A matching replay returns the originally stored copy and changes nothing.
- No unresolved missed workout returns 404.

## Regenerate and plan transitions

Unchanged. They only replace unstarted planned workouts, so `resolution` on historical missed
workouts is always preserved.

## New stable error code

`409 RESOLUTION_CONFLICT`:

```json
{
  "error": "workout cannot be skipped",
  "code": "RESOLUTION_CONFLICT",
  "details": { "status": "completed", "resolution": null },
  "resource": { "id": "<uuid>", "revision": 5 }
}
```
