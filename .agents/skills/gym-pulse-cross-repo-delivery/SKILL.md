---
name: gym-pulse-cross-repo-delivery
description: Plan, implement, review, and ship coordinated GymPulse changes spanning the Go API and Expo app repositories. Use when a feature, bug, contract, authentication flow, data model, rollout, or acceptance test may require changes in both gym-pulse-api and gym-pulse-app, or when repository ownership is initially unclear.
---

# GymPulse Cross-Repo Delivery

Coordinate two independent Git repositories through one explicit contract and rollout plan. Keep
Git history, verification, and pull requests separate while preserving a coherent product change.

## Establish Scope

1. Locate both repositories and record each branch, status, remote, and unrelated user changes.
2. Read both `AGENTS.md` files and invoke the applicable repository engineering skill.
3. Read the relevant API contract, product specification, implementation, and tests on both sides.
4. Classify the work as API-only, app-only, or cross-repository. Do not force two-repo work when one
   side already supports the required behavior.
5. Identify the user-visible outcome and the end-to-end acceptance evidence.

Preserve independent repository boundaries. Never stage, commit, push, or discard changes in one
repository as an implicit side effect of work in the other.

## Define the Integration Contract

Before implementation, state:

- endpoint, method, request, response, validation, status, and stable error behavior;
- authenticated identity, ownership, revision, idempotency, retry, and partial-failure semantics;
- app loading, empty, error, cached, offline, and session-expiry behavior;
- additive or breaking compatibility and the supported mixed-version window;
- database migration, backfill, and rollout ordering when applicable;
- feature flags, mock fixtures, generated documentation, and observability changes;
- end-to-end tests that prove the integrated behavior.

Treat `gym-pulse-api/docs/CONTRACTS.md` as client-facing truth. Update it with observable API changes
before or with dependent app implementation. Never let the app invent an undocumented shape.

## Decompose Expert Work

Use these roles when parallel analysis or independent review adds value:

- API engineer: contract, Go architecture, authentication, transactions, persistence, and API tests.
- App engineer: Expo architecture, session behavior, API integration, cache consistency, UX states,
  accessibility, and mobile tests.
- Quality reviewer: read-only review of the final diffs and end-to-end failure modes.
- Engineering lead: integration decisions, sequencing, conflict resolution, and final evidence.

Give each specialist a bounded question, required source files, allowed repositories and files,
verification expectations, and output format. Prefer parallel read-only analysis. Permit parallel
writing only in disjoint repositories or explicitly disjoint files; one lead owns integration.

## Plan Compatibility and Sequence

Prefer additive delivery:

1. Add backward-compatible API support and contract tests.
2. Deploy or make the API support available.
3. Update the app to consume the new behavior with compatible failure handling.
4. Observe and verify the integrated flow.
5. Remove deprecated behavior only in a later coordinated change.

When additive delivery is impossible, document the breaking window, deployment ordering, rollback or
roll-forward strategy, and how old app versions behave. Do not assume mobile clients update together.

For schema changes, separate expand, migrate/backfill, switch reads or writes, and contract steps when
a rolling deployment or old client can encounter mixed versions.

## Implement by Repository

For API work:

- invoke `$gym-pulse-api-engineering`;
- implement the contract, security, persistence, and tests;
- update `docs/CONTRACTS.md` and generated documentation as required;
- verify the API independently before depending on it from the app.

For app work:

- invoke `$gym-pulse-app-engineering` when available;
- update types, API wrapper usage, hooks, cache behavior, screens, and tests coherently;
- keep Supabase limited to authentication and route application data through the Go API;
- update mock fixtures whenever they represent the changed contract.

Use separate branches, commits, and PRs for the two repositories. Link the PRs and state their merge
or deployment dependency explicitly.

## Verify the Product Change

Run each repository's required gates. Then verify the integration across:

- authenticated success and foreign-resource denial;
- validation and stable error handling;
- stale revision, retry, duplicate submission, and partial failure when applicable;
- loading, empty, error, offline/cache, and session-expiry states;
- old app against new API and new app against the supported API version;
- migration or mixed-version behavior;
- real end-to-end user outcome, not only isolated unit tests.

Do not claim cross-repository completion when only one side is implemented or only isolated tests have
passed. Report unavailable integration environments and residual risk.

## Review and Report

Perform a final read-only review of both diffs. Challenge contract drift, duplicated business rules,
authorization gaps, incompatible sequencing, cache invalidation, missing UX states, and tests that do
not prove the user outcome.

Return:

```text
User outcome:
API repository branch, commit, PR, and verification:
App repository branch, commit, PR, and verification:
Contract and compatibility decisions:
Deployment or merge order:
Cross-repository acceptance evidence:
Independent review findings:
Skipped gates and remaining risks:
```
