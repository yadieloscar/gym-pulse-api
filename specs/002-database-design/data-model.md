# Data Model: Complete Database Design

This artifact is the planning-level logical model. The authoritative physical catalog, current ERD,
constraints, indexes, lifecycle rules, and target roadmap are delivered in
[`docs/DATABASE_DESIGN.md`](../../docs/DATABASE_DESIGN.md).

## Identity and preferences

| Entity | Identity | Owner | Role |
|---|---|---|---|
| Auth user | External UUID | Identity provider | Stable authenticated subject and cascade root |
| User profile | Auth UUID | User | Display name, avatar URL, onboarding completion |
| User settings | Auth UUID | User | Weight unit, weekly goal, palette |
| Training profile | Auth UUID | User | Goal, availability, experience, equipment, timezone, revision |
| Body weight | UUID | User | One measurement per user/date |

## Catalog and legacy workout model

| Entity | Identity | Owner | Role |
|---|---|---|---|
| Exercise catalog item | UUID | System | Curated read-only exercise definition |
| Workout template | UUID | User | Reusable named workout |
| Template exercise | UUID | Via template | Ordered strength/cardio prescription |
| Day log | UUID | User | Legacy date-oriented workout/rest record |
| Exercise override | UUID | Via day log | Legacy per-exercise aggregate |
| Set log | UUID | Via exactly one parent | Per-set performed history shared by legacy and session domains |
| Weekly plan day | UUID | User | Legacy recurring weekday assignment |
| Plan override | UUID | User | Legacy dated assignment |

## Goal-based planning

| Entity | Identity | Owner | Role |
|---|---|---|---|
| Starter program | UUID + slug/version uniqueness | System | Immutable/versioned program catalog |
| Starter workout | UUID | Via starter program | Ordered catalog workout |
| Starter exercise | UUID | Via starter workout | Ordered catalog prescription |
| Program | UUID | User | User-owned copy/custom plan with optimistic revision |
| Program workout | UUID | Via program | Ordered user plan workout |
| Program exercise | UUID | Via program workout | Ordered user prescription and optional provenance |

## Scheduled execution and history

| Entity | Identity | Owner | Role |
|---|---|---|---|
| Scheduled workout | UUID | User | Dated immutable plan snapshot plus mutable outcome |
| Scheduled set | UUID | Via scheduled workout | Dated target snapshot |
| Workout session | UUID | User | One performed workout; multiple sessions may share a date |
| Performed set | Shared `set_logs` UUID | Via workout session | Required or extra performed-set snapshot |
| Day participation | UUID + user/date uniqueness | User | Finalized opportunity/participation fact |

## Sports and athlete-authored criteria blocks

| Entity | Identity | Owner | Role |
|---|---|---|---|
| Sport activity | UUID | User | Independent completed sport plus atomic participation |
| Criteria training block | UUID | User | Revisioned athlete-authored block; optional program association |
| Criteria stage | UUID + block/order uniqueness | Via block | Ordered load/target/qualification criteria |
| Criteria exposure | UUID | Via block and same-block stage | Performed facts and next-morning response |
| Criteria transition | UUID | Via block | Explicit stage/action audit history |

Qualification is derived from completed-as-planned and baseline response; no stage advances
automatically. Program deletion nulls the block association; account deletion cascades through
blocks and their composition. Migrations 016/018/019 harden Data API access, and 017 validates
legacy identity ownership FKs. Hosted rollout remains unverified.

## Workflow support

| Entity | Identity | Owner | Role |
|---|---|---|---|
| Idempotency record | UUID + user/scope/key uniqueness | User | Request fingerprint and replay response |
| Legacy adoption | User UUID | User | One-time mapping from legacy data to a program |

## Relationship rules

1. User-owned roots derive ownership from the authenticated UUID.
2. Composition children cascade with their parent: template exercises, program workouts/exercises,
   scheduled sets, and legacy detail rows.
3. Historical snapshots retain names/categories/modalities/targets when provenance FKs are set null.
4. A `set_logs` row belongs to exactly one of `day_logs` or `workout_sessions`.
5. A performed required set references one scheduled set; an extra set does not.
6. A user may have at most one active program.
7. A participation fact is unique per user/date and retains its finalized timezone/local date basis.
8. A matching idempotency key replays only when its request hash matches.

## State transitions

### Program

`inactive ↔ active`, with a database-enforced maximum of one active program per user. Every mutation
increments `revision`.

### Scheduled workout

`planned → in_progress → completed|incomplete|missed`.

Finalized outcomes retain `finalized_at`. Required-set completion determines the final outcome;
clients do not set it directly.

### Workout session

`draft → active → completed` or an eligible current draft may become `discarded`. Mutations use
expected revisions.

### Performed set

A required set is upserted by `(workout_session_id, scheduled_set_id)`. An extra set is created once
per `(workout_session_id, operation_key)`. Set changes advance both set and session revisions.

### Participation

Finalize is insert-once for a scheduled opportunity. Preserve may monotonically change
`participated` to true; it never changes true back to false.

## Validation boundary

PostgreSQL enforces row-local durable invariants: identities, ownership FKs, uniqueness, allowed
states, positive revisions, exclusive parents, and snapshot shape. The service layer enforces
cross-row ownership, date/timezone parsing, state-transition policy, and user-friendly error details.
