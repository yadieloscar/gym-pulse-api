---
name: gym-pulse-api-engineering
description: Design, implement, debug, refactor, and review production GymPulse Go API changes. Use for handlers, middleware, validation, services, DAOs, PostgreSQL queries or migrations, authentication, authorization, contracts, errors, idempotency, revisions, concurrency, performance, dependencies, and backend tests; also use when a client change may alter API behavior.
---

# GymPulse API Engineering

Apply expert engineering judgment in addition to mechanical lint and test gates. Treat a green
pipeline as necessary evidence, not proof that the design is correct.

## Establish Context

1. Read `AGENTS.md` and `.specify/memory/constitution.md`.
2. Read the relevant parts of `docs/CONTRACTS.md`, implementation, tests, schema, and migrations.
3. Inspect `.golangci.yml` and `.github/workflows/ci.yml` before changing quality controls.
4. Inspect the client contract usage for cross-repository or compatibility-sensitive work.
5. Preserve unrelated user changes and record the initial Git status.

For a new feature, follow the repository Spec Kit workflow from `$speckit-specify` through
`$speckit-converge`. For a focused fix, keep the process proportional while retaining the design
and verification gates below.

## Classify the Change

Identify every affected lane before editing:

- HTTP transport or public contract
- authentication, authorization, or resource ownership
- business rules or state transitions
- persistence, transaction, query, index, or migration
- idempotency, optimistic revision, retry, or concurrency behavior
- dependency, security, performance, or operational behavior
- app compatibility or coordinated rollout

Do not implement until the affected invariants and observable behavior are clear. When important
facts are missing, inspect the code and tests; ask the user only when a material product decision
cannot be derived safely.

## Produce a Design Assessment

For non-trivial work, state concisely:

- contract behavior, statuses, validation, and stable error codes;
- authenticated identity, authorization, and ownership flow;
- invariant location and handler -> service -> DAO placement;
- transaction boundary, idempotency, revision, and race behavior;
- schema, query, index, migration, and existing-data effects;
- backward compatibility, deployment ordering, and client effects;
- test evidence required to demonstrate the behavior.

Prefer the smallest design that preserves these properties. Reject speculative abstractions,
unnecessary concurrency, and dependencies without a demonstrated benefit.

## Use Specialist Agents Selectively

Keep the primary agent responsible for design and integration. Delegate only when a bounded lane can
run independently and the expected time saved or review quality exceeds coordination overhead.

Useful API subagents include read-only repository mapping, contract impact analysis, PostgreSQL or
migration review, authentication and ownership review, concurrency analysis, and independent test or
security review. Parallel implementation is appropriate only for explicitly disjoint files with a
stable contract. Stay single-agent for focused fixes, sequential design work, or overlapping edits.

Give each specialist the relevant raw files and one concrete output contract without leaking the
intended conclusion. Inspect and reconcile every result; never delegate final integration or
completion claims.

## Apply Idiomatic Go Judgment

- Keep packages cohesive, dependency direction clear, and public APIs narrow.
- Prefer concrete types. Define small interfaces at the consuming boundary when substitution is
  required by a real collaborator or test seam.
- Avoid generic `util`, `common`, `helper`, or `interfaces` packages.
- Make zero values useful when practical and make invalid states hard to represent.
- Keep handlers responsible for transport, services for business policy, and DAOs for persistence.
- Pass `context.Context` first to database, network, and concurrent operations; propagate the
  caller's context rather than replacing it.
- Prefer synchronous code. When concurrency is justified, define goroutine ownership,
  cancellation, error propagation, backpressure, and shutdown behavior.
- Avoid global mutable state and hidden side effects.
- Check meaningful errors. Wrap with `%w` only when adding useful context, preserve error identity,
  and map internal causes to stable public errors at the boundary.
- Close resources, check `rows.Err()`, and keep transaction commit/rollback behavior explicit.
- Use `gofmt` and `goimports`; do not hand-format around the standard tools.
- Do not add `//nolint` without identifying the exact false positive and keeping the suppression as
  narrow as possible.

Use official Go guidance as the baseline for language and package idioms:

- `https://go.dev/doc/effective_go`
- `https://go.dev/wiki/CodeReviewComments`
- `https://go.dev/doc/modules/layout`
- `https://go.dev/doc/security/fuzz/`

Prefer current standard-library documentation and repository patterns when older guidance does not
cover newer language or library features.

## Engineer the HTTP Contract

Evaluate each changed endpoint for:

- correct method, route, success status, and failure status;
- path, query, header, and body semantics;
- validation, normalization, size limits, and unknown-field behavior;
- stable machine-readable errors without internal or user-data leakage;
- authentication, authorization, and indistinguishable foreign/missing owned resources;
- idempotency, revision conflicts, retries, and partial failure;
- pagination, deterministic ordering, and `[]` rather than `null` collections;
- timeout and cancellation behavior;
- additive compatibility or an explicit coordinated migration plan.

Update `docs/CONTRACTS.md`, generated API documentation, implementation, and contract tests together
when observable behavior changes.

## Protect Data Integrity

- Put invariants in database constraints when the database can enforce them reliably; keep useful
  request validation for client feedback.
- Define transaction boundaries around complete invariants, not individual DAO calls.
- Analyze concurrent requests for lost updates, duplicate creation, stale revisions, and lock order.
- Use foreign keys, unique constraints, checks, and deletion behavior deliberately.
- Derive indexes from real filters, joins, ordering, uniqueness, and scale assumptions. Inspect query
  plans for performance-sensitive changes.
- Avoid N+1 queries and unbounded reads. Make pagination and ordering deterministic.
- Write forward migrations with existing-data, rollout, rollback or roll-forward, and mixed-version
  compatibility behavior documented.
- Never trust client-supplied ownership or allow authentication identity to disappear between layers.

Apply `$supabase-postgres-best-practices` when it is available for schema, SQL, transaction, index,
query-performance, or database-configuration work. Reconcile generic advice with the GymPulse
constitution and the actual pgx/PostgreSQL architecture.

## Build Evidence Before and After Implementation

Identify a failing test or missing executable assertion before changing behavior when practical.
Choose evidence by risk:

- pure business behavior: focused table-driven unit tests;
- handler or contract behavior: HTTP tests covering statuses, bodies, and error codes;
- authentication or ownership: middleware/service tests including foreign-resource cases;
- persistence or migration: PostgreSQL-backed integration tests and constraint/race cases;
- concurrency: deterministic synchronization tests plus the race detector;
- parsing or untrusted structured input: seed cases and targeted fuzz tests;
- cross-layer behavior: smoke or acceptance tests against the real service boundary.

Test observable behavior rather than duplicating implementation details. Coverage is a signal, not a
substitute for meaningful assertions.

## Run Verification

Always run the narrowest useful test during iteration, then complete the repository gates:

```bash
gofmt -w <changed-go-files>
go test ./...
```

Run the configured lint suite when `golangci-lint` is available:

```bash
golangci-lint run
```

Run additional gates when triggered:

- handler, validator, middleware, status, error, JSON, or contract change:
  `./scripts/smoke-toggle.sh`
- concurrency, synchronization, shared state, or suspected race: `go test -race ./...`
- dependency or security-sensitive change, when available: `govulncheck ./...`
- parser or validator suited to fuzzing: run the targeted fuzz test with a bounded time budget;
- generated Swagger surface: regenerate it and verify `docs/` has no unintended drift;
- persistence or migration: run applicable PostgreSQL integration and migration tests.

Do not claim a gate passed if it was unavailable or skipped. Report the reason and the resulting
risk. Do not weaken, delete, or bypass tests and lint rules merely to obtain green output.

## Review Independently

For non-trivial or high-risk work, perform a separate review pass or assign a read-only reviewer.
Review the final diff rather than the implementation plan. Challenge:

- simpler designs and unnecessary abstractions;
- authorization gaps and user-data exposure;
- transaction, retry, idempotency, and concurrency failures;
- API compatibility and partial-deployment behavior;
- incorrect error mapping or insufficient operational context;
- query scale, missing constraints or indexes, and migration safety;
- tests that pass without proving the important behavior.

Resolve concrete findings or report them explicitly before completion.

## Report Completion

Return:

```text
Scope and design:
Contract impact:
Authentication and ownership impact:
Persistence, transaction, and migration impact:
Compatibility and rollout impact:
Files changed:
Tests and quality gates run:
Skipped or unavailable gates:
Independent review findings:
Remaining risks:
```
