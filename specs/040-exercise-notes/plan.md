# Implementation Plan: Exercise notes

**Feature ID**: `gympulse-exercise-notes` | **Spec**: [spec.md](spec.md)

## Constitution check

I contract 1.19.0 in `docs/CONTRACTS.md` in the same change; II every query is scoped by the
authenticated `user_id`, which starts the primary key; III additive migration 052, idempotent by
natural key with last write wins (recorded as a decision, since the route carries no operation key
or revision); IV tests named in the spec's acceptance table, service coverage stays above 90%;
V handler → service → DAO, no new dependency, no concurrency.

## Design assessment

| Area | Decision |
| --- | --- |
| Contract | `GET /exercise-notes` (200), `PUT /exercise-notes` (200). 422 `VALIDATION_ERROR` for `exercise_name` and `note`; 400 `BAD_REQUEST` for a body that is not JSON. |
| Identity | `middleware.MustGetUserID`. There is no id to guess: the key is the athlete's own exercise name under their own `user_id`, so there is no foreign-resource case, only two athletes with the same name. |
| Invariants | Key derivation and trimming: service (`model.ExerciseNoteKey`). Lengths: service for the message, database checks as the backstop. One note per athlete and key: primary key. |
| Transaction | One statement per request. Save is `INSERT … ON CONFLICT DO UPDATE`; clear is one `DELETE`. Two saves of the same key race to last write wins, which is the documented rule. |
| Limit | The insert selects only when the athlete has fewer than 500 notes or already has this key. Two concurrent first saves can both pass the count, so the cap is a bound on read size, not an exact quota. |
| Schema | `exercise_notes (user_id, exercise_key, exercise_name, note, created_at, updated_at)`, PK `(user_id, exercise_key)`, length checks, cascade from `auth.users`, `secure_table`. No secondary index: every read and write is by owner. |
| Export | One more reader in the export snapshot; `schema_version` stays 1 because the section is additive. |
| Compatibility | Additive. Old clients never call the routes; rows exist only after an athlete saves a note. |
| Privacy | Note text is health-adjacent free text. Nothing logs it; errors name the field, never the value. |

Simpler alternative rejected: a JSONB document of all notes per athlete (the stretch library's
shape). Every edit would rewrite the whole document, so two devices editing different exercises
would overwrite each other unless the document carried a revision and the client merged.

## Files

`migrations/052_exercise_notes.{up,down}.sql`, `internal/model/exercise_note.go`,
`internal/dao/exercise_note_dao.go`, `internal/dao/account_dao.go`, `internal/dao/export_dao.go`,
`internal/model/export.go`, `internal/service/exercise_note_svc.go`,
`internal/handler/exercise_note.go`, `internal/router/router.go`, `cmd/server/main.go`, tests
beside each, `docs/CONTRACTS.md`, `docs/DATABASE_DESIGN.md`, generated swagger,
`scripts/smoke.sh` step 8e.

## Verification

`go test ./...`, `golangci-lint run`, `./scripts/validate-database-design.sh`, the tagged
integration suite and the migration round trip against a scratch PostgreSQL 16,
`validate-supabase-security.sh` against the same database, and the `smoke` job in CI.
