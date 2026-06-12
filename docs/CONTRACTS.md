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
  "session_notes": "string|null"
}
```

### `GET|PUT|DELETE /api/v1/logs/{date}` — `date` is `YYYY-MM-DD`.

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

### `GET /api/v1/stats/distribution`
Response 200 — **wraps the array under a `types` key**:
```json
{ "types": [ { "type_id": "upper", "count": 3, "subtypes": [{ "subtype_id": "hypertrophy", "count": 3 }] } ] }
```
`types` may be `null` for a new user. The client must unwrap and coerce:
```ts
const safe = Array.isArray(resp?.types) ? resp.types : [];
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
| `PUT /settings {weekly_goal: 2}` | `invalid settings` | old `min=3` rule | rebuild API container (see `gym-pulse-dev-loop` skill) |
| `GET /stats/distribution` then `.reduce` | `reduce is not a function` | handler returns `null` for empty | coerce client-side or use `useStats`'s guard |
| `POST /templates` from onboarding before signup | 401 | not authenticated | buffer in MMKV, drain in `_layout.tsx` |
| edits to `internal/model/*.go` don't take effect | container still has old binary | docker layer cache reused stale binary | `docker compose up -d --build api` |
