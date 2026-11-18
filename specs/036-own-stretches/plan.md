# Implementation Plan: The athlete's own stretches, and routines on program workouts

**Feature ID**: `gympulse-own-stretches` | **Spec**: [spec.md](spec.md) | **Date**: 2026-11-18

## Summary

Two additive parts, contract 1.16.0, one migration (050).

1. `GET` and `PUT /api/v1/stretching/library`: one JSONB document per athlete in the new table `stretch_libraries`, replaced whole with an expected revision and an idempotency record in one transaction.
2. `program_workouts` gains nullable `warmup_routine_id` and `cooldown_routine_id`. Program and starter units always return both keys; a program update keeps the stored choice of a unit whose request leaves a key out.

## Workflow and Specialist Scope

API engineering, Google Go style, and the PostgreSQL practices apply. Cross-repository delivery applies: the app sibling is spec 044, API first. One primary agent; no delegation.

## Constitution Check

- Contract first: `docs/CONTRACTS.md` gains "Stretch library (contract 1.16.0)" and the unit fields in the Program shape, with older-server detection.
- Owned data: every read and write is keyed by the authenticated `user_id`; the table is secured with `gym_pulse_private.secure_table`.
- Idempotent evolution: revision and ledger for the library; the migration is additive and re-runnable with a full down.
- Executable evidence: see tasks.md. Service coverage stays above 90%.
- Simple boundaries: handler → service → DAO, no new dependency, no new abstraction.

## Design

### Stretch library

- `migrations/050_stretch_libraries_and_program_unit_routines`: `stretch_libraries(user_id PK → auth.users CASCADE, revision BIGINT DEFAULT 1 CHECK > 0, content JSONB, updated_at)`. `content` must be a JSON object of at most 262,144 bytes as text: a backstop only, since the service bounds it to 50 moves and 30 routines of 30 items.
- `internal/model/stretch_library.go`: `StretchMove`, `StretchRoutine`, `StretchRoutineItem`, `StretchLibrary`, `SaveStretchLibraryRequest`, and the limits.
- `internal/service/stretch_library_svc.go`: trims names, cue, areas, and the operation key, then checks every rule in FR-003 and FR-004. A fault names the list (`moves` or `routines`) and the zero-based position of the entry. The request is hashed after trimming.
- `internal/dao/stretch_library_dao.go`: `Get` returns the empty library at revision 0 when no row exists. `Save` takes the `stretch-library:` advisory lock (two first saves have no row to lock), reads the ledger, locks the row, compares the revision, upserts, and writes the ledger, in one transaction. A stale revision is a `ConflictError` carrying the current library, or the empty one when none was saved.
- `internal/handler/stretch_library.go` with godoc; routes beside `/mobility-sessions`. Both answers are 200: the spec gives the first save no separate status.
- Account deletion deletes the row; the identity cascade does too.

### Program unit routines

- `model.ProgramWorkout` is the request and the response type, so the two fields are `model.OptionalText`: it marshals the value (`null` when unset) and records on decode whether the key was in the body.
- `validateProgramShape` checks a set value with the slug rule templates use (`sportIDPattern`, at most 64) and refuses any routine on a sport unit, both as `details.field = "workouts"`. A sport unit is marked as a clear, so a unit changed from exercises to sport drops its choice.
- `insertProgramWorkouts` stores the value. The unit `UPDATE` in `replaceProgramUnits` writes a column only when its key was present. The response takes the routine values from the rows re-read at the end of that function, so a kept choice is returned.
- Reads: `programWorkoutsQuery` and `scanProgramWorkouts` carry the two columns. Starter units have no such columns and read as unset; `cloneStarterWorkouts` copies that.
- Not changed, by design: `scheduled_workouts`, the rules engine, generated drafts, and `sameUnitContent`. A routine-only save yields an empty unit diff, so no session is re-laid or re-snapshotted.

## Decisions the spec left open

| Question | Decision |
| --- | --- |
| `moves` or `routines` missing or `null` in the PUT | 422 on that field. A client that drops a list must not empty it by accident; `[]` empties it on purpose. |
| `expected_revision` missing | Read as 0, as `PUT /nutrition/profile` does. It succeeds only when no library exists. |
| Stale revision when no library was ever saved | 409 with the empty revision-0 library under `resource`, so `resource` is always a library. |
| Which position a routine item fault reports | The routine's position in `routines`. |
| A unit saved as `sport` that stored a routine | The choice is cleared. |

## Risks

- A retry of a program create or update that spans this deploy hashes differently, because the hashed request now carries the two keys. Such a retry gets `409 IDEMPOTENCY_CONFLICT` once; the client's next save uses a new key. `weekdays` (1.10.0) had the same effect.
- The hash cannot tell an absent routine key from `null`. Two bodies that differ only in that way under one operation key replay the first answer.
- `programDAO.Replace`, the fallback used only when the service has no mutation DAO (tests), rewrites every unit and so does not keep an absent key. Production always wires the mutation DAO.
- Removing a routine from the library leaves its slug on any saved workout or program unit that chose it. The app decides what to show; the API does not look routines up.
- An app on a server older than 1.16.0 gets 404 without the JSON envelope from the library route and units without the keys.

## Rollout

Additive. Deploy the API (migration 050 runs in the pre-deploy step) before the app release that calls the library route or sends the unit fields. Rolling the API back leaves the table and columns unused; the down migration drops them.

## Verification

`go build ./...`, `go vet ./...`, `go vet -tags=integration ./internal/dao ./internal/dbmigrate`, `go test ./...`, `golangci-lint run`, `./scripts/validate-database-design.sh`, `bash -n scripts/smoke.sh`, swagger regenerated with swag v1.16.4, and in CI the integration suite and `scripts/smoke.sh` step 8c.
