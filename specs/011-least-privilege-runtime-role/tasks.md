---
description: "Implementation tasks for the least-privilege runtime database role"
---

# Tasks: Least-privilege runtime database role

**Input**: Design documents from `/specs/011-least-privilege-runtime-role/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, quickstart.md

**Tests**: Required by the API constitution; identified before each implementation task.

## Format: `[ID] [P?] [Story] Description`

## Requirement and Verification Evidence

| Requirement / acceptance | Tasks | Verification | Result / limitation |
| --- | --- | --- | --- |
| FR-001, FR-003, US1/AC1, AC3, SC-001 | T003, T010 | `smoke.sh` with `DATABASE_URL`=`gym_pulse_api`, `DATABASE_MIGRATE_URL`=owner on an isolated stack | Passed 2026-10-21: 23/23; `/ready` 200; no `permission denied` in API logs |
| FR-002, US1/AC2, SC-002 | T003, T006, T010 | validator block; `psql` as the role | Passed: validator green; `CREATE TABLE` → permission denied for schema public; `ALTER`/`DROP`/`CREATE INDEX`/`DISABLE RLS`/`CREATE POLICY` → must be owner; `TRUNCATE`, `schema_migrations`, `INSERT auth.users` → permission denied; attributes all false; 30 policies |
| FR-004, US3 | T004, T005 | `TestLoad_ProductionRequiresOwnerMigrateURL`, TLS migrate cases, `TestLoad_DevelopmentDefaultsMigrateURLToDatabaseURL` | Passed (`go test ./internal/config/`) |
| FR-005, US2/AC1–AC2 | T003, T011 | fresh cluster: all `*.up.sql` via `psql`, role absent beforehand; isolated stack: role pre-created `LOGIN` by `scripts/ci/runtime-role.sql`; down → up → up | Passed: fresh cluster created `gym_pulse_api` `NOLOGIN` with no privileged attribute and the validator passed; pre-created role kept `LOGIN`; down removed 30 policies and DML; up twice succeeded and the validator passed again |
| FR-006, US2/AC3 | T007 | `TestSupabaseSecurityMigrationProtectsEveryPublicApplicationTable`, `TestRuntimeRoleMigrationGrantsOnlyDML` | Passed; the `PASSWORD` guard caught a comment during implementation |
| FR-007, US2/AC4 | T006 | validator on the shared dev database at migration 25 | Failed as intended: `runtime role gym_pulse_api does not exist; migration 026 has not been applied` |
| FR-008 | T008, T009 | `docs/DATABASE_DESIGN.md` §13 and §19, `docs/RELEASE_RUNBOOK.md` rollout section, `research.md` | Written; design gate passed (31 tables, 292 columns, 27 indexes) |
| SC-003 | T012 | `go test ./...`, `golangci-lint run`, `./scripts/validate-database-design.sh` | Passed: all packages ok; 0 lint issues |
| SC-004 | T012 | `git diff --stat docs/CONTRACTS.md` | Empty |
| Hosted rollout | — | staging run of the runbook steps | Unverified: no staging access from this change; `auth.users` grant on hosted Supabase may need the manual step |

## Phase 1: Setup

- [x] T001 Record `git status --short`; fetch `origin/main`, confirm `026` is the next migration number
- [x] T002 [P] Write spec, research (RLS strategy and rejected alternatives), data model, quickstart

## Phase 2: Foundational

- [x] T003 [US1][US2] `migrations/026_create_runtime_role.{up,down}.sql`: guarded role creation, schema usage, per-table DML, per-table `gym_pulse_api_access` policy, default DML, guarded `auth` grants, final coverage assertion
- [x] T004 [US3] `internal/config`: `DatabaseMigrateURL`, `ErrProductionDatabaseMigrate`, shared session-capable predicate, TLS check over all three URLs, development default
- [x] T005 [US3] `internal/config/config_test.go`: new tests and release-env helper updates
- [x] T006 [US2] `scripts/validate-supabase-security.sh`: runtime-role assertions (existence, attributes, membership, ownership, schema `CREATE`, DML and policy per table, `schema_migrations`, `auth.users`)
- [x] T007 [US2] `internal/dao/supabase_security_migration_test.go`: per-table coverage by 026 or own policy; 026 grants only DML and contains no credential

## Phase 3: Startup and rollout

- [x] T008 [US1] `cmd/server/main.go`: migrate with `cfg.DatabaseMigrateURL` before opening request pools
- [x] T009 [P] `.env.example`, `README.md` env table, `docs/RELEASE_RUNBOOK.md` pre-deployment items and "Runtime role rollout (migration 026)"; `docs/DATABASE_DESIGN.md` role table, strategy section, P0 support, provenance 025/026, decision row, header
- [x] T010 [US1] `.github/workflows/ci.yml` smoke boots as `gym_pulse_api` with `scripts/ci/runtime-role.sql` mounted into initdb

## Phase 4: Verification

- [x] T011 Isolated stack and fresh-cluster evidence recorded above
- [x] T012 `gofmt`, `go test ./...`, `golangci-lint run`, `./scripts/validate-database-design.sh`
- [ ] T013 Staging rollout per runbook; record validator output and `pg_stat_activity` roles in release evidence (operator task)
