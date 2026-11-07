# Implementation Plan: Delete a Past Workout

**Branch**: `021-delete-past-workout` | **Spec**: [spec.md](spec.md) | **Feature ID**: `gympulse-delete-past-workout`

## Summary

Lift the past-date guard on `DELETE /api/v1/logs/{date}` and add `DELETE
/api/v1/scheduled-workouts/{id}`. No migration: existing foreign keys cascade sets from sessions
and scheduled workouts; the two non-FK dependants (`health_exports`, `day_participation`) are
handled in the delete statements.

## Design Assessment

- **Contract**: additive route plus a relaxed rule on an existing route. Old clients never sent a
  past log delete, so no client depends on the removed `409`. `docs/CONTRACTS.md`, Swagger, and
  smoke step 13i change together.
- **Auth and ownership**: every statement filters by the authenticated `user_id`; the workout is
  locked `FOR UPDATE` by id and owner, so a foreign id is indistinguishable from a missing one (404).
- **Placement**: eligibility (finalized or past) lives in the service beside the identical
  `PatchWorkout` historical rule; the DAO owns the transaction and the active-session check.
- **Transaction**: the schedule advisory lock (`schedule:` namespace) serializes with
  regenerate, recover, and materialize. Order: lock workout → reject active session → delete
  sessions (cascades set logs) → delete export record → delete workout (cascades required sets;
  sport activity link is `SET NULL`) → recompute participation → commit. Eligibility only grows
  over time (past stays past, finalized stays finalized), so checking it before the transaction is
  race-safe.
- **Day logs**: one statement with data-modifying CTEs deletes the log and its export record
  atomically; overrides and set logs cascade. Day logs do not feed `day_participation`.
- **Participation**: recomputed rather than decremented because the row is an OR of several
  sources (scheduled outcome, completed session, sport activity).
- **Idempotency**: deletes are naturally idempotent; a repeat answers 404 and the app treats 404 as
  already deleted. No operation key is required, matching other `DELETE` routes.
- **Rollout**: deploy the API before the app (sibling app spec 022). An old app never calls the
  new route; the new app against an old API gets 405/409 and shows the error state.

## Verification

`gofmt`, `go vet`, `golangci-lint run`, `go test ./...`, DAO integration suite with
`-tags=integration` against a disposable migrated database, `swag init` drift check, and
`scripts/smoke.sh` against an isolated stack built from this branch.
