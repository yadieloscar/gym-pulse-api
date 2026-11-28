# Feature Specification: Per-user row-level security backstop

**Feature Branch**: `spec/029-per-user-rls-backstop`

**Feature ID**: `gympulse-per-user-rls-backstop`

**Sibling Spec**: N/A (API only; no contract change)

**Created**: 2026-11-15

**Status**: Draft

**Input**: Owner request (2026-11-15, "Yes, spec it") after the pre-deploy audit of the API's
public exposure. Finding 4: the runtime role's row-level security policy is unconditional, so a
single request path that forgets its owner filter would expose every athlete's rows. Make the
database refuse rows that do not belong to the authenticated athlete, as a second line behind the
API's own ownership scoping.

**Amends**: `specs/011-least-privilege-runtime-role/spec.md`, clarification 2 ("Does the policy
restrict rows by user? No") and FR-003 (one unconditional policy per table). Every other
requirement of spec 011 stays in force.

## Clarifications

### Session 2026-11-15

- Q: Does this replace the API's ownership scoping? → A: No. Every owned query keeps its owner
  filter and the shared not-found behavior for foreign and missing ids. The database check is a
  backstop that makes a missed filter return nothing instead of someone else's data.
- Q: What happens when a request reaches the database without an authenticated athlete attached?
  → A: It fails closed. Owned tables return no rows and refuse writes. Nothing falls back to the
  old read-everything behavior.
- Q: Which data is not owned by one athlete? → A: The shared reference data: the exercise
  catalog, the starter program catalog, the shared food catalog, and the shared record of barcodes
  that no source knows. Any athlete may read it. Only the paths that already write it today (the
  food lookup cache) may write it. Migrations and seed data keep running as the owner.
- Q: Must this ship before the first hosted deployment? → A: No. The audit found no cross-user
  access, and the cross-user smoke step added in API PR #55 guards regressions. This feature is
  required before a second person's data is stored in production.
- Q: Does the mobile app change? → A: No. Requests, responses, statuses, and error codes are
  unchanged, so `docs/CONTRACTS.md` is unchanged.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A missed owner filter exposes nothing (Priority: P1)

An athlete's rows stay private even when a request path has a bug that drops its owner filter.
The database itself returns only rows that belong to the athlete the request authenticated as,
and refuses to insert, change, or delete rows on behalf of anyone else.

**Why this priority**: This is the whole point. Today ownership rests on about 200 hand-written
query sites across 29 data-access files; one mistake in one of them is a data leak with no second
line of defense.

**Independent Test**: On a migrated database, connect as the runtime role with athlete A attached
and run unfiltered reads, updates, and deletes against every owned table that also holds athlete
B's rows. Only A's rows are returned or affected. Insert a row owned by B while A is attached; the
database refuses it.

**Acceptance Scenarios**:

1. **Given** two athletes with rows in every owned table, **When** the runtime role reads a table
   with no owner filter while athlete A is attached, **Then** only A's rows are returned.
2. **Given** the same data, **When** an unfiltered update or delete runs with A attached, **Then**
   B's rows are unchanged.
3. **Given** A is attached, **When** a row is inserted or updated so that it would belong to B,
   **Then** the database refuses the write.
4. **Given** a child row that carries no owner of its own (for example a set inside a scheduled
   workout), **When** it is read or written with A attached, **Then** it is reachable only when
   its parent belongs to A.
5. **Given** no athlete is attached, **When** the runtime role reads or writes an owned table,
   **Then** no rows are returned and writes are refused.

---

### User Story 2 - Every existing flow behaves exactly as before (Priority: P1)

An athlete using the app notices nothing. Every documented request returns the same status, body,
and error code as before, including idempotent replays, revision conflicts, the shared not-found
answer for foreign ids, data export, history import, and account deletion.

**Why this priority**: A backstop that breaks or changes a working flow is worse than no backstop.
The identity must follow the request through every database interaction it makes, including
multi-step transactions and the per-athlete operation lock.

**Independent Test**: Run the full smoke suite, including the cross-user step, and the full
database integration suite with the API connected as the runtime role under the new policies.
All pass with no contract or assertion changes.

**Acceptance Scenarios**:

1. **Given** the new policies, **When** the full smoke suite runs as the runtime role, **Then**
   every step passes without changes to its expected statuses or bodies.
2. **Given** a request that performs several database steps in one transaction, **When** it runs,
   **Then** every step sees the same attached athlete and the transaction's all-or-nothing
   behavior is unchanged.
3. **Given** consecutive requests from different athletes, **When** they reuse the same database
   connection, **Then** the second request never sees or acts as the first athlete.
4. **Given** account deletion, including a retry after the identity is already gone, **When** it
   runs, **Then** it removes exactly that athlete's rows and still returns the documented result.
5. **Given** the readiness check and the check that a token's subject still exists, **When** they
   run without an owned-table read, **Then** they keep working.

---

### User Story 3 - A gap in coverage cannot ship unnoticed (Priority: P2)

A maintainer who adds a table, or a new database call that does not carry the athlete, learns
about it from a failing check before the change merges, not from a production incident.

**Why this priority**: The protection only holds while it is complete. Spec 011 already fails the
build when a table lacks a runtime-role policy; that guard has to follow the stricter rule.

**Independent Test**: Add a migration that creates an owned table with an unconditional policy, or
with none; the repository tests fail. Run the security validator against a database where one
owned table still has an unconditional policy; it fails and names the table.

**Acceptance Scenarios**:

1. **Given** a new migration that creates a table without classifying it as owned or shared and
   giving it the matching policy, **When** the repository tests run, **Then** they fail.
2. **Given** a database where any owned table's policy does not restrict rows to the attached
   athlete, **When** the security validator runs, **Then** it fails and names the table.
3. **Given** the two-athlete database test from Story 1, **When** a new owned table is added,
   **Then** the test covers it without being edited by hand, or fails until it is listed.

---

### User Story 4 - The stricter rule rolls out without an outage (Priority: P2)

An operator can release this to an environment that is already serving requests without a window
in which requests see no data, and can roll it back.

**Why this priority**: Stricter policies and the API change that attaches the athlete depend on
each other. Applied in the wrong order, every request returns empty results.

**Independent Test**: On a database at the previous version with the previous API running, apply
the release in the documented order; requests succeed at every step. Roll back in the documented
order; requests succeed at every step.

**Acceptance Scenarios**:

1. **Given** an API version that attaches the athlete and a database that still has the
   unconditional policies, **When** requests run, **Then** they behave exactly as before.
2. **Given** that API version is fully deployed, **When** the stricter policies are applied,
   **Then** requests keep succeeding with no empty-result window.
3. **Given** a rollback is needed, **When** the policies are relaxed before the API is rolled
   back, **Then** requests keep succeeding.

### Edge Cases

- A request is cancelled or fails midway: the attached athlete must not remain on a connection
  that the next request reuses.
- A long-held connection used for per-athlete operation locks: the lock connection must not leak
  one athlete's identity into another athlete's locked operation.
- Rows that link two owned resources (for example a sport activity linked to a scheduled
  session): both sides must belong to the attached athlete; the documented not-found answer for a
  foreign side is unchanged.
- The shared food catalog is written while an athlete is attached: the write succeeds and the
  row stays shared, not owned by that athlete.
- Idempotency records: a replay key from athlete A must never match or reveal athlete B's record.
- The identity table that Supabase owns: this feature does not change how it is read; the
  id-only read from API PR #55 stays.
- A value attached as the athlete that is not a valid identifier: treated as no athlete, so the
  request fails closed.

## Non-Goals *(mandatory)*

- No change to any request, response, status, or error code; `docs/CONTRACTS.md` is unchanged and
  the mobile app is not touched.
- No removal or weakening of the API's own owner filters or of the shared not-found behavior.
- No use of Supabase's Data API, its `authenticated` or `anon` roles, or its token claims inside
  the database. The API stays the only client of the application tables.
- No change to the runtime role's privileges from spec 011, to FORCE RLS (stays off), or to how
  migrations run as the owner.
- No per-row sharing between athletes, coach or team access, or admin read-all path.
- No rate limiting, request-size limits, or other audit findings; those are tracked separately.
- No hosted deployment work; the release runbook is updated, but deploying is a separate step.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Every application table MUST be classified as exactly one of: owned directly by an
  athlete, owned through a parent row, or shared reference data. The classification MUST be
  recorded where the repository tests and the security validator can check it.
- **FR-002**: For tables owned directly, the runtime role MUST be able to read, insert, update,
  and delete only rows whose owner is the athlete attached to the current request.
- **FR-003**: For tables owned through a parent, the runtime role MUST be able to reach a row
  only when the row's parent chain ends at a row owned by the attached athlete.
- **FR-004**: For shared reference data, the runtime role MUST be able to read every row
  regardless of the attached athlete, and MUST be able to write only the shared tables the API
  already writes at runtime today.
- **FR-005**: When no valid athlete is attached, owned tables MUST return no rows and MUST refuse
  writes. There MUST be no fallback to unrestricted access for the runtime role.
- **FR-006**: The API MUST attach the authenticated athlete to every database interaction a
  request makes on owned tables, including multi-step transactions, batched statements, and
  operations run under the per-athlete lock, using the identity verified from the token and never
  a client-supplied value.
- **FR-007**: The attached athlete MUST NOT persist on a database connection beyond the unit of
  work it was attached for, whether that work succeeds, fails, or is cancelled.
- **FR-008**: The readiness check, the token-subject existence check, and account deletion
  (including its retry after the identity is gone) MUST keep their documented behavior.
- **FR-009**: The API's own owner filters and the shared not-found behavior MUST remain in place;
  this feature MUST NOT be the only ownership check on any path.
- **FR-010**: The repository tests MUST fail when a migration adds a table without a
  classification and the matching policy, replacing the spec 011 guard that accepts an
  unconditional policy.
- **FR-011**: The security validator MUST fail against a live database when any owned table's
  runtime-role policy is unconditional or missing, and when the runtime role can read another
  athlete's row in any owned table.
- **FR-012**: An automated database test MUST prove FR-002, FR-003, and FR-005 for every owned
  table with two athletes' rows present, and MUST run in CI.
- **FR-013**: The release MUST be deployable in an order that never leaves a serving API unable
  to see its own athletes' rows, and reversible in an order with the same property. The release
  runbook MUST state both orders.
- **FR-014**: `specs/011-least-privilege-runtime-role` MUST be amended in the same change to
  record that the policy is now per-athlete, with the rejected alternatives and the reason the
  earlier decision was reversed.
- **FR-015**: The change MUST NOT add a measurable slowdown that a person would notice: the
  smoke suite's total run time and the integration suite's total run time MUST stay within 15% of
  their values before the change on the same machine.

### Key Entities

- **Attached athlete**: the authenticated athlete's identifier made known to the database for one
  unit of work, taken from the verified token.
- **Owned table**: a table whose rows belong to one athlete, directly or through a parent row.
  Today 24 tables carry the owner directly and about 10 are reached through a parent.
- **Shared reference table**: catalog data every athlete may read (exercise catalog, starter
  programs and their workouts and exercises, shared food catalog, unknown-barcode record).
- **Runtime role policy**: the per-table rule from spec 011 that lets the runtime role reach rows;
  it changes from unconditional to restricted by the attached athlete on owned tables.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With two athletes' rows in every owned table, an unfiltered read as the runtime
  role with one athlete attached returns zero of the other athlete's rows, for 100% of owned
  tables.
- **SC-002**: With no athlete attached, the runtime role reads zero rows from 100% of owned
  tables and every write to them is refused.
- **SC-003**: The full smoke suite, including the cross-user step, passes with zero changed
  expectations.
- **SC-004**: The full database integration suite and the unit suite pass; service and
  middleware coverage stay at or above 90%.
- **SC-005**: The security validator passes on a migrated database and fails when any single
  owned table is given back an unconditional policy.
- **SC-006**: `docs/CONTRACTS.md` has no diff.
- **SC-007**: Smoke and integration suite run times are within 15% of their pre-change values.
- **SC-008**: The documented rollout and rollback orders are each rehearsed once on a local
  database with the API serving requests, with zero failed requests during either.

## Assumptions

- The API remains the only client of the application tables, connecting as `gym_pulse_api` for
  requests and as the owner for migrations, as spec 011 established.
- The verified token subject is the athlete identifier used as the owner in every owned table.
- Hosted Supabase lets the owner role replace policies on the application tables it owns.
- Request traffic reaches the database through a session-mode connection, as the release
  configuration already requires for the lock connection; the plan must confirm that attaching
  an athlete is safe on whatever pooling mode the request connection uses.
- Nothing is hosted yet, so the first environment may receive the API change and the stricter
  policies together; FR-013 still applies to every environment after that.
- The parent chain for every indirectly owned table can be followed with existing keys and
  indexes; the plan must check each one and add an index only where a query plan shows the need.
- Background work that touches owned tables without a request does not exist today. If the plan
  finds any, it must attach the athlete it acts for or be listed as an explicit exception here.
