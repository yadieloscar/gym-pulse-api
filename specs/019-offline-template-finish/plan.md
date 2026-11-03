# Implementation Plan: Replay-safe off-plan workout logs

**Spec**: [spec.md](spec.md) · **Feature ID**: `gympulse-offline-template-finish`

## Design

- `model.CreateDayLogRequest` gains `operation_key` (`omitempty`).
- Handler `LogHandler.Create`: if the header or body carries a key, `matchIdempotencyKey` requires both
  and equal (422 otherwise). No key → legacy path.
- Service `logService.Create`: validation and template ownership run first (unchanged). Unkeyed →
  `LogDAO.Create`. Keyed → `hashPayload(req)` (same SHA-256 helper as training) and
  `LogDAO.CreateOnce`.
- DAO `CreateOnce`: one transaction: `pg_advisory_xact_lock("day-log:" + user)`, `findIdempotency`
  (scope `logs/create`), replay stored body or insert the log + set logs + overrides and
  `insertIdempotency` (`resource_type = day_log`, status 201, no expiry). `Create` and `CreateOnce`
  share `insertDayLog`.

## Trade-offs

- Replay is checked after validation, so a replay whose template was deleted in between returns 404
  instead of the stored response. The app treats 404/422 on replay as a dead letter it shows to the
  athlete; this window (finish online-lost, then delete template, then replay) is accepted.
- No record expiry: day-log records are small and per finish; account deletion cascades them.

## Verification

- `go test ./...`, golangci-lint, swag drift.
- `GYMPULSE_TEST_DATABASE_URL=… go test -race -tags=integration ./internal/dao` (replay, conflict,
  concurrency).
- `scripts/smoke.sh` step 12b.
