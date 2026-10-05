# Quickstart: Validate Release-Safe Training Workflows

## Prerequisites

- Go 1.26.5 and Docker with Compose.
- A clean local database started through the repository smoke workflow.
- One authenticated fixture user with a training profile, two owned legacy templates, and non-rest weekly assignments.

## Automated gates

```bash
go test ./internal/model ./internal/dao ./internal/service ./internal/handler ./internal/router
go test -race ./...
golangci-lint run
./scripts/smoke-toggle.sh
```

Against a disposable migrated PostgreSQL database with DDL permissions, run the
mandatory CI acceptance suite (the smoke job supplies its database URL):

```bash
GYMPULSE_TEST_DATABASE_URL='postgres://gympulse:gympulse@127.0.0.1:15432/gympulse?sslmode=disable' \
  go test -race -tags=integration ./internal/dao -run '^TestRelease' -count=1 -timeout=5m
golangci-lint run --build-tags integration
```

The integration suite fails when its database URL is missing; it never skips
rollback or concurrency scenarios. Each case owns an isolated UUID user and
removes its fault triggers and fixtures. Use a disposable database because fault
injection requires temporary trigger/function creation.

Regenerate Swagger with the pinned CI command and confirm no uncommitted drift:

```bash
swag init -g cmd/server/main.go --output docs
git diff --exit-code docs/
```

## Acceptance 1: Legacy adoption and replay

1. Save a training profile with a known timezone.
2. Create two legacy templates and assign them to distinct non-rest weekdays.
3. Call `POST /api/v1/programs/adopt-legacy` with matching body/header operation keys and `expected_revision: 0`.
4. Confirm one active program, one program workout per assigned weekday, and only next-week dated snapshots.
5. Repeat the exact request 100 times. Confirm every response identifies the same resources and no row count changes after the first call.
6. Reuse the operation key with changed JSON. Confirm HTTP 409 and no row changes.
7. Confirm all legacy tables are byte-for-byte equivalent at the row level before and after.

## Acceptance 2: Atomic retry outcomes

For profile, program, scheduled-workout snapshot, scheduled-set target, required set, extra set, scheduled completion, session create, and session patch mutations:

1. Submit a successful mutation and capture status/body.
2. Repeat the exact operation after the resource revision has advanced. Confirm the original status/body is returned.
3. Reuse the same operation key with one changed field. Confirm HTTP 409.
4. Run two first attempts concurrently. Confirm one state transition and identical responses.
5. Inject a database error immediately before participation or idempotency insertion. Confirm the workout/session change also rolls back.

## Acceptance 3: Range ceiling

For schedule, materialization, regeneration, plan-transition preview/apply, sessions, and participation:

1. Submit `from=2026-01-01` and `to=2027-01-01` (366 inclusive days). Confirm normal processing.
2. Submit `from=2026-01-01` and `to=2027-01-02` (367 inclusive days). Confirm HTTP 422, field `range`, and no DAO invocation.
3. Repeat malformed and reversed ranges to confirm stable field-specific errors.

## Cross-repository handoff

After this API branch is deployed to the target environment, run the sibling app specification's live contract checks against it. Do not enable dependent app behavior before the adoption route and mutation semantics are available.

## 2026-08-12 verification record

- `go test ./... -count=1`: passed.
- `go test -race ./... -count=1`: passed.
- `golangci-lint run`: passed with zero issues.
- CI package coverage thresholds: service 90.1%, middleware 90.4%, handler
  91.9%, router 100%, config 100%.
- `govulncheck ./...` with the CI/production Go 1.26.5 toolchain: zero
  reachable vulnerabilities. The host's older Go 1.26.2 toolchain reported
  standard-library findings fixed by the pinned 1.26.5 toolchain.
- Swagger generation and `git diff --check`: passed.
- `bash -n scripts/smoke.sh scripts/smoke-toggle.sh`: passed.
- Local container acceptance was unavailable because the Docker daemon was not
  running. GitHub Actions run `31662834536` passed the expanded PostgreSQL smoke,
  unit/race/lint/security CI, and container scan on pull request 18. The smoke
  covers concurrent adoption, exact replay after revision advancement,
  changed-payload conflicts, rollback of lazy session creation, atomic
  completion/participation, and the 366/367-day wire boundary.

## 2026-10-05 review follow-up verification

- `go test -race -tags=integration ./internal/dao -run '^TestRelease' -count=1`:
  passed against disposable PostgreSQL 16 with migrations through 019.
- `golangci-lint run --build-tags integration`: passed with zero issues.
- Four injected completion failures (scheduled/session × participation/replay
  insertion) preserved every persisted domain row.
- Regeneration failures at replacement-set insertion and replay insertion
  restored deleted originals and their required sets.
- PostgreSQL-observed duplicate requests for clone, materialize, recovery,
  finalization, and session completion produced one mutation and one identical
  replay. Replays stayed exact after revision advancement; mismatches conflicted
  without changes.
- Both session-start/regeneration lock orders preserved the winning operation:
  an active session prevented regeneration; completed regeneration made the
  superseded workout unavailable without creating an orphan session.
- Fresh adoption keys returned current renamed/deactivated programs and
  regenerated schedule identities, while 100 original-key replays remained
  identical and changed no persisted state.
- The CI smoke job now requires this race-enabled PostgreSQL suite. This local
  record does not claim a completed new remote CI run or deployed acceptance.
