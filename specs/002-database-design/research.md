# Research: Complete Database Design

## Decision 1: Treat migrations as physical truth and code as behavioral truth

**Decision**: Reconstruct the current physical schema from migrations 001–019, then reconcile it
with DAO SQL, `docs/CONTRACTS.md`, service invariants, and mobile client types.

**Rationale**: Migrations state what a new database receives, while DAO and service code reveal
filters, joins, transaction boundaries, and invariants not always enforced in SQL. Neither source is
complete by itself.

**Alternatives considered**:

- Document only the intended domain model: rejected because it would hide current constraints,
  legacy compatibility, and migration gaps.
- Generate documentation only from a live database: rejected because an environment may be
  unavailable and a live instance may already have drift.

## Decision 2: Keep current UUID identity

**Decision**: Keep UUID primary keys for public and user-owned resources. Do not introduce UUIDv7,
ULID, or integer surrogate keys in the documentation-only change.

**Rationale**: UUIDs are already public contract identifiers and appear throughout both repositories.
Changing key strategy would be a breaking, high-cost migration without measured index-locality
evidence at current scale.

**Alternatives considered**:

- `bigint identity`: smaller and insert-friendly, but exposes a second identity system and requires
  contract migration.
- UUIDv7: improves locality but requires an extension or application generator and mixed-ID rollout.
  Revisit for new high-volume append-only tables if measurement justifies it.

## Decision 3: Preserve snapshot layers

**Decision**: Preserve the copy/snapshot chain:

`starter catalog → user program → scheduled workout/sets → performed set`.

**Rationale**: Historical exercise names, categories, modalities, and targets must survive edits or
deletion of mutable planning sources. Nullable foreign keys are provenance; snapshot text/numeric
fields are the historical record.

**Alternatives considered**:

- Normalize all history back to current catalog/program rows: rejected because edits would rewrite
  the meaning of past activity.
- Store full snapshots in one JSON document: rejected because typed constraints, joins, and
  progression queries need relational fields.

## Decision 4: Continue dual legacy and goal-based domains during compatibility

**Decision**: Keep legacy `day_logs` and UUID `workout_sessions` as separate persistence roots.
New goal-based work uses session UUID identity; stats/history explicitly union both domains until a
separate contract deprecation removes legacy writes.

**Rationale**: Installed mobile clients cannot be upgraded atomically. The existing contract promises
both surfaces.

**Alternatives considered**:

- Immediately migrate all legacy rows and remove date-addressed endpoints: rejected as breaking.
- Make `day_logs` a view over sessions now: rejected because the existing payload and template
  override semantics are not a lossless one-to-one mapping.

## Decision 5: Make the Go API the sole application-data gateway

**Decision**: Production application tables must not be directly reachable by `anon` or
`authenticated` Data API roles. Prefer disabling the Supabase Data API for this database or removing
application schemas from exposed schemas and revoking grants. If any table remains exposed, enable
RLS and explicitly define least-privilege policies.

**Rationale**: The app architecture routes all application data through the Go API, which carries
authenticated ownership into every DAO query. A second direct-data path would bypass service
invariants and stable error behavior. Current Supabase guidance distinguishes object grants from RLS
and recommends both for exposed objects.

**Alternatives considered**:

- Duplicate all API authorization as RLS while retaining direct client access: rejected because the
  client does not use direct access and duplicated business rules would drift.
- Trust default Supabase grants on `public`: rejected because exposure defaults vary by project age
  and are not an acceptable security boundary.

References:

- [Supabase: Securing your API](https://supabase.com/docs/guides/api/securing-your-api)
- [Supabase: User management](https://supabase.com/docs/guides/auth/managing-user-data)
- [Supabase: Securing your data](https://supabase.com/docs/guides/database/secure-data)

## Decision 6: Reference only the managed auth primary key

**Decision**: In Supabase-hosted production, user-owned roots reference `auth.users(id)` with
`ON DELETE CASCADE`; never reference other managed auth columns. The local development stub is a
fixture compatibility mechanism, not an independently authoritative identity store.

**Rationale**: `sub` from a validated JWT is the ownership key. Supabase documents the primary key as
the stable supported FK target. Legacy user-owned roots should converge on the same FK behavior.

**Alternatives considered**:

- No database FK to auth identity: simpler locally, but permits orphaned owned data.
- Copy auth identity into a second complete user table: unnecessary while production shares the
  Supabase Postgres instance; an application tombstone can solve deletion-resurrection risk without
  duplicating auth.

## Decision 7: Use constraints for durable invariants and service validation for feedback

**Decision**: The target model adds missing domain checks, ownership FKs, and necessary uniqueness
where PostgreSQL can enforce the rule without relying on mutable cross-table state. Service
validation remains for actionable 4xx responses.

**Rationale**: Database constraints protect every write path, including future tools and migrations.
Cross-table ownership checks that cannot be expressed as simple constraints remain transactionally
enforced in DAO SQL.

**Alternatives considered**:

- Service-only validation: rejected because it allows invalid data through alternate write paths.
- Triggers for all cross-table policy: rejected due hidden behavior and migration complexity.

## Decision 8: Index from measured access patterns

**Decision**: Keep B-tree indexes for equality/range/order paths, add missing FK and high-value
composite indexes in a future migration, remove redundant prefix indexes only after plan evidence,
and avoid speculative GIN, BRIN, covering indexes, or partitioning.

**Rationale**: Current queries are dominated by user ID, date ranges, parent IDs, and ordered child
loads. PostgreSQL does not automatically index referencing FK columns. `set_logs` is the first likely
large table but should not be partitioned before roughly 100 million rows or measured maintenance
pressure.

**Alternatives considered**:

- Index every selectable column: rejected because write amplification and cache footprint would grow.
- Partition by date immediately: rejected because current scale is unknown and parent-scoped queries
  would gain complexity before proven benefit.

## Decision 9: Define explicit idempotency retention

**Decision**: Target 30-day retention for completed idempotency records, with scheduled deletion in
small batches and an index on `expires_at` for non-null expirations. Account deletion always removes
the user's records immediately through the identity cascade.

**Rationale**: Mobile retries may occur well after a response is lost. Indefinite response-body
retention is unnecessary and increases private-data footprint.

**Alternatives considered**:

- 24 hours: may be too short for offline mobile retry recovery.
- Permanent retention: unnecessary once replay windows close.

## Decision 10: Separate release migration execution from application startup

**Decision**: The target operating model runs reviewed forward migrations as a release step before
deploying code that depends on them. Application startup verifies compatibility but does not own
schema rollout.

**Rationale**: Startup migrations couple availability to DDL, complicate multi-instance rollout, and
make long backfills unsafe. Expand/backfill/validate/cutover/cleanup stages support old mobile and API
versions.

**Alternatives considered**:

- Continue automatic startup migrations: acceptable only while migrations are fast and additive, but
  not a durable model for backfills and `CREATE INDEX CONCURRENTLY`.

## Decision 11: Document operational defaults as targets, not current claims

**Decision**: Set target objectives of encrypted transport and storage, daily recoverable backups,
at least seven days of point-in-time recovery when supported, RPO ≤ 15 minutes, RTO ≤ 4 hours, and
quarterly restore exercises. Mark provider configuration as requiring verification.

**Rationale**: A complete design needs recovery expectations, but repository code cannot prove
hosted-provider configuration.

**Alternatives considered**:

- Omit recovery objectives: rejected because backup without tested restore expectations is
  incomplete.
- Claim provider settings are active: rejected because no environment evidence was supplied.

## Supabase changelog review

The 2026 breaking-change index was reviewed on 2026-07-27. Its listed platform, extension-pinning,
and self-hosted gateway changes do not alter this design's use of hosted PostgreSQL, `auth.users(id)`,
or the Go API data boundary.


## Reconciliation evidence — 2026-10-05

The preserved July design was reconciled against current `origin/main` migrations 001–019
and current sports, training-block, account-deletion, active-user, and pool configuration code.

- 016 enables RLS without direct-client policies and revokes Data API/PUBLIC/default privileges.
- 017 installs and validates cascade identity FKs on six legacy roots; this gap is schema-closed.
- 018 adds independent sport activities with idempotent atomic participation preservation.
- 019 adds four criteria-block tables, same-block composite references, checks, indexes, RLS,
  and role revocations. Qualification remains derived and stage movement remains explicit.
- Account deletion now removes all app roots, then avatars, then provider identity; partial failure
  is visible and retryable. Active-user checks and identity FKs close signature-only resurrection.
- Separate bounded query and session-advisory-lock pools replace the old default-only pool claim.
- Durable deletion jobs, expiry cleanup, missing domain checks/indexes, measured capacity, hosted
  role/exposure configuration, and restore exercises remain targets or external evidence gaps.
- Release-safety PR #18 is not current `main`; its range/replay/legacy adoption behavior is not
  assumed shipped by this reconciliation.

No production migration, data repair, API contract, or runtime code was changed. The earlier
Supabase changelog review above is historical evidence; no new provider-documentation review or
hosted inspection was performed in this documentation reconciliation.
