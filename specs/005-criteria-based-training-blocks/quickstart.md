# Quickstart: Criteria-Based Training Blocks API

## Apply and test

```bash
go test ./internal/model ./internal/service ./internal/dao ./internal/handler ./internal/router
go test ./...
go test -race ./...
./scripts/smoke-toggle.sh
```

Apply migration 019 through the existing migration workflow before exercising the routes.

## Manual contract sequence

1. Create a block with two stages using `POST /api/v1/training-blocks`, a UUID `operation_key`, and an equal `Idempotency-Key` header.
2. List active summaries with `GET /api/v1/training-blocks?status=active&limit=20&offset=0`.
3. Record a current-stage exposure using the returned block revision.
4. Record the next-morning response using the new revision; confirm `qualifies=true` only for completed-as-planned plus baseline.
5. Explicitly advance when `current_stage_progress.criteria_complete=true`.
6. Retry each mutation verbatim and confirm the same aggregate and no additional child record.
7. Retry with a changed payload or stale revision and confirm a stable conflict with no partial write.

## Cross-domain regression

Capture program, schedule, workout, sport activity, participation, and statistics responses before the sequence and confirm they remain unchanged afterward.
