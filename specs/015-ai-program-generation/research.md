# Research: Generate a Program Draft

## Decisions

### A read-only generation resource, not a mutation on programs

- **Decision**: `GET /api/v1/program-generation` reports whether generation is on and how many
  drafts remain today; `POST /api/v1/program-generation` builds one draft and returns it without
  saving. Saving stays with the existing `POST /api/v1/programs`.
- **Rationale**: The spec forbids side effects (FR-006) and the draft is non-deterministic, so an
  idempotency key would promise a replay the server cannot honor. A separate status read lets the
  app hide the entry when the feature is off (app FR-008) and show "tomorrow" before spending a
  provider call.
- **Rejected**: `POST /programs/generate` returning a saved program (violates FR-006; forces the
  athlete to delete a program they did not want). Embedding an `enabled` flag in the training
  profile response (couples an unrelated resource to provider configuration).

### Provider: Anthropic Claude through the official Go SDK

- **Decision**: `github.com/anthropics/anthropic-sdk-go` v1.84.0 (already in the local module
  cache; module `go 1.24`, compatible with the repository's Go 1.26). Model from
  `PROGRAM_GENERATION_MODEL`, default `claude-opus-5-5`. Thinking left unset (always on for this
  model); `OutputConfig.Effort` set to `low` because the task is bounded and latency matters more
  than depth. `MaxTokens` 6000.
- **Rationale**: One request, one JSON answer; no tools, no loop. The SDK gives typed structured
  outputs, request timeouts, and stable error types without hand-written HTTP.
- **Verified bindings (from `message.go` in the installed module)**: `anthropic.MessageNewParams`
  with `OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortLow, Format:
  anthropic.JSONOutputFormatParam{Schema: map[string]any{...}}}`; the `Type` of the format elides to
  `json_schema`. Client options: `option.WithAPIKey`, `option.WithBaseURL` (tests),
  `option.WithRequestTimeout`, `option.WithMaxRetries`. Refusals arrive as
  `resp.StopReason == anthropic.StopReasonRefusal`. Non-2xx errors unwrap with `errors.As` into
  `*anthropic.Error` and branch on `StatusCode`.
- **Cost**: roughly 3–4K input tokens (system rules plus the catalog) and 1–2K output tokens, about
  $0.04–0.06 per draft at Opus 5.5 list prices; the daily limit caps one athlete at about $0.30 a
  day. `PROGRAM_GENERATION_MODEL` lets operations trade quality for price (for example
  `claude-sonnet-5-5`) without a code change.
- **Refusal fallback**: the Claude API skill recommends enabling server-side fallbacks by default
  for Opus 5.5 code. A program draft from a fitness profile has no realistic refusal path, and the
  fallback requires the beta Messages client. Decision: handle `refusal` as "unavailable" and do
  not enable fallbacks in this release; revisit if logs show refusals. The owner can reverse this
  during review.
- **Rejected**: Raw `net/http` against the Messages API (the skill's output rule prefers the SDK
  where one exists); the Managed Agents surface (no loop, no sandbox needed); streaming (the output
  is small and the 45-second deadline is enforced by context, so a single non-streaming call
  suffices; switch to `Messages.NewStreaming` if p95 latency nears the deadline).

### Structured output constrained to the catalog

- **Decision**: The request carries a JSON schema for the draft: `name`, `rationale`, `workouts[]`
  with `name` and `exercises[]` of `{catalog_id, target_sets, target_reps, target_duration_seconds,
  rest_seconds, notes}`; `catalog_id` is a string enum of every catalog id in the request. The system
  prompt lists the catalog as `id | name | category | modality | mechanic` and the profile fields.
- **Rationale**: The schema makes the shape parseable and the enum makes an invented exercise
  impossible at the grammar level; server-side validation (next decision) still runs because the
  schema cannot express "at most available days" or "no duplicates".
- **Prompt caching**: the system block (rules plus catalog) is identical across athletes and is
  marked with `cache_control` ephemeral; the catalog is about 1.5K tokens, which may fall below the
  model's minimum cacheable prefix, so a cache miss is acceptable and never affects correctness.

### Validate and repair in the model package, never trust the draft

- **Decision**: `model.RepairGeneratedDraft(profile, catalog, raw)` returns a `ProgramDraft` or a
  `GenerationInvalidError`. It: resolves every `catalog_id` and copies name, category, and modality
  from the catalog; drops unknown ids and duplicate ids within a workout; caps exercises at 8 and
  workouts at `min(len(available_days), 6)`; requires 3–8 exercises per surviving workout and at
  least one workout; clamps `target_sets` 1–10, strength `target_reps` 1–50 with duration cleared,
  cardio `target_duration_seconds` 30–3600 with reps cleared, `rest_seconds` 0–600 (default 90);
  never sets `target_weight`; truncates `name` to 80 and `rationale` to 500 runes; sets
  `primary_goal` from the profile; assigns `kind: exercises`, `sequence_position` 1..n, and
  `preferred_weekday` from the sorted available days; sets `roadmap` to
  `{"origin": "generated", "model": <model>, "request": <request|null>}`.
- **Rationale**: FR-003 through FR-005 are testable as a pure function with table-driven cases;
  `POST /programs` then accepts the draft unchanged because it already passes
  `validateProgramShape`.

### Daily limit as a reservation ledger

- **Decision**: Table `program_generations(id, user_id, local_date, status, model, created_at,
  completed_at)` with `status IN ('pending', 'succeeded', 'failed')`. Before calling the provider
  the DAO runs one short transaction: `pg_advisory_xact_lock(hashtext('program-generation:' ||
  user_id))`, counts rows for the athlete's local date with `status = 'succeeded'` or `status =
  'pending' AND created_at > now() - interval '2 minutes'`, and inserts a `pending` row when the
  count is below the limit, otherwise returns `GenerationLimitError{ResetsAt}`. After the provider
  answers, the row is set to `succeeded` or `failed`. Only `succeeded` counts after two minutes, so
  crashes and failures never charge the athlete (FR-009).
- **Rationale**: The lock serializes concurrent requests from one athlete so two in-flight calls
  cannot both pass a count of four (edge case "never undercounts"). The transaction holds no
  provider call. The local date comes from the saved profile timezone, matching sport activities.
- **Rejected**: In-memory `requestBudget` as food lookups use (lost on restart, wrong across
  replicas, and per-minute rather than per-day). A counter column on `training_profiles` (needs
  a date rollover and mixes concerns).
- **Per-minute guard**: the route also uses `middleware.RateLimitPerUser(ProgramGenerationLimit)`
  with a limit of 5 per minute, matching the existing pattern for expensive endpoints.

### Configuration and the disabled state

- **Decision**: `ANTHROPIC_API_KEY` (empty disables the feature), `PROGRAM_GENERATION_MODEL`
  (default `claude-opus-5-5`), `PROGRAM_GENERATION_DAILY_LIMIT` (default 5),
  `PROGRAM_GENERATION_TIMEOUT` (default `45s`), and `ANTHROPIC_BASE_URL` (tests and staging
  override; empty uses the SDK default). Staging and production log a warning when the key is
  empty, as they do for the USDA key; startup never fails on it. The key is read in `config.Load`,
  passed to the service constructor, and appears nowhere else.
- **Status answer**: `GET` returns `enabled: false` when the key is empty; `POST` returns `503
  GENERATION_UNAVAILABLE` with `details.reason = "disabled"`.

### Stable answers

| Situation | Status | Code | Details |
|---|---|---|---|
| No training profile | 422 | `VALIDATION_ERROR` | `field: "training_profile"` |
| `request` blank or over 200 runes | 422 | `VALIDATION_ERROR` | `field: "request"` |
| Per-minute guard | 429 | `RATE_LIMITED` | existing shape |
| Daily limit reached | 429 | `GENERATION_LIMIT_REACHED` | `limit`, `resets_at` (RFC3339, local midnight) |
| Disabled, provider error, timeout, refusal, invalid draft | 503 | `GENERATION_UNAVAILABLE` | `reason: "disabled" \| "unavailable"` |

Timeouts and provider errors share `unavailable` so the app shows one notice; the reason split
exists only so the app can hide the entry when the feature is off.

### Privacy boundary

- **Decision**: `buildGenerationRequest(profile, catalog, request, model)` is a pure function that
  produces the SDK params; a test marshals them and asserts the JSON contains only the profile
  fields named in FR-007, the catalog, the request text, and the schema. Logs record outcome,
  duration, model, and token counts, never the prompt, the draft, or the request text.

## Resolved Technical Questions

- Migration number: `031`; main is at `030` (food logging).
- The draft uses `model.ProgramWorkout` and `model.ProgramExercise` with zero `ID`s and no
  `source_starter_exercise_id`, so `POST /programs` accepts it as-is.
- `available_days` has 7 entries at most; the draft uses at most 6 workouts, leaving at least one
  rest day, and the prompt says so.
- Equipment fit is instructed, not verified (spec assumption); the prompt maps each equipment value
  to plain-language guidance (for example `bodyweight`: no external load).
- Account deletion adds `DELETE FROM program_generations WHERE user_id = $1` to `account_dao.go`
  beside the cascade.
- The validator already enforces `name` ≤ 200 on create; the draft's 80-rune cap is stricter and
  lives in the repair step.
