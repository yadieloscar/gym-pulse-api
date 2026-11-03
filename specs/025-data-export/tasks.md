# Tasks: Export My Data

**Spec**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md)

- [x] T001 Export model types with `[]` defaults (`internal/model/export.go`) — FR-001, FR-007
- [x] T002 Snapshot DAO reading every section (`internal/dao/export_dao.go`) — FR-003, FR-004
- [x] T003 Service ordering and CSV rendering (`internal/service/export_svc.go`) — FR-002
- [x] T004 Handlers and rate-limited routes (`internal/handler/export.go`, router) — FR-001, FR-002, FR-006
- [x] T005 Wire in `cmd/server/main.go`
- [x] T006 Unit, handler, router, and static DAO tests — all FRs
- [x] T007 Integration tests incl. runtime role (`internal/dao/export_integration_test.go`) — FR-003, FR-004, FR-005
- [x] T008 `docs/CONTRACTS.md` Data export section and rate-limit note; regenerate Swagger — FR-008

## Evidence (2026-11-03)

| Check | Result |
| --- | --- |
| `go test ./...` | passed |
| `golangci-lint run ./...` | 0 issues |
| `go test -race -tags=integration ./internal/dao -run Export` on a disposable migrated PostgreSQL 16 | passed (3 tests, runtime role not skipped) |
| `swag init -g cmd/server/main.go --output docs` | additions only |
| Wire check: worktree server on a disposable migrated DB, HS256 token | JSON and CSV 200 with attachment headers; formula cell escaped; no token 401; 7th request in a minute 429 |
| `./scripts/smoke-toggle.sh` | not run: it recreates the shared `gym-pulse-api-1` container other sessions use, from the main checkout image |
| Production deploy | not done (owner) |
