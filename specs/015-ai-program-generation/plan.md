# Implementation Plan: Generate a Program Draft

**Branch**: `spec/015-ai-program-generation` | **Date**: 2026-10-31 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/015-ai-program-generation/spec.md`

## Summary

Add a read-only program-generation resource: `GET /api/v1/program-generation` reports availability
and today's remaining drafts; `POST /api/v1/program-generation` reserves one of the athlete's five
daily drafts, sends the saved training profile and the exercise catalog to Claude through the
official Go SDK with a JSON schema whose `catalog_id` enum is the catalog itself, repairs and
validates the answer in the model package, and returns an unsaved draft in the custom-program
create shape. Saving stays with the existing `POST /api/v1/programs`. One new table,
`program_generations`, holds the daily-limit reservations and no draft content. The feature is off
when `ANTHROPIC_API_KEY` is empty.

## Workflow and Specialist Scope

- **Feature**: `gympulse-ai-program-generation`, new scope, sibling
  [app spec](../../../gym-pulse-app/specs/019-ai-program-generation/spec.md) (app plan follows
  after this repository). This repository ships first.
- **gym-pulse-api-engineering**: contract, handler → service → DAO placement, validation
  placement, reservation transaction, migration, tests (this plan, `research.md`, `data-model.md`,
  `contracts/`, `quickstart.md`).
- **gym-pulse-cross-repo-delivery**: additive rollout and the status read that lets older and
  newer apps coexist (`quickstart.md`).
- **claude-api** (Claude API skill): SDK choice, model default, structured outputs, effort,
  timeout, refusal handling, privacy and caching decisions (`research.md`).
- **supabase-postgres-best-practices**: advisory-lock reservation, index on the daily count, RLS
  and runtime-role policy on the new table.
- Single-agent execution; one sequential lane. A read-only review pass follows implementation.

## Technical Context

**Language/Version**: Go 1.26+

**Primary Dependencies**: chi, pgx/pgxpool, google/uuid, validator (existing);
`github.com/anthropics/anthropic-sdk-go` v1.84.0 (new, justified below)

**Storage**: PostgreSQL; new `program_generations`; reads `training_profiles` and `exercise_catalog`

**Testing**: Go unit, handler, and DAO tests; httptest provider double; PostgreSQL integration
tests for the reservation; `go test ./...`, `go test -race` on the reservation package,
`./scripts/smoke-toggle.sh`, `./scripts/validate-database-design.sh`,
`./scripts/validate-supabase-security.sh`

**Target Platform**: Linux HTTP service on Railway, Supabase-hosted PostgreSQL, outbound HTTPS to
the Anthropic API

**Project Type**: Go web service

**Performance Goals**: One provider call per draft, bounded at 45 seconds by context; status read
is one indexed count; reservation is one short transaction under a per-athlete advisory lock

**Constraints**: Additive; owner-scoped; no side effects on programs, schedules, or profile; no
draft persistence; provider receives only profile fields, catalog, and request text; key only in
config; disabled without a key

**Scale/Scope**: One migration, one model file, one DAO, one service plus one provider adapter,
one handler, two routes, config, contract and schema documentation, smoke steps

## Constitution Check

*GATE: Passed before Phase 0 and re-checked after Phase 1 design.*

- [x] Contract changes are identified in `contracts/program-generation.openapi.yaml` and land in
  `docs/CONTRACTS.md` ("Program generation") with the code; Swagger is regenerated.
- [x] Authentication, ownership, idempotency, and error behavior: both routes sit inside the
  authenticated, active-user group; every read and write carries `user_id`; the POST is read-only
  with respect to owned resources and non-deterministic, so it takes no `Idempotency-Key` and
  the contract says so; stable codes `GENERATION_LIMIT_REACHED` (429) and `GENERATION_UNAVAILABLE`
  (503) join `VALIDATION_ERROR` and the existing `RATE_LIMITED`.
- [x] Tests are identified before implementation in `quickstart.md`, including contract smoke,
  schema-document validation, the privacy assertion, and the concurrent-reservation race test.
- [x] Migration 031 creates one new table with RLS enabled, Data API privileges revoked, and the
  runtime-role grant and policy; the down migration drops it; existing data is untouched.
- [x] New dependency and concurrency justified in Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/015-ai-program-generation/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/program-generation.openapi.yaml
├── checklists/requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
cmd/server/main.go
go.mod, go.sum
.env.example, README.md
docs/CONTRACTS.md
docs/DATABASE_DESIGN.md
docs/{docs.go,swagger.json,swagger.yaml}
migrations/031_create_program_generations.{up,down}.sql
internal/
├── config/{config.go,config_test.go}
├── dao/{program_generation_dao.go,program_generation_dao_test.go,account_dao.go}
├── handler/{program_generation.go,program_generation_test.go,response.go}
├── middleware/rate_limit.go
├── model/{program_generation.go,program_generation_test.go}
├── router/{router.go,training_routes_test.go}
└── service/{program_generation_svc.go,program_generation_svc_test.go,
             program_generator_anthropic.go,program_generator_anthropic_test.go}
scripts/smoke.sh
```

**Structure Decision**: Mirror the health-export and food-logging features: one handler → service →
DAO path owning the new table, a provider adapter behind a small `DraftGenerator` interface at the
service boundary (the only seam needed for tests, as `FoodSource` is for food lookups), and the
repair logic as pure functions in `model` so the contract invariants are table-tested without a
database or provider.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| New dependency `anthropic-sdk-go` | Typed structured outputs, request timeout, retries, and stable error types for the one provider call | Hand-written `net/http` against the Messages API duplicates the SDK's request shapes and error parsing and drifts as the API evolves; the Claude API skill's output rule prefers the official SDK where one exists |
| Advisory lock in the reservation transaction | Two concurrent requests from one athlete must not both pass a count of four (spec edge case) | A plain count-then-insert races under read committed; a unique constraint cannot express "at most five per day" |
| Rate-limited route (`RateLimitPerUser`) | Each call costs money and up to 45 seconds; the daily ledger alone allows a burst of pending rows | Existing pattern for expensive endpoints; no new mechanism |
