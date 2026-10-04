<!--
Sync Impact Report
- Version: 1.0.1 -> 1.1.0 (MINOR: expanded routing, resumption, and evidence guidance)
- Modified principles: clarified specification reuse and the scope of implementation obligations
- Expanded section: Spec-Driven Delivery; no principles removed
- Updated: AGENTS.md, CLAUDE.md, docs/SPEC_WORKFLOW.md, docs/CODEX_PLAYBOOK.md, docs/SPEC_DRIVEN_DEVELOPMENT.md,
  engineering skills, .specify/templates/plan-template.md, .specify/templates/tasks-template.md
- Reviewed unchanged: .specify/templates/spec-template.md; core speckit-* skills
- Impact: existing feature artifacts remain valid; amend only affected scope and evidence
- Migration: no product, schema, or contract migration; preserve existing feature/task identifiers
- Follow-up TODOs: none
-->
# GymPulse API Constitution

## Core Principles

### I. Contract First
Every observable API change MUST be governed by a version-controlled specification and MUST update
`docs/CONTRACTS.md` with request fields, validation, responses, statuses, and errors in the same
change. Reuse an existing spec when restoring its documented behavior; amend it before changing
intended behavior. That document is client-facing truth; generated Swagger is secondary. JSON remains
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
MUST update contract tests and run `scripts/smoke-toggle.sh`; implementation changes MUST pass
`go test ./...`.
Tests MUST NOT be skipped, weakened, or deleted merely to pass a gate.

### V. Simple Go Boundaries
Code MUST follow handler → service → DAO separation and Google Go style. Network, database, and
concurrent calls take `context.Context` first. Errors are lowercase, preserve causes with `%w`, and
map to stable public codes without leaking internals. New dependencies, abstraction layers, or
background concurrency MUST be justified in the plan.

## Technical Constraints

- Go 1.26.8+, chi, pgx/pgxpool, PostgreSQL, and SQL migrations are the supported stack.
- Response structs and fixtures use keyed fields and documented JSON tags.
- Validator, JSON, response, status, or error changes update `docs/CONTRACTS.md` in the same commit.
- Cross-repository features declare the same stable feature ID and link their dependent app
  specification. Local numeric Spec Kit directory prefixes may differ.

## Spec-Driven Delivery

Use `docs/SPEC_WORKFLOW.md` to classify work and resume the intended feature. New non-trivial scope
follows `$speckit-specify` → optional `$speckit-clarify` → `$speckit-plan` → `$speckit-tasks` →
`$speckit-analyze` → `$speckit-implement` → `$speckit-converge`. Add focused requirements checklists
when risk or ambiguity warrants them. Existing features MUST reuse valid artifacts and preserve
completed work; changed intent MUST update affected requirements, design, tasks, and evidence before
implementation. Focused corrections may reuse a governing spec without restarting the full cycle.

Plans MUST pass the Constitution Check. Relevant engineering and specialist skills MUST inform the
stages where their expertise affects decisions. Changed requirements MUST map to implementation
tasks and verification evidence; unchecked or unrun verification MUST remain explicitly unverified.
A completed task list or clean convergence report alone does not establish release readiness.

API implementation changes MUST pass `go test ./...`; contract changes also require
`./scripts/smoke-toggle.sh`. Plans MUST identify migrations and contract effects.
Documentation/workflow-only changes use direct edits and validation of skills, links, consistency,
and applicable tooling; they do not require a new product spec or application test suites solely for
prose changes. Executable behavior, dependency, or configuration changes retain their code gates.
Artifacts live under `specs/<number>-<feature>/`; review-only requests remain read-only unless
remediation is authorized. Follow the user's delivery boundary and reuse existing authorization.

## Governance

This constitution supersedes conflicting workflow guidance. Amendments require a pull request,
impact analysis, and migration plan for breaking governance. Versions follow semantic versioning:
MAJOR for removed or redefined governance, MINOR for new or expanded obligations, and PATCH for
clarification. Every plan and review MUST verify compliance; exceptions require explicit Complexity
Tracking with the simpler rejected alternative.

**Version**: 1.1.0 | **Ratified**: 2026-07-18 | **Last Amended**: 2026-10-04
