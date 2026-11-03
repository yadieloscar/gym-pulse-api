# Tasks: Replay-safe off-plan workout logs

- [x] T001 Add optional `operation_key` to `CreateDayLogRequest` (`internal/model/log.go`) — FR-001
- [x] T002 Handler key matching + tests (`internal/handler/logs.go`, `logs_test.go`) — FR-001, AS-5, AS-6
- [x] T003 Service keyed path with stable hash + tests (`internal/service/log_svc.go`, `log_svc_test.go`) — FR-002, FR-004
- [x] T004 DAO `CreateOnce` with advisory lock and idempotency record (`internal/dao/log_dao.go`) — FR-002, FR-003
- [x] T005 DAO integration tests: replay, conflict, concurrency (`internal/dao/log_once_integration_test.go`) — SC-001, AS-2..4
- [x] T006 Contract, Swagger, smoke step 12b — FR-005

## Evidence

| Check | Result |
| --- | --- |
| `go test ./...` | pass |
| golangci-lint v2.6.2 (CI pins v2.11.4) | 0 issues |
| DAO integration (`-race -tags=integration ./internal/dao`, disposable CI-style Postgres 16) | pass, incl. replay/conflict/concurrency |
| `scripts/smoke.sh` against a CI-style stack built from this branch | 29 passed, 0 failed |
