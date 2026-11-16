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

## Amendment A1: private pre-launch access (2026-11-16)

**Why:** the owner will host the API on Railway before anyone else uses the app and asked how to
make sure nobody else gets in. A public URL cannot be hidden (it ships inside the app), so access is
closed at the identity layer instead.

- **FR-A1-001:** Optional `ALLOWED_USER_IDS` (comma-separated Supabase user UUIDs). When set, every
  `/api/v1/*` route, including `DELETE /api/v1/account`, returns
  `403 ACCOUNT_NOT_ALLOWED` for a verified token whose subject is not listed. The check runs right
  after token verification, before rate limiting and the active-user lookup.
- **FR-A1-002:** Unset or blank means today's behaviour: every verified user is allowed.
- **FR-A1-003:** A malformed or nil UUID in the list stops startup, so a typo cannot silently open
  the deployment or lock the owner out.
- **FR-A1-004:** `/health` and `/ready` are unaffected.
- **FR-A1-005:** The release runbook pairs the list with turning off Supabase sign-up until launch.

Acceptance: middleware table test (empty list, listed, unlisted, missing and malformed subject),
router test (owner 200, stranger 403 on a data route and on account deletion without reaching the
active-user lookup, `/health` 200), config tests (unset, trimmed list, malformed and nil ids).
Contract 1.10.0, additive. The app does not yet show a dedicated message for this 403; it is only
set on the owner's own deployments, so no app change ships with it.
