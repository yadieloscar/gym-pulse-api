# Implementation Plan

## Constitution check

- Contract First: deletion and readiness behavior update `docs/CONTRACTS.md` and executable tests.
- Owned Authenticated Data: Supabase roles lose application-table access; Go ownership checks remain.
- Idempotent Evolution: deletion and migrations are retry-safe and forward-compatible.
- Executable Evidence: security, partial-failure, contract, and coverage assertions precede completion.
- Simple Go Boundaries: handlers own transport, services coordinate deletion policy, DAOs own SQL,
  and storage/auth clients remain narrow interfaces.

## Phase 1 — Supabase application-data boundary

Add a forward migration that enables RLS and revokes existing/default application-object privileges
from Data API roles. Add a validation script or PostgreSQL-backed test proving the resulting grants
and RLS state. Do not use `FORCE ROW LEVEL SECURITY`, because the direct API database role must
continue to operate.

## Phase 2 — Complete account deletion

Extend avatar storage with idempotent removal. Serialize account deletion and avatar upload with a
cross-replica per-user lock held on a separate bounded database pool, recheck identity inside that
boundary, and coordinate application-data, avatar, and final auth-user deletion without returning
`204` after a failed required step. Auth stays last so an ambiguous provider response cannot strand
application-held personal data after the identity is gone. Keeping lock sessions outside the main
query pool prevents pool-saturation deadlocks. Preserve stable public errors and make the entire
operation safely repeatable.

## Phase 3 — HTTP and operational readiness

Fix CORS method/header drift, add database readiness, configure bounded pgx pool behavior, and include
request correlation without logging secrets or user data. Keep `/health` backward-compatible.

## Phase 4 — Runtime and CI

Move to supported patched Go and Alpine images, update affected dependencies, add vulnerability and
container gates, enforce accurate statement coverage, and run CI on the protected production branch.

## Verification

- Focused service, middleware, handler, migration, and client tests.
- `gofmt` on changed Go files.
- `go test ./...`
- `go test -race ./...`
- `golangci-lint run`
- `govulncheck ./...`
- `./scripts/smoke-toggle.sh`
- Container build and vulnerability scan.
- Direct Supabase-role denial checks in staging before production migration.

## Rollout

1. Back up and validate staging.
2. Apply the privilege/RLS migration and run direct denial plus API smoke tests.
3. Deploy the backward-compatible API.
4. Observe readiness and errors.
5. Release the linked app only after API acceptance.
