# Tasks: Fewer database round trips behind Today

**Spec**: [spec.md](spec.md) · **Plan**: [plan.md](plan.md)

- [X] T001 Reproduce the baseline on a scratch stack (method in spec.md). Recorded below.
- [X] T002 FR-001 `ParticipationReader` batch read and `participationService.List` without the
  schedule read. Tests: `TestParticipationIsOneReadWhenNothingNeedsFinalizing`,
  `TestParticipationFinalizesThePastThenReadsItBack`,
  `TestParticipationRejectsWhatTheScheduleReadRejects`,
  `TestParticipationAnswersMatchTheFullScheduleRead`, integration
  `TestReadParticipationMatchesTheSeparateReads`.
- [X] T003 FR-002 `ScheduleReader` batch read, `needsFill`, `finalizePastSessions`. Tests:
  `TestScheduleListIsOneReadWhenNothingNeedsWork`, `TestScheduleListFillsOnlyWhenTheWatermarkIsShort`,
  `TestScheduleListFinalizesThePastThenReadsItBack`, `TestScheduleListWithoutAProfile`,
  integration `TestReadScheduleMatchesTheSeparateReads`,
  `TestReadScheduleWithoutAProfileOrActiveProgram`; existing
  `TestScheduleListCostDoesNotGrowWithPlannedSessions` still passes.
- [ ] T004 (held, needs the owner) Identity-check cache: built and tested on branch
  `perf/active-user-cache`, not merged, because it changes `docs/CONTRACTS.md` and the release
  runbook (spec, "Held").
- [X] T005 FR-003 pool floor default and `.env.example`. Test: config defaults and
  `unset min follows a lowered max`.
- [X] T006 Gates and re-measurement (below).

## Evidence (2026-11-20)

Same scratch stack and method as the baseline; nine GETs fired at once; times are medians or
ranges over repeated runs. A statement in a batch shares its round trip.

| Route | Statements before | Round trips before | Statements after | Round trips after |
| --- | --- | --- | --- | --- |
| `GET /schedule` | 6 | 6 | 6 | 2 |
| `GET /participation` | 7 | 7 | 4 | 2 |
| `GET /programs` | 3 | 3 | 3 | 3 |
| each of the other 6 Today GETs | 2 | 2 | 2 | 2 |
| **All nine** | **28** | **28** | **25** | **19** |

Every "after" count includes the identity check (one trip per request).

| Today ready, 65 ms per trip | Before | After |
| --- | --- | --- |
| Warm (15 bursts) | 649 ms (501–1017) | 354 ms (219–464) |
| After 35 s idle | 640–940 ms | 314–377 ms |
| First burst after a restart | 1253 ms | ~540 ms (floor 10; ~700 ms with floor 1) |

With the held identity cache as well: 16 statements / 10 round trips, Today warm ~157 ms.

| Gate | Result |
| --- | --- |
| `go test -race ./...` | pass |
| `golangci-lint run` | 0 issues |
| `go test -p 1 -race -tags=integration ./internal/dao ./internal/dbmigrate -count=1` | pass |
| `scripts/smoke.sh` against the scratch stack | 45 passed, 0 failed |
| `scripts/validate-supabase-security.sh` | pass |
| Production timings | not measured |
