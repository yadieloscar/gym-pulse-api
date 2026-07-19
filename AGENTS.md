# GymPulse API Agent Guide

Use `$gym-pulse-api-engineering` for non-trivial backend planning, implementation, debugging,
refactoring, or review. It applies the design, API, Go, PostgreSQL, security, testing, and independent
review strategy that complements the automated quality gates.

Use `$gym-pulse-cross-repo-delivery` when work may affect the Expo app, API contract compatibility,
deployment sequencing, or end-to-end acceptance. Keep the two repositories on separate branches and
PRs, and link their dependency explicitly.

Read `.specify/memory/constitution.md` before non-trivial planning or implementation. New features
use GitHub Spec Kit (`$speckit-specify` through `$speckit-converge`).

## Verification

```bash
golangci-lint run # when available; mirrors the CI lint policy
go test ./...
./scripts/smoke-toggle.sh # handler, validator, middleware, or contract changes
```

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
