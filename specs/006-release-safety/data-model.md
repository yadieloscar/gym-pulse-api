# Data Model: Release-Safe Training Workflows

No migration is required. This feature composes existing entities under stronger transaction rules.

## Legacy adoption

Existing durable fields:

- `user_id`: authenticated owner and one-per-user primary key.
- `program_id`: adopted owned program.
- `operation_key`: first successful adoption identity.
- `adopted_at`: commit timestamp.

Relationships:

- Belongs to one authoritative auth user.
- References one owned program; scheduled workouts reference that program and its program workouts.
- Has a corresponding `idempotency_records` document containing the complete adoption response.

State transitions:

- **Absent → adopted**: one transaction creates program/workouts/schedule, adoption marker, and replay record.
- **Adopted → adopted**: resources are returned without copying or mutating legacy data.
- There is no public transition back to absent.

## Idempotency record

Existing durable fields:

- `(user_id, scope, operation_key)`: unique replay identity.
- `request_hash`: canonical JSON payload fingerprint.
- `response_status` and `response_body`: exact committed result.
- Optional resource identity/revision metadata.
- Optional expiry; goal-training workflow records in this scope do not require expiry.

Rules:

- Exact hash returns the stored status/body before revision checks.
- Different hash returns an idempotency conflict.
- The record commits in the same transaction as the mutation.

## Program import mapping

| Legacy source | Goal-training target | Mapping rule |
|---|---|---|
| Weekly non-rest assignment | Program workout | One per ISO weekday, ordered Monday through Sunday |
| Template name | Workout name | Preserved |
| Template type | Exercise category fallback | Preserved when no narrower legacy category exists |
| Exercise order | Exercise order | Converted from zero-based legacy order to positive order |
| Sets/repetitions/weight/rest/notes | Strength target | Preserved; target sets defaults to one only when legacy data omits it |
| Duration minutes | Cardio duration | Converted to seconds; one scheduled set |
| Catalog ID | Catalog provenance | Preserved as nullable ownership-safe provenance |

Program `primary_goal` comes from the athlete's saved training profile. Program name is a stable import label. Legacy rows are never referenced as mutable target state.

## Participation consistency

- Scheduled finalization creates or finalizes the same local-date participation row in the same transaction.
- Session completion preserves `participated=true` in the same transaction.
- Existing date participation combines flags using logical OR and never loses prior participation.
- Failed transactions expose neither side of the update.
- Later required-set or target corrections may re-derive the finalized
  scheduled-workout result, but never rewrite the already finalized
  participation row.

## Validation

- Every mutation requires an operation identity. A supplied body key must match
  its header; existing header-only profile/custom-program clients are
  normalized during the API-first migration window.
- Revision is zero only for singleton/profile or create semantics explicitly documented; existing resources require revision at least one.
- Inclusive date ranges contain at most 366 calendar dates.
- All identifiers loaded for mutation are filtered by authenticated `user_id`.
