# Quickstart: Validate the Database Design

## Prerequisites

- Work from the `gym-pulse-api` repository root.
- No production database credentials are required for the mandatory static validation.
- A disposable or read-only database connection is optional for catalog verification.

## 1. Run the design drift check

```bash
./scripts/validate-database-design.sh
```

Expected result:

```text
database design validation passed: 30 tables, 277 columns, and 24 named migration indexes documented
```

The script does not connect to or modify a database.

## 2. Review the main artifact

Open `docs/DATABASE_DESIGN.md` and verify:

- the Mermaid ERD renders;
- every table heading is present in the physical catalog;
- **Current** and **Target** behavior are visually distinct;
- each roadmap item has priority, rollout stages, compatibility, and evidence.

## 3. Run repository evidence

```bash
go test ./...
golangci-lint run
```

Documentation does not change Go behavior, but these gates ensure the inspected repository baseline
remains healthy.

`./scripts/smoke-toggle.sh` is not required because `docs/CONTRACTS.md` and observable API behavior do
not change.

## 4. Optional live catalog verification

Use a disposable database migrated through 019 or a production read replica/read-only connection.
Never point destructive migration tests at production.

```sql
select count(*)
from information_schema.tables
where table_schema in ('public', 'auth')
  and table_type = 'BASE TABLE';
```

Then inspect:

```sql
select table_schema, table_name, column_name, data_type, is_nullable, column_default
from information_schema.columns
where table_schema in ('public', 'auth')
order by table_schema, table_name, ordinal_position;

select schemaname, tablename, indexname, indexdef
from pg_indexes
where schemaname in ('public', 'auth')
order by schemaname, tablename, indexname;
```

Expected interpretation:

- **Pass**: the static script passes and the live catalog matches the migration-derived current
  catalog.
- **Drift**: the script reports missing documentation or the live catalog differs from migrations;
  stop schema work and reconcile intentionally.
- **Environment unavailable**: static validation can still pass; report live verification as
  skipped rather than failed.

## Verification record — 2026-07-27

| Gate | Result |
|---|---|
| `./scripts/validate-database-design.sh` | Passed: 25 tables, 227 exact table/column pairs, and 18 named migration indexes |
| `go test ./...` | Passed with `GOCACHE=/private/tmp/gym-pulse-go-build-cache`; the default host cache was sandbox-inaccessible |
| `golangci-lint run ./...` | Passed: 0 issues; final sandbox rerun used isolated `GOCACHE` and `GOLANGCI_LINT_CACHE` paths |
| `git diff --check` | Passed |
| `scripts/smoke-toggle.sh` | Skipped: public API contract and runtime behavior are unchanged |
| App tests | Skipped: the app repository is unchanged |
| Live PostgreSQL catalog comparison | Skipped: no disposable/read-only database connection was required or used |


## Reconciliation verification — 2026-10-05

| Gate | Result |
|---|---|
| `./scripts/validate-database-design.sh` | Passed: 30 tables, 277 exact table/column pairs, 24 named indexes through migration 019 |
| Validator disposable fixtures | Passed baseline; correctly rejected stale declared counts, a missing sport column, a missing new index, and a missing inventory declaration |
| `bash -n scripts/validate-database-design.sh` | Passed |
| `git diff --check` and untracked-file `git diff --no-index --check` | Passed, including the preserved new artifacts |
| `go test ./...` and `golangci-lint run` | Passed on reconciled API baseline; 0 lint issues |
| Contract smoke/app tests for database-document change | Not required: public contract/runtime sources unchanged by this package |
| Disposable PostgreSQL 16 catalog | Exact match: 30 tables/277 columns, all 24 named indexes present; 29/29 public application tables have RLS; six legacy auth FKs are validated cascades |
| Hosted RLS/grants/roles and production rollout | Unverified: local PostgreSQL is not hosted Supabase/deployment evidence |
| Independent reconciliation review | Completed by primary engineer against migrations 016–019, config/account-service sources and disposable catalog; earlier T015 remains historical |

The static checker proves inventory/name coverage, not SQL types/defaults/checks/FK actions, query
performance, RLS/grants, or production state. Those remain source-review/live-verification duties.
Supabase-managed `auth` tables and columns beyond the supported `auth.users.id` are excluded from
GymPulse's 30/277 inventory; broad information-schema counts on hosted Supabase will be larger.

The documentation branch was rebased onto merged API release-safety commit d96d764. Transaction
boundaries and range/replay prose now reflect that implementation; the schema inventory is unchanged.
Deployment and provider/device acceptance remain unverified.
