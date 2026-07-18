# Validation Log: goal-based-training API

## Subagent consensus

### 3/3 (very high confidence)

- **Applied:** Freeze exact wire routes/shapes before app implementation.
- **Applied:** Replace date-only identity with UUID-addressed scheduled workouts
  and performed sessions so off-plan activity can coexist on a date.
- **Applied:** Add planned/in-progress/finalized lifecycle and lazy timezone-aware
  end-of-day finalization.
- **Applied:** Enumerate canonical profile values and deterministic starter
  coverage/cadence expectations.
- **Applied:** Make all past workout history non-deletable.
- **Applied:** Include onboarding/existing-user adoption semantics.
- **Applied:** Replace idempotency-only replay with revisions, conflicts, and
  durable mutation ordering requirements.

### 2/3 (high confidence)

- **Applied:** Snapshot exercise identity independently of mutable templates.
- **Applied:** Define the participation-opportunity streak algorithm.
- **Applied:** Require populated legacy migration and account-deletion coverage.

### 1/3 (discarded)

- Reminder trigger details are app-only and did not reach consensus for this API
  spec.

## Expert findings

No experts are registered in the project catalog.

## Summary

Consensus issues were applied directly; wording-only findings were omitted.
