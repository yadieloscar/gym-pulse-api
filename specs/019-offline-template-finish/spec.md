# Feature Specification: Replay-safe off-plan workout logs

**Feature Branch**: `spec/019-offline-template-finish`

**Feature ID**: `gympulse-offline-template-finish`

**Sibling Spec**: [App specification](../../../gym-pulse-app/specs/023-offline-template-finish/spec.md)

**Created**: 2026-11-03

**Status**: Approved (owner direction: "finishing an off-plan template workout works offline, matching the product promise")

**Input**: `docs/PRODUCT.md` promises "starting, logging, and finishing a workout never need a connection".
Finishing a planned workout already queues offline. Finishing an off-plan (saved-workout) session is
a direct `POST /api/v1/logs`, which the app must now be able to deliver from an outbox, possibly more
than once.

## Problem

Migration 012 removed `UNIQUE (user_id, date)` from `day_logs`, so `POST /api/v1/logs` is not
idempotent. A retry after a lost response, or an outbox replay after the request actually landed,
creates a second log for the same workout. That double-counts the day, the week, and the streak.

## Clarifications

### Session 2026-11-03

- Q: Required or optional key? → A: Optional. Released apps never send it and must keep working
  unchanged. New apps always send it for template finishes.
- Q: Does this restore one log per date? → A: No. A date is a filter, not identity (migration 012).
  Only the same operation is collapsed; two different finishes on one date still create two logs.
- Q: Storage? → A: The existing `idempotency_records` table, scope `logs/create`. No migration.

## User Scenarios & Testing

### User Story 1 - An offline finish counts once (Priority: P1)

An athlete finishes a saved workout with no signal. The phone keeps it and sends it later, maybe
twice (the first response was lost, or the app was killed mid-send). The service records one log.

**Acceptance Scenarios**:

1. **Given** no prior request with key K, **When** the app posts a log with key K, **Then** one log is
   created and `201` returns it.
2. **Given** a log created with key K and payload P, **When** the app posts K and P again, **Then** the
   response is `201` with the original `id` and `logged_at`, and no second log exists.
3. **Given** K was used with P, **When** the app posts K with a different payload, **Then** the
   response is `409 IDEMPOTENCY_CONFLICT` and nothing is written.
4. **Given** several concurrent deliveries of K and P, **Then** exactly one log exists and each
   delivery returns it.
5. **Given** a key in only the header, only the body, or different values, **Then** `422
   VALIDATION_ERROR` with `field = Idempotency-Key`.
6. **Given** no key in either location, **Then** behavior is unchanged from today.
7. **Given** K used by user A, **When** user B posts K, **Then** B's request is independent (keys are
   scoped per user).

## Requirements

- **FR-001**: `POST /api/v1/logs` MUST accept an optional `operation_key` body field equal to the
  `Idempotency-Key` header.
- **FR-002**: A keyed create MUST store its `201` response atomically with the log, and return it
  for an identical replay without writing.
- **FR-003**: Keyed creates for one user MUST be serialized so concurrent deliveries cannot both
  insert.
- **FR-004**: An unkeyed create MUST keep current behavior.
- **FR-005**: `docs/CONTRACTS.md`, Swagger, and smoke MUST describe and assert the behavior.

## Success Criteria

- **SC-001**: Replaying any keyed finish N times yields one `day_logs` row (DAO integration test).
- **SC-002**: Existing smoke and unit suites pass unchanged.

## Rollout

Additive and backward compatible. Deploy before the app release that sends the key (app spec 023).
An app that sends the key to an older API gets today's non-idempotent create, so the app must not
ship ahead of this deploy.
