# Using Codex with GymPulse

This playbook is mirrored in both GymPulse repositories. Keep the copies aligned when changing the
shared workflow.

## Use natural language

Describe the product or engineering outcome directly. Codex reads `AGENTS.md`, matches the request
to the repository skills, and applies the relevant expert workflow. You do not need to name roles,
agents, or skills.

A useful direction usually includes:

1. The outcome or problem.
2. Important scope or constraints.
3. What evidence should count as done.

For example:

> Fix workout edits so changing notes never deletes completed sets. Preserve compatibility, add
> regression tests, run the required checks, and open a PR.

Explicit `$skill-name` invocation remains available when you intentionally want to override the
normal routing, but it is not the default usage.

## Start from the right workspace

- Start Codex inside `gym-pulse-api` for backend-only work.
- Start Codex inside `gym-pulse-app` for mobile-only work.
- This local workspace also has a parent `gym-pulse` router for requests that may span both
  repositories.

When work crosses repositories, Codex must keep their Git histories, branches, commits, checks, and
pull requests separate.

## Common directions

### Investigate before changing anything

> Review authentication across the app and API. Do not edit yet. Rank concrete findings by user and
> security impact, show the evidence, and recommend the smallest safe plan.

### Fix an API problem

> Diagnose and fix duplicate workout creation during retries. Preserve the public contract, verify
> ownership and transaction behavior, add regression tests, and run the API gates.

### Build or improve mobile UI

> Improve the workout completion screen so loading, success, failure, reduced-motion, and
> accessibility behavior are coherent. Reuse the design system and verify the interaction on the
> relevant platforms.

User-visible UI automatically applies both app engineering and interaction-design rules.

### Deliver a feature across both repositories

> Add persistent exercise preferences across the app and API. Establish the contract first, keep old
> app versions compatible, implement API support before dependent app behavior, run end-to-end
> acceptance, and open linked PRs.

### Use multiple agents only when useful

> Find and resolve the reliability problems in workout logging. Use subagents only for independent
> lanes that can progress safely in parallel or for an independent final review. Keep one engineering
> lead accountable for integration.

You normally do not need to mention agents. The same efficiency rule is already part of the
engineering-lead and cross-repository workflows.

### Take work through delivery

> Implement the approved plan completely. Preserve unrelated changes, commit coherent phases, push
> the branches, open linked draft PRs, repair any failing checks, and report remaining risk.

## How Codex routes the request

| Request | Applied workflow |
| --- | --- |
| Go, handlers, middleware, PostgreSQL, migrations, or API contracts | API engineering |
| Expo, React Native, navigation, state, cache, or mobile tests | App engineering |
| Screens, components, styling, gestures, motion, or accessibility | App engineering + interaction design |
| Architecture, security-sensitive, ambiguous, or multi-lane work | Engineering lead |
| Contract, authentication, rollout, or acceptance spanning both repos | Engineering lead + cross-repo delivery + relevant specialists |
| New non-trivial feature | Relevant engineering workflow + Spec Kit |

Codex should apply the narrowest complete workflow. It should not force multi-repository work or
spawn agents when a focused sequential change is more efficient.

## What completion should include

Unless you narrow the request, implementation work should finish with:

- the requested user-visible behavior and compatible contracts;
- targeted regression tests plus the repository-required verification;
- design and security review appropriate to the changed boundary;
- clean Git status and intentional commits;
- linked PRs and explicit rollout order for cross-repository delivery;
- a concise statement of anything still unverified.

Green automation is necessary evidence, not proof by itself. The selected engineering workflow also
requires design reasoning, boundary checks, and final integration review.
