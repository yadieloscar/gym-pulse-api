# Feature Specification: Least-privilege runtime database role

**Feature Branch**: `security/least-privilege-runtime-role`

**Feature ID**: `gympulse-least-privilege-runtime-role`

**Created**: 2026-10-21

**Status**: Approved

**Input**: Engineering-lead follow-up to the 2026-10 security sweep (PR #32): "Design and implement a least-privilege runtime database role for the API so migrations run as the owner role and requests run as a role holding only DML on application tables, with RLS semantics that still let the API read every user's rows."

## Clarifications

### Session 2026-10-21

- Q: How does a non-owner role see rows when RLS is enabled with no policies? → A: Through an explicit permissive policy per table addressed to the runtime role. `BYPASSRLS` is rejected because granting it requires a superuser, which hosted Supabase does not provide, and because it would hide a table created without a policy. FORCE RLS stays off; it is irrelevant to a non-owner and would only change the owner's own behavior.
- Q: Does the policy restrict rows by user? → A: No. The API already scopes every owned query to the authenticated UUID in SQL and that remains the ownership boundary. The policy is unconditional so the API can read any user's rows the way it does today.
- Q: Who creates the role? → A: Production operators create it with a login credential before the release that grants it. Local and CI databases get it created automatically, without login, by the migration when it is absent. The migration never carries a credential and never alters an existing role.
- Q: What about the identity table that Supabase owns? → A: The API needs read access to it to confirm a token's subject still exists. The migration attempts the grant and warns if the owner cannot make it; the release runbook names the manual grant and the validator proves the result.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Requests run without schema power (Priority: P1)

An operator deploys the API so that request traffic connects as a database role that can only read and change rows in application tables. A bug or injection in request handling can no longer create, alter, or drop tables, change grants, or read the migration ledger.

**Why this priority**: This is the last structural gap from the security sweep: today the request path owns every table and has full DDL.

**Independent Test**: Start the API with the request connection pointing at the restricted role and the migration connection at the owner. Every smoke flow passes, and a query as the restricted role for a `CREATE TABLE`, `ALTER TABLE`, or `SELECT FROM schema_migrations` is denied.

**Acceptance Scenarios**:

1. **Given** a database migrated to the current version, **When** the API starts with the restricted request role and the owner migration role, **Then** it becomes ready and every existing smoke flow passes unchanged.
2. **Given** the restricted role, **When** it attempts any DDL, truncation, trigger creation, or a read of the migration ledger, **Then** the database denies it.
3. **Given** the restricted role, **When** the API reads or writes any user's rows in any application table, **Then** the rows are reachable exactly as before.

---

### User Story 2 - Provisioning is repeatable and verifiable (Priority: P1)

An operator can apply the release to a fresh local database, a CI database, staging, and production and get the same role, grants, and policies without hand-written SQL, then prove the result with the existing security validator.

**Why this priority**: Grants that drift between environments are the usual way a least-privilege design silently becomes an owner connection again.

**Independent Test**: Apply migrations to a database where the role does not exist and to one where an operator already created it; both end with identical grants and policies. The validator passes on both and fails when any one grant, policy, or attribute is wrong.

**Acceptance Scenarios**:

1. **Given** a database without the role, **When** migrations run, **Then** the role exists without login, holds DML on every application table, and has a policy on each.
2. **Given** a database where the operator pre-created the role with login, **When** migrations run, **Then** the role keeps its login and credential and gains the same grants and policies.
3. **Given** a future migration that creates a table without a policy for the role, **When** the repository tests run, **Then** they fail before the migration ships.
4. **Given** a provisioned database, **When** the security validator runs, **Then** it reports the role has no DDL, ownership, superuser, bypass, or role-creation power and has DML plus a policy on every application table.

---

### User Story 3 - Configuration refuses an unsafe release (Priority: P2)

A release deployment that forgets the separate owner migration connection, or points it at a transaction-mode pooler or a plaintext connection, fails at startup with a clear error instead of running migrations through the request pool.

**Independent Test**: Start the binary in staging or production without the migration connection, with the pooler's transaction port, and without TLS; each refuses to start with a distinct message. Development and test keep working with only the request connection.

**Acceptance Scenarios**:

1. **Given** staging or production, **When** the migration connection is missing, **Then** startup fails and names the missing setting.
2. **Given** staging or production, **When** the migration connection uses the transaction-mode pooler port or lacks TLS, **Then** startup fails like the existing lock connection rules.
3. **Given** development or test, **When** only the request connection is set, **Then** migrations use it and startup succeeds.

### Edge Cases

- The owner cannot re-grant read access to the identity table: the migration finishes with a warning, the runbook names the manual grant, and the validator fails until it is made.
- A table exists in the database but is missing from the migration's list: the migration fails loudly rather than leaving the table unreachable for the role.
- Rolling the migration back while a deployment still connects as the restricted role would break requests; the runbook requires switching the connection back first.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Request traffic MUST run as a database role with only `SELECT`, `INSERT`, `UPDATE`, and `DELETE` on the application tables, `USAGE` on the application and identity schemas, and read access to the identity table.
- **FR-002**: The runtime role MUST NOT be a superuser, bypass row-level security, create roles or databases, own any object, create objects in the application schema, hold `TRUNCATE`, `REFERENCES`, or `TRIGGER`, or reach the migration ledger.
- **FR-003**: Every application table MUST keep RLS enabled and not forced and MUST carry one permissive policy for all commands addressed to the runtime role.
- **FR-004**: Migrations MUST run through a separate owner connection. Staging and production MUST require it explicitly, with the same session-mode and TLS rules as the lock connection; development and test MUST default it to the request connection.
- **FR-005**: The migration MUST create the role without login when absent, MUST never alter an existing role or contain a credential, and MUST be safe to apply where the hosted identity roles do not exist.
- **FR-006**: The repository MUST fail its tests when a migration creates an application table without provisioning it for the runtime role.
- **FR-007**: The security validator MUST assert FR-001 through FR-003 against a live database and fail when the role is missing.
- **FR-008**: Documentation MUST record the chosen RLS strategy, the rejected alternatives, and the rollout order: create role, grant, deploy with the new configuration, verify.

### Key Entities

- **Runtime role** `gym_pulse_api`: the request identity; DML only.
- **Owner connection**: the existing migration identity (`postgres` on Supabase); DDL at startup only.
- **Access policy** `gym_pulse_api_access`: one per application table, unconditional, addressed to the runtime role.

## Success Criteria *(mandatory)*

- **SC-001**: The full smoke suite passes with the API connected as the runtime role on a database migrated by the owner.
- **SC-002**: The security validator passes on that database and lists no DDL, ownership, or privileged attribute for the runtime role.
- **SC-003**: `go test ./...`, `golangci-lint run`, and the database-design gate pass.
- **SC-004**: No public API contract changes; `docs/CONTRACTS.md` is unchanged.

## Assumptions

- Hosted Supabase's `postgres` role holds `CREATEROLE` and owns the application tables, so it can create the role locally when absent and grant on every table.
- The identity table in hosted projects may need a manual grant or policy by its owning role; this is an operator step, not an application behavior.
