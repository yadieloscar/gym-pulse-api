# GymPulse API Agent Guide

## Natural-language routing

The user does not need to name a skill. Infer the applicable workflows from the requested outcome
and apply the skill rules below. Explicit `$skill-name` mentions are optional overrides. When Codex
starts from the parent `gym-pulse` directory, the workspace router performs this classification.
See `docs/CODEX_PLAYBOOK.md` for practical prompt examples.

Use `$gym-pulse-engineering-lead` for complex, ambiguous, high-risk, or multi-lane work. The lead owns
architecture, selects the applicable engineering workflows, delegates only when efficient, integrates
specialist evidence, and decides readiness.

Use `$gym-pulse-api-engineering` for non-trivial backend planning, implementation, debugging,
refactoring, or review. It applies the design, API, Go, PostgreSQL, security, testing, and independent
review strategy that complements the automated quality gates.

Use `$gym-pulse-cross-repo-delivery` when work may affect the Expo app, API contract compatibility,
deployment sequencing, or end-to-end acceptance. Keep the two repositories on separate branches and
PRs, and link their dependency explicitly. Coordinated specifications share one stable feature ID;
their local numeric Spec Kit directory prefixes may differ.

Use subagents only when independent analysis, implementation, or review lanes are likely to save time
or improve evidence. Keep focused, sequential, and overlapping-file work with the primary agent.

Read `.specify/memory/constitution.md` and `docs/SPEC_WORKFLOW.md` before non-trivial planning,
implementation, or feature resumption. Reuse specs for focused fixes, amend them for changed intent,
and use the full Spec Kit cycle for new non-trivial scope. Apply specialists at the stage where their
expertise affects decisions. Documentation-only work uses direct edits and documentation validation.

## Verification

For documentation/workflow-only changes, validate the edited skills, links, mirrored guidance, and
`git diff --check`; run the parent context lint when available. The application gates below apply
when code, dependencies, configuration, or executable behavior changes.

```bash
golangci-lint run # when available; mirrors the CI lint policy
go test ./...
./scripts/validate-database-design.sh # any migration or docs/DATABASE_DESIGN.md change
./scripts/smoke-toggle.sh # handler, validator, middleware, or contract changes
```

`./scripts/validate-database-design.sh` needs no database: it compares the tables, columns, and
named indexes in `docs/DATABASE_DESIGN.md` against `migrations/*.up.sql` as exact sets. Run it in
the same change that touches a migration, because that document is a manually maintained inventory
and this is the only thing that keeps it honest. CI enforces it in the `ci` job.

`./scripts/validate-supabase-security.sh` asserts row-level security and Data API privileges against
a live migrated schema, so it needs `DATABASE_URL` and `psql`. Run it for migration, RLS, grant, or
Supabase-exposure changes — against a local stack, or via `make acceptance` from the parent
workspace. CI enforces it in the `smoke` job, where a migrated database already exists.

Add `go test -race ./...` for concurrency or shared-state changes, `govulncheck ./...` for dependency
or security-sensitive changes when available, and targeted fuzz or PostgreSQL integration tests when
the changed boundary warrants them. A green linter and test suite are evidence, not proof of sound
design; complete the skill's design assessment and final review.

## Boundaries

- `docs/CONTRACTS.md` is client-facing truth and changes with the code it describes.
- Preserve authentication, ownership, idempotency, revisions, and stable error shapes.
- Follow handler → service → DAO separation and Google Go style.
- Pass `context.Context` first for database, network, and concurrent calls.
- Keep errors lowercase, wrap causes with `%w`, and never expose secrets or user data.
- Preserve explicit transaction invariants, migration compatibility, cancellation, and goroutine
  ownership; avoid speculative abstractions and concurrency.
