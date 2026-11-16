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
  Production validates the configured issuer and audience using bounded JWKS
  retrieval. Issuer and JWKS are derived from the canonical `SUPABASE_URL`, and
  explicit legacy overrides must match that project. Development may use
  legacy HMAC with `SUPABASE_JWT_SECRET`.
- Production accepts only expiring `RS256` RSA or `ES256` P-256 tokens whose
  JWKS algorithm exactly matches the token, and `sub` must be a UUID.
- `exp`, `nbf`, and `iat` are checked with 30 seconds of clock-skew leeway so a
  device or host a few seconds off does not get a spurious `401`.
- After signature and claim validation, every `/api/v1/*` request verifies
  that the JWT subject still exists in `auth.users`. A deleted identity is
  rejected even while its previously issued access token remains unexpired.
- Every authentication failure is
  `401 { "error": "<message>", "code": "AUTHENTICATION_REQUIRED" }` with a
  `WWW-Authenticate` challenge (RFC 6750): `Bearer realm="gym-pulse"` when no
  bearer token was presented, and
  `Bearer realm="gym-pulse", error="invalid_token"` when one was presented but
  failed validation or its subject no longer exists. Clients branch on `code`
  and the challenge, never on the message text. A transient identity-check
  failure is `503 AUTHENTICATION_UNAVAILABLE` and must not sign the user out.
- Contract 1.10.0 (additive): a deployment may set `ALLOWED_USER_IDS` to keep
  itself private. A valid token whose `sub` is not on that list gets
  `403 { "error": "<message>", "code": "ACCOUNT_NOT_ALLOWED" }` on every
  `/api/v1/*` route, including `DELETE /api/v1/account`. The session is valid,
  so the client must not sign the user out or retry; it shows that the account
  has no access. With the variable unset, every verified user is allowed.
- All bodies are JSON, snake_case. **The client must not camelCase anything.**
- Browser preflight accepts `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, and
  `OPTIONS`, including `Authorization`, `Content-Type`, and `Idempotency-Key`.
  `X-Request-ID` is exposed to browser clients for support correlation. The
  server generates it for every request as 32 lowercase hex characters; a
  client-supplied `X-Request-ID` is ignored.
- JSON bodies are limited to 1 MiB and exactly one JSON value. Oversized bodies
  return `413 REQUEST_TOO_LARGE` with `details.max_bytes: 1048576`; malformed or
  trailing JSON returns `400 BAD_REQUEST`.
- Every response carries `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`,
  and `Referrer-Policy: no-referrer`; `/api/v1/*` responses add
  `Cache-Control: no-store`. Staging and production also send
  `Strict-Transport-Security: max-age=31536000; includeSubDomains`. The Swagger UI at `/docs/*` is not served in
  staging or production; this document is the contract.
- Every authenticated route is rate limited **per authenticated user** and
  returns `429 { "error": "too many requests, retry shortly", "code": "RATE_LIMITED" }`
  when exceeded. All authenticated requests share 600 per minute. The costliest
  endpoints have tighter limits on top: `PUT /api/v1/profile/avatar` and `DELETE /api/v1/account`
  allow 10 requests per minute; `POST /api/v1/schedule/materialize`,
  `/schedule/regenerate`, `/schedule/recover`, `/plan-transitions/preview`,
  and `/plan-transitions/apply` allow 30 per minute; `GET /api/v1/export`,
  `/export/workout-sets`, `POST /api/v1/imports/preview`, and
  `POST /api/v1/imports` share 6 per minute. Clients should back off
  rather than retry immediately.
- Validation failures return `422 { "error": "<message>", "code": "VALIDATION_ERROR", "details": {...} }`;
  the **whole struct is validated** so partial PUTs that omit `required` fields
  will 422 even if you only intend to change one field. See "gotchas" below.

---

## Operational checks

### `GET /health`

Process liveness. Returns `200 {"status":"ok"}` while the HTTP process is
running; it does not query dependencies.

### `GET /ready`

Database-backed readiness. Returns `200 {"status":"ready"}` when PostgreSQL
responds within the bounded readiness timeout. Otherwise returns
`503 {"status":"unavailable"}`. Neither endpoint requires authentication.

---

## Settings

### `GET /api/v1/settings`
Response 200:
```json
{ "weight_unit": "lb" | "kg", "weekly_goal": 1..7, "palette": "obsidianEmber" | "abyssCerulean" | "mossChartreuse" | "voltLime" | "oliveCitrine" | "slateMint" | "irisViolet" }
```
An account that has never saved settings reads the defaults
`{ "weight_unit": "lb", "weekly_goal": 5, "palette": "voltLime" }`.

Contract 1.9.0 (additive) adds the palette value `irisViolet` and changes the
default palette from `obsidianEmber` to `voltLime`. Stored palettes are not
rewritten: an account that already saved any setting keeps the palette it has.
All seven values stay valid, including `mossChartreuse`, `oliveCitrine`, and
`slateMint`, which current app builds no longer offer. A client must tolerate
a palette name it does not know (a newer build may have stored it) and fall
back to its own default for display.

### `PUT /api/v1/settings`
Body is partial; omitted fields preserve their current values:
```json
{ "weight_unit": "lb" | "kg", "weekly_goal": 1..7, "palette": "obsidianEmber" | "abyssCerulean" | "mossChartreuse" | "voltLime" | "oliveCitrine" | "slateMint" | "irisViolet" }
```
Old two-field clients remain compatible and preserve the server palette.
Palette-only updates preserve weight unit and weekly goal. Response 200 is the
full authoritative settings object.

`irisViolet` is accepted since contract 1.9.0. A first write that omits a
field stores that field's default (`lb`, `5`, `voltLime`). Any other palette
name is `422 VALIDATION_ERROR` with `details.field = "body"`.

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
`avatar_url` (contract 1.11.0): the avatar bucket is private. For a photo uploaded
through `PUT /api/v1/profile/avatar`, every profile response carries a signed
Supabase Storage link that works for one hour and is then refused. The same link
is returned for up to 30 minutes, so an image cache keeps working; after that a
new one is issued. Clients show it, never store it as the photo's address, and
read the profile again for a fresh link (a cached profile may hold an expired
one). `null` means no photo, or that a link could not be made right now; the
profile is still returned. An external `https://` URL set through
`PUT /api/v1/profile` is returned unchanged.

Omitting `onboarding_completed` preserves it. `true` completes onboarding;
`false` is rejected with 422 and onboarding cannot be reset through this API.
Unrelated display-name or avatar URL changes never complete onboarding.

### `PUT /api/v1/profile/avatar`

Authenticated `multipart/form-data` with exactly one `file` part containing a
JPEG or PNG of at most 5 MiB. Under the per-user cross-replica lock, the server
loads the current profile and uploads to the inactive bounded slot:
`<user_uuid>/avatar-0` or `<user_uuid>/avatar-1`. It then persists a stable,
versioned reference to that object (never served to clients; the response
carries a signed link, see `GET /api/v1/profile`). The previous active object is never overwritten before
the database update commits. After any database-write error, the new object is
never deleted. A detached read that positively confirms its exact URL turns the
response into success. An unresolved read or one that still shows the old URL
returns an error, because the original autocommit may still complete afterward.
Avatar replacement never deletes any supported avatar object, including after
normal success or positive error confirmation. This preserves compatibility
with generic `PUT /api/v1/profile`, which may repoint `avatar_url` to a
previously issued supported URL. Replacement safely overwrites only the
inactive slot selected from the current database URL. The legacy object and
both slots—at most three objects—remain accessible until account deletion
explicitly removes all three. Response 200 is the full profile snapshot with
the new URL.
Missing/multiple files → 400; oversized → 413; unsupported bytes → 415;
storage failure → 502; unconfigured storage → 503.

### `PUT /api/v1/profile`
Body — **all fields optional** (uses `omitempty`):
```json
{
  "display_name": "string (2-50 chars)",
  "avatar_url": "string (must be an https:// URL)",
  "onboarding_completed": true
}
```
`avatar_url` may be an external `https://` URL, or a link this API issued for one
of the caller's own avatar objects (stored as its stable reference, so a signed
link sent back does not expire in the database). A link to another account's
avatar object is `422 VALIDATION_ERROR` with `details.field = "avatar_url"`.

---

## Goal-based training

These resources use UUID identity. Dates are filters and snapshot attributes,
never mutation URLs. Every owned-resource lookup is scoped to the authenticated
user; a missing or foreign UUID returns the same `404` shape.

All mutations below require `Idempotency-Key: <stable client operation key>`
and an equal non-empty `operation_key` JSON field. During the API-first mobile
migration window, training-profile PUT and custom-program POST/PUT also accept
an omitted JSON field when the header is present; the server normalizes the
header into the request before validation and hashing. New clients always send
both locations, and mismatched values are rejected.
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
A skip or unskip that does not apply to the workout's current outcome returns:
```json
{
  "error": "workout cannot be skipped",
  "code": "RESOLUTION_CONFLICT",
  "details": { "status": "completed", "resolution": null },
  "resource": { "id": "<uuid>", "revision": 5 }
}
```
A sport activity recorded against a scheduled sport session that is already
completed returns:
```json
{
  "error": "sport session is already completed",
  "code": "SPORT_SESSION_CONFLICT",
  "details": { "status": "completed" },
  "resource": { "id": "<uuid>", "kind": "sport", "status": "completed", "revision": 2 }
}
```
Link and unlink add `details.reason` (`activity_linked`, `session_completed`,
`not_linked`).
All four conflicts are HTTP 409. Standard errors are `401 AUTHENTICATION_REQUIRED`,
`404 NOT_FOUND`, and `422 VALIDATION_ERROR`; all have the top-level
`{"error":"string","code":"CODE","details":{}}` shape.

All inclusive `from`/`to` ranges in this section may contain at most 366
calendar dates. A larger range returns `422 VALIDATION_ERROR` with
`details.field = "range"` before repository access or schedule generation.

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
  "expected_revision": 0,
  "operation_key": "profile-create-123"
}
```
Response 200 is the full training-profile shape at its authoritative revision.

### Starter programs

#### `GET /api/v1/starter-programs`

Optional filters: `primary_goal`, `available_days` (1..7),
`available_weekdays` (comma-separated unique ISO weekdays, at most 7), `usual_activity`,
`experience`, `equipment` (comma-separated canonical values, at most 32), and
`session_duration_minutes` (20..120). A non-numeric or non-positive
`available_days` or `session_duration_minutes`, a weekday outside 1..7, or a
list above its cap → 422 `VALIDATION_ERROR`. Matching goal and constraints rank first;
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
    "id": "<uuid>", "kind": "exercises", "name": "Full Body A", "preferred_weekday": 1,
    "sequence_position": 1,
    "exercises": [{
      "id": "<uuid>", "catalog_id": "<uuid|null>",
      "source_starter_exercise_id": "<uuid|null>", "name": "Back Squat",
      "category": "legs", "modality": "strength", "exercise_order": 1,
      "target_sets": 3, "target_reps": 5, "target_weight": null,
      "target_duration_seconds": null, "rest_seconds": 180, "notes": null
    }]
  }, {
    "id": "<uuid>", "kind": "sport", "name": "Long ride", "preferred_weekday": 6,
    "sequence_position": 2,
    "sport": {
      "sport_id": "cycling", "sport_name": "Cycling",
      "target_duration_minutes": 90, "target_distance_meters": 40000, "notes": null
    },
    "exercises": []
  }],
  "created_at": "<RFC3339>", "updated_at": "<RFC3339>"
}
```
Planned sport sessions (contract 1.2.0): every program unit carries `kind`,
`exercises` (the default when omitted on write) or `sport`. An `exercises`
unit has at least one exercise and no `sport`. A `sport` unit has a `sport`
target and no exercises: `sport_id` is a lowercase slug of at most 64
characters, `sport_name` is trimmed and 1–80 characters (`other` requires a
custom name), `target_duration_minutes` is 1–1,440, optional
`target_distance_meters` is 1–1,000,000 and allowed only for `cycling`,
`running`, `swimming`, and `other`, and optional `notes` is trimmed and at most
2,000 characters. Violations return `422 VALIDATION_ERROR` with
`details.field = "workouts"`. Starter programs, `from-starter`, and
`adopt-legacy` produce `exercises` units only.

Program size limits: a program has at most 14 workouts, an `exercises` unit at
most 50 exercises, and `target_sets` is 1–20. `POST /api/v1/programs` and
`PUT /api/v1/programs/{id}` reject anything larger with `422 VALIDATION_ERROR`
and `details.field = "workouts"`. `roadmap` is limited to 16,384 bytes of
compact JSON; a larger one is `422 VALIDATION_ERROR` with
`details.field = "roadmap"`. `adopt-legacy` clamps a legacy template's
set count to 20.

#### `GET /api/v1/programs/{id}`

Response 200: one full `Program`. Foreign/missing ID → 404.

#### `POST /api/v1/programs`

Creates a custom owned program. Body is `name`, `primary_goal`, `operation_key`, optional
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
owned copied `Program`. A matching replay returns the exact originally stored
response. Creating the active copy atomically deactivates the user's previously
active program. Unknown ID/version → 404; key/payload mismatch → 409.

#### `PUT /api/v1/programs/{id}`

Full replacement body: `name`, `primary_goal`, `roadmap`, `active`, `workouts`,
`expected_revision`, and `operation_key`. Response 200: full authoritative `Program`. Replacing
the program never changes already materialized scheduled workouts.

#### `POST /api/v1/programs/adopt-legacy`

```json
{ "operation_key": "adopt-legacy-123", "expected_revision": 0 }
```
Idempotently copies only owned, non-rest weekly template assignments to one
program and the next Monday-through-Sunday week strictly after the athlete's
current local date. Assignment order follows ISO weekday; repeated template
assignments remain distinct workouts. Strength targets and duration-based
cardio targets are preserved as detached snapshots. It never deletes or updates
legacy rows. No adoptable assignment returns 404. Response 200 is
`{"program":Program,"schedule":[ScheduledWorkout],"adopted":true}`. Repeating
that same operation key returns that exact response. A later adoption request
with a new operation key returns the same authoritative resources with
`adopted:false` and creates no duplicate program or schedule. Fresh operation
keys reload the current owned program and schedule for the original adoption
week, including regenerated replacements; renamed or deactivated programs and
corrected workouts therefore reflect their current revisions. Each operation's
subsequent same-key retries still return its own exact committed response,
including when those resources have changed again.

### Program generation

GymPulse builds one unsaved program draft from the athlete's answers (the saved
or proposed training profile plus the optional steers below). Generation is
read-only with respect to programs, schedules, and the profile; the draft
exists only in the response. The athlete reviews it in the app and saves it,
unchanged, through `POST /api/v1/programs`.

Two providers serve the same route. The default, `rules`, is a deterministic
engine inside the API: no provider call, no daily limit, nothing about the
athlete leaves the process, and equal answers give an identical draft. The
`model` provider (`PROGRAM_GENERATION_PROVIDER=model` with `ANTHROPIC_API_KEY`)
sends the answers to the configured Claude model, keeps a per-athlete daily
count, and honours the free-text `request`. `provider` in both answers says
which served; `enabled` is `false` only when `model` is configured without a
key and the app hides its entry points then.

#### `GET /api/v1/program-generation`

Response 200:
```json
{ "enabled": true, "provider": "rules", "limit": 5, "remaining_today": 5, "resets_at": null }
```
With `provider: "rules"` the limit never applies: `remaining_today` equals
`limit` and `resets_at` is `null`. With `provider: "model"`, `remaining_today`
counts succeeded and in-progress drafts in the athlete's current local day
(saved training-profile timezone) against `limit` and `resets_at` is the next
local midnight; without a training profile `remaining_today` equals `limit`
and `resets_at` is `null`.

#### `POST /api/v1/program-generation`

Optional body:
```json
{
  "request": "no deadlifts",
  "secondary_goals": ["conditioning"],
  "avoid": ["hinge", "vertical_push"],
  "focus": "upper",
  "variant": 1,
  "proposed_profile": {
    "primary_goal": "strength", "available_days": [1, 3, 5],
    "usual_activity": "moderate", "experience": "intermediate",
    "equipment": ["barbell"], "session_duration_minutes": 60,
    "timezone": "America/New_York", "preferences": {}
  }
}
```
`request` is one optional steer, trimmed, 1..200 characters; a blank or longer
value returns `422 VALIDATION_ERROR` with `details.field = "request"`.
`secondary_goals` is optional: up to two valid training goals other than the
primary (duplicates collapse; the primary itself, an unknown goal, or a third
goal returns `422 VALIDATION_ERROR` with `details.field = "secondary_goals"`).
The primary goal sets the draft's structure; a secondary goal steers at most
about a third of it. `avoid` (optional, at most 6 distinct catalog movement
patterns: `squat`, `hinge`, `lunge`, `horizontal_push`, `vertical_push`,
`horizontal_pull`, `vertical_pull`, `carry`, `core`, `explosive`, `locomotion`,
`isolation`) keeps those patterns out of the draft; `focus` (optional: `upper`,
`lower`, or `core`) gives that area about two thirds of the lifting slots;
`variant` (optional integer 0..1000) rotates the exercise chosen within each
pattern so "try another" yields a different, equally valid draft. Unknown values
return `422 VALIDATION_ERROR` naming `avoid`, `focus`, or `variant`. The `rules`
provider ignores `request`; the `model` provider ignores `avoid`, `focus`, and
`variant`. `proposed_profile` is optional and uses the plan-transition preview shape; when
present it replaces the saved profile for this request only (onboarding builds a
draft before the profile is saved), is validated like `PUT /training-profile`,
and is never stored. The body may be omitted or `{}`. The route takes no
`Idempotency-Key` (the `rules` provider is deterministic by construction; the
`model` provider is not). It is rate limited to 5 requests per minute per
athlete (`429 RATE_LIMITED`).

Response 200:
```json
{
  "draft": {
    "name": "Three-day upper lower", "primary_goal": "hypertrophy",
    "roadmap": { "origin": "generated", "model": "gympulse-rules-v1", "request": null },
    "workouts": [{
      "kind": "exercises", "name": "Upper A", "preferred_weekday": 1, "sequence_position": 1,
      "exercises": [{
        "catalog_id": "<uuid>", "name": "Barbell Bench Press", "category": "push",
        "modality": "strength", "exercise_order": 1, "target_sets": 4, "target_reps": 8,
        "target_weight": null, "target_duration_seconds": null, "rest_seconds": 120, "notes": null
      }]
    }]
  },
  "rationale": "string, at most 500 characters",
  "request": "no deadlifts|null",
  "secondary_goals": ["conditioning"],
  "provider": "rules",
  "avoid": ["hinge", "vertical_push"],
  "focus": "upper",
  "remaining_today": 5,
  "resets_at": "<RFC3339>"
}
```
`draft` is the `POST /api/v1/programs` body without `operation_key` and
without server ids. Every draft the API returns satisfies: 1 to
`min(available_days, 6)` workouts, each `kind: exercises` with 3 to 8
exercises; every `catalog_id` resolves to an exercise-catalog entry and is
unique within its workout, with `name`, `category`, and `modality` copied from
the catalog; strength exercises carry `target_reps` (1..50) and a null
duration, cardio exercises carry `target_duration_seconds` (30..3600) and null
reps; `target_sets` 1..10; `rest_seconds` 0..600; `target_weight` always null;
`primary_goal` equals the profile's; `name` 1..80 characters; `preferred_weekday`
is assigned from the profile's sorted available days. A generated answer that
cannot be repaired into that shape is never returned. With the `rules`
provider every exercise carries `notes`: for strength work the rep range and
effort in athlete words ("8 to 12 reps, leave 1 to 2 in reserve."), for cardio
the intensity ("Easy pace, you can talk."); `target_reps` is the lower bound of
the range. Strength, hypertrophy, and power drafts with two or more days
alternate heavier and moderate days; full-body days alternate two fixed sets
(A and B); the rationale ends with one progression sentence.

Errors:

- `422 VALIDATION_ERROR` with `details.field = "training_profile"` when no
  profile is saved and none is proposed, or `details.field =
  "proposed_profile.<field>"` when the proposed profile is invalid; no provider
  call is made. With `details.field = "avoid"` and the message "Those limits
  leave too little to build a workout from; drop one and try again." when the
  `rules` provider cannot fill three exercises for some workout.
- `429 GENERATION_LIMIT_REACHED` (`model` provider only) when the athlete
  already received `limit` drafts in the current local day:
  ```json
  { "error": "daily program generation limit reached", "code": "GENERATION_LIMIT_REACHED",
    "details": { "limit": 5, "resets_at": "<RFC3339>" } }
  ```
  No provider call is made.
- `503 GENERATION_UNAVAILABLE` with `details.reason` `"disabled"` (`model`
  provider configured without a key) or `"unavailable"` (provider error,
  timeout after 45 seconds, refusal, or a draft that failed validation):
  ```json
  { "error": "program generation is unavailable", "code": "GENERATION_UNAVAILABLE",
    "details": { "reason": "unavailable" } }
  ```
  The attempt does not count toward the daily limit and nothing is stored.

With the `rules` provider nothing leaves the API. With the `model` provider
only the training-profile fields (`primary_goal`, `available_days`,
`usual_activity`, `experience`, `equipment`, `session_duration_minutes`),
`secondary_goals`, the exercise catalog, and `request` are sent. The athlete's
identity, timezone, history, body metrics, nutrition, and health data never
leave the API either way.

### Schedule and scheduled workouts

#### `GET /api/v1/schedule?from=YYYY-MM-DD&to=YYYY-MM-DD`

Both bounds are required and inclusive and follow the shared 366-date maximum.
Response 200:
```json
{ "scheduled_workouts": [ScheduledWorkout] }
```
`ScheduledWorkout` is:
```json
{
  "id": "<uuid>", "kind": "exercises|sport", "program_id": "<uuid|null>",
  "program_workout_id": "<uuid|null>", "date": "2026-07-20",
  "name": "Full Body A", "sequence_position": 1,
  "sport": {
    "sport_id": "cycling", "sport_name": "Cycling",
    "target_duration_minutes": 90, "target_distance_meters": 40000, "notes": null,
    "sport_activity_id": "<uuid> (present once completed)",
    "performed_date": "2026-07-20 (present once completed)"
  },
  "status": "planned|in_progress|completed|incomplete|missed",
  "finalized_at": "<RFC3339|null>",
  "resolution": "skipped|recovered (omitted when unresolved)",
  "resolved_at": "<RFC3339> (omitted when unresolved)", "revision": 3,
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
`sport` is present only when `kind` is `sport` (contract 1.2.0). A scheduled
sport session snapshots its sport target at generation, has empty
`required_sets`, and only ever holds `planned`, `completed`, or `missed`. It
becomes `completed` when a sport activity is recorded against it (see "Sport
activities"), and `sport.sport_activity_id` and `sport.performed_date` then
name that activity and its date. Materialize, regenerate, plan transitions,
skip, and unskip treat sport sessions exactly like workouts. `complete`, set
check, target edit, extra set, and a `PATCH` carrying non-empty
`required_sets` answer `422 VALIDATION_ERROR` with `details.field = "kind"` on
a sport session; a `PATCH` with only `name` is accepted. `schedule/recover`
selects exercise workouts only, and health exports never apply to sport
sessions.
`resolution` records how the athlete resolved a missed workout: `skipped`
(undoable) or `recovered` (a copy was planned through recover). It is
meaningful only when `status` is `missed`; clients treat a missed workout as
unresolved when the field is absent. It never changes `status`, participation,
streaks, records, volume, or history. If a correction changes the outcome to
`completed` or `incomplete`, `resolution` and `resolved_at` are cleared.

#### `POST /api/v1/schedule/materialize`

```json
{
  "program_id": "<uuid>", "from": "2026-07-20", "to": "2026-07-26",
  "operation_key": "materialize-week-123", "expected_revision": 4
}
```
Response 201: `{ "scheduled_workouts": [ScheduledWorkout] }`. A matching replay
returns the exact stored result and cannot advance the program sequence twice.

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
A matching apply replay returns the exact originally stored response.

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
The workout copy and its idempotency record are committed atomically. A matching
replay returns the exact originally stored recovered workout.
Explicitly creates a new planned occurrence on the requested date from the
earliest unresolved missed workout (no `resolution`), ordered by
`sequence_position` then `date`, on or before the requested date. In the same
transaction the source is marked `resolution: "recovered"` and its revision
increments, so it is never selected again. The missed historical occurrence is
not moved and its outcome is not rewritten. Response 201 is the new
`ScheduledWorkout`; no unresolved missed workout returns 404.

#### `PATCH /api/v1/scheduled-workouts/{id}`

Body contains optional `name` and full `required_sets`, plus `operation_key` and
`expected_revision`. Response 200: full `ScheduledWorkout`. The edit affects
only this date snapshot. Historical or finalized workouts are immutable and
return 409. This API does not expose a scheduled-workout delete route.

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
and `actual_duration_seconds` from the performed set. Required-set changes,
their lazy workout-session creation, status, revision, and exact replay record
commit together. A finalized workout accepts dated-record corrections and
re-derives only its scheduled outcome; its day participation remains immutable.

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
Revision conflicts return 409. Finalized workouts accept target corrections;
the finalized outcome and participation do not change.

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
`is_extra:true` and `scheduled_set_id:null`. The set, session/live status,
workout revision, and exact replay record commit together. Finalized workouts
reject extra sets with 409. `exercise_id` may reference an owned legacy
exercise. For migration compatibility, a recognized global catalog UUID in
that field is accepted but detached to `null`; immutable exercise snapshots
remain authoritative, and new clients send `null` for catalog-derived sets.

#### `POST /api/v1/scheduled-workouts/{id}/skip`

```json
{ "operation_key": "skip-<id>-<revision>", "expected_revision": 3 }
```
Marks the athlete's own missed workout `resolution: "skipped"`. Response 200:
full `ScheduledWorkout` with the revision incremented. Skipping an already
skipped workout returns 200 unchanged. A workout whose outcome is not `missed`,
or that is `recovered`, returns `409 RESOLUTION_CONFLICT` with the
authoritative workout as `resource`. Another athlete's workout returns 404.

#### `POST /api/v1/scheduled-workouts/{id}/unskip`

Same body. Clears a skip so the workout is unresolved again. Response 200: full
`ScheduledWorkout` without `resolution`, revision incremented. Any workout that
is not skipped and not recovered returns 200 unchanged; a `recovered` workout
returns `409 RESOLUTION_CONFLICT`. For both skip and unskip, `expected_revision`
is checked first, so a stale revision returns `REVISION_CONFLICT` even when the
operation would be a no-op. A source stays `recovered` even if its planned copy
is later removed by regenerate or a plan transition; the sequence moves on.

#### `DELETE /api/v1/scheduled-workouts/{id}`

No body. Permanently removes the athlete's own past or finalized scheduled
workout. Response 204. In one transaction it deletes the workout, its required
sets, every workout session linked to it (with their performed sets), and its
Apple Health export record, then recomputes that date's participation from
what remains: the day keeps a scheduled opportunity only if another scheduled
workout is on it, and counts as participated only if another completed or
incomplete scheduled workout, a completed workout session, or a sport activity
remains. A day left with neither is removed, so stats and the day streak no
longer count the deleted workout. A linked sport activity stays in history,
unlinked. The workout already written to Apple Health stays on the device.

Today's or a future workout that is not finalized returns `409 CONFLICT` with
the authoritative workout as `resource`; the plan edits, skips, or regenerates
those instead. A workout with an active linked session returns
`409 ACTIVE_SESSION_CONFLICT`. A missing or another athlete's workout returns
404, so a repeated delete is also 404.

#### `POST /api/v1/scheduled-workouts/{id}/complete`

```json
{ "operation_key": "complete-workout-123", "expected_revision": 5 }
```
Response 200: finalized `ScheduledWorkout`. All required sets → `completed`,
some → `incomplete`, none → `missed`. Clients cannot submit a status directly.
The workout outcome, any active linked session, day participation, and exact
replay response commit in one transaction, so replicas and retries cannot
observe a partial completion.

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
never rewrites the plan. A foreign scheduled workout ID → 404. Session creation
and its exact replay response commit together.

#### `GET /api/v1/workout-sessions/{id}`

Response 200: full `WorkoutSession`.

#### `PATCH /api/v1/workout-sessions/{id}`

Optional `name`, `notes`, and `status`, plus required `operation_key` and
`expected_revision`. Response 200: full authoritative session. Only a
current-day unfinalized draft may become `discarded`; a past session linked to
a scheduled workout is removed by deleting that workout.
When the session becomes completed, the session revision, participation, and
exact replay response commit in one transaction.

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

### Sport activities

Sport activities are completed athlete-owned sports, separate from structured
workout sessions. All collection dates are inclusive and a range may span at
most 366 days. Results are ordered by `date`, `created_at`, then `id`, newest first.

#### `GET /api/v1/sport-activities?from=YYYY-MM-DD&to=YYYY-MM-DD`

Response 200 is a JSON array. An athlete with no activities receives `[]`.

#### `POST /api/v1/sport-activities`

Requires `Idempotency-Key` equal to `operation_key`.

```json
{
  "date": "2026-08-02",
  "sport_id": "basketball",
  "sport_name": "Basketball",
  "duration_minutes": 60,
  "notes": "Pickup game",
  "operation_key": "sport-create-123"
}
```

`date` may be omitted and then defaults to today in the athlete's saved
training-profile timezone. Future dates are rejected. `sport_id` is a lowercase
slug of at most 64 characters. `sport_name` is trimmed and 1–80 characters;
the `other` identifier requires a custom name. Duration is a whole number from
1 through 1,440 minutes. Notes are optional, trimmed, and at most 2,000 characters.

Imported health workouts (contract 1.1.0) add optional fields:

```json
{
  "date": "2026-10-11",
  "sport_id": "running",
  "sport_name": "Running",
  "duration_minutes": 42,
  "operation_key": "hk:9B3C2E1A-6F0D-4A51-9C7E-2B1F0D8E4A77",
  "source": "apple_health",
  "external_id": "9B3C2E1A-6F0D-4A51-9C7E-2B1F0D8E4A77",
  "distance_meters": 8120,
  "energy_kcal": 512
}
```

`source` is `manual` (default when omitted) or `apple_health`. `external_id` is
the health store's identity for the workout, trimmed and 1–128 characters; it is
required when `source` is not `manual` and rejected when it is. `distance_meters`
is 1–1,000,000, `energy_kcal` is 1–20,000, and `avg_heart_rate_bpm` is 30–250;
each is optional and stored only when supplied. Zero is rejected. Measurements
are display facts and never feed records, volume, or distribution.

Planned sport sessions (contract 1.2.0) add optional `scheduled_workout_id`.
When present it must name an owned scheduled sport session (a missing or
foreign id is the shared 404; an exercise workout is `422` on
`scheduled_workout_id`) for the same `sport_id` (`422` on `sport_id`) that is
not yet completed (`409 SPORT_SESSION_CONFLICT` with the authoritative session
as `resource`). The session may be `planned` or `missed`; the activity `date`
is the performed date and may differ from the scheduled date. In one
transaction the activity is created with the link, the session becomes
`completed` with any skip cleared and its revision incremented, and
participation is credited to the activity date. The scheduled date's
participation row is left as finalization wrote it. An imported activity
(`source` not `manual`) sent without `scheduled_workout_id` fulfils the
open session for the same sport and date whose `target_duration_minutes` is
closest to the activity's duration, then the lowest `sequence_position`; a
session planned for more than three times the activity's duration is skipped
(contract 1.3.0). A manual activity never links on its own. An import
duplicate (same `source` and `external_id`) returns 200 and never links.
GymPulse never changes a link on its own after it is made; the athlete changes
it with link and unlink below.

Contract 1.3.0 adds optional `perceived_effort`, a whole number from 1 (very
easy) to 10 (maximal) given after the session; it is returned when set and
never feeds records, volume, distribution, or participation. Out-of-range values
return `422` on `perceived_effort`.

#### `POST /api/v1/sport-activities/{id}/link` (contract 1.3.0)

```json
{ "scheduled_workout_id": "<uuid>", "expected_revision": 2, "operation_key": "link-123" }
```

Requires `Idempotency-Key` equal to `operation_key`; `expected_revision` is the
session's. Links an owned unlinked activity to an owned open (`planned` or
`missed`) sport session of the same sport and completes the session exactly as
recording against it would, without crediting participation again. Response
200 is `{ "sport_activity": SportActivity, "scheduled_workout": ScheduledWorkout }`.
A missing or foreign activity or session returns the shared 404; an exercise
workout returns `422` on `kind`; a different sport returns `422` on `sport_id`;
a stale revision returns `409 REVISION_CONFLICT`; an already linked activity or
a completed session returns `409 SPORT_SESSION_CONFLICT` with
`details.reason` `activity_linked` or `session_completed` and the session as
`resource`. A verbatim retry replays the stored response.

#### `POST /api/v1/sport-activities/{id}/unlink` (contract 1.3.0)

```json
{ "expected_revision": 3, "operation_key": "unlink-123" }
```

`expected_revision` is the linked session's. Clears the link. The session
returns to `planned` with `finalized_at` cleared (omitted) when its date is today or later
in the athlete's timezone, otherwise to `missed` (still finalized, any skip
cleared), and that scheduled date counts as a missed opportunity. The activity
and its own date's participation are unchanged. Response 200 is the same pair.
An unlinked activity returns `409 SPORT_SESSION_CONFLICT` with
`details.reason: "not_linked"`; a stale revision returns `409 REVISION_CONFLICT`.

Response 201 is the authoritative activity. `source` is always present;
`external_id`, measurements, and `scheduled_workout_id` appear only when set:

```json
{
  "id": "<uuid>", "date": "2026-08-02",
  "sport_id": "basketball", "sport_name": "Basketball",
  "duration_minutes": 60, "notes": "Pickup game",
  "source": "manual", "scheduled_workout_id": "<uuid> (when linked)",
  "created_at": "<RFC3339>", "updated_at": "<RFC3339>"
}
```

Creation, any session completion, and date participation commit atomically. An identical idempotent retry
returns the original activity with 201; a changed payload returns 409
`IDEMPOTENCY_CONFLICT`. When an imported workout's `source` and `external_id`
already exist for the athlete, the existing activity is returned with 200 and
nothing is written or re-credited, whatever the `operation_key`. Multiple
activities may share a date, but weekly progress counts the date once. A sport
preserves participation on a missed planned date without changing the
planned-workout result. It does not increase `total_workouts`, workout
distribution, or lifted volume.

#### `GET /api/v1/sport-activities/{id}`

Response 200 is the owned activity. A missing or foreign UUID returns the same
404 `NOT_FOUND` response. Sport activities have no edit or delete route in v1;
account deletion removes them.

### Mobility sessions (contract 1.5.0)

A mobility session is one finished stretching routine. Routines are bundled in
the app; the API stores what was performed as facts and does not check
`routine_id` against a catalog. Entries are never counted: stats
(`this_week`, `streak_weeks`, totals), participation, records, exercise
history, volume, distribution, plan, schedule, sport activities, and health
exports ignore them, so a rest day with only stretching stays out of
participation. Entries cannot be edited; the owner may delete one. Additive in
contract 1.5.0; older clients never call these routes.

#### `GET /api/v1/mobility-sessions?from=YYYY-MM-DD&to=YYYY-MM-DD`

Both dates are required and inclusive; `from` after `to` or a range longer
than 366 days returns `422 VALIDATION_ERROR`. Response 200 is a JSON array of
`MobilitySession` ordered by `date`, `created_at`, then `id`, newest first. An
athlete with no entries in the range receives `[]`.

#### `POST /api/v1/mobility-sessions`

Requires `Idempotency-Key` equal to `operation_key` (otherwise `422` on
`Idempotency-Key`).

```json
{
  "date": "2026-11-03",
  "routine_id": "hips-and-hamstrings",
  "routine_name": "Hips and hamstrings",
  "context": "cooldown",
  "duration_seconds": 540,
  "moves_completed": 7,
  "moves_total": 8,
  "content_version": "2026.11.1",
  "operation_key": "mobility-create-123"
}
```

| Field | Rule |
|---|---|
| `date` | Optional `YYYY-MM-DD`; omitted means today in the athlete's saved training-profile timezone. Future dates are rejected. |
| `routine_id` | Lowercase slug (`a-z`, `0-9`, single hyphens between parts), 1–64 characters. |
| `routine_name` | Trimmed, 1–80 characters. |
| `context` | `warmup`, `cooldown`, `rest_day`, or `anytime`. |
| `duration_seconds` | Whole number 1–7,200. |
| `moves_completed` | Whole number 1–100, at most `moves_total`. |
| `moves_total` | Whole number 1–100. |
| `content_version` | Trimmed, 1–32 characters. |
| `operation_key` | Trimmed, 1–128 characters. |

A violation returns `422 VALIDATION_ERROR` with `details.field` naming the
field (`moves_completed` when it exceeds `moves_total`). An athlete without a
training profile receives the shared `404 NOT_FOUND`, as for sport activities.

Response 201 is the stored `MobilitySession`:

```json
{
  "id": "<uuid>", "date": "2026-11-03",
  "routine_id": "hips-and-hamstrings", "routine_name": "Hips and hamstrings",
  "context": "cooldown", "duration_seconds": 540,
  "moves_completed": 7, "moves_total": 8,
  "content_version": "2026.11.1", "operation_key": "mobility-create-123",
  "created_at": "<RFC3339>"
}
```

An identical retry with the same `operation_key` returns **200** with the
original entry unchanged (same `id` and `created_at`), including a retry that
omits `date` and arrives after local midnight. The same key with a different
body returns `409 IDEMPOTENCY_CONFLICT`. Keys are scoped to the athlete. Once an
entry is deleted its key is free again, so a later create with that key records
a new entry.

#### `GET /api/v1/mobility-sessions/{id}`

Response 200 is the owned entry. A missing or foreign UUID returns the shared
`404 NOT_FOUND`; a malformed id returns `400 BAD_REQUEST`.

#### `DELETE /api/v1/mobility-sessions/{id}` → 204

Removes the owned entry. A repeat delete, or a missing or foreign id, returns
`404 NOT_FOUND`. Account deletion removes every entry.

### Knee Kit days (contract 1.7.0)

A Knee Kit day is one athlete's daily knee checklist and pain log for one local
calendar date: the checklist items they ticked, an optional morning and
activity pain score, and two warning flags. The checklist content is bundled in
the app; the API stores item ids as facts and does not check them against a
catalog. The API never diagnoses and computes no guidance, traffic light, or
streak: the app derives those from the range read. Days are never counted:
stats (`this_week`, `streak_weeks`, totals), participation, records, exercise
history, volume, distribution, plan, schedule, day status, sport activities,
mobility sessions, and health exports ignore them. Additive in contract 1.7.0;
older clients never call these routes. Neither route carries a route-specific
rate limit, as for nutrition days.

`KneeKitDay`:

```json
{
  "id": "<uuid>", "date": "2026-11-14",
  "completed_items": ["isometric-hold", "morning-check"],
  "morning_pain": 3, "activity_pain": null,
  "sharp_pain": false, "limping": false,
  "revision": 1, "created_at": "<RFC3339>", "updated_at": "<RFC3339>"
}
```

`completed_items` is always an array (`[]` when nothing is ticked), stored and
returned **sorted ascending by id**, not in request order: it is a set.
`morning_pain` and `activity_pain` are always present, `null` when not logged.

#### `GET /api/v1/knee-kit/days?from=YYYY-MM-DD&to=YYYY-MM-DD`

Both dates are required and inclusive; a malformed date, `from` after `to`, or
a range longer than 366 days returns `422 VALIDATION_ERROR` (the same rule and
error shape as nutrition days). Response `200` is `{ "days": [KneeKitDay] }`,
oldest first; `days` is `[]` when the athlete has none in the range. The read
does not need a training profile.

#### `PUT /api/v1/knee-kit/days/{date}`

Creates or **fully replaces** the athlete's record for one date. Requires
`Idempotency-Key` equal to `operation_key` (otherwise `422` on
`Idempotency-Key`). `date` is `YYYY-MM-DD` and may not be after the athlete's
local today in their saved training-profile timezone; an athlete without a
training profile receives the shared `404 NOT_FOUND`, as for nutrition days.

```json
{
  "completed_items": ["morning-check", "isometric-hold"],
  "morning_pain": 3,
  "activity_pain": null,
  "sharp_pain": false,
  "limping": false,
  "expected_revision": 0,
  "operation_key": "knee-day:2026-11-14:<uuid>"
}
```

| Field | Rule |
|---|---|
| `date` (path) | `YYYY-MM-DD`, a real calendar date, not after the athlete's local today. |
| `completed_items` | Required array; may be `[]`; absent or `null` is rejected. At most 12 entries, each a lowercase slug (`a-z`, `0-9`, single hyphens between parts) of 1–64 characters, no duplicates. Order is not significant. |
| `morning_pain`, `activity_pain` | Optional whole number 0–10; omitted or `null` means not logged. |
| `sharp_pain`, `limping` | Optional boolean; omitted means `false`. |
| `expected_revision` | Required whole number, 0 or greater: `0` when the client believes no record exists for the date, otherwise the stored `revision`. |
| `operation_key` | Trimmed, 1–128 characters. |

A violation returns `422 VALIDATION_ERROR` with `details.field` naming the
field (`date`, `completed_items`, `morning_pain`, `activity_pain`,
`expected_revision`, or `operation_key`). A value of the wrong JSON type (a
fractional pain score, a non-boolean flag, a non-array `completed_items`) is
`400 BAD_REQUEST`.

Response `201` with the stored `KneeKitDay` when the record was created
(`revision` 1), `200` when an existing record was replaced (`revision`
incremented by one). Every field is replaced: a pain value left out becomes
`null` and a flag left out becomes `false`. There is no delete-by-empty: a save
with no items, no pain values, and both flags false is still stored, because
the athlete unticked things. A record is removed only by account deletion.

A stale `expected_revision` returns `409 REVISION_CONFLICT` with
`details.expected_revision`, `details.actual_revision`, and the stored day
under `resource`; `actual_revision` is `0` and `resource` is omitted when no
record exists. An identical retry with the same `operation_key` replays the
original status and body, even after the day has since changed; the same items
in a different order are the same payload. The same key with a different date
or body returns `409 IDEMPOTENCY_CONFLICT`. Keys are scoped to the athlete.

### Health exports

Health exports remember the health-store identity the device received when it
wrote a finished GymPulse workout to Apple Health, so the same workout is never
written twice across retries, reinstalls, or devices. The service never talks
to a health store; the device writes and then records.

A finished workout is a planned workout finalized with a `completed` or
`incomplete` outcome (`workout_kind: scheduled_workout`, the scheduled workout
id) or an off-plan log (`workout_kind: day_log`, the log id). A missed planned
workout, a rest log, and an imported log (`import_id` set) never export and
answer the shared 404. Deleting the
workout (`DELETE /api/v1/scheduled-workouts/{id}` or `DELETE /api/v1/logs/{date}`)
deletes its export record; the service cannot remove the copy in Apple Health.

#### `PUT /api/v1/health-exports/{workout_kind}/{workout_id}`

Requires `Idempotency-Key` equal to `operation_key`.

```json
{
  "source": "apple_health",
  "external_id": "B5D7E9C2-3F44-4B1A-9E0D-7C6A2F1B8D10",
  "exported_at": "2026-10-13T18:42:00Z",
  "operation_key": "hk-export-<workout_id>"
}
```

`source` is `apple_health`. `external_id` is trimmed and 1–128 characters.
`exported_at` is optional RFC3339 and defaults to now; a future value is
clamped to now.

Response 201 is the record, including the workout's local `date`:

```json
{
  "id": "<uuid>", "workout_kind": "scheduled_workout", "workout_id": "<uuid>",
  "date": "2026-10-13", "source": "apple_health",
  "external_id": "B5D7E9C2-3F44-4B1A-9E0D-7C6A2F1B8D10",
  "exported_at": "<RFC3339>", "created_at": "<RFC3339>"
}
```

A verbatim idempotent retry replays 201. The same identity under any
`operation_key` returns the stored record with 200 and writes nothing. A
different identity for a workout that already has one returns 409
`HEALTH_EXPORT_CONFLICT` with the stored record under `resource`. A missing,
foreign, or not-finished workout returns the shared 404 `NOT_FOUND`. Recording
never changes the workout, participation, records, or volume.

#### `GET /api/v1/health-exports?from=YYYY-MM-DD&to=YYYY-MM-DD`

Response 200: `{ "health_exports": [HealthExport] }` for owned exports whose
workout date falls in the inclusive range (at most 366 days), newest workout
date first, then `exported_at`. An athlete with no exports receives `[]`.

### Criteria-based training blocks

Training blocks are private records of criteria the athlete chooses. They do
not diagnose, prescribe treatment, or provide return-to-play clearance. They
never update programs, schedules, workouts, sport activities, participation,
streaks, volume, or records. An optional owned `program_id` is context only.

All mutations require an `operation_key` UUID equal to the `Idempotency-Key`
header. Except create, they also require `expected_revision`. A verbatim retry
replays the original full aggregate; a changed operation payload returns 409
`IDEMPOTENCY_CONFLICT`. A stale revision returns 409 `REVISION_CONFLICT` with
the current aggregate under `resource`. Every nested identifier is ownership
scoped; a missing or foreign resource returns the same 404 `NOT_FOUND` shape.

#### `GET /api/v1/training-blocks?status=active&limit=20&offset=0`

`status` is `active`, `completed`, `archived`, or `all` (default `active`).
`limit` defaults to 20 and is bounded from 1 through 100; `offset` is zero or
greater. Summaries are ordered by `updated_at`, then `id`, newest first.

```json
{
  "training_blocks": [{
    "id": "<uuid>", "name": "Return to spiking", "status": "active",
    "revision": 3,
    "current_stage": { "id": "<uuid>", "stage_order": 1, "name": "Controlled contacts", "load_level": "easy", "required_qualifying_exposures": 2 },
    "current_stage_progress": { "required_qualifying_exposures": 2, "qualifying_exposures": 1, "criteria_complete": false },
    "pending_next_morning_count": 1, "updated_at": "<RFC3339>"
  }],
  "next_offset": null
}
```

#### `POST /api/v1/training-blocks`

Creates 2–12 immutable ordered stages. `name` is 1–120 characters; optional
`purpose` is at most 500. Stage names are 1–120, instructions at most 1,000,
`load_level` is `easy` or `demanding`, count is 1–10,000, duration is 1–1,440
minutes, intensity is 1–100 percent, and required qualifying exposures are
1–20. Optional targets may be omitted. Response 201 is the full aggregate at
stage one, active, revision 1, with empty exposure and transition arrays.

#### `GET /api/v1/training-blocks/{block_id}`

Response 200 is the full aggregate: ordered `stages`, newest-first `exposures`,
oldest-first `transitions`, and authoritative current-stage progress.

#### `POST /api/v1/training-blocks/{block_id}/exposures`

```json
{
  "performed_on": "2026-08-04", "activity_label": "Volleyball hitting",
  "load_level": "demanding", "performed_count": 20,
  "duration_minutes": 15, "performed_intensity_percent": 70,
  "session_outcome": "completed_as_planned", "notes": "Controlled setting",
  "expected_revision": 1, "operation_key": "<uuid>"
}
```

The local date cannot be future in the saved training-profile timezone. An
active block accepts an exposure only for its current stage. The response is
the revision-incremented aggregate with `next_morning_response` absent and
`qualifies:false`.

#### `POST /api/v1/training-blocks/{block_id}/exposures/{exposure_id}/next-morning`

Body fields are `response` (`baseline` or `above_baseline`), revision, and
operation key. A response can be recorded once. `qualifies` is server-derived
and true only for `completed_as_planned` plus `baseline`; the client never sends
it. Modified, stopped, pending, and above-baseline exposures remain visible but
do not qualify.

#### `POST /api/v1/training-blocks/{block_id}/transitions`

`action` is `advance`, `regress`, `complete`, or `archive`. Advance requires
sufficient authoritative evidence and moves exactly one stage. Regression
requires an earlier `to_stage_id` and a 1–500 character `reason`. Completion
requires sufficient evidence at the final stage. Archive is explicit and may
follow active or completed state. Each success appends transition history,
increments the block revision once, and returns the full aggregate. No action
is automatic and no individual history record has an edit or delete route.

---

## Exercise catalog (read-only v1)

### `GET /api/v1/exercises` (optional `?category=<type_id>`)
Response 200 — **wraps the array under an `exercises` key** (same wrapper
convention as `/stats/distribution`):
```json
{
  "exercises": [
    { "id": "<uuid>", "name": "Barbell Bench Press", "category": "push",
      "modality": "strength", "mechanic": "compound", "sort_order": 1,
      "pattern": "horizontal_push", "muscle_group": "chest",
      "equipment_required": ["barbell"], "impact": "low", "sub_pattern": null }
  ]
}
```
- `modality`: `"strength" | "cardio"`. `mechanic`: `"compound" | "isolation"`,
  **null for cardio entries** — client prefill keys off these two fields.
- Programming metadata (migration 032, used by the program engine): `pattern`
  (`squat`, `hinge`, `lunge`, `horizontal_push`, `vertical_push`,
  `horizontal_pull`, `vertical_pull`, `carry`, `core`, `explosive`,
  `locomotion`, `isolation`), `muscle_group` (`chest`, `back`, `shoulders`,
  `arms`, `quads`, `hamstrings`, `glutes`, `calves`, `core`, `full_body`),
  `equipment_required` (non-empty subset of the training-profile equipment
  values; `["bodyweight"]` means always available), `impact` (`low` | `high`),
  and `sub_pattern` (migration 034; `incline_push`, `chest_isolation`,
  `shoulder_isolation`, `rear_delt`, `triceps`, `biceps`, `quad_isolation`,
  `hamstring_isolation`, `glute_isolation`, `calf`, or `null`).
- Unknown `category` → 422 `VALIDATION_ERROR`. Valid categories are the
  workout type ids (push, pull, legs, upper, lower, full, core, cardio, …).
- Catalog is seeded by migration (83 entries); no write endpoints in v1.

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
  "session_notes": "string|null",
  "operation_key": "string (optional)"
}
```

**Optional idempotency (offline finish).** A client that may deliver the same
finish more than once (an offline outbox, a retry after a lost response) sends
`Idempotency-Key: <stable client operation key>` and an equal `operation_key`
body field. The identical key and identical JSON payload returns the
originally stored `201` body, including the original `id` and `logged_at`,
and never creates a second log. Reusing the key with a different payload
returns `409 IDEMPOTENCY_CONFLICT` (same shape as goal-based training). A key
in only one location, or different values in the two locations, returns
`422 VALIDATION_ERROR` with `details.field = "Idempotency-Key"`. Omitting both
keeps the legacy, non-idempotent create, so released clients are unaffected.
Replay is checked after request validation, so a replay must still be a valid
request (the template must still be owned, and the date not in the future).

`set_logs` is the per-set history behind the active workout player.
`set_index` is 1-based (≥ 1). A `completed: true` set must include `actual_reps`
or `duration_seconds` (cardio) → 422 otherwise. `target_*` snapshot the plan at
session start; `actual_*` are what was performed. `overrides` (per-exercise
aggregates: notes/skipped) and `set_logs` coexist.
Exercise references must belong to the authenticated user and, for a templated
log, to that template; invalid or foreign references return 422 without writing
any part of the log. Immutable exercise name/category/modality snapshots are
derived server-side so released clients keep their existing payload shape.

### `GET|PUT|DELETE /api/v1/logs/{date}` — `date` is `YYYY-MM-DD`.

`GET` returns the full `DayLog` including a `set_logs[]` array (flat; group by
`exercise_id` client-side).

Since contract 1.6.0 (additive), every `DayLog` and weekly `DayLog` summary
carries `import_id` (`null` unless the log was created by a workout import,
see "Workout history import"), and each set log carries the immutable
`exercise_name`, `exercise_category`, and `exercise_modality` snapshot.
`exercise_id` is `null` when the set has no saved-workout exercise (an
imported set with no single saved-workout exercise of the same name, or a set
whose exercise was later deleted); clients group such sets by
`exercise_name`. Set logs are ordered by when they were performed, then
`exercise_id` and `set_index`.

**PUT also replaces the day's workout** when any of `type_id`, `subtype_id`,
`template_id` are present:
- `template_id` given → it is **authoritative**: ownership checked (404 if
  not yours), `type_id`/`subtype_id` derive from the template, anything sent
  in the body is ignored.
- type-only replacement requires BOTH `type_id` and `subtype_id` (valid ids) → 422 otherwise.
- For an ordinary partial update, omitted `overrides`, `set_logs`, and
  `session_notes` preserve their existing values. A present collection
  replaces the named collection; `[]` or `null` explicitly clears it.
- A present `session_notes` replaces the notes. `""` and `null` clear them and
  are stored canonically as `null`.
- Replacing the workout clears omitted `overrides` and `set_logs`, because
  exercise detail from the old workout is incompatible. Collections supplied
  with the replacement become the new detail. Omitted notes remain unchanged.
- Replacing a day to `type_id: "rest"` while sending `overrides` → 422
  (rest days carry no overrides, mirroring POST).
- Rest day + non-empty `set_logs` → 422.

`DELETE` permanently removes the athlete's log for that date, including past
dates, with its overrides, set logs, and Apple Health export record. Response
204. A date with no log (or another athlete's) → 404, so a repeated delete is
also 404. A malformed date → 422. The workout already written to Apple Health
stays on the device.

### `GET /api/v1/exercises/history?ids=<uuid,uuid>` → `ExerciseHistory[]`
For each requested exercise id, returns the completed sets from the **most
recent day** that exercise was performed — powers "last time you did X" hints.
One round-trip for a whole workout. Unknown ids are simply omitted; a malformed
id or more than 100 ids → 422.
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
streak, and `total_workouts` (this matches the client mock's behavior). A sport
activity contributes its local date to `this_week.completed` and the week streak,
at most once per date, but is not included in `total_workouts`.
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

## Nutrition

Nutrition is the athlete's own daily totals and the daily target they chose.
GymPulse stores totals only: no food, meal, or portion data, and it never
computes or changes a target. All dates are the athlete's local calendar dates
in their saved training-profile timezone; the two `PUT` routes and `GET
/api/v1/nutrition/target` return the shared `404 NOT_FOUND` for an athlete
with no training profile, while the range read does not need one. Both mutations require
`Idempotency-Key` equal to `operation_key`; an identical retry replays the
original status and body, and a changed payload returns `409
IDEMPOTENCY_CONFLICT`. Energy is whole kilocalories (0–20,000); macros are
grams with at most one decimal (0–2,000). Validation failures are `422` with
`details.field` naming the offending field.

### `PUT /api/v1/nutrition/days/{date}`

Saves, replaces, or removes the athlete's totals for one date. `date` is
`YYYY-MM-DD` and may not be after the athlete's local today.

```json
{
  "energy_kcal": 2150,
  "protein_g": 162.5,
  "carbs_g": 230,
  "fat_g": 70,
  "source": "manual",
  "expected_revision": 0,
  "operation_key": "nutrition-day:2026-10-25:<uuid>"
}
```

Every value is optional. `source` is `manual` (default when omitted) or
`apple_health`. A stored day may also carry `source: "foods"`, meaning its
totals are the sum of its food entries (see "Food logging"); clients never send
that value.

An **athlete save** (`manual`) carries `expected_revision`: `0` when no entry
exists for the date, otherwise the stored revision. A stale value returns `409
REVISION_CONFLICT` with the stored day under `resource` (`actual_revision` is
`0` when the day no longer exists). A save over an `apple_health` day converts
it to `manual`. A save with all four values absent removes the day and returns
`204`; with no day to remove it also returns `204` and writes nothing. Removal
carries `expected_revision` like any athlete save.

A **Health save** (`apple_health`) carries no `expected_revision` (one that is
sent is ignored) and must carry at least one value. It creates the day or
replaces an existing `apple_health` day. Over a `manual` or `foods` day it
returns the stored day unchanged with `200` and writes nothing: the athlete's
entry wins.

A **typed save or remove on a `foods` day** is refused with `409
DAY_HAS_FOOD_ENTRIES` and the stored day under `resource`; change or remove the
entries instead (`POST /days/{date}/entries` with `quick_add` adds typed totals
beside them).

Response `201` on create, `200` on replace or on an ignored Health save:

```json
{
  "id": "<uuid>", "date": "2026-10-25",
  "energy_kcal": 2150, "protein_g": 162.5, "carbs_g": 230, "fat_g": 70,
  "source": "manual", "revision": 1,
  "logged_at": "<RFC3339>", "created_at": "<RFC3339>", "updated_at": "<RFC3339>"
}
```

Absent values are omitted from the response. Saving never changes body weight,
participation, workouts, records, or volume.

### `GET /api/v1/nutrition/days?from=YYYY-MM-DD&to=YYYY-MM-DD`

Response `200`: `{ "days": [NutritionDay], "targets": [NutritionTarget] }`.
`days` holds the owned days inside the inclusive range (at most 366 days),
oldest first. `targets` holds the target in effect on `from` (the latest
`effective_from` on or before it, if any) plus every target whose
`effective_from` falls inside the range, oldest first, so a client resolves each
day's target as the latest `effective_from` on or before that day. Both are
`[]` when empty.

### `GET /api/v1/nutrition/target`

Response `200` is the target in effect on the athlete's local today. No saved
target returns the shared `404 NOT_FOUND`.

```json
{
  "id": "<uuid>", "effective_from": "2026-10-25",
  "protein_g": 150, "energy_kcal": null, "carbs_g": null, "fat_g": null,
  "revision": 1, "created_at": "<RFC3339>", "updated_at": "<RFC3339>"
}
```

Optional target values are always present and `null` when unset.

### `PUT /api/v1/nutrition/target`

```json
{
  "effective_from": "2026-10-25",
  "protein_g": 150,
  "energy_kcal": 2200,
  "carbs_g": null,
  "fat_g": null,
  "expected_revision": 0,
  "operation_key": "nutrition-target:<uuid>"
}
```

`protein_g` is required and greater than zero; `energy_kcal` (1–20,000),
`carbs_g`, and `fat_g` are optional. `effective_from` defaults to the athlete's
local today and may not be in the future. `expected_revision` is the current
target's revision, `0` when none exists; a stale value returns `409
REVISION_CONFLICT` with the current target under `resource`. Saving for an
`effective_from` that already has a target replaces that row; a different date
adds one and earlier targets are kept for the days they covered. Values
identical to the current target with the same `effective_from` return it
unchanged.

Response `201` on create, `200` on replace or unchanged; the body is the saved
target.

---

## Food logging

Food logging adds foods to a day. Facts come from USDA FoodData Central first
and Open Food Facts when a barcode is not found there; the API makes those
calls with server-held credentials and keeps a copy of every food it returns in
a shared catalog, so repeat lookups never leave GymPulse. A food the athlete
creates from a label is private (`source: "own"`). Every entry stores the values
it was logged with; a day's totals (`nutrition_days`, `source: "foods"`) are the
sum of its entries and are rewritten in the same transaction as every entry
change. Mutations require `Idempotency-Key` equal to `operation_key` (the header
alone on body-less deletes); a verbatim retry replays the original status and
body. Dates follow "Nutrition": local calendar dates, never after local today.
Search and barcode lookups are limited to 60 per athlete per minute (`429
RATE_LIMITED`).

### `Food`

```json
{
  "id": "<uuid>", "source": "usda",
  "name": "Greek yogurt, plain, nonfat", "brand": "Kirkland Signature",
  "barcode": "0096619123452",
  "servings": [{ "label": "1 container (170 g)", "grams": 170 }, { "label": "100 g", "grams": 100 }],
  "per_100g": { "energy_kcal": 59, "protein_g": 10.2, "carbs_g": 3.6, "fat_g": 0.4 },
  "attribution": null, "revision": null, "archived": false
}
```

`source` is `usda`, `open_food_facts`, or `own`. Nullable keys are always
present. `servings` always includes `100 g`. `attribution` is set only for Open
Food Facts (`{ "name": "Open Food Facts", "url": "https://world.openfoodfacts.org/product/<code>", "license": "ODbL" }`)
and must be shown wherever that food is displayed. `revision` and `archived`
are set only for `own` foods. `barcode` is the 13-digit form when the GTIN fits
it, otherwise 14 digits.

### `GET /api/v1/nutrition/foods/search?q=<text>&limit=25`

`q` is 2–100 characters; `limit` is 1–25 (default 25). Response `200`:
`{ "foods": [Food], "incomplete": false }`. The athlete's own foods come first,
then USDA matches in source order, then catalog foods, de-duplicated. `foods`
is `[]` when nothing matches. `incomplete: true` means USDA could not be asked
(no key, timeout, `429`, `5xx`) and only stored foods are listed.

### `GET /api/v1/nutrition/foods/barcode/{code}`

`code` is 8, 12, 13, or 14 digits (spaces and hyphens allowed) with a valid GS1
check digit, otherwise `422` on `code`. Equivalent forms match the same product.
Order: the athlete's own food with that barcode → catalog → USDA (Branded,
exact GTIN match) → Open Food Facts. Response `200 Food`. `404 NOT_FOUND` when
no source has a usable product; a miss is remembered for seven days. When the
Open Food Facts budget (shared by every athlete, 12 reads a minute) is spent
before the code could be asked, the `404` carries
`details.retry_after_seconds` and nothing is remembered.

### `GET /api/v1/nutrition/foods/recent`

Response `200`: `{ "foods": [RecentFood] }`, up to 20 foods logged in the last
30 days, most often then most recently first, `[]` when none. Quick adds are
excluded.

```json
{ "food": { "...": "Food" }, "last_serving": { "label": "1 container (170 g)", "grams": 170 }, "last_amount": 1.5, "times_logged": 6, "last_logged_at": "<RFC3339>" }
```

### `POST /api/v1/nutrition/foods` → `201 Food`

Creates a private food from a label, typed per serving.

```json
{
  "name": "Whey isolate", "brand": "Acme", "barcode": "096619123452",
  "serving": { "label": "1 scoop", "grams": 30 },
  "energy_kcal": 120, "protein_g": 24, "carbs_g": 3, "fat_g": 1.5,
  "operation_key": "user-food:<uuid>"
}
```

`name` 1–200, `brand` ≤ 120 (optional), `barcode` optional (same rules as the
lookup; `422` on `barcode`), serving label 1–60, serving grams 0–5,000 with one
decimal, `energy_kcal` 0–20,000, macros 0–2,000 with one decimal. The response
is a `Food` with `source: "own"`, `revision: 1`, the typed serving plus `100 g`,
and `per_100g` derived from the label.

### `PUT /api/v1/nutrition/foods/{id}` → `200 Food`

Same body plus `expected_revision` (the food's current revision, ≥ 1). Stale →
`409 REVISION_CONFLICT` with the food under `resource`. Existing entries keep
the values they were logged with.

### `DELETE /api/v1/nutrition/foods/{id}?expected_revision=<n>` → `204`

Archives the food (`Idempotency-Key` header is the operation key). An archived
food no longer appears in search or barcode lookups and cannot start a new
entry, but existing entries and recents still resolve it. Unknown, foreign, and
malformed ids are the shared `404`.

### `GET /api/v1/nutrition/days/{date}/entries`

Response `200`: `{ "day": NutritionDay | null, "entries": [FoodEntry] }`,
entries oldest first, `[]` when none.

```json
{
  "id": "<uuid>", "date": "2026-10-27", "kind": "food",
  "food": { "id": "<uuid>", "source": "usda", "name": "Greek yogurt, plain, nonfat", "brand": "Kirkland Signature" },
  "serving": { "label": "1 container (170 g)", "grams": 170 }, "amount": 1.5,
  "energy_kcal": 150, "protein_g": 26, "carbs_g": 9.2, "fat_g": 1,
  "section": "breakfast",
  "revision": 1, "logged_at": "<RFC3339>", "created_at": "<RFC3339>", "updated_at": "<RFC3339>"
}
```

`section` (contract 1.8.0, additive) is the day heading the athlete put the
entry under: `breakfast`, `lunch`, `dinner`, `snacks`, or `null` for none. It
is always present. A heading is a label only: it changes no value and no total,
carries no time or target, and the API never infers one. Entries logged before
1.8.0 or by an older client have `null`. Entries are still listed oldest first;
grouping is the client's.

A `quick_add` entry has `food`, `serving`, and `amount` null and omits absent
values. Entry values are `per_100g × grams × amount / 100`, energy rounded to a
whole kcal and macros to one decimal.

### `POST /api/v1/nutrition/days/{date}/entries` → `201`

```json
{ "food": { "id": "<uuid>" }, "serving": { "label": "1 container (170 g)", "grams": 170 }, "amount": 1.5, "operation_key": "food-entry:<uuid>" }
```

or `{ "quick_add": { "energy_kcal": 600, "protein_g": 40 }, "operation_key": "..." }`.
Exactly one of `food` or `quick_add`. With `food`, `serving` must be one of the
food's servings or a grams-only serving `{ "label": "<n> g", "grams": n }`, and
`amount` is 0.1–50 with at most two decimals. `quick_add` needs at least one
value and follows the day-total bounds. `logged_at` is optional (default now).
Optional `section` (contract 1.8.0) puts the entry, food or quick add, under
one of the four headings; absent or `null` means none, and any other value is
`422` on `section`. An unknown, foreign, or archived food is the shared `404`.

Response: `{ "entry": FoodEntry, "day": NutritionDay, "converted": [FoodEntry] }`.
When the date already held typed totals (`manual` or `apple_health`) and no
entries, those totals become one `quick_add` entry, returned under `converted`,
so nothing the athlete saved disappears. `day.source` is `foods` from then on
and its `revision` advances on every entry change.

### `PUT /api/v1/nutrition/entries/{id}` → `200`

`{ "serving": {...}, "amount": 2, "expected_revision": 1, "operation_key": "..." }`
for a food entry (values are recomputed from the food's current facts), or
`{ "quick_add": {...}, "expected_revision": 1, "operation_key": "..." }` for a
quick add. Mixing the shapes is `422`. Stale → `409 REVISION_CONFLICT` with the
entry under `resource`. Response `{ "entry": FoodEntry, "day": NutritionDay }`.

The entry keeps its `section`; this route neither reads nor changes it.

### `PUT /api/v1/nutrition/entries/{id}/section` → `200` (contract 1.8.0)

`{ "section": "lunch", "expected_revision": 2, "operation_key": "..." }` moves
the entry under another heading; `"section": null` removes the heading. Only
`section`, `revision`, and `updated_at` change: unlike the route above, the
entry's serving, amount, and values are not recomputed, and the day's totals
and revision are untouched. Stale → `409 REVISION_CONFLICT` with the entry
under `resource`; an unknown heading is `422` on `section`; unknown, foreign,
and malformed ids are the shared `404`. Response
`{ "entry": FoodEntry, "day": NutritionDay }` with the day as it stands.

### `DELETE /api/v1/nutrition/entries/{id}?expected_revision=<n>` → `200`

`Idempotency-Key` header is the operation key. Response `{ "day": NutritionDay | null }`;
`day` is `null` when the last entry was removed and the day's totals with it.

### Codes

`VALIDATION_ERROR` (`details.field` ∈ `q`, `limit`, `code`, `date`, `name`,
`brand`, `barcode`, `serving`, `amount`, `food`, `quick_add`, `energy_kcal`,
`protein_g`, `carbs_g`, `fat_g`, `section`, `expected_revision`,
`operation_key`, `Idempotency-Key`), `NOT_FOUND`, `IDEMPOTENCY_CONFLICT`,
`REVISION_CONFLICT`, `DAY_HAS_FOOD_ENTRIES` (on `PUT /days/{date}` only), `RATE_LIMITED`.

### Configuration

`USDA_FDC_API_KEY` (free key from FoodData Central; empty means degraded,
catalog-only search and a startup warning in staging and production),
`FOOD_SOURCE_USER_AGENT` (identifying agent Open Food Facts requires; defaults
to `GymPulse/1.0 (...)`), `USDA_FDC_BASE_URL` and `OPEN_FOOD_FACTS_BASE_URL`
(test and staging overrides). Keys never appear in logs or responses.

## Saved meals

Saved meals (contract 1.8.0, additive) let an athlete save foods they log
together as a named meal and log it again in one request. A meal is only a
name and an ordered list of foods, each with a serving and an amount. It stores
no energy or macro values: `items[].energy_kcal` and the other three, and
`totals`, are computed on every read from each food's current facts. GymPulse
supplies, suggests, and shares no meals. Logging a meal creates one ordinary
`FoodEntry` per item; the entries carry no link to the meal, so the entry
routes under "Food logging" apply to them unchanged and changing or deleting a
meal never changes a day already logged. Older clients never call these routes.
Mutations require `Idempotency-Key` equal to `operation_key` (the header alone
on the body-less delete); a verbatim retry replays the original status and body.

### `SavedMeal`

```json
{
  "id": "<uuid>", "name": "Usual breakfast", "section": "breakfast",
  "items": [
    {
      "food": { "...": "Food" },
      "serving": { "label": "1 cup (81 g)", "grams": 81 }, "amount": 1,
      "available": true,
      "energy_kcal": 307, "protein_g": 10.7, "carbs_g": 54.8, "fat_g": 5.3
    }
  ],
  "totals": { "energy_kcal": 307, "protein_g": 10.7, "carbs_g": 54.8, "fat_g": 5.3 },
  "revision": 1, "last_logged_at": null,
  "created_at": "<RFC3339>", "updated_at": "<RFC3339>"
}
```

`items` has 1–20 entries in the saved order and each carries the full `Food`
(including `attribution` for Open Food Facts, and `archived: true` for an own
food archived after it was added). Item values are `per_100g × grams × amount /
100`, energy to a whole kcal and macros to one decimal, the same rule as an
entry; `totals` is their sum. When a food's current facts cannot be shown the
item has `available: false` and omits its four values, and `totals` is `null`.
`section` is the day heading the meal remembers (`breakfast`, `lunch`,
`dinner`, `snacks`, or `null`); its entries are logged under it unless the log
names another. `revision` advances when the name, heading, or items change;
logging the meal sets `last_logged_at` and does not advance it.

A saved item keeps its serving's label and weight. On every read and on a log
as saved, the label is matched to the food's current servings and the food's
own weight is used, so the list and the log always agree and a serving the
athlete re-weighed ("1 scoop" from 30 g to 32 g) follows the food. When the
food no longer has a serving with that label, the saved label and weight are
used as they are. `items[].serving` is the serving that will be logged.

### `GET /api/v1/nutrition/meals`

Response `200`: `{ "meals": [SavedMeal] }`, most recently logged first, then
most recently saved; `[]` when none. At most 50.

### `POST /api/v1/nutrition/meals` → `201 SavedMeal`

```json
{
  "name": "Usual breakfast", "section": "breakfast",
  "items": [
    { "food": { "id": "<uuid>" }, "serving": { "label": "1 cup (81 g)", "grams": 81 }, "amount": 1 }
  ],
  "operation_key": "saved-meal:<uuid>"
}
```

`name` is trimmed, 1–60 characters, and unique among the athlete's saved meals
ignoring case (`422` on `name`). `items` holds 1–20 foods. Each item needs a
`food.id` from search, barcode, recent, or create; a `serving` that is one of
the food's servings or a grams-only serving `{ "label": "<n> g", "grams": n }`
with `grams` 0.1–5,000; and an `amount` of 0.1–50 with at most two decimals.
Serving weights are stored to one decimal. The same food and serving may
appear once (compared after that rounding); the same food with two different
servings is allowed.
Optional `section` is one of the four headings; absent or `null` means the
meal remembers none (`422` on `section` otherwise). `expected_revision` must
be absent. A fault in one item is `422` with
`details.field: "items"` and `details.index` set to the item's zero-based
position; a food that is unknown, another athlete's, or archived gets one
shared message. An athlete may hold 50 saved meals; one more is
`409 SAVED_MEAL_LIMIT_REACHED` with `details.limit: 50`.

### `PUT /api/v1/nutrition/meals/{id}` → `200 SavedMeal`

Same body plus `expected_revision` (the meal's current revision, ≥ 1). Replaces
the name, the heading (absent clears it), and the whole item list. Stale → `409 REVISION_CONFLICT` with the
stored meal under `resource`. A food the meal already holds may stay after the
athlete archives it; an archived food cannot be added. Unknown, foreign, and
malformed ids are the shared `404`.

### `DELETE /api/v1/nutrition/meals/{id}?expected_revision=<n>` → `204`

Deletes the meal (`Idempotency-Key` header is the operation key). Entries it
logged stay on their days. Stale → `409 REVISION_CONFLICT`; unknown, foreign,
and malformed ids are the shared `404`.

### `POST /api/v1/nutrition/days/{date}/meal-logs` → `201`

```json
{ "meal": { "id": "<uuid>" }, "operation_key": "meal-log:<uuid>" }
```

Logs the saved meal to `date` (a local calendar date, never after local today)
as one food entry per item, in item order, each with the food's facts at that
moment. All entries and the day's totals change in one transaction, or nothing
changes. Optional `items` (same shape and rules as on save) replaces the saved
items for this log only, for a different amount or a food left out, and never
changes the saved meal; an archived own food is accepted only when the saved
meal holds it. Override servings are new input and must match the food's
current servings exactly, as on save. Optional `section` puts every entry of
this log under that heading; absent or `null` uses the meal's own heading, or
none when it has none. `logged_at` is optional (default now).

Response: `{ "entries": [FoodEntry], "day": NutritionDay, "converted": [FoodEntry] }`.
`converted` follows `POST /days/{date}/entries`: typed totals already on the
date (`manual` or `apple_health`, no entries) become one `quick_add` entry
first. An item that cannot be logged is `422` on `items` with `details.index`
and nothing is logged. An unknown or foreign meal is the shared `404`. To undo
a log, delete the returned entries with `DELETE /entries/{id}`.

### Codes

`VALIDATION_ERROR` (`details.field` ∈ `name`, `section`, `items` (with
`details.index` when one item is at fault), `date`, `meal`, `expected_revision`,
`operation_key`, `Idempotency-Key`), `NOT_FOUND`, `IDEMPOTENCY_CONFLICT`,
`REVISION_CONFLICT`, `SAVED_MEAL_LIMIT_REACHED`.

## Workout history import

Workout history import (contract 1.6.0, additive) brings an athlete's history
over from a Strong or Hevy CSV export. The file is parsed by the API, never
stored, and becomes one day log per new calendar date: `type_id: "other"`,
`subtype_id: "general"`, no template, `import_id` set, every set `completed`.
The day's `session_notes` hold the source workout names and notes. Supported
layouts are the exact header sets in
`specs/026-workout-history-import/spec.md`; anything else is rejected.

Rules shared by preview and commit:

- A date that already holds any day log (recorded in GymPulse or imported
  earlier) is skipped whole and counted in `days_already_present`; an import
  never modifies an existing log. Importing the same file twice adds nothing.
- Dates after today in the athlete's training-profile timezone (UTC when none)
  are skipped and counted in `days_skipped_future`. Export times carry no
  timezone and are read as written in that timezone.
- Warm-up sets (Strong `W`, Hevy `warmup`) are skipped and counted. Drop and
  failure sets import as ordinary sets. Strong `Rest Timer` and `Note` rows are
  ignored. A set with neither reps nor seconds is skipped and counted in
  `empty_sets_skipped`. Distance and RPE are not stored; kept sets that had a
  distance are counted in `sets_losing_distance`.
- Weights are converted to the athlete's `settings.weight_unit`
  (1 lb = 0.45359237 kg, rounded to 0.01). The file's unit comes from its
  header or rows; the legacy Strong layout states none, so the preview reports
  `source_weight_unit: null` and the commit must send one.
- Exercise names resolve to the catalog (alias table, exact name,
  `Movement (Equipment)` → `Equipment Movement`, then the movement alone) and
  take its name, category, and modality. Other names are unmatched: `keep`
  imports them under their own name with category `other`; `skip` drops their
  sets. A set links to a saved-workout exercise (`exercise_id`) only when
  exactly one of the athlete's saved-workout exercises has its resolved or
  source name, so records and "last time" include it.
- Imports write no participation, scheduled workouts, templates, or health
  export records. Imported days count toward total workouts, weekly volume,
  and the weekly goal like any off-plan log.
- Limits: file ≤ 10 MiB (`413 REQUEST_TOO_LARGE`, `details.max_bytes`),
  ≤ 100,000 rows and ≤ 5,000 dates (`422 IMPORT_TOO_LARGE`, `details.max_rows`
  or `details.max_days`).

File errors are `422` with one of these codes; nothing is written:

| Code | When | `details` |
| --- | --- | --- |
| `IMPORT_UNRECOGNIZED` | Header is not a supported layout | `expected` (where each export lives) |
| `IMPORT_INVALID_ROW` | Unreadable CSV row, date, number, set order, or set type; weight above 99,999.99 | `line`, `reason` |
| `IMPORT_TOO_LARGE` | Row or date limit exceeded | `max_rows` or `max_days` |
| `IMPORT_UNIT_REQUIRED` | Commit of a file that states no unit, without `source_weight_unit` | `field` |

Both POST routes take `multipart/form-data` with exactly one `file` part
(otherwise `400 BAD_REQUEST`); other fields are at most 256 bytes.

### `POST /api/v1/imports/preview` → `200 ImportPreview`

Fields: `file`, optional `source_weight_unit` (`lb`|`kg`). Writes nothing.

```json
{
  "source": "strong",
  "source_weight_unit": "kg",
  "weight_unit": "lb",
  "file_first_date": "2023-01-02", "file_last_date": "2025-03-24",
  "file_workout_count": 214,
  "keep": { "workout_count": 210, "day_count": 208, "set_count": 4310,
            "first_date": "2023-01-02", "last_date": "2025-03-24" },
  "skip": { "workout_count": 209, "day_count": 207, "set_count": 4102,
            "first_date": "2023-01-02", "last_date": "2025-03-24" },
  "days_already_present": 3,
  "days_skipped_future": 0,
  "warmup_sets_skipped": 388,
  "empty_sets_skipped": 2,
  "sets_losing_distance": 14,
  "matched_exercise_count": 31,
  "unmatched_exercises": [ { "name": "Cable Kickback", "set_count": 208 } ]
}
```

`keep` and `skip` are what a commit with that `unmatched` choice writes;
their `first_date`/`last_date` are `null` when nothing would be written.
`unmatched_exercises` is in file order and `[]` when every name matched.

### `POST /api/v1/imports` → `201 WorkoutImport`

Requires `Idempotency-Key` equal to the `operation_key` field. Fields: `file`,
`unmatched` (`keep`|`skip`), `operation_key` (1–128 characters), and
`source_weight_unit` when the file states no unit. All days commit in one
transaction or none do.

```json
{
  "id": "<uuid>", "source": "strong", "source_weight_unit": "kg",
  "weight_unit": "lb", "unmatched": "keep",
  "first_date": "2023-01-02", "last_date": "2025-03-24",
  "workout_count": 210, "day_count": 208, "set_count": 4310,
  "created_at": "<RFC3339>"
}
```

The identical file and options under the same key replay the stored import
with `200`; the same key with a different file or options is
`409 IDEMPOTENCY_CONFLICT`. A file with nothing new to import is
`422 VALIDATION_ERROR` with `details.field = "file"`. A day that gained a log
between preview and commit is `409 CONFLICT`; preview again.

### `GET /api/v1/imports` → `200 { "imports": [WorkoutImport] }`

Newest first; `[]` when there are none.

### `DELETE /api/v1/imports/{id}` → `204`

Removes the import and every day log it created, with their set logs,
including changes made to those days since. Other logs are untouched. A
missing or foreign id, or a repeated delete, is `404 NOT_FOUND`.

## Data export

Data export (contract 1.4.0) lets the athlete take everything they authored or
performed out of GymPulse. Both endpoints are read-only, scoped to the
authenticated user, and read every section from one repeatable-read snapshot,
so a workout finished during the export appears whole or not at all. They
share a per-user limit of 6 requests per minute (`429 RATE_LIMITED`). Both
responses carry `Content-Disposition: attachment;
filename="gympulse-export-YYYY-MM-DD.<json|csv>"` (UTC date of `exported_at`).
A read failure answers the standard JSON error with no attachment header.

### `GET /api/v1/export` → `200 DataExport`

```json
{
  "schema_version": 1,
  "exported_at": "2026-11-03T14:05:06Z",
  "profile": { "display_name": "Sam" | null, "created_at": "RFC3339" } | null,
  "settings": { "weight_unit": "lb" | "kg", "weekly_goal": 5 } | null,
  "training_profile": {
    "primary_goal": "strength", "available_days": [1, 3, 5],
    "usual_activity": "moderate", "experience": "beginner",
    "equipment": ["barbell"], "session_duration_minutes": 45, "timezone": "UTC"
  } | null,
  "workouts": [{
    "id": "uuid", "source": "session" | "day_log", "date": "YYYY-MM-DD",
    "name": "Push day", "status": "draft" | "active" | "completed" | "logged" | "rest",
    "planned": true, "notes": "text" | null,
    "started_at": "RFC3339" | null, "completed_at": "RFC3339" | null,
    "sets": [{
      "exercise_name": "Bench Press", "exercise_category": "push",
      "exercise_modality": "strength" | "cardio", "set_index": 1,
      "target_reps": 5 | null, "target_weight": 80 | null,
      "actual_reps": 5 | null, "actual_weight": 82.5 | null,
      "duration_seconds": 600 | null, "completed": true, "extra": false
    }],
    "exercises": [{
      "exercise_name": "Squat", "actual_sets": 3 | null, "actual_reps": 5 | null,
      "actual_weight": 100 | null, "notes": "text" | null, "skipped": false
    }]
  }],
  "templates": [{
    "id": "uuid", "name": "Upper A", "type_id": "strength", "subtype_id": "general",
    "created_at": "RFC3339", "updated_at": "RFC3339",
    "exercises": [{ "name": "Row", "sets": 3 | null, "reps": 8 | null, "weight": 60 | null,
      "rest_seconds": 90 | null, "duration_minutes": null, "intensity": null, "notes": null }]
  }],
  "sport_activities": [{
    "id": "uuid", "date": "YYYY-MM-DD", "sport_id": "tennis", "sport_name": "Tennis",
    "duration_minutes": 60, "perceived_effort": 7 | null, "distance_meters": null,
    "energy_kcal": null, "avg_heart_rate_bpm": null,
    "source": "manual" | "apple_health", "notes": null
  }],
  "body_weights": [{ "date": "YYYY-MM-DD", "weight": 80.4, "unit": "lb" | "kg" }],
  "nutrition": {
    "days": [{ "date": "YYYY-MM-DD", "energy_kcal": 2400 | null, "protein_g": 140.5 | null,
      "carbs_g": null, "fat_g": null, "source": "manual" | "apple_health" | "foods" }],
    "targets": [{ "effective_from": "YYYY-MM-DD", "protein_g": 150, "energy_kcal": null,
      "carbs_g": null, "fat_g": null }],
    "food_entries": [{ "date": "YYYY-MM-DD", "kind": "food" | "quick_add",
      "food_name": "Oats" | null, "serving_label": "1 bowl" | null, "serving_g": 250 | null,
      "amount": 1 | null, "energy_kcal": 400 | null, "protein_g": 30 | null,
      "carbs_g": 50 | null, "fat_g": 8 | null, "logged_at": "RFC3339" }],
    "own_foods": [{ "name": "Oats", "brand": null, "serving_label": "1 bowl", "serving_g": 250,
      "energy_kcal": 400, "protein_g": 30, "carbs_g": 50, "fat_g": 8, "archived": false }]
  },
  "knee_kit_days": [{ "date": "YYYY-MM-DD", "completed_items": ["morning-check"],
    "morning_pain": 3 | null, "activity_pain": null, "sharp_pain": false, "limping": false }]
}
```

- Every collection is `[]`, never `null`. A section the athlete never created
  (`profile`, `settings`, `training_profile`) is `null`.
- `workouts` combines goal-based sessions (`source: "session"`) and legacy day
  logs (`source: "day_log"`), oldest date first. Discarded sessions are left
  out. `planned` is true when the session fulfilled a scheduled workout. A
  legacy day log has `status` `logged` (or `rest` for a rest day), `started_at`
  `null`, and `completed_at` set to when it was logged; its `exercises` carries
  per-exercise summaries recorded before sets were logged one by one (always
  `[]` for sessions).
- Set and template weights carry no unit of their own; they are in
  `settings.weight_unit`. Body weights carry the unit they were entered in.
- Sets are ordered by planned exercise order, then exercise name, then
  `set_index`.
- `schema_version` increases only when a field is removed or changes meaning.
  New fields and sections are additive and keep the version; readers must
  ignore unknown fields.
- `knee_kit_days` (added in contract 1.7.0, additive, `schema_version` still
  1) holds every Knee Kit day oldest first; `completed_items` is sorted and
  `[]` when nothing was ticked. Exports made by an older API omit the key.
- `nutrition.saved_meals` (added in contract 1.8.0, additive, `schema_version`
  still 1) holds every saved meal in the order saved:
  `{ "name": "Usual breakfast", "section": "breakfast", "items": [{ "food_name": "Oats", "serving_label": "1 cup (81 g)", "serving_g": 81, "amount": 1 }] }`.
  It carries no values; `[]` when none. Exports made by an older API omit the
  key.
- `nutrition.food_entries[].section` (added in contract 1.8.0, additive) is the
  entry's day heading or `null`.
- Programs, the schedule, weekly plans, criteria-based training blocks, and
  Health export receipts are not in version 1.

### `GET /api/v1/export/workout-sets` → `200 text/csv; charset=utf-8`

One RFC 4180 row per recorded set from the same snapshot, oldest workout first,
for spreadsheets and other training apps. Columns, in order:

```text
date,workout_name,workout_source,workout_status,planned,exercise_name,
exercise_category,exercise_modality,set_index,target_reps,target_weight,
actual_reps,actual_weight,weight_unit,duration_seconds,completed,extra,workout_notes
```

- A workout with no sets still has one row with empty set columns, so its date
  and notes survive. Rest-day logs are left out.
- Empty cells mean no value. `weight_unit` is filled only on rows that carry a
  weight. Booleans are `true`/`false`.
- Athlete-authored text that starts with `=`, `+`, `-`, `@`, tab, or carriage
  return is prefixed with `'` so spreadsheets do not evaluate it as a formula.
- Columns are only ever appended.

---

## Account

### `DELETE /api/v1/account` → 204

**Irreversible.** Explicitly batch-deletes the complete bounded Storage object
set—legacy `<authenticated_user_uuid>/avatar` plus
`<authenticated_user_uuid>/avatar-0` and
`<authenticated_user_uuid>/avatar-1`—every user-owned application row in one
database transaction, and the Supabase auth user.
Deletion and avatar mutation share a cross-replica PostgreSQL advisory lock.
The lock uses a dedicated direct or session-mode database connection; Supavisor
transaction mode cannot preserve the session-level lock across statements.
Within it, the server deletes application rows, the avatar, and finally the
auth identity. Auth deletion is deliberately last: if the provider accepts the
delete but its response is lost, no application-held personal data is stranded
after the user loses the ability to authenticate. Avatar upload rechecks the
auth identity inside the same lock before touching Storage. Production
requires configured Supabase admin credentials.

`204` means every required server-side step completed. Missing avatar objects
and already-deleted auth users count as success, so the operation can be
retried safely after a partial failure. A Storage, database, or Auth failure
returns:

```json
{
  "error": "account deletion temporarily unavailable",
  "code": "ACCOUNT_DELETION_INCOMPLETE"
}
```

Status is `503`. The client must remain on the deletion flow and offer a retry;
only after `204` may it sign out, cancel notifications, and clear all
user-scoped local data. A cryptographically valid, unexpired JWT may call only
this deletion endpoint after its auth row is gone, allowing final cleanup to
be retried; every other API route still rejects that identity with `401`.

---

## Auth contract for the client

JWT must have:
- `sub`: a v4 UUID — this becomes the `userID` everywhere.
- `kid` header: required when `SUPABASE_JWKS_URL` is set (asymmetric path).
  HMAC path (no JWKS) doesn't require it.

A cryptographically valid token whose `sub` no longer exists returns:

```json
{
  "error": "authentication required",
  "code": "AUTHENTICATION_REQUIRED"
}
```

Status is `401`; the client must clear its session. If the server cannot verify
identity existence because PostgreSQL is unavailable, it fails closed with:

```json
{
  "error": "authentication service temporarily unavailable",
  "code": "AUTHENTICATION_UNAVAILABLE"
}
```

Status is `503`; the client may retry and must not treat this as a terminal
sign-out.

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
