<!--
Sync Impact Report
- Version: unratified template -> 1.0.0
- Added: Contract First; Owned Authenticated Data; Idempotent Evolution; Executable Evidence;
  Simple Go Boundaries
- Updated: plan and tasks templates; AGENTS.md; CLAUDE.md
- Deferred items: none
-->
# GymPulse API Constitution

## Core Principles

### I. Contract First
Every observable API change MUST begin in a version-controlled specification and MUST update
`docs/CONTRACTS.md` with request fields, validation, responses, statuses, and errors in the same
change. That document is client-facing truth; generated Swagger is secondary. JSON remains
`snake_case`, and promised collections return `[]`, never `null`.

### II. Owned Authenticated Data
Every `/api/v1/*` resource MUST be scoped to the authenticated user. Missing and foreign owned IDs
MUST share the documented not-found behavior. Handler, service, and DAO boundaries MUST carry
authenticated identity and `context.Context` without trusting client-supplied ownership. Secrets,
JWTs, credentials, and production user data MUST NOT enter logs, fixtures, specs, or commits.

### III. Idempotent Evolution
Mutations MUST preserve documented idempotency keys and optimistic revisions. Identical operations
MUST replay safely; payload mismatch and stale revision MUST produce documented conflicts. Schema
changes MUST use forward migrations and document existing-data, rollback, and compatibility behavior.
Additive contract evolution is preferred; breaking changes require a coordinated app migration plan.

### IV. Executable Evidence
Changed behavior MUST have tests identified before implementation and failing for the intended gap
where practical. Service and middleware logic MUST maintain at least 90% coverage. Contract changes
MUST update contract tests and run `scripts/smoke-toggle.sh`; all work MUST pass `go test ./...`.
Tests MUST NOT be skipped, weakened, or deleted merely to pass a gate.

### V. Simple Go Boundaries
Code MUST follow handler → service → DAO separation and Google Go style. Network, database, and
concurrent calls take `context.Context` first. Errors are lowercase, preserve causes with `%w`, and
map to stable public codes without leaking internals. New dependencies, abstraction layers, or
background concurrency MUST be justified in the plan.

## Technical Constraints

- Go 1.23+, chi, pgx/pgxpool, PostgreSQL, and SQL migrations are the supported stack.
- Response structs and fixtures use keyed fields and documented JSON tags.
- Validator, JSON, response, status, or error changes update `docs/CONTRACTS.md` in the same commit.
- Cross-repository features share a feature name and link their dependent app specification.

## Spec-Driven Delivery

New work follows `$speckit-specify` → optional `$speckit-clarify` → `$speckit-plan` →
`$speckit-tasks` → `$speckit-analyze` → `$speckit-implement` → `$speckit-converge`. Plans MUST name
migrations and contract effects and pass the Constitution Check. Before review, run `go test ./...`
and, for contract work, `./scripts/smoke-toggle.sh`. Artifacts live under
`specs/<number>-<feature>/`.

## Governance

This constitution supersedes conflicting workflow guidance. Amendments require a pull request,
impact analysis, and migration plan for breaking governance. Versions follow semantic versioning:
MAJOR for removed or redefined governance, MINOR for new or expanded obligations, and PATCH for
clarification. Every plan and review MUST verify compliance; exceptions require explicit Complexity
Tracking with the simpler rejected alternative.

**Version**: 1.0.0 | **Ratified**: 2026-07-18 | **Last Amended**: 2026-07-18
