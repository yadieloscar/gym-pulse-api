# Quickstart: Rule-Based Program Engine

## Prerequisites

- gym-pulse-api#39 (spec 015) merged; this branch rebased on it so `DraftGenerator`,
  `RepairGeneratedDraft`, the ledger DAO, and `goalGuide` exist.
- Local stack per `docs/RELEASE_RUNBOOK.md`; no `ANTHROPIC_API_KEY` needed.

## Implementation order

1. Migration 032 (up: add columns, seed all 79 rows by `(category, name)`, add CHECKs and
   `NOT NULL`, index; down: drop), `docs/DATABASE_DESIGN.md`, `./scripts/validate-database-design.sh`.
2. `model.CatalogExercise` fields and the catalog DAO select; catalog handler test shows the
   fields; `docs/CONTRACTS.md` catalog section.
3. `model`: `avoid`, `focus`, `variant` on the request, `NormalizeGenerationConstraints`, response
   `provider`/`avoid`/`focus`, `ErrTooConstrained` mapping; table tests.
4. `internal/engine`: `tables.go` (every table from research R3 as typed data), `engine.go`
   (`Build`), `rationale.go`; tests: table coverage, eligibility, slots per session, beginner
   cap, splits per day count, focus share, avoid, variant rotation, determinism, secondary share,
   the exhaustive goal × days × experience × equipment matrix, rationale deny list and length.
5. `service/program_generator_rules.go` adapter; service: provider selection, skip the ledger for
   rules, pass constraints, map `ErrTooConstrained`; tests with the real engine and a fake DAO
   asserting no reserve/complete calls.
6. Config `PROGRAM_GENERATION_PROVIDER`, `main.go` wiring, README env table; config tests.
7. Handler and router tests for the new fields; `swag init`; CONTRACTS.md "Program generation"
   amendment; smoke steps 16b/16c.
8. `gofmt`, `go vet`, `golangci-lint run`, `go test ./...`, `./scripts/smoke-toggle.sh`,
   `./scripts/validate-supabase-security.sh`; record p95 from the smoke run.

## Tests identified before implementation

| Requirement | Evidence |
|---|---|
| FR-001 | Config and main wiring tests: default provider `rules` with no key; `model` with key; `model` without key falls back with a warning; status `provider` field |
| FR-002 | Adapter test: engine output passes `RepairGeneratedDraft` unchanged for the exhaustive matrix |
| FR-003, SC-003 | Engine test: `Build` twice equals; variants 0..3 differ where eligible lists allow; snapshot of one draft committed as a fixture |
| FR-004 | Model tests: unknown `avoid`/`focus`, negative or >1000 `variant` → 422 naming the field; free text ignored by the engine (adapter test) |
| FR-005, SC-004 | Migration test on PostgreSQL: all rows non-null; CHECK rejects bad values; catalog handler returns the fields |
| FR-006 | `tables_test.go`: each goal × experience row and each split × days row asserted; sources comment per table |
| FR-007 | Engine tests: slots by minutes, beginner cap and no explosive, equipment eligibility per value, `full_gym` opens all, bodyweight always eligible |
| FR-008 | Engine tests: avoided patterns absent; focus two-thirds share; too-constrained → `ErrTooConstrained`; service maps to 422 `avoid` |
| FR-009 | Rationale tests: length ≤ 500, sentences present for split/main lifts/secondary/avoid, deny list (diet, weight, lose, burn, medical, injury) |
| FR-010, SC-002 | Service test: no ledger calls on the rules path; smoke timing step asserts p95 < 200 ms over 20 calls |
| FR-011 | Log capture test: provider, duration, split, workouts only |
| FR-012 | CONTRACTS.md and OpenAPI updated in the same commits; Swagger diff reviewed |
| SC-001 | Exhaustive matrix test (6 goals × 1..6 days × 3 experience × 8 equipment sets): valid draft or `ErrTooConstrained` never (no constraints in the matrix) |
| SC-005 | `go test ./...` with spec 015 tests unchanged apart from new fields |

## Manual acceptance (local stack, no key)

1. `GET /program-generation` → `enabled: true`, `provider: "rules"`.
2. `POST /program-generation` with the smoke profile → 200 in well under a second, three
   workouts, rationale names the split; repeat → identical body.
3. `POST` with `variant: 1` → same split and targets, different exercises.
4. `POST` with `avoid: ["hinge"]`, `focus: "upper"` → no deadlift family, upper-heavy, rationale
   says so.
5. `POST` with `avoid` of every pattern → 422 on `avoid` with the plain message.
6. `POST /programs` with the draft body → 201.

## Rollout

Deploy after gym-pulse-api#39. Default provider is `rules`, so the key can be removed from Railway
once the app amendment ships. Rollback is `PROGRAM_GENERATION_PROVIDER=model` with a key, or a
revert; migration 032 down drops the columns.
