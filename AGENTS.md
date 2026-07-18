# GymPulse API Agent Guide

Read `.specify/memory/constitution.md` before non-trivial planning or implementation. New features
use GitHub Spec Kit (`$speckit-specify` through `$speckit-converge`).

## Verification

```bash
go test ./...
./scripts/smoke-toggle.sh # handler, validator, middleware, or contract changes
```

## Boundaries

- `docs/CONTRACTS.md` is client-facing truth and changes with the code it describes.
- Preserve authentication, ownership, idempotency, revisions, and stable error shapes.
- Follow handler → service → DAO separation and Google Go style.
- Pass `context.Context` first for database, network, and concurrent calls.
- Keep errors lowercase, wrap causes with `%w`, and never expose secrets or user data.
