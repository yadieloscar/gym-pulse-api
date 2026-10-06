---
name: google-go-style
description: Apply Google's Go coding standards when writing, refactoring, or reviewing Go source and tests. Use for naming, formatting, documentation, errors, contexts, interfaces, and concurrency; complement the repository's engineering workflow.
---

# Google Go Style

Apply this skill alongside `gym-pulse-api-engineering` in GymPulse; that workflow owns architecture,
contracts, and verification.

## Authoritative guidance

Reviewed against Google's official documentation on 2026-10-05:

- [Overview](https://google.github.io/styleguide/go/): document roles and scope.
- [Style Guide](https://google.github.io/styleguide/go/guide): canonical core rules.
- [Style Decisions](https://google.github.io/styleguide/go/decisions): detailed normative guidance,
  subordinate to the core guide.
- [Best Practices](https://google.github.io/styleguide/go/best-practices): advisory patterns.

This is a concise application guide, not a complete copy. Consult the relevant official section
when a decision needs detail or current language guidance. Do not require Google-internal tooling,
libraries, or review processes. Honor current user direction and repository governance; identify
material conflicts rather than silently inventing exceptions.

## Core standards

Prioritize reader clarity, then simplicity, concision, maintainability, and consistency. Explain
non-obvious reasons in comments; avoid commentary that repeats the code. Prefer language constructs
and standard-library tools before adding machinery or dependencies.

Use `gofmt`. Go identifiers use `MixedCaps` or `mixedCaps`, including constants. Do not impose a
fixed source line width; simplify complicated expressions rather than mechanically wrapping them.
Follow nearby conventions for matters the guide leaves open. Apply improvements to new or touched
code without unrelated repository-wide style churn.

## Detailed decisions

Use the corresponding sections of [Style Decisions](https://google.github.io/styleguide/go/decisions):

- **Naming:** Choose concise lowercase package names and avoid repetition at call sites. Preserve
  initialisms (`userID`, `HTTPClient`). Receiver names abbreviate their type, usually in one or two
  letters, consistently across methods; avoid `this` and `self`. Scale variable names to scope.
- **Imports:** Avoid dot imports and unnecessary aliases; use consistent aliases when needed.
- **Comments:** Document exported APIs and important contracts, including ownership and concurrency.
- **Errors:** Return `error` last. Handle failures explicitly; do not use panic for ordinary failure.
  Keep messages lowercase without terminal punctuation. Inspect error identity with `errors.Is`
  or `errors.As`, rather than comparing message strings.
- **Contexts:** Pass `context.Context` first and carry the caller's context through the call chain.
  Do not use custom context types or store contexts in structs except where a required external
  interface prevents explicit parameters.
- **Goroutine lifetimes:** Make termination and synchronization evident; account for cancellation
  and blocked channel operations.

## Advisory design and testing

Apply [Best Practices](https://google.github.io/styleguide/go/best-practices) with judgment:

- Introduce interfaces for actual consumers, keeping their required method sets small. Prefer
  concrete return types where appropriate; established contracts such as `error` remain interfaces.
- Tests should make behavior and failures understandable. Use tables when they reveal meaningful
  case differences, rather than forcing every test into a table.
- Keep test setup and helpers readable; diagnostics should identify the operation, actual result,
  and expected result. Avoid helpers or assertions that obscure the behavior under test.

## GymPulse requirements

These are local obligations from `AGENTS.md`, `CLAUDE.md`, and the constitution, not additional
universal Google rules:

- Preserve handler → service → DAO separation, authenticated ownership, and public error codes.
- Preserve error causes with `%w` when adding useful context; keep internal details out of responses.
- Separate standard-library, third-party, and internal imports with blank lines; use `goimports`
  when available and preserve the configured lint policy.
- Use keyed fields in response structs and fixtures. Go identifiers and wire names are distinct:
  JSON tags remain `snake_case`; collections promised by the contract return `[]`.
- Propagate context through database, network, and concurrent calls. Define cancellation, resource
  cleanup, transaction completion, and goroutine ownership explicitly.

## Apply and verify

Inspect the affected package, repository instructions, Go version, and lint configuration before
editing. Review naming, comments, failure paths, context propagation, and lifetime management in
the final diff. Distinguish a documented rule from an advisory preference in review findings;
include a source section and explain the concrete readability or correctness impact.

For code changes, format changed Go files and run focused tests, then the owning repository's
required checks (`go test ./...` and configured lint in GymPulse; race checks when concurrency
changes). Do not install tools, alter lint policy, or add dependencies solely to enforce a style
preference. Report unavailable checks accurately. Instruction-only edits need documentation and
skill validation, not application tests.
