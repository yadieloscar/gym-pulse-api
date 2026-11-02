# Quickstart: Generate a Program Draft

## Implementation order

1. Migration 031 (up and down) for `program_generations`, `docs/DATABASE_DESIGN.md` table, index,
   and ledger rows, `./scripts/validate-database-design.sh`.
2. `go get github.com/anthropics/anthropic-sdk-go@v1.84.0`; config fields and README env table.
3. Model: `ProgramDraft`, `GenerationStatus`, `GenerationLimitError`, `GenerationUnavailableError`,
   and `RepairGeneratedDraft` with table-driven tests (every FR-003 and FR-004 clause).
4. Provider adapter `service/program_generator_anthropic.go` behind a small `DraftGenerator`
   interface; `buildGenerationRequest` with the privacy test; httptest-backed tests for success,
   refusal, 429/5xx, timeout, and malformed JSON.
5. DAO `program_generation_dao.go`: reserve and complete, with PostgreSQL integration tests for
   the limit, the two-minute pending window, failed rows, and concurrent reservations.
6. Service `program_generation_svc.go`: status, generate (profile → reserve → provider → repair →
   complete), error mapping; tests with a fake generator and fake DAO.
7. Handler, routes with `RateLimitPerUser(ProgramGenerationLimit)`, `response.go` codes, wiring in
   `main.go`, account deletion entry, router test.
8. `docs/CONTRACTS.md` section "Program generation"; `swag init`; smoke steps.
9. `gofmt`, `go test ./...`, `golangci-lint run`, `./scripts/smoke-toggle.sh`,
   `./scripts/validate-supabase-security.sh` on a migrated database.

## Tests identified before implementation

| Requirement | Evidence |
|---|---|
| FR-001, FR-002 | Handler test: 200 with `draft`, `rationale`, `request` echo, `remaining_today`; draft marshals into `CreateProgramRequest` and passes `validateProgramShape` |
| FR-003, FR-004, FR-005 | `RepairGeneratedDraft` table tests: unknown id dropped, duplicate collapsed, 7 workouts capped to days, 9 exercises capped to 8, 2 exercises → invalid, reps/duration by modality, clamps, weight cleared, goal forced, name and rationale truncated |
| FR-006 | Service test: the fake DAO records no program, schedule, or profile writes; DAO test: only `program_generations` statements |
| FR-007, SC-004 | `buildGenerationRequest` test marshals the params and asserts the allowed key set only; no `user_id`, `timezone`, or email |
| FR-008, SC-005 | DAO test: fifth reservation succeeds, sixth returns `GenerationLimitError` with local-midnight `resets_at`; service test: no generator call after the limit; concurrent reservation test under `-race` |
| FR-009, SC-003 | Adapter tests: timeout, 5xx, refusal, bad JSON → `GenerationUnavailableError`; service marks the row `failed`; DAO test: failed and stale pending rows do not count; handler returns 503 within the deadline |
| FR-010 | Service test: missing profile → `ValidationError{Field: "training_profile"}` and no reservation or generator call |
| FR-011 | Config test: empty key → disabled status and 503 `disabled`; key never appears in `GenerationStatus` or logs (log capture test) |
| FR-012 | Account cleanup test includes `program_generations` |
| FR-013, SC-006 | `./scripts/smoke-toggle.sh` with the new steps; `go test ./...` unchanged elsewhere |
| SC-001, SC-002 | Fixture-driven test over six goals × {1, 3, 6} days through the fake generator returning recorded provider JSON; every draft passes repair and `validateProgramShape` |

## Manual acceptance

With `ANTHROPIC_API_KEY` set locally: save a profile (hypertrophy, days 1/3/5, intermediate,
barbell and dumbbells, 60 min), `GET /program-generation` and see `enabled: true, remaining_today:
5`; `POST` with no body and see a three-workout draft within 45 seconds; `POST` the draft to
`/programs` with an operation key and see 201. `POST` with `{"request": "no deadlifts"}` and see no
deadlift. Repeat until the sixth call returns 429 `GENERATION_LIMIT_REACHED` with `resets_at`.
Unset the key, restart, and see `enabled: false` and 503 `disabled`.

In the smoke compose (no key): `GET` returns `enabled: false`; `POST` returns 503 with
`reason: "disabled"`; these two steps are the deterministic smoke coverage.

## Compatibility and rollback

Additive: one table, two routes, one optional dependency on a provider key. Deploy before the app
release; older apps never call the routes. Roll back the app first, then the API; the down
migration drops the table. Leaving `ANTHROPIC_API_KEY` empty is the operational kill switch and
needs no deploy beyond a restart.
