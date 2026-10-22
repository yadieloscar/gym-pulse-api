# Implementation Plan: Least-privilege runtime database role

**Branch**: `security/least-privilege-runtime-role` | **Date**: 2026-10-21 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/011-least-privilege-runtime-role/spec.md`

## Summary

Split the API's database identity in two. Startup migrations keep running as the schema owner
through a new `DATABASE_MIGRATE_URL`; request traffic connects as `gym_pulse_api`, a role that
migration 026 provisions with `SELECT`, `INSERT`, `UPDATE`, `DELETE` on the 30 application tables
and one unconditional permissive RLS policy per table. RLS stays enabled and not forced; `BYPASSRLS`
is rejected (see `research.md`). The security validator gains a block that proves the role has no
DDL, ownership, or privileged attribute, CI smoke boots the API as the role, and the runbook records
the rollout order: create role, grant, deploy with the new configuration, verify.

## Workflow and Specialist Scope

- **Feature**: `gympulse-least-privilege-runtime-role`, new scope in this repository only; no app
  change and no contract change.
- **gym-pulse-engineering-lead**: authentication/authorization and migration risk; RLS strategy
  decision and rejected alternatives recorded in `research.md`.
- **gym-pulse-api-engineering**: config validation, startup ordering, migration, validator, tests.
- **supabase-postgres-best-practices**: role attributes, grants versus policies, default privileges,
  hosted `postgres` limits (no superuser, owns `public`, `auth.users` owned by `supabase_auth_admin`).
- Single-agent execution; one sequential lane.

## Technical Context

**Language/Version**: Go 1.26+

**Primary Dependencies**: chi, pgx/pgxpool, golang-migrate (no additions)

**Storage**: PostgreSQL; no table or column change; new role, grants, 30 policies, default privileges

**Testing**: `internal/config` tests, static migration tests in `internal/dao`, `go test ./...`,
`golangci-lint run`, `./scripts/validate-database-design.sh`, `./scripts/validate-supabase-security.sh`
and `scripts/smoke.sh` on an isolated stack with the API connected as `gym_pulse_api`

**Target Platform**: Linux HTTP service and Supabase-hosted PostgreSQL

**Project Type**: Go web service

**Performance Goals**: `USING (true)` policies add a constant-true qual per scan; no measurable cost

**Constraints**: No credential in any migration; migration idempotent and safe without hosted roles;
rollback is a configuration change, not a schema change

**Scale/Scope**: One migration pair, one config field, one `main.go` reorder, one validator block,
two static tests, docs, CI compose environment

## Constitution Check

*GATE: Passed before Phase 0 and re-checked after Phase 1 design.*

- [x] Contract: no observable API change; `docs/CONTRACTS.md` untouched (SC-004).
- [x] Owned authenticated data: ownership remains in DAO SQL; the policy is reachability for the
  role the API already authorizes, not a new authorization path. No secret enters the repository.
- [x] Idempotent evolution: 026 is forward-only in effect (role creation is cluster-level and
  guarded), re-runnable, and documents existing-data, rollback, and mixed-version behavior.
- [x] Executable evidence: tests identified in `quickstart.md` before implementation; config and
  static migration tests fail without the change.
- [x] Simple Go boundaries: one config field, one shared URL predicate, no new dependency or
  concurrency.

## Project Structure

### Documentation (this feature)

```text
specs/011-least-privilege-runtime-role/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── checklists/requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
cmd/server/main.go
internal/config/{config.go,config_test.go}
internal/dao/supabase_security_migration_test.go
migrations/026_create_runtime_role.{up,down}.sql
scripts/{validate-supabase-security.sh,ci/runtime-role.sql}
.github/workflows/ci.yml
.env.example, README.md
docs/{DATABASE_DESIGN.md,RELEASE_RUNBOOK.md}
```

## Design Assessment

- **Authentication and ownership**: unchanged at the HTTP boundary. The identity check
  (`AuthUserDAO`) now depends on `SELECT` on `auth.users` for `gym_pulse_api`; the migration
  grants it with a guarded fallback and the validator asserts it.
- **Persistence and migration**: migration 026 creates the role when absent, grants DML and schema
  usage, recreates the per-table policy, sets default DML for future owner-created tables, and
  raises if any public table is unprovisioned. Down removes grants and policies and keeps the role.
- **Startup ordering**: migrations run before any request pool opens, so a schema that needs the
  owner never meets a pool that only has DML.
- **Configuration**: `DATABASE_MIGRATE_URL` follows the lock-URL rules (session-capable, TLS) because
  golang-migrate holds a session advisory lock.
- **Compatibility and rollout**: an older image keeps connecting as the owner and ignores 026.
  Rollout is create role → deploy with 026 and owner URLs → verify → switch `DATABASE_URL` and
  `DATABASE_LOCK_URL` → verify. Rollback is pointing those two back at the owner.

## Complexity Tracking

None. No constitution exception is needed.
