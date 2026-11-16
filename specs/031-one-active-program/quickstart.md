# Quickstart: Validate One Active Program on Fixed Weekdays

**Spec**: [spec.md](spec.md) · **Contract draft**: [contracts/](contracts/one-active-program.openapi.yaml)

## Prerequisites

- Go 1.26.11+ (the repository's `go.mod`).
- A disposable PostgreSQL with migrations applied (parent workspace `docker-compose.yml`), and an
  athlete seeded in `auth.users` for smoke runs.

## Automated gates

```bash
go test ./...
GYMPULSE_TEST_DATABASE_URL='postgres://gympulse:gympulse@127.0.0.1:15432/gympulse?sslmode=disable' \
GYMPULSE_TEST_RUNTIME_DATABASE_URL='postgres://gym_pulse_api:gym_pulse_api@127.0.0.1:15432/gympulse?sslmode=disable' \
  go test -race -tags=integration ./internal/dao ./internal/dbmigrate -count=1 -timeout=5m
./scripts/validate-database-design.sh
./scripts/smoke-toggle.sh
govulncheck ./...
```

## Scenarios (integration or smoke)

"P" is an active custom program. "Q" is a second owned program.

| # | Steps | Expected |
| --- | --- | --- |
| Q1 | Create Q with Push `[1]`, Pull `[3]`, Legs `[5]`, a cycling unit `[4]`, volleyball `[6]`. Start it. `GET /schedule` for four weeks. | Each unit only on its weekdays, every week. |
| Q2 | With P active, `POST /programs` for Q with `active: false`. | 201; P still active; schedule rows unchanged. Without `active`, Q is active as today. |
| Q3 | Preview then apply a start of Q with `from` = tomorrow and no `proposed_profile`. | Q is the only active program; P's planned sessions from tomorrow on are gone, including those past `to`; the profile revision is unchanged. |
| Q4 | Repeat Q3 with one in-progress, one completed, and one missed session of P present. | Those three sessions are unchanged. |
| Q5 | With P active on `[1]`, `[3]`, `[5]`, apply a transition with only `proposed_profile` whose `available_days` are `[2, 4, 6]`. | P still active; no session changed; `off_availability` lists all three units; `recommended_starter_program` is returned and not applied. |
| Q6 | Start Q, move the clock 40 days, `GET /schedule` for the next 14 days twice. | Sessions present on their weekdays; the second read inserts nothing. A session skipped, moved, or deleted before the clock moved is not re-created. |
| Q7 | Move a planned session to the next day; replay the request; move a completed session; move to yesterday. | Date and revision change once; replay returns the same body; 409 `CONFLICT`; 422 with `details.field = "date"`. |
| Q8 | On active Q: move one Legs session, then `PUT` with Push on `[2]`, one more set on Pull, a new unit on `[7]`, and volleyball removed, sending unit IDs. | Future Push on Tuesdays; future Pull snapshots carry the new set; the new unit appears; volleyball sessions gone; the moved Legs session keeps its date; today's sessions unchanged. |
| Q9 | `DELETE /programs/{id}` for inactive P that has finished sessions; then for active Q. | 204, finished sessions keep their names; 409 `CONFLICT`. |
| Q10 | Use another athlete's program, unit, and session IDs in every new field and route. | Shared 404 for programs and sessions; a foreign unit ID becomes a new unit in the caller's own program; the owner's data is unchanged. |
| Q11 | Send pre-feature bodies: create, replace without unit IDs then regenerate, materialize twice, preview and apply with `proposed_profile` and `to`. | All succeed; no duplicate sessions; create then "transition with `program_id`" gives the same first week as before this feature. |
