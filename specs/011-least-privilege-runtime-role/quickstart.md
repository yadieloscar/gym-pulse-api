# Quickstart: Least-privilege runtime database role

## Implementation order

1. Migration 026 up/down; `docs/DATABASE_DESIGN.md` role table, strategy section, provenance, decision rows; `./scripts/validate-database-design.sh`.
2. `internal/config`: `DatabaseMigrateURL`, `ErrProductionDatabaseMigrate`, shared session-capable URL check; tests for missing, `:6543`, non-postgres scheme, TLS, and the development default.
3. `cmd/server/main.go`: migrate with `cfg.DatabaseMigrateURL` before opening the request pools.
4. `scripts/validate-supabase-security.sh`: runtime-role block (existence, attributes, membership, ownership, schema `CREATE`, per-table DML and policy, `schema_migrations`, `auth.users`).
5. `internal/dao/supabase_security_migration_test.go`: every public table listed in 026 or given its own policy; 026 grants only DML and carries no credential.
6. `.env.example`, `README.md`, `docs/RELEASE_RUNBOOK.md` rollout section; CI smoke boots as the restricted role via `scripts/ci/runtime-role.sql`.
7. Gates: `gofmt`, `go test ./...`, `golangci-lint run`, design validator, security validator and `smoke.sh` on an isolated stack with the API connected as `gym_pulse_api`.

## Tests identified before implementation

| Requirement | Evidence |
|---|---|
| FR-001, FR-003, SC-001 | `smoke.sh` against an API whose `DATABASE_URL` is `gym_pulse_api`; manual `psql` as the role: DML succeeds on an application table |
| FR-002, SC-002 | Validator block in `validate-supabase-security.sh`; manual `psql` as the role: `CREATE TABLE`, `ALTER TABLE`, `SELECT FROM schema_migrations` denied |
| FR-004 | `TestLoad_ProductionRequiresOwnerMigrateURL`, `TestLoad_ProductionRequiresDatabaseTLS` migrate cases, `TestLoad_DevelopmentDefaultsMigrateURLToDatabaseURL` |
| FR-005 | Apply 026 to a database with the role pre-created (`LOGIN`) and to one without it; `TestRuntimeRoleMigrationGrantsOnlyDML` forbids `PASSWORD` and `GRANT ALL` |
| FR-006 | `TestSupabaseSecurityMigrationProtectsEveryPublicApplicationTable` runtime-role coverage |
| FR-007 | Validator fails on a database before 026 (role missing) and passes after |
| FR-008 | `docs/DATABASE_DESIGN.md` §13 and §19, `docs/RELEASE_RUNBOOK.md` rollout section, this spec's `research.md` |
| SC-003 | `go test ./...`, `golangci-lint run`, `./scripts/validate-database-design.sh` |
| SC-004 | `git diff --stat docs/CONTRACTS.md` is empty |

## Local verification recipe

```bash
# isolated stack: db with the role pre-created, api as gym_pulse_api, migrations as the owner
docker build -t gym-pulse-api:lpr gym-pulse-api/.worktrees/least-privilege-role
docker compose -p lpr -f <scratch compose> up -d
docker compose -p lpr exec -T db psql -U gympulse -d gympulse -c "INSERT INTO auth.users (id) VALUES ('00000000-0000-0000-0000-000000000001') ON CONFLICT DO NOTHING;"
API=http://localhost:18081 gym-pulse-api/.worktrees/least-privilege-role/scripts/smoke.sh
docker compose -p lpr exec -T db bash -c 'DATABASE_URL=postgres://gympulse:gympulse@localhost:5432/gympulse bash -s' < scripts/validate-supabase-security.sh
```

## Manual acceptance

In staging, follow the runbook's "Runtime role rollout" in order and confirm `pg_stat_activity`
shows request traffic as `gym_pulse_api` with no `permission denied` in the logs.
