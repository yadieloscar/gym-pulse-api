---
description: "Implementation tasks for generating a program draft from the training profile"
---

# Tasks: Generate a Program Draft

**Input**: Design documents from `/specs/015-ai-program-generation/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/program-generation.openapi.yaml, quickstart.md

**Tests**: Required by the API constitution; identified before each implementation task. The provider is always an `httptest` fixture or a fake `DraftGenerator` in tests; no test calls the Anthropic API.

## Format: `[ID] [P?] [Story] Description`

## Requirement and Verification Evidence

| Requirement / acceptance | Tasks | Verification | Result / limitation |
| --- | --- | --- | --- |
| FR-001, FR-002, US1/AC1, AC3 | T014, T017, T019, T020 | handler test: 200 with `draft`, `rationale`, `request`, `remaining_today`, `resets_at`; draft marshals into `CreateProgramRequest` and passes `validateProgramShape` | Planned |
| FR-003, FR-004, FR-005, US1/AC2, AC5 | T008, T009 | `RepairGeneratedDraft` table tests: unknown id dropped, duplicate collapsed, workouts capped to `min(days, 6)`, exercises capped at 8, fewer than 3 → invalid, reps/duration by modality, clamps, weight cleared, goal forced, name 80 and rationale 500 truncated, weekday from sorted days | Planned |
| FR-006 | T016, T018 | service test: fake DAOs record no program, schedule, or profile writes; DAO source test: only `program_generations` statements | Planned |
| FR-007, SC-004 | T012, T013 | `buildGenerationRequest` test marshals params and asserts the key set is exactly profile fields, catalog, request, schema; no `user_id`, `timezone`, email | Planned |
| FR-008, SC-005, US3/AC1, AC3 | T026, T027, T028 | DAO integration test: fifth reservation succeeds, sixth returns `GenerationLimitError` with local-midnight `resets_at`; per-athlete isolation; concurrent reservations under `-race`; service test: no generator call after the limit | Planned |
| FR-009, SC-003, US3/AC2 | T012, T013, T026, T029 | adapter tests: timeout, 5xx, 429, refusal, bad JSON → `GenerationUnavailableError`; service marks the row `failed`; DAO test: failed and stale pending rows do not count; handler 503 `unavailable` | Planned |
| FR-010, US1/AC4 | T016, T017 | service test: missing profile → `ValidationError{Field: "training_profile"}`, no reservation, no generator call | Planned |
| FR-011 | T006, T007, T019 | config test: empty key → disabled; key absent from `GenerationStatus` JSON and from captured logs | Planned |
| FR-012 | T021 | account cleanup enumeration test includes `program_generations` | Planned |
| FR-013, SC-006 | T002, T031, T032 | `docs/CONTRACTS.md` "Program generation"; `./scripts/smoke-toggle.sh` with steps 16a–16b; `go test ./...` | Planned |
| US2/AC1–AC3 | T023, T024, T025 | service test: request trimmed, 1–200 runes, blank → 422 `request`, over 200 → 422; echoed in response and in `roadmap.request`; prompt carries the request as user text with an out-of-scope instruction | Planned |
| SC-001, SC-002 | T022 | fixture test over six goals × {1, 3, 6} days with recorded provider JSON through the fake generator; every draft passes repair and `validateProgramShape`; every `catalog_id` resolves | Planned |
| Schema document | T003, T005, T031 | `./scripts/validate-database-design.sh` | Planned |
| Supabase security and runtime role | T005, T033 | `supabase_security_migration_test.go`; `./scripts/validate-supabase-security.sh` on a migrated database | Planned |

## Phase 1: Setup

- [ ] T001 Record `git status --short`; confirm against `origin/main` that `031` is the next migration number and update plan.md, research.md, and data-model.md if it moved
- [ ] T002 [P] Draft the "Program generation" section in `docs/CONTRACTS.md` after "Programs" from `contracts/program-generation.openapi.yaml` (status read, generate, draft shape, the four stable answers, no `Idempotency-Key`, per-minute and daily limits)
- [ ] T003 [P] Add the `program_generations` table, check, index row, and migration ledger row to `docs/DATABASE_DESIGN.md`; update the table, column, and index counts
- [ ] T004 [P] Add `github.com/anthropics/anthropic-sdk-go@v1.84.0` to `go.mod` and `go.sum`; run `go mod tidy` and confirm no other dependency moved

## Phase 2: Foundational

- [ ] T005 Add `migrations/031_create_program_generations.{up,down}.sql` per data-model.md: table, status check, `idx_program_generations_user_date`, RLS enabled, Data API role revoke, runtime-role grant and `gym_pulse_api_access` policy (the 029/030 block); down drops the table; extend `internal/dao/supabase_security_migration_test.go` to the new table count; run `./scripts/validate-database-design.sh`
- [ ] T006 [P] Add `internal/config/config_test.go` cases: `ANTHROPIC_API_KEY` empty → `ProgramGenerationEnabled()` false; defaults `PROGRAM_GENERATION_MODEL=claude-opus-5-5`, `PROGRAM_GENERATION_DAILY_LIMIT=5`, `PROGRAM_GENERATION_TIMEOUT=45s`; invalid limit or duration → load error; staging and production warn (not fail) on an empty key
- [ ] T007 Extend `internal/config/config.go`: `AnthropicAPIKey`, `AnthropicBaseURL`, `ProgramGenerationModel`, `ProgramGenerationDailyLimit`, `ProgramGenerationTimeout`; document all five in `README.md` and `.env.example` (key never printed)
- [ ] T008 [P] Add `internal/model/program_generation_test.go`: `RepairGeneratedDraft` table cases listed in the evidence table; `GenerationStatus` and `GenerateProgramResponse` JSON shape (nullable fields always present); `GenerationLimitError` and `GenerationUnavailableError` messages
- [ ] T009 Add `internal/model/program_generation.go`: `ProgramDraft` (reuses `ProgramWorkout` and `ProgramExercise`), `RawGeneratedDraft` (the provider JSON shape), `GenerationStatus`, `GenerateProgramRequest`, `GenerateProgramResponse`, `GenerationLimitError{Limit, ResetsAt}`, `GenerationUnavailableError{Reason}`, `RepairGeneratedDraft(profile, catalog, raw, model, request)`, `GenerationSchema(catalogIDs)` building the JSON schema map, and `NextLocalMidnight(now, timezone)`
- [ ] T010 [P] Add `middleware.ProgramGenerationLimit = 5` with its comment in `internal/middleware/rate_limit.go`
- [ ] T011 [P] Add `internal/service/program_generator.go`: `DraftGenerator` interface (`Generate(ctx, GenerationInput) (*model.RawGeneratedDraft, GenerationUsage, error)`), `GenerationInput` (profile fields, catalog, request, model), `ErrGeneratorUnavailable`, `ErrGeneratorRefused`

## Phase 3: User Story 1 - Get a draft program from my profile (P1) 🎯 MVP

- [ ] T012 [P] [US1] Add `internal/service/program_generator_anthropic_test.go` with `httptest` fixtures: success parses the JSON text block into `RawGeneratedDraft` and reports usage; request body asserts `model`, `max_tokens` 6000, `output_config.effort` low, `output_config.format.schema` with the `catalog_id` enum, `cache_control` on the system block, and the privacy key set (FR-007); `stop_reason: refusal` → `ErrGeneratorRefused`; `429`, `500`, `529` → `ErrGeneratorUnavailable`; a server slower than the context deadline → `ErrGeneratorUnavailable` within the deadline; malformed JSON → `ErrGeneratorUnavailable`; the key travels only in the `x-api-key` header and never in logs
- [ ] T013 [US1] Implement `internal/service/program_generator_anthropic.go`: `NewAnthropicDraftGenerator(apiKey, baseURL, model, timeout)` using `option.WithAPIKey`, `option.WithBaseURL` when set, `option.WithRequestTimeout(timeout)`, `option.WithMaxRetries(1)`; `buildGenerationRequest` (system block: rules, equipment guidance, session-length guidance, "at most N workouts, leave rest days", catalog lines `id | name | category | modality | mechanic`; user block: profile fields and the request text; `OutputConfig{Effort: Low, Format: JSONOutputFormatParam{Schema: ...}}`); `errors.As` into `*anthropic.Error` by status; `context.WithTimeout` per call
- [ ] T014 [P] [US1] Add `internal/service/program_generation_svc_test.go`: `Status` with and without a profile (`remaining_today` from the DAO count, `resets_at` from the profile timezone, disabled when the generator is nil); `Generate` happy path order (profile → reserve → generate → repair → complete succeeded) with a call recorder; missing profile → `ValidationError{Field: "training_profile"}` before any reservation; repair failure → `GenerationUnavailableError{unavailable}` and the row completed `failed`; generator unavailable or refused → same; disabled → `GenerationUnavailableError{disabled}` with no reservation; no program, schedule, or profile writes (FR-006)
- [ ] T015 [P] [US1] Add `internal/dao/program_generation_dao_test.go`: source-order markers for `Reserve` (advisory lock → count → insert or limit error → commit) and `Complete`; count predicate pins `status = 'succeeded' OR (status = 'pending' AND created_at > now() - interval '2 minutes')`; every statement carries `user_id`; `CountToday` used by status
- [ ] T016 [US1] Implement `internal/dao/program_generation_dao.go`: `ProgramGenerationDAO` with `Reserve(ctx, userID, localDate, limit, model) (reservation, error)`, `Complete(ctx, id, status)`, `CountToday(ctx, userID, localDate)`; transaction per data-model.md; `GenerationLimitError` carries `ResetsAt` computed by the caller
- [ ] T017 [US1] Implement `internal/service/program_generation_svc.go`: `ProgramGenerationService` (`Status`, `Generate`) over `TrainingProfileDAO`, `ExerciseCatalogDAO`, `ProgramGenerationDAO`, the optional `DraftGenerator`, limit, model, and `now`; local date and `resets_at` from the profile timezone; structured log of outcome, duration, model, and token counts only
- [ ] T018 [P] [US1] Add `internal/handler/program_generation_test.go`: `Status` 200 shape; `Generate` 200 shape; empty body and `{}` both accepted; 422 field surfacing for `training_profile`; 429 `GENERATION_LIMIT_REACHED` with `details.limit` and `details.resets_at`; 503 `GENERATION_UNAVAILABLE` with `details.reason` disabled and unavailable; no `Idempotency-Key` required
- [ ] T019 [US1] Implement `internal/handler/program_generation.go` (`Status`, `Generate`) with swag godoc; map `GenerationLimitError` → 429 `GENERATION_LIMIT_REACHED` and `GenerationUnavailableError` → 503 `GENERATION_UNAVAILABLE` in `internal/handler/response.go`
- [ ] T020 [US1] Wire DAO, generator (nil when the key is empty), service, and handler in `cmd/server/main.go`; routes `GET` and `POST /api/v1/program-generation` inside the `RequireActiveUser` group with `RateLimitPerUser(middleware.ProgramGenerationLimit)` on the POST in `internal/router/router.go`; extend `internal/router/training_routes_test.go` (routes present, POST limited, 6th call in a minute → 429 `RATE_LIMITED`)
- [ ] T021 [US1] Add `DELETE FROM program_generations WHERE user_id = $1` to `internal/dao/account_dao.go` and its enumeration test
- [ ] T022 [US1] Add `internal/service/program_generation_fixture_test.go` with recorded provider JSON under `internal/service/testdata/program_generation/` for six goals × {1, 3, 6} days; assert every fixture repairs, resolves every `catalog_id` against the seeded catalog names, and passes `validateProgramShape` (SC-001, SC-002)

**Checkpoint**: a profile yields a valid draft that `POST /programs` accepts unchanged; disabled and unavailable answers are stable.

## Phase 4: User Story 2 - Ask for a different draft (P2)

- [ ] T023 [P] [US2] Extend `internal/service/program_generation_svc_test.go`: `request` trimmed; blank after trim → 422 `request`; 201 runes → 422; 200 runes accepted; echoed in `GenerateProgramResponse.Request` and `roadmap.request`; null when omitted
- [ ] T024 [P] [US2] Extend `internal/service/program_generator_anthropic_test.go`: the request text appears once, in the user block, after an instruction that out-of-scope asks are declined in one line of the rationale and never change the catalog-only rule
- [ ] T025 [US2] Implement request validation in `internal/service/program_generation_svc.go` and the request block in `buildGenerationRequest` in `internal/service/program_generator_anthropic.go`

**Checkpoint**: "no deadlifts" produces a draft without a deadlift in the fixture test and the request is echoed.

## Phase 5: User Story 3 - Fair use and safe failure (P3)

- [ ] T026 [P] [US3] Add PostgreSQL integration cases to `internal/dao/program_generation_dao_test.go` (build tag as the existing `*_integration_test.go` files): five reservations succeed and the sixth returns the limit error; a `failed` row and a `pending` row older than two minutes do not count; a fresh `pending` row counts; two athletes are independent; ten concurrent `Reserve` calls under `go test -race` yield exactly five reservations
- [ ] T027 [US3] Implement any DAO changes the integration cases surface in `internal/dao/program_generation_dao.go`
- [ ] T028 [P] [US3] Extend `internal/service/program_generation_svc_test.go`: limit error from the DAO returns before the generator is called and carries `resets_at` at the next local midnight; `remaining_today` decrements per success and not per failure
- [ ] T029 [US3] Extend `internal/handler/program_generation_test.go` and the service for the timeout path: a generator that blocks until the context deadline returns 503 within the configured timeout plus a small margin

**Checkpoint**: the sixth draft in a day is refused before any provider call; failures never count.

## Phase 6: Polish and Verification

- [ ] T030 Run `gofmt -w` on changed files, `go vet ./...`, `golangci-lint run`
- [ ] T031 Run `./scripts/validate-database-design.sh`, `go test ./...`, `go test -race ./internal/dao/... ./internal/service/...`; regenerate Swagger with `swag init -g cmd/server/main.go --output docs` and confirm no unintended drift
- [ ] T032 Add smoke steps 16a (`GET /program-generation` → `enabled: false`, `limit: 5`, `remaining_today: 5`) and 16b (`POST` → 503 `GENERATION_UNAVAILABLE` with `reason: disabled`) to `scripts/smoke.sh`; run `./scripts/smoke-toggle.sh` or the isolated acceptance stack
- [ ] T033 Run `./scripts/validate-supabase-security.sh` against the migrated database (inside the db container per `api-smoke-against-worktree` notes); record results in the evidence table
- [ ] T034 With a real `ANTHROPIC_API_KEY` locally, run the manual acceptance in quickstart.md once (three-workout draft, `POST /programs` 201, "no deadlifts", sixth call 429); record latency and token usage in the evidence table; never commit the key or the responses
- [ ] T035 Fill the Result column of the evidence table; update `research.md` if the observed latency argues for streaming or a different effort

## Dependencies & Execution Order

T001–T004 parallel. T005 → T009 (model) after T008 tests; T006/T007 config, T010, T011 parallel with the model. US1: T012, T014, T015, T018 (tests) parallel, then T013 → T016 → T017 → T019 → T020 → T021 → T022. US2 after T017 and T013. US3 after T016. Phase 6 last. Deploy before the app release.

## Implementation Strategy

MVP is Phase 1 through Phase 3: a draft from the profile with disabled and unavailable answers, which the app can integrate against. US2 adds the request line; US3 adds the ledger's integration proof and the timeout path. Each checkpoint is independently demonstrable with the fake generator, and T034 is the only step that spends provider credit.
