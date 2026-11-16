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

### Ship one reviewed image

A commit merged to `main` that passes CI is built into one container image. That exact image (same
digest) reaches staging first and production only after an explicit approval. Pending migrations run
as a separate release step before the new version takes traffic, over a session-mode connection, and
a staging or production server refuses to start against a schema that is behind or dirty. Operators
can tell which build is serving.

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
10. A successful `main` CI run for the commit still at the tip of `main` produces one pushed image;
    staging and production deploy that image's digest, and production waits for an approval on the
    `production` environment. An older commit whose CI finishes late never deploys.
    Deploys are serialized per environment, so a pending production approval never blocks staging.
11. Staging and production servers never run migrations on boot. The release step `./migrate`
    applies pending migrations over `DATABASE_MIGRATE_URL` (the schema owner, direct or session
    mode) before deployment; request connections may use the least-privilege runtime role
    (spec 011).
12. A staging or production server exits at startup when the recorded schema version is lower than
    the newest migration in its image, or is dirty at or below that version. A newer schema
    (rollback image) is accepted. A schema left dirty by a newer release's failed migration is
    served with a repair warning, so the previous build keeps restarting.
13. Development and test servers keep migrating on boot so local compose and CI smoke are unchanged.
14. `GET /health` reports the build version, and a deploy is complete only when the target
    environment's `/health` reports the deployed commit and `/ready` succeeds.

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
- Railway hosts staging and production from a GHCR image built by GitHub Actions. Environment tags
  (`:staging`, `:production`) are moved to the accepted digest, then Railway redeploys. Railway's
  pre-deploy command is the migration release step; config lives in `.railway/railway.ts`.
- Deployments require Railway and GitHub environment credentials configured by the owner; the
  pipeline never provisions accounts or secrets.

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
