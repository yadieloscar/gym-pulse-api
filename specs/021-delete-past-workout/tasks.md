---
description: "Implementation tasks for deleting a past workout"
---

# Tasks: Delete a Past Workout

**Input**: [spec.md](spec.md), [plan.md](plan.md)

## Requirement and Verification Evidence

| Requirement | Tasks | Verification | Result |
| --- | --- | --- | --- |
| FR-001 | T001, T002 | `TestLogService_Delete/historical_workout_can_be_deleted`; smoke 13i | Passed 2026-11-03 |
| FR-002 | T003, T004 | `TestScheduleServiceDeleteAllowsOnlyPastOrFinishedWorkouts` (past, missed, today finished allowed; today unfinished, future rejected) | Passed 2026-11-03 |
| FR-003, SC-002 | T005, T007 | `TestScheduledWorkoutDeleteRemovesPerformanceAndParticipation` | Passed (integration, isolated DB) |
| FR-004 | T005, T006, T007 | same test plus `TestDayLogDeleteRemovesHealthExport` | Passed (integration) |
| FR-005, SC-001 | T005, T007 | `...RemovesPerformanceAndParticipation` (total workouts 1 → 0, row dropped), `...KeepsSameDayEvidence` | Passed (integration) |
| FR-006, FR-007 | T005, T007, T008 | `...RejectsActiveSessionAndForeignOwner`; `TestScheduleServiceDeleteMissingWorkoutIsNotFound`; smoke 13i repeat → 404 | Passed |

## Tasks

- [x] T001 Remove the past-date conflict from `logService.Delete` (`internal/service/log_svc.go`)
- [x] T002 Replace the "historical is immutable" test with a "historical can be deleted" test
- [x] T003 Add `ScheduleService.Delete` with the finalized-or-past eligibility rule
- [x] T004 Service table test for eligibility and 404
- [x] T005 Add `scheduleDAO.Delete` and `recomputeParticipation` in one transaction
- [x] T006 Delete the day log's export record in the same statement (`logDAO.Delete`)
- [x] T007 DAO integration tests for removal, same-day evidence, active session, ownership, export
- [x] T008 Handler `ScheduleHandler.Delete` (204), route, Swagger annotations, handler test fake
- [x] T009 Update `docs/CONTRACTS.md` (logs, scheduled workouts, sessions, health exports) and regenerate Swagger
- [x] T010 Smoke step 13i: past log and finished scheduled workout → 204, repeat → 404
