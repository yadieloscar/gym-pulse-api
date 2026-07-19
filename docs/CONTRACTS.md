# API Contracts (hand-maintained, for the client)

> This is the **client-facing source of truth** for `/api/v1/*` request bodies,
> validator rules, and response shapes. `swagger.yaml` is auto-generated and
> sometimes lags reality; this file is curated for the people writing the
> Expo app and is what `scripts/smoke.sh` asserts against.
>
> **Update rules**: if you change a `validate:` tag, a `json:` field name, or a
> response shape, update this file in the same commit and re-run
> `scripts/smoke.sh`.

## Conventions

- Base: `http://localhost:8080` (dev) / Railway prod URL.
- Auth: all `/api/v1/*` endpoints require `Authorization: Bearer <jwt>`.
  Dev validates via `SUPABASE_JWKS_URL`; if unset, HMAC with `SUPABASE_JWT_SECRET`.
- All bodies are JSON, snake_case. **The client must not camelCase anything.**
- Validation failures return `422 { "error": "<message>", "code": "VALIDATION_ERROR", "details": {...} }`;
  the **whole struct is validated** so partial PUTs that omit `required` fields
  will 422 even if you only intend to change one field. See "gotchas" below.

---

## Settings

### `GET /api/v1/settings`
Response 200:
```json
{ "weight_unit": "lb" | "kg", "weekly_goal": 1..7 }
```

### `PUT /api/v1/settings`
Body — **all fields required**:
```json
{ "weight_unit": "lb" | "kg", "weekly_goal": 1..7 }
```
Validator: `weight_unit oneof=lb kg`, `weekly_goal min=1,max=7`. Response 200 echoes the saved struct.

> ⚠️ Onboarding gotcha: when seeding from MMKV after signup, send **both**
> fields. Sending just `{weekly_goal}` returns `invalid settings`.

---

## Profile

### `GET /api/v1/profile`
Response 200:
```json
{
  "id": "<uuid>",
  "display_name": "string|null",
  "avatar_url": "string|null",
  "onboarding_completed": false
}
```

### `PUT /api/v1/profile`
Body — **all fields optional** (uses `omitempty`):
```json
{
  "display_name": "string (2-50 chars)",
  "avatar_url": "string (must be URL)",
  "onboarding_completed": true
}
```

---

## Goal-based training

These resources use UUID identity. Dates are filters and snapshot attributes,
never mutation URLs. Every owned-resource lookup is scoped to the authenticated
user; a missing or foreign UUID returns the same `404` shape.

All mutations below require `Idempotency-Key: <stable client operation key>`.
The identical key and identical JSON payload returns the originally stored
status/body. Reusing a key with a different payload returns:
```json
{
  "error": "idempotency key was already used with a different payload",
  "code": "IDEMPOTENCY_CONFLICT",
  "details": { "operation_key": "op-uuid-or-client-stable-string" }
}
```
Revisioned mutation bodies also carry `expected_revision` (`0` only when
creating a singleton training profile, otherwise ≥1). A stale revision returns:
```json
{
  "error": "resource revision conflict",
  "code": "REVISION_CONFLICT",
  "details": { "expected_revision": 3, "actual_revision": 4 },
  "resource": { "id": "<uuid>", "revision": 4 }
}
```
Both conflicts are HTTP 409. Standard errors are `401 AUTHENTICATION_REQUIRED`,
`404 NOT_FOUND`, and `422 VALIDATION_ERROR`; all have the top-level
`{"error":"string","code":"CODE","details":{}}` shape.

### Training profile

#### `GET /api/v1/training-profile`

Response 200:
```json
{
  "primary_goal": "general_health|strength|hypertrophy|conditioning|power|body_composition",
  "available_days": [1, 3, 5],
  "usual_activity": "sedentary|light|moderate|high",
  "experience": "beginner|intermediate|advanced",
  "equipment": ["bodyweight|bands|dumbbells|barbell|machines|cardio_machine|full_gym"],
  "session_duration_minutes": 60,
  "timezone": "America/New_York",
  "preferences": {},
  "revision": 1,
  "created_at": "<RFC3339>",
  "updated_at": "<RFC3339>"
}
```

No saved profile returns `404 NOT_FOUND`; clients then create it with revision
0. `available_days` contains 1..7 unique ISO weekdays (Monday=1), duration is
20..120, equipment values are unique, and `timezone` must be an IANA name.

#### `PUT /api/v1/training-profile`

Partial body; omitted profile fields retain their value. The merged complete
profile is validated. First creation must still supply every profile field.
```json
{
  "primary_goal": "strength",
  "available_days": [1, 3, 5],
  "usual_activity": "moderate",
  "experience": "intermediate",
  "equipment": ["barbell", "dumbbells"],
  "session_duration_minutes": 60,
  "timezone": "America/New_York",
  "preferences": {},
  "expected_revision": 0
}
```
Response 200 is the full training-profile shape at its authoritative revision.

### Starter programs

#### `GET /api/v1/starter-programs`

Optional filters: `primary_goal`, `available_days` (1..7),
`available_weekdays` (comma-separated unique ISO weekdays), `usual_activity`,
`experience`, `equipment` (comma-separated canonical values), and
`session_duration_minutes` (20..120). Matching goal and constraints rank first;
the response remains a catalog, not medical advice. Response 200:
```json
{
  "starter_programs": [{
    "id": "<uuid>", "slug": "strength-3-day", "version": 2,
    "name": "Three-day Strength", "description": "string",
    "primary_goal": "strength", "min_days": 3, "max_days": 4,
    "experience": ["beginner", "intermediate"],
    "equipment": ["barbell"], "duration_minutes": 60,
    "rationale": "string", "roadmap": {},
    "workouts": [{
      "id": "<uuid>", "name": "Full Body A", "preferred_weekday": 1,
      "sequence_position": 1,
      "exercises": [{
        "id": "<uuid>", "catalog_id": "<uuid|null>",
        "name": "Back Squat", "category": "legs", "modality": "strength",
        "exercise_order": 1, "target_sets": 3, "target_reps": 5,
        "target_weight": null, "target_duration_seconds": null,
        "rest_seconds": 180, "notes": null
      }]
    }]
  }]
}
```
The list is `[]`, never `null`. Catalog versions are immutable; choosing one
copies it and later starter updates never change a user program.

### Programs

#### `GET /api/v1/programs`

Response 200: `{ "programs": [Program] }`, where `Program` is:
```json
{
  "id": "<uuid>", "starter_program_id": "<uuid|null>",
  "starter_version": 2, "name": "My Strength Plan", "primary_goal": "strength",
  "roadmap": {}, "active": true, "revision": 1,
  "workouts": [{
    "id": "<uuid>", "name": "Full Body A", "preferred_weekday": 1,
    "sequence_position": 1,
    "exercises": [{
      "id": "<uuid>", "catalog_id": "<uuid|null>",
      "source_starter_exercise_id": "<uuid|null>", "name": "Back Squat",
      "category": "legs", "modality": "strength", "exercise_order": 1,
      "target_sets": 3, "target_reps": 5, "target_weight": null,
      "target_duration_seconds": null, "rest_seconds": 180, "notes": null
    }]
  }],
  "created_at": "<RFC3339>", "updated_at": "<RFC3339>"
}
```

#### `GET /api/v1/programs/{id}`

Response 200: one full `Program`. Foreign/missing ID → 404.

#### `POST /api/v1/programs`

Creates a custom owned program. Body is `name`, `primary_goal`, optional
`roadmap`, and one or more `workouts` using the nested shape above but omitting
server IDs/source fields. Response 201: full `Program`.

#### `POST /api/v1/programs/from-starter`

```json
{
  "starter_program_id": "<uuid>", "starter_version": 2,
  "name": "optional override", "operation_key": "choose-starter-123"
}
```
`operation_key` must equal the `Idempotency-Key` header. Response 201: the
owned copied `Program`. Unknown ID/version → 404; key/payload mismatch → 409.

#### `PUT /api/v1/programs/{id}`

Full replacement body: `name`, `primary_goal`, `roadmap`, `active`, `workouts`,
and `expected_revision`. Response 200: full authoritative `Program`. Replacing
the program never changes already materialized scheduled workouts.

#### `POST /api/v1/programs/adopt-legacy`

```json
{ "operation_key": "adopt-legacy-123", "expected_revision": 0 }
```
Idempotently copies owned legacy templates/weekly assignments to one program and
the next eligible future week. It never deletes legacy rows. Response 200 is
`{"program":Program,"schedule":[ScheduledWorkout],"adopted":true}`; replay may
return `adopted:false` with the same authoritative resources.

### Schedule and scheduled workouts

#### `GET /api/v1/schedule?from=YYYY-MM-DD&to=YYYY-MM-DD`

Both bounds are required and inclusive. Response 200:
```json
{ "scheduled_workouts": [ScheduledWorkout] }
```
`ScheduledWorkout` is:
```json
{
  "id": "<uuid>", "program_id": "<uuid|null>",
  "program_workout_id": "<uuid|null>", "date": "2026-07-20",
  "name": "Full Body A", "sequence_position": 1,
  "status": "planned|in_progress|completed|incomplete|missed",
  "finalized_at": "<RFC3339|null>", "revision": 3,
  "required_sets": [{
    "id": "<uuid>", "program_exercise_id": "<uuid|null>",
    "catalog_id": "<uuid|null>", "exercise_name": "Back Squat",
    "exercise_category": "legs", "exercise_modality": "strength",
    "exercise_order": 1, "set_index": 1, "target_reps": 5,
    "target_weight": null, "target_duration_seconds": null,
    "rest_seconds": 180, "notes": null, "checked": false,
    "performed_set_id": null, "actual_reps": null,
    "actual_weight": null, "actual_duration_seconds": null
  }],
  "extra_sets": [PerformedSet],
  "created_at": "<RFC3339>", "updated_at": "<RFC3339>"
}
```
Exercise name/category/modality and targets are immutable snapshots. Nullable
program/catalog IDs are provenance only and deletion never erases history.

#### `POST /api/v1/schedule/materialize`

```json
{
  "program_id": "<uuid>", "from": "2026-07-20", "to": "2026-07-26",
  "operation_key": "materialize-week-123", "expected_revision": 4
}
```
Response 201: `{ "scheduled_workouts": [ScheduledWorkout] }`. A matching replay
returns the stored result and cannot advance the program sequence twice.

#### `POST /api/v1/schedule/regenerate`

Preview body:
```json
{
  "program_id": "<uuid>", "from": "2026-07-27", "to": "2026-08-09",
  "apply": false, "operation_key": "regen-preview-123", "expected_revision": 4
}
```
Response 200:
```json
{
  "preview_token": "opaque", "retained_from": "2026-07-27",
  "retained_to": "2026-07-29", "replaced_from": "2026-07-30",
  "replaced_to": "2026-08-09", "scheduled_workouts": [ScheduledWorkout]
}
```
Apply repeats the body with `apply:true` and `preview_token`. It atomically
replaces only unstarted future work. Any active session → 409
`ACTIVE_SESSION_CONFLICT` with the authoritative session in `resource`.

#### `POST /api/v1/plan-transitions/preview`

Previews a primary-goal/profile change together with its target program and
dated replacement. It is read-only and authenticated.
```json
{
  "proposed_profile": {
    "primary_goal": "strength", "available_days": [1, 3, 5],
    "usual_activity": "moderate", "experience": "intermediate",
    "equipment": ["barbell"], "session_duration_minutes": 60,
    "timezone": "America/New_York", "preferences": {}
  },
  "program_id": "<owned-program-uuid|null>",
  "starter_program_id": "<starter-uuid|null>", "starter_version": 2,
  "from": "2026-07-20", "to": "2026-08-02"
}
```
When no program/starter is supplied, the highest-ranked goal-matching starter
is selected. Response 200 contains `preview_token`, `proposed_profile`, the full
`target_program`, nullable `recommended_starter_program`, nullable
`first_affected_date`, and `scheduled_workouts`. Materialization uses the exact
ISO weekdays in `proposed_profile.available_days`.

#### `POST /api/v1/plan-transitions/apply`

Repeats the exact preview request and adds `preview_token`, `operation_key`, and
`expected_profile_revision`; the operation key must match `Idempotency-Key`.
The server verifies the opaque preview, then atomically saves the profile,
deactivates the prior plan, activates exactly one target plan, and replaces
only unstarted workouts in the requested range. A stale preview or profile
revision returns 409. A matching replay returns the authoritative target plan.

#### `POST /api/v1/schedule/recover`

```json
{ "date": "2026-07-21", "operation_key": "recover-123" }
```
Explicitly creates a new planned occurrence on the requested date from the
earliest unresolved missed sequence item. The missed historical occurrence is
not moved or rewritten. Response 201 is the new `ScheduledWorkout`; no missed
workout returns 404.

#### `PATCH /api/v1/scheduled-workouts/{id}`

Body contains optional `name` and full `required_sets`, plus `operation_key` and
`expected_revision`. Response 200: full `ScheduledWorkout`. The edit affects
only this date snapshot. A past finalized workout cannot be deleted; this API
does not expose a scheduled-workout delete route.

#### `PUT /api/v1/scheduled-workouts/{id}/sets/{set_id}`

```json
{
  "operation_key": "check-set-123", "expected_revision": 3,
  "actual_reps": 5, "actual_weight": 185,
  "duration_seconds": null, "completed": true
}
```
Response 200: authoritative `ScheduledWorkout`. Before finalization zero checked
sets is `planned`, some is `in_progress`; extra sets never alter this count.
The returned required set restores its nullable `actual_reps`, `actual_weight`,
and `actual_duration_seconds` from the performed set.

#### `PATCH /api/v1/scheduled-workouts/{id}/sets/{set_id}/target`

```json
{
  "target_reps": 3, "target_weight": 200,
  "target_duration_seconds": null, "rest_seconds": 180, "notes": null,
  "operation_key": "target-edit-123", "expected_revision": 4
}
```
Updates one dated scheduled-set target without replacing set identity or
changing the program/template. Response 200 is the authoritative workout.
Revision conflicts return 409. If the workout was already finalized, its
derived status remains one of `completed|incomplete|missed`, never a live state.

#### `POST /api/v1/scheduled-workouts/{id}/extra-sets`

```json
{
  "operation_key": "extra-set-123", "expected_revision": 4,
  "exercise_id": "<uuid|null>", "exercise_name": "Push-Up",
  "exercise_category": "push", "exercise_modality": "strength",
  "set_index": 1, "actual_reps": 20, "actual_weight": null,
  "duration_seconds": null, "completed": true
}
```
Response 201: authoritative `ScheduledWorkout`; the returned set always has
`is_extra:true` and `scheduled_set_id:null`.

#### `POST /api/v1/scheduled-workouts/{id}/complete`

```json
{ "operation_key": "complete-workout-123", "expected_revision": 5 }
```
Response 200: finalized `ScheduledWorkout`. All required sets → `completed`,
some → `incomplete`, none → `missed`. Clients cannot submit a status directly.

### Workout sessions

#### `GET /api/v1/workout-sessions?from=YYYY-MM-DD&to=YYYY-MM-DD`

Response 200: `{ "workout_sessions": [WorkoutSession] }`. Multiple UUIDs may
share a date. `WorkoutSession` is:
```json
{
  "id": "<uuid>", "scheduled_workout_id": "<uuid|null>",
  "date": "2026-07-20", "name": "Evening accessories",
  "status": "draft|active|completed|discarded", "notes": null,
  "started_at": "<RFC3339|null>", "completed_at": "<RFC3339|null>",
  "revision": 2,
  "sets": [{
    "id": "<uuid>", "scheduled_set_id": "<uuid|null>",
    "exercise_id": "<uuid|null>", "is_extra": true,
    "exercise_name": "Push-Up", "exercise_category": "push",
    "exercise_modality": "strength", "set_index": 1,
    "target_reps": null, "target_weight": null, "actual_reps": 20,
    "actual_weight": null, "duration_seconds": null, "completed": true,
    "operation_key": "extra-set-123", "revision": 1
  }],
  "created_at": "<RFC3339>", "updated_at": "<RFC3339>"
}
```

#### `POST /api/v1/workout-sessions`

```json
{
  "scheduled_workout_id": "<uuid|null>", "date": "2026-07-20",
  "name": "Off-plan workout", "notes": null,
  "operation_key": "session-123", "expected_revision": 0
}
```
Response 201: full `WorkoutSession`. Null `scheduled_workout_id` is off-plan and
never rewrites the plan. A foreign scheduled workout ID → 404.

#### `GET /api/v1/workout-sessions/{id}`

Response 200: full `WorkoutSession`.

#### `PATCH /api/v1/workout-sessions/{id}`

Optional `name`, `notes`, and `status`, plus required `operation_key` and
`expected_revision`. Response 200: full authoritative session. Past sessions
cannot be deleted; only a current-day unfinalized draft may become `discarded`.

### Participation

#### `GET /api/v1/participation?from=YYYY-MM-DD&to=YYYY-MM-DD`

Response 200:
```json
{
  "participation": [{
    "id": "<uuid>", "date": "2026-07-20",
    "scheduled_opportunity": true, "participated": true,
    "finalized_at": "<RFC3339>", "timezone": "America/New_York",
    "local_date": "2026-07-20", "revision": 1
  }]
}
```
Participation is server-derived and has no public write route. Any performed set
on a scheduled day yields `participated:true`, even when the planned workout is
missed. Zero performed sets yields false. Rest/unscheduled dates are neutral and
omitted. Finalized timezone/date basis is immutable.

Completing an off-plan workout preserves participation for that local date but
does not rewrite a missed scheduled outcome. Stats, record, history, and volume
queries union legacy day logs with UUID workout sessions; an off-plan session is
counted as its own performed workout while weekly participation remains date-based.

---

## Exercise catalog (read-only v1)

### `GET /api/v1/exercises` (optional `?category=<type_id>`)
Response 200 — **wraps the array under an `exercises` key** (same wrapper
convention as `/stats/distribution`):
```json
{
  "exercises": [
    { "id": "<uuid>", "name": "Barbell Bench Press", "category": "push",
      "modality": "strength", "mechanic": "compound", "sort_order": 1 }
  ]
}
```
- `modality`: `"strength" | "cardio"`. `mechanic`: `"compound" | "isolation"`,
  **null for cardio entries** — client prefill keys off these two fields.
- Unknown `category` → 422 `VALIDATION_ERROR`. Valid categories are the
  workout type ids (push, pull, legs, upper, lower, full, core, cardio, …).
- Catalog is seeded by migration (~80 entries); no write endpoints in v1.

---

## Templates

### `GET /api/v1/templates` → `TemplateSummary[]`
### `POST /api/v1/templates`
Body:
```json
{
  "name": "string (1-200)",
  "type_id": "string (required)",
  "subtype_id": "string (required)",
  "exercises": [
    { "name": "string (1-100)", "sort_order": 1, "sets": 4, "reps": 8,
      "catalog_id": "uuid (optional)", "duration_minutes": 20, "intensity": "easy|moderate|hard" }
  ]
}
```
**Exercise shape rule (enforced server-side, 422 on violation):** each
exercise is EITHER strength (`sets` + `reps`) OR cardio (`duration_minutes`)
— never neither, never both. `intensity` is only valid alongside
`duration_minutes`. `catalog_id` is optional; free-text `name`-only
exercises (legacy payloads) keep working unchanged.

### `PUT /api/v1/templates/{id}` — same body as POST.
### `DELETE /api/v1/templates/{id}` → 204.

---

## Logs

### `GET /api/v1/logs?week=YYYY-WW` → `DayLog[]`
### `POST /api/v1/logs`
Body:
```json
{
  "date": "YYYY-MM-DD (required)",
  "type_id": "string (required)",
  "subtype_id": "string (required)",
  "template_id": "uuid|null",
  "overrides": [
    { "exercise_id": "uuid", "sets": 4, "reps": 8, "weight": 0 }
  ],
  "set_logs": [
    { "exercise_id": "uuid", "set_index": 1, "target_reps": 8, "target_weight": 135,
      "actual_reps": 8, "actual_weight": 135, "duration_seconds": null, "completed": true }
  ],
  "session_notes": "string|null"
}
```

`set_logs` is the per-set history behind the active workout player.
`set_index` is 1-based (≥ 1). A `completed: true` set must include `actual_reps`
or `duration_seconds` (cardio) → 422 otherwise. `target_*` snapshot the plan at
session start; `actual_*` are what was performed. `overrides` (per-exercise
aggregates: notes/skipped) and `set_logs` coexist.

### `GET|PUT|DELETE /api/v1/logs/{date}` — `date` is `YYYY-MM-DD`.

`GET` returns the full `DayLog` including a `set_logs[]` array (flat; group by
`exercise_id` client-side).

**PUT also replaces the day's workout** when any of `type_id`, `subtype_id`,
`template_id` are present:
- `template_id` given → it is **authoritative**: ownership checked (404 if
  not yours), `type_id`/`subtype_id` derive from the template, anything sent
  in the body is ignored.
- type-only replacement requires BOTH `type_id` and `subtype_id` (valid ids) → 422 otherwise.
- An update always rewrites the override set from the request body — a
  replacement without `overrides` therefore clears them (old overrides
  reference the old workout's exercises).
- ⚠️ **This applies to EVERY PUT, not just replacements.** A notes-only
  `PUT {"session_notes":"..."}` with no `overrides` field wipes all existing
  overrides for that day. The client must always re-send the full override
  set on any `PUT /logs/{date}`.
- Replacing a day to `type_id: "rest"` while sending `overrides` → 422
  (rest days carry no overrides, mirroring POST).
- ⚠️ **`set_logs` follow the SAME replace rule as overrides** — every `PUT`
  rewrites the day's entire set list, so the client must re-send all sets on
  any `PUT /logs/{date}`. Rest day + `set_logs` → 422.

### `GET /api/v1/exercises/history?ids=<uuid,uuid>` → `ExerciseHistory[]`
For each requested exercise id, returns the completed sets from the **most
recent day** that exercise was performed — powers "last time you did X" hints.
One round-trip for a whole workout. Unknown ids are simply omitted; a malformed
id → 422.
```json
[
  { "exercise_id": "uuid", "date": "2026-06-10",
    "sets": [ { "set_index": 1, "actual_reps": 8, "actual_weight": 135, "completed": true } ] }
]
```

### `GET /api/v1/exercises/records?ids=<uuid,uuid>` → `ExerciseRecord[]`
Per requested exercise, the all-time bests from completed weighted sets:
heaviest weight (ties broken by more reps) and best estimated 1RM
(Epley: `weight × (1 + reps/30)`). Exercises with no weighted sets are omitted;
a malformed id → 422. The client compares this to the just-finished session to
detect PRs, and shows "top lifts".
```json
[
  { "exercise_id": "uuid",
    "max_weight": 185, "max_weight_reps": 1, "max_weight_date": "2026-06-05",
    "best_e1rm": 196.3, "e1rm_weight": 155, "e1rm_reps": 8, "e1rm_date": "2026-06-10" }
]
```

---

## Weekly plan

### `GET /api/v1/plan?from=YYYY-MM-DD&to=YYYY-MM-DD`
Response 200 (window defaults to today ±4 weeks; applies to overrides only):
```json
{
  "weekly":    [ { "weekday": 1, "template_id": "uuid|null", "rest": false } ],
  "overrides": [ { "date": "2026-06-15", "template_id": null, "rest": true } ]
}
```
- `weekday` is ISO: 1=Monday … 7=Sunday. Missing weekday = unplanned.
- **Effective plan for a date = override ?? weekly[isoWeekday] — resolved
  CLIENT-side.** The API stores, never resolves.
- `rest: true` ⇔ `template_id: null` (enforced, 422).

### `PUT /api/v1/plan/weekly`
Body `{ "days": [{weekday, template_id|null, rest}] }` — **full replace**,
sparse allowed. Duplicate weekdays → 422; foreign/unknown template → 404.
Response 200 echoes the stored plan.

### `PUT /api/v1/plan/overrides/{date}` → 204
Body `{ "template_id": "uuid|null", "rest": bool }` — upsert one-day override.

### `DELETE /api/v1/plan/overrides/{date}` → 204 (404 if no override)
Date falls back to the recurring weekly plan.

---

## Stats

### `GET /api/v1/stats` (alias `/stats/summary`)
Response 200:
```json
{ "current_streak": 0, "longest_streak": 0, "weekly_goal": 5, "total_workouts": 0 }
```

**Counting semantics:** `rest`-type day logs are logged intent, not workouts.
They are excluded from `this_week.completed`, the week streak, the day
streak, and `total_workouts` (this matches the client mock's behavior).
For goal-based training, the participation streak counts consecutive finalized
scheduled opportunities with participation. Incomplete and completed outcomes
count; a finalized missed opportunity without off-plan participation resets it.
Rest and unscheduled dates are skipped rather than breaking the streak, and an
unfinalized current opportunity does not yet extend or reset it.

### `GET /api/v1/stats/distribution`
Response 200 — **wraps the array under a `types` key**:
```json
{ "types": [ { "type_id": "upper", "count": 3, "subtypes": [{ "subtype_id": "hypertrophy", "count": 3 }] } ] }
```
`types` may be `null` for a new user. The client must unwrap and coerce:
```ts
const safe = Array.isArray(resp?.types) ? resp.types : [];
```

### `GET /api/v1/stats/volume?weeks=N` → `WeeklyVolume[]`
Total lifted volume (Σ `actual_weight × actual_reps` over completed sets) per
week for the last `N` weeks (default 8, max 52, `N<1` → 422), **oldest first**.
The series is **continuous** — weeks with no logged volume are returned as `0`,
so the chart has no gaps. `week_start` is the Monday of each week.
```json
[ { "week_start": "2026-06-08", "volume": 0 }, { "week_start": "2026-06-15", "volume": 5400 } ]
```

> ⚠️ Drift gotcha: WorkoutSplit.tsx originally crashed on `.reduce` because the
> hook assigned the whole `{types: [...]}` object to a `TypeDistribution[]`
> state and the runtime threw. `scripts/smoke.sh` step 4 asserts this shape.

---

## Body weight

### `POST /api/v1/body/weight`
Body:
```json
{ "date": "YYYY-MM-DD", "weight": 180.0, "unit": "lb" | "kg" }
```
Validator: `weight gt=0`, `unit oneof=lb kg`.

### `GET /api/v1/body/weight` → `BodyWeight[]` (may be `null`; same coercion rule).
### `DELETE /api/v1/body/weight/{id}` → 204.

---

## Account

### `DELETE /api/v1/account` → 204

**Irreversible.** Deletes every row belonging to the authenticated user
(templates + exercises, day logs + set logs + overrides, plans, settings,
body weights, profile) in one transaction, then — when the server is
configured with `SUPABASE_URL` + `SUPABASE_SERVICE_ROLE_KEY` — deletes the
Supabase auth user via the Admin API. An auth-provider failure after data
deletion is logged server-side, not surfaced: the response is still 204 and
the client must sign out and clear local state.

Client flow: confirm twice → `DELETE /account` → `supabase.auth.signOut()` →
`clearCache()`.

---

## Auth contract for the client

JWT must have:
- `sub`: a v4 UUID — this becomes the `userID` everywhere.
- `kid` header: required when `SUPABASE_JWKS_URL` is set (asymmetric path).
  HMAC path (no JWKS) doesn't require it.

Local dev tip: see `scripts/smoke.sh` for how to mint a short-lived HMAC token
that the API will accept.

---

## Common gotchas (the ones that have bitten us in this repo)

| You did | API says | Why | Fix |
|---|---|---|---|
| `PUT /settings {weekly_goal: 5}` | `invalid settings` | the validator runs on the **whole struct**, `weight_unit` is `required` | always send both fields |
| `PUT /settings {weekly_goal: 2}` | `invalid settings` | old `min=3` rule | rebuild the local API container |
| `GET /stats/distribution` then `.reduce` | `reduce is not a function` | handler returns `null` for empty | coerce client-side or use `useStats`'s guard |
| `POST /templates` from onboarding before signup | 401 | not authenticated | buffer in MMKV, drain in `_layout.tsx` |
| edits to `internal/model/*.go` don't take effect | container still has old binary | docker layer cache reused stale binary | `docker compose up -d --build api` |
