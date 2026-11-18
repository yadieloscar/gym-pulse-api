# Tasks: Weekly heaviest set per exercise

**Feature ID**: `gympulse-progress-payoff` | **Spec**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md)

- [X] T001 `ExerciseProgression` and `ProgressionWeek` models.
- [X] T002 DAO: `WeeklyHeaviestSets`, one query.
- [X] T003 Service: `ExerciseProgression` with `ids` and `weeks` validation and requested order.
- [X] T004 Handler with godoc, and the route.
- [X] T005 Service and handler unit tests.
- [X] T006 Integration test: tie-breaks, week bucketing, window start, ownership, discarded sessions, not-completed sets, runtime role.
- [X] T007 `docs/CONTRACTS.md` (contract 1.15.0), regenerated swagger, Bruno request, smoke step 8b.
- [ ] T008 Run the integration suite and `./scripts/smoke-toggle.sh` against a disposable stack (CI `smoke` job runs both).

## Evidence

| Requirement | Test | Result |
| --- | --- | --- |
| FR-001, FR-005 response shape and order | `TestLogService_ExerciseProgression` (requested order), `TestLogHandler_ExerciseProgression` (exact JSON) | passed |
| FR-002 `ids` rules, empty → `[]` | `TestLogService_ExerciseProgression` (malformed, 101 ids, empty) | passed |
| FR-003 `weeks` default, 1 to 52, window start | `TestLogService_ExerciseProgression` (window and rejection cases) | passed |
| FR-003 Monday weeks; scenarios 1 to 3 tie-breaks and gaps | `TestWeeklyHeaviestSets` | written, not run locally |
| FR-004; scenarios 4 to 6 omission, discarded, not completed, ownership | `TestWeeklyHeaviestSets` | written, not run locally |
| FR-006 read-only | route is a `GET` with no write path; handler test | passed |
| FR-007 no migration, runtime role | `TestWeeklyHeaviestSets` (runtime role subtest) | written, not run locally |
| FR-008 contract, swagger, Bruno, smoke | `swag init` leaves no diff; `scripts/smoke.sh` step 8b | swagger passed; smoke not run locally |
| SC-002 gates | `go test ./...`, `golangci-lint run`, service coverage 91.1% | passed |

The Postman collection does not list `/exercises/records`, so it was not changed.
