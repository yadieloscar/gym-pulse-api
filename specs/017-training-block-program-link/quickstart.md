# Quickstart: Validate Training Blocks Linked to Real Training

**Spec**: [spec.md](spec.md) · **Contract draft**: [contracts/](contracts/training-block-program-link.openapi.yaml)

## Prerequisites

- Go 1.26.11+ (the repository's `go.mod`).
- A disposable PostgreSQL with migrations applied (parent workspace `docker-compose.yml`), and an
  athlete seeded in `auth.users` for smoke runs.

## Automated gates

```bash
go test ./...
GYMPULSE_TEST_DATABASE_URL='postgres://gympulse:gympulse@127.0.0.1:15432/gympulse?sslmode=disable' \
  go test -race -tags=integration ./internal/dao -count=1 -timeout=5m
./scripts/smoke-toggle.sh
govulncheck ./...
```

## Scenarios (integration or smoke)

| # | Steps | Expected |
| --- | --- | --- |
| Q1 | Create a block whose stage 1 names owned program P. Finalize an `exercises` workout of P as `completed`. `GET` the block. | One entry in `exposure_suggestions` with the prefill from data-model.md. |
| Q2 | `POST /exposures` with that `scheduled_workout_id`. Repeat with a new key. | First: exposure carries the reference, suggestion gone, revision +1. Second: 409 `EXPOSURE_SOURCE_ALREADY_RECORDED`. |
| Q3 | Finalize a workout, then advance the block; `GET` it. | The workout is not suggested to the new stage (SC-006). |
| Q4 | Set stage program mid-stage; workouts finalized before the change exist. | They are not suggested; a workout finalized after is. |
| Q5 | Finalize a sport unit and an off-plan session of P. | Neither is suggested; referencing either returns 422. |
| Q6 | Dismiss a suggestion; replay the same request. | Gone from aggregate and due read; replay returns the identical body. |
| Q7 | Record an exposure dated yesterday and one today; `GET /training-blocks/due`. | Only yesterday's is in `due_checks`. |
| Q8 | `DELETE /scheduled-workouts/{id}` for a referenced workout. | Exposure remains with no `scheduled_workout_id`; its dismissal row is gone. |
| Q9 | Use another athlete's workout, stage, or program IDs in every new field and route. | Shared 404; nothing changes. |
| Q10 | Change the program of a stage before the current one, or of an archived block. | 409 `CONFLICT` with `resource`. |
| Q11 | Call the existing routes with pre-feature request bodies. | Responses unchanged apart from the added fields (SC-004). |
