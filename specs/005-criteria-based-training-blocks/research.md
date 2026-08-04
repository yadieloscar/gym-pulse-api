# Research: Criteria-Based Training Blocks API

## Aggregate and ownership boundary

**Decision**: Treat a block as one athlete-owned aggregate. Stages are immutable after creation; exposures and transitions are append-only; the next-morning response is a one-time update on its exposure.

**Rationale**: One ownership check and one locked block row can protect every nested mutation. Immutable definitions keep historical exposures interpretable.

**Alternatives considered**: Editing stages in place was rejected because it changes the meaning of earlier evidence. Independent stage ownership columns were rejected because they duplicate and can diverge from block ownership.

## Qualification and stage movement

**Decision**: Derive `qualifies` and current-stage progress on the server. An exposure qualifies only when `session_outcome=completed_as_planned` and `next_morning_response=baseline`. Meeting criteria never moves a stage; a separate transition mutation does.

**Rationale**: This yields one authoritative rule and preserves the athlete's explicit decision without implying a clinical clearance.

**Alternatives considered**: Client calculation risks drift. Database triggers obscure product behavior. Automatic progression conflicts with the approved interaction and safety boundary.

## Mutation consistency

**Decision**: Every mutation accepts `expected_revision` (except create) and `operation_key`, requires a matching `Idempotency-Key` header, locks the owned block, validates the revision, changes state, increments the revision, stores the complete response in `idempotency_records`, and commits once.

**Rationale**: This follows existing workflow transaction conventions and makes retries and competing devices predictable.

**Alternatives considered**: Per-child revisions complicate aggregate reconciliation. Application-only de-duplication cannot guarantee durability across process restarts.

## Listing and detail

**Decision**: List summaries through `status`, `limit`, and `offset`, ordered by `updated_at DESC, id DESC`; return `next_offset`. Full nested history is available only on detail and mutation responses.

**Rationale**: This bounds routine Plan reads while giving the detail workflow one authoritative aggregate.

**Alternatives considered**: Returning every nested record in list results is unbounded. Cursor pagination adds a contract shape not otherwise needed for this first release.

## Program relationship and cross-domain behavior

**Decision**: Validate an optional `program_id` through an ownership-scoped lookup and store it with `ON DELETE SET NULL`. No schedule, workout, sport activity, participation, or statistics code is called.

**Rationale**: The program is context only. Deleting it must not delete personal block history.

## Migration and dependencies

**Decision**: Add migration 019 with four tables and indexes; rollback drops only these tables in dependency order. Add no Go dependency or concurrent worker.

**Rationale**: The existing stack already provides validation, UUIDs, transactions, authentication, and idempotency.
