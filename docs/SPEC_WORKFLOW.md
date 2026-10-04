# Spec Kit and Specialist Workflow

This guide is mirrored in both GymPulse repositories. It defines routing and resumption for agents;
`CODEX_PLAYBOOK.md` supplies human examples. Read this guide before choosing a workflow for substantive
work or resuming a feature. The constitution owns engineering obligations; this guide explains their
execution. The user's current scope and previously granted authority remain controlling.

## Choose the Smallest Complete Workflow

Inspect current intent, relevant specs, code, tests, and repository status before classifying work.
Explain the repository, feature, next stage, and necessary skills in one short sentence. The user
does not need to select commands or roles.

| Request | Route |
| --- | --- |
| Explanation, investigation, or read-only review | Read relevant artifacts and inspect evidence; do not create a feature or implement a fix unless requested. |
| Focused correction restoring documented behavior | Reuse the governing spec and contract, record affected acceptance evidence, implement the fix, and run relevant engineering gates. A new feature directory is unnecessary. |
| Change to existing intended behavior | Amend the owning spec and affected design/tasks before implementation; analyze consistency and converge on the changed scope. Preserve unrelated completed work. |
| New non-trivial capability | Create one bounded feature and follow the full cycle below. |
| Significant refactor, migration, authentication, or compatibility change | Use the engineering lead and relevant specialists. Reuse an owning spec when suitable; record preserved behavior, design, risks, and executable acceptance before implementation. Use the full cycle for new scope. |
| Documentation, workflow instructions, or mechanical maintenance only | Update the owning documents directly and validate their links, consistency, and applicable tooling. Do not create a product spec or run application suites solely for prose edits. Changes to executable behavior still require the affected code gates. |
| Resume or check completion | Resolve the intended feature and evidence first, then continue at the first incomplete or invalidated stage within the requested scope. |

Non-trivial implementation MUST have a governing specification. Existing specifications satisfy this
requirement when they describe the intended outcome; if none exists, establish a bounded spec before
implementation. A small change to intended public API behavior still needs a specification and
contract update. Never use the focused-fix route to bypass ownership, tests, compatibility, or other
constitution requirements.

## Run or Resume the Feature Cycle

The full cycle is specify → clarify when needed → plan → tasks → analyze → implement → converge.
Use focused requirements checklists when ambiguity or risk warrants them, usually before tasks.
Read the complete installed `speckit-*` skill for each stage before executing it; natural-language
routing invokes those instructions rather than approximating the stage from memory.

1. Resolve the repository and feature from the user's request, stable feature ID, sibling links,
   and current artifacts. Inspect `.specify/feature.json` and `SPECIFY_FEATURE_DIRECTORY` when set.
   Git branch names and numeric prefixes do not independently establish feature identity.
2. Read the relevant spec, plan, tasks, and acceptance evidence. A checked task or an existing file
   alone does not prove completion. Confirm implementation and relevant checks before claiming it.
3. Select the first missing or invalidated stage: requirements before design, design before tasks,
   and consistency analysis before implementing changed scope. Fix upstream decisions before
   dependent work. Preserve valid artifacts, task IDs, completed work, and existing evidence.
4. If a saved pointer names a different feature than the explicit request, use the requested feature
   and a scoped `SPECIFY_FEATURE_DIRECTORY` override for each stage's scripts. Explain the mismatch;
   do not silently choose the saved feature. Some scripts persist the override, so inspect and
   preserve unrelated pointer state. Never rewrite a shared pointer while another task uses it.
   Use an isolated checkout when simultaneous feature work needs independent mutable state.
5. If several features plausibly match and the request cannot distinguish them, ask one concise
   question before feature-specific mutations. Continue independent read-only investigation.

For read-only feature resolution, use `check-prerequisites.sh --paths-only --json` from
`.specify/scripts/bash/`, with the intended feature override if needed, then inspect prerequisite
files separately. Paths-only mode does not validate their existence. Normal prerequisite/setup
scripts can persist the override even when their surrounding stage is described as read-only; use
an isolated checkout if a stage requires such a helper and the shared pointer must remain untouched.

For an existing feature, edit the affected artifacts in place. Do not blindly rerun a setup or
generation script over a populated plan or task list; inspect its behavior and preserve prior work.
Use `analyze` to report artifact inconsistencies. Use `converge` only after implementation has run
on the current task list; it assesses code and appends remaining tasks rather than fixing code.
Implement those tasks and repeat convergence when appropriate. If repeated findings do not improve,
diagnose the blocker and report it instead of appending duplicate work indefinitely.

Completion checks are read-only unless remediation was requested or already authorized. Because
`converge` can append tasks, use a read-only assessment for requests such as "check whether this is
complete"; report findings and the next action without changing task state.

## Combine Stage Instructions with Specialist Expertise

Spec Kit owns the stage and its artifacts. Repository engineering skills own project architecture
and verification. Applicable external skills contribute domain expertise to the same artifacts.

| Boundary | Skills to apply when relevant |
| --- | --- |
| Go API, contracts, ownership, persistence | `gym-pulse-api-engineering` |
| Expo, navigation, client state, cache, native behavior | `gym-pulse-app-engineering` |
| User-visible UI or interaction creation, change, review, or testing | `gym-pulse-interaction-design` alongside app engineering |
| Ambiguous, high-risk, architectural, or multi-lane work | `gym-pulse-engineering-lead` |
| Both repositories, contract compatibility, or coordinated rollout | `gym-pulse-cross-repo-delivery` and affected engineering skills |
| Expo authentication, protected navigation, session lifecycle | `expo-auth` |
| Supabase functionality, client, session, or configuration | `supabase`; retain GymPulse's auth-only boundary |
| PostgreSQL schema, queries, indexes, transactions, or configuration | `supabase-postgres-best-practices`; retain the Go API's pgx/PostgreSQL architecture |
| Requested document, spreadsheet, presentation, or image deliverable | Applicable artifact skill |
| Reusable skill creation or revision | `skill-creator` |

Read relevant available skills before relying on them. If a required skill is unavailable, report
the limitation and determine whether repository guidance and authoritative documentation suffice;
do not pretend to have applied it or automatically install plugins. Honor an explicitly requested
skill's availability requirements. Generic advice does not silently replace project constraints;
surface material conflicts and resolve them against current intent and governing guidance.

- During specify/clarify, use specialist knowledge to expose missing behavior, failure states,
  ownership, accessibility, or offline decisions. Keep implementation design in the plan.
- During planning, resolve technical choices and capture compatibility, migration, and verification
  obligations in the relevant design artifacts.
- During task generation, turn those obligations into bounded tasks and evidence. Constitution-
  required tests remain required even when upstream templates describe tests as optional.
- During implementation and review, apply the same relevant specialists and assess actual behavior.

A selected skill does not imply a separate agent. Delegate only independent bounded work or review
when it improves evidence or efficiency; one primary agent owns integration and completion.

## Preserve Decisions, Evidence, and Authority

Record user behavior in the spec; design decisions in the plan/research; public API behavior in
`docs/CONTRACTS.md` in the API repository; implementation and verification work in tasks; durable
engineering rules in the constitution or owning skill. Avoid parallel competing plans.

For changed requirements, link requirement/acceptance identifiers to tasks and verification in the
existing task list or a compact evidence table. Distinguish planned, passed, failed, and unverified
checks; record relevant commands and outcomes. Custom requirements checklist approval establishes
requirements quality, not runtime correctness. Do not mark it approved merely to unblock coding.

Reuse authorization already given in the conversation. Honor requested review stops, read-only
scope, and delivery boundaries. Routine stage completion does not itself require a new approval;
ask about material unresolved product choices or genuinely missing authority. A request to plan,
review, or implement does not by itself authorize issue creation, deployment, or other unrelated
external actions. Follow any applicable stage gate without manufacturing extra gates.

Cross-repository features keep linked specs with the same stable feature ID, separate Git histories
and PRs, and explicit API-first compatibility/rollout evidence. Local numeric prefixes may differ.

Finish with the feature path, completed scope/stage, verification evidence, unresolved decisions,
and next action when work remains. Keep these details concise; never infer completion from task
checkboxes alone.

## Verify Workflow Changes

After changing routing or upgrading Spec Kit, run documentation/skill checks and forward-test
representative start, fix, resume, review-only, and cross-repository requests. When the parent
workspace is available, run `make context` and `make spec-check` there. The latter exercises each
repository's installed scripts in disposable fixtures, including stale pointers, preservation of
plans/tasks, and missing prerequisites; it does not use live feature state.

Script checks and instruction review do not establish end-to-end agent reliability. Validate the
next bounded real feature through acceptance, record concrete workflow failures, and correct the
owning instruction or script. Keep improvements tied to observed failures rather than adding gates
for hypothetical problems.
