# Tasks: The athlete's own stretches, and routines on program workouts

**Feature ID**: `gympulse-own-stretches` | **Spec**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md)

- [X] T001 Migration 050 (up and down): `stretch_libraries`, the two `program_workouts` columns and their checks.
- [X] T002 Models: stretch library types and limits; `OptionalText` routine fields on `ProgramWorkout`.
- [X] T003 `StretchLibraryDAO`: `Get`, `Save` (lock, ledger, revision, upsert in one transaction).
- [X] T004 `StretchLibraryService`: FR-003, FR-004, FR-005 validation, trimming, request hash.
- [X] T005 `StretchLibraryHandler` with godoc, routes, wiring in `cmd/server/main.go`.
- [X] T006 Account deletion deletes `stretch_libraries`; public table count is 44.
- [X] T007 Program units: read, insert, and keep-when-absent update of the routine columns; validation in `validateProgramShape`.
- [X] T008 Unit tests: library service and handler, program routine rules, unit JSON, unit diff.
- [X] T009 Integration tests: library DAO and program unit columns.
- [X] T010 `docs/CONTRACTS.md` (contract 1.16.0), `docs/DATABASE_DESIGN.md`, regenerated swagger, Bruno requests, smoke step 8c.
- [ ] T011 Run the integration suite and `./scripts/smoke-toggle.sh` against a disposable stack (CI `smoke` job runs both).

## Evidence

| Requirement | Test | Result |
| --- | --- | --- |
| FR-001 empty library shape; US1 scenario 1 | `TestStretchLibraryHandlerGet` (exact JSON), `TestStretchLibraryServiceGet` | passed |
| FR-002 PUT body, header, 200, same shape as GET; US1 scenario 2 | `TestStretchLibraryHandlerSave` (exact JSON), `TestStretchLibraryHandlerSaveErrors` (header cases) | passed |
| FR-003 every move rule | `TestStretchLibraryServiceSaveRejectsInvalidRequests` (move cases), `TestStretchLibraryServiceSaveAcceptsTheLimits` | passed |
| FR-004 every routine and item rule | `TestStretchLibraryServiceSaveRejectsInvalidRequests` (routine and item cases), `TestStretchLibraryServiceSaveAcceptsTheLimits` | passed |
| FR-005 422 with `details.field` `moves` or `routines` | `TestStretchLibraryHandlerSaveErrors` (bad move, bad routine, lists left out) | passed |
| FR-006 one transaction, replay, key reuse, stale revision with `resource`; US1 scenarios 3 and 4 | `TestStretchLibraryCreateReplayReplaceAndConflict`, `TestStretchLibraryConcurrentFirstSavesCreateOnce`; handler mapping in `TestStretchLibraryHandlerSaveErrors` | DAO tests written, not run locally; handler passed |
| FR-007 table, ownership, deletion, runtime role; US1 scenarios 5 and 6 | `TestStretchLibraryBelongsToOneAthlete`, `TestStretchLibraryIsRemovedWithTheAccount`, `TestStretchLibraryWorksAsRuntimeRole`, `TestStretchLibraryTableRejectsContentThatIsNotAnObject` | written, not run locally |
| FR-007 migration rules and security inventory | `internal/dbmigrate` lint, `TestSupabaseSecurityMigrationProtectsEveryPublicApplicationTable` (44 tables), `./scripts/validate-database-design.sh` | passed |
| FR-008 both keys always in unit JSON; US2 scenario 1 | `TestProgramUnitRoutineJSON`, `TestStarterCopyHasNoRoutines`; `TestStarterUnitsHaveNoRoutines` | unit passed; integration written, not run locally |
| FR-008 column checks | `TestProgramUnitRoutineColumnsRejectValuesThatAreNotSlugs` | written, not run locally |
| FR-009 stored and returned; US2 scenario 2 | `TestProgramUnitRoutinesAreStoredOnCreateAndUpdate`; `TestProgramUnitKeepsItsChosenRoutines` | unit passed; integration written, not run locally |
| FR-009 absent keeps, null or "" clears; US2 scenario 3 | `TestProgramUnitRoutineJSON` (decode), `TestProgramUnitRoutinesAreStoredOnCreateAndUpdate` (what reaches the DAO); `TestProgramUnitKeepsItsChosenRoutines` (rows) | unit passed; integration written, not run locally |
| US2 scenario 4: not a slug, routine on a sport unit → 422 `workouts` | `TestProgramUnitRoutinesAreValidated`, `TestSportUnitClearsItsRoutines` | passed |
| US2 scenario 5: a routine-only change leaves sessions alone | `TestDiffUnitsIgnoresRoutineChoices`; session snapshot in `TestProgramUnitKeepsItsChosenRoutines` | unit passed; integration written, not run locally |
| FR-010 contract, swagger, Bruno, smoke | `swag init` (v1.16.4) leaves no diff after regeneration; `bash -n scripts/smoke.sh`; step 8c | swagger and syntax passed; smoke not run locally |
| SC-001 two library requests; program routes gain fields only | `TestTrainingRoutesRegistered` | passed |
| SC-002 gates | `go build ./...`, `go vet ./...`, `go vet -tags=integration ./internal/dao ./internal/dbmigrate`, `go test ./...`, `golangci-lint run` (0 issues), service coverage 91.4% | passed |

No disposable PostgreSQL was reachable at `127.0.0.1:15432`, so the integration suite and the smoke script were not run locally. The Postman collection does not list `/mobility-sessions`, so it was not changed.
