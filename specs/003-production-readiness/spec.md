# Production Readiness

## Outcome

GymPulse can be deployed for real users without exposing application tables through Supabase,
leaving account-owned storage behind, accepting unsafe release configuration, or relying on
unverified runtime dependencies. The API remains backward-compatible with the current mobile app
while adding the operational evidence required for a controlled release.

This specification is coordinated with the app repository's
`specs/002-production-readiness/spec.md`.

## User journeys

### Use private application data

Signed-out and signed-in Supabase clients cannot read or mutate GymPulse application tables. All
application data remains accessible only through the authenticated Go API, which continues to scope
resources to the JWT subject.

### Delete an account completely

Deleting an account removes its avatar object, application data, and Supabase auth user. A partial
external failure is reported as retryable rather than falsely returning success. Repeating the
request is safe.

### Reach a healthy service

Production exposes liveness and database-backed readiness signals, uses bounded database resources,
accepts the app's documented methods and idempotency header, and runs on supported patched images.

### Trust release evidence

CI uses real statement coverage, runs on protected-branch pushes, and reports reachable Go and
container vulnerabilities. Contract documentation matches executable behavior.

## Acceptance scenarios

1. `anon`, `authenticated`, and `service_role` Data API roles have no privileges on application
   tables or sequences, and every application table has RLS enabled.
2. Default privileges do not automatically expose future public tables, functions, or sequences.
3. The direct Go database role still completes the full authenticated smoke flow.
4. Account deletion removes `<user_uuid>/avatar`, all application rows, and the auth user.
5. Avatar deletion, database deletion, auth deletion, missing objects, retries, and partial failures
   have explicit tests and stable outcomes.
6. CORS preflight accepts every documented app method plus `Idempotency-Key`.
7. `/health` proves process liveness and `/ready` fails when PostgreSQL is unavailable.
8. The runtime image is supported and vulnerability checks report no unaccepted reachable high or
   critical finding.
9. Service and middleware statement coverage are at least 90 percent under the same calculation CI
   enforces.

## Non-goals

- Exposing application data directly through Supabase.
- Redesigning workout, planning, or statistics contracts.
- Breaking currently installed mobile clients.
- Performing production deployment without explicit environment access and release approval.

## Product and architecture decisions

- Supabase is authentication and avatar storage only; the Data API is not an application-data path.
- RLS plus privilege revocation provides defense in depth. No client RLS policies are created.
- Account deletion remains `DELETE /api/v1/account`; `204` means every required deletion step
  completed. Retryable external failures use a stable non-2xx error.
- Deletion operations are idempotent; missing avatar or auth resources count as the desired state.
- API additions are backward-compatible and deploy before the linked app release.
- Database changes are forward migrations. Rollback is roll-forward if reverting would re-expose
  tables.
