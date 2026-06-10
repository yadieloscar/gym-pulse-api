# CLAUDE.md (Go API)

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run unit tests
go test -v ./...

# Run unit tests with coverage profile
go test -coverprofile=coverage.out ./...

# Display coverage function breakdown
go tool cover -func=coverage.out
```

## Go Development Conventions

Follow the Google Go Style Guide at all times:
- **Receiver Names:** Use 1-2 letter lowercase receiver names (e.g. `s` for Service, `r` for Repository/DAO, `h` for Handler). Avoid generic names like `this` or `self`.
- **Context Handling:** Pass `context.Context` as the first parameter to functions that make network, DB, or concurrent calls. Name the parameter `ctx`.
- **Error Formatting:** Error strings must be lowercase and have no trailing punctuation.
- **Error Wrapping:** Wrap external database or API errors with `fmt.Errorf("doing action: %w", err)` to preserve context.
- **Imports Grouping:** Group imports into standard library, third-party libraries, and internal project packages, separated by empty lines.
- **Keyed Struct Fields:** Always use keyed struct fields when initializing structs (e.g. `model.DayLog{Date: date}`).
- **JSON Slices:** Use `nil` slices for JSON elements where `null` is acceptable; use empty slices (`[]Type{}`) for endpoints that must return an empty list `[]`.

## Testing Conventions

- **TDD / Unit Testing:** Write unit tests alongside implementation. Mock internal DAOs manually using structs and function variables (defined in `mocks_test.go`).
- **Coverage Target:** Core business logic packages (`internal/service/` and `internal/middleware/`) must target and maintain >90% test coverage.

## Contracts & delivery workflow

- **`docs/CONTRACTS.md` is the client-facing source of truth** for request/response shapes and validator rules. If you change a `validate:` tag, `json:` field, or response shape, update CONTRACTS.md in the same commit — CI rejects PRs that don't.
- Run `./scripts/smoke-toggle.sh` after any handler/validator/middleware change (4 wire-level contract assertions, ~10s).
- Multi-file or cross-repo features: use the task brief at `../gym-pulse-app/docs/TASK_TEMPLATE.md` and the `gym-pulse-ship` skill.
