# Quickstart: Sport Activity Tracking API

## Implementation order

1. Apply migration 018 and add model/DAO tests.
2. Add service validation and atomic creation.
3. Add handler, routes, wiring, and contract documentation.
4. Include sport dates in weekly aggregation and account cleanup.
5. Run formatting, focused tests, `go test ./...`, and `./scripts/smoke-toggle.sh`.

## Manual acceptance

Create a 60-minute basketball activity without `date`; confirm athlete-local today. Repeat the request
and confirm the same ID. Reuse the key with a different duration and confirm 409. List the range and
verify newest-first order. Weekly progress changes at most once for the date. A second user gets the same
404 for the activity ID as a nonexistent UUID.

## Compatibility and rollback

The change is additive and needs no backfill. Deploy this API before the app. Roll back the app first,
then drop `sport_activities`; retained participation rows remain valid consistency evidence.
