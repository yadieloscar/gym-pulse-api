# PRD: goal-based-training (API)

## Why

The current API persists reusable workout templates, weekday plan mappings,
day logs, and aggregate streaks, but it cannot represent the product decisions
needed by the new training experience: one primary goal, a user-owned program,
immutable dated workout snapshots, required-versus-extra sets, honest scheduled
workout outcomes, or a participation streak that can survive an off-plan workout
without erasing the original miss.

The API must become the cross-device source of truth for planning and logging.
It must preserve what the user planned, what they performed, and how the day was
finalized as separate facts.

## User story

As a Gym Pulse user on any device, I want my goal-led program, dated schedule,
set checks, workout outcomes, and participation streak to resolve consistently,
so going offline, changing a goal, missing a workout, or doing something off-plan
never rewrites history or produces conflicting answers.

## Definition of done

1. **The training profile has one primary goal.** The authenticated user's
   profile persists `primary_goal` (`general_health`, `strength`, `hypertrophy`,
   `conditioning`, `power`, or `body_composition`), available training days,
   usual activity, experience, equipment access, preferred session duration,
   optional preferences, and IANA timezone. Validation and partial-update
   behavior are explicit in `docs/CONTRACTS.md`.
2. **Starter programs and user programs are different resources.** The API can
   list versioned curated starter programs filtered by profile inputs. Choosing
   one clones its exercises, order, set targets, and roadmap into a user-owned
   editable program. Users can also create a program from scratch. Catalog
   updates never mutate existing user copies. Starter metadata explains cadence
   and goal rationale; when availability permits, defaults expose major muscle
   groups at least twice weekly and distribute goal-appropriate work across the
   user's available days instead of enforcing one universal split.
3. **Scheduling creates dated snapshots.** The API materializes concrete dated
   workout commitments one week at a time while retaining a longer program
   roadmap. Users can request additional future dates. Every scheduled workout
   snapshots its program/workout source, exercise order, required sets, targets,
   and sequence position.
4. **Goal changes replace only eligible future work.** Accepting a new generated
   schedule replaces unstarted future scheduled workouts atomically. Historical
   dates and an active workout are never changed. The response identifies the
   retained and replaced date range so clients can explain the result.
5. **Per-date edits are isolated.** Editing exercises or required-set targets on
   a scheduled workout changes only that dated snapshot. It cannot mutate its
   source program, starter catalog entry, or another scheduled workout.
6. **Required and extra sets are modeled explicitly.** Required set checks drive
   scheduled-workout status. Extra performed sets remain attributable to the
   workout/date and count for volume, history, progression, and personal records,
   but never help satisfy required planned sets.
7. **Scheduled-workout status has one deterministic rule.** At finalization, all
   required sets checked is `completed`, some is `incomplete`, and none is
   `missed`. An authenticated explicit-complete request can finalize early; an
   idempotent end-of-day process finalizes anything still open after the user's
   local day closes. Clients cannot directly forge a derived status.
8. **Day participation is a separate finalized fact.** On a scheduled training
   day, at least one performed set in any workout makes the day participating and
   preserves the streak even if the scheduled workout remains missed. Zero sets
   across all workouts makes the day non-participating and resets it. Rest and
   unscheduled days neither extend nor reset the streak. Completed and incomplete
   scheduled workouts therefore both preserve it.
9. **History cannot be deleted or silently recalculated.** Past scheduled
   workouts and finalized day outcomes reject delete operations. Authorized edits
   to sets, notes, or the dated snapshot remain auditable. Such an edit may alter
   the currently displayed workout status (for example, completed to incomplete
   after adding an unchecked required set) but never retroactively changes the
   stored historical day-participation/streak decision.
10. **Off-plan work does not rewrite the plan.** It is logged against the date and
    included in participation and training history, but does not change the goal,
    sequence, program, or future schedule. Rollover/adaptation is a separate,
    explicit user-authorized operation; no automatic injury/safety recommendation
    is exposed.
11. **Offline reconciliation is idempotent.** Mutating schedule and set-log
    operations accept a stable client operation/idempotency key and return the
    authoritative resource/version. Replays do not duplicate sets, advance a
    sequence twice, or overwrite newer data silently. Conflicts are contractually
    visible to the client.
12. **The contract and verification surface are complete.** All resources obey
    authentication/ownership isolation, use `snake_case`, have migrations, DAO,
    service and handler coverage, and are documented in `docs/CONTRACTS.md` and
    generated OpenAPI. `go test ./...` passes. `scripts/smoke.sh` exercises the
    profile, program, schedule, set-check, status, and participation happy path;
    the normal smoke-toggle deployment check remains required before release.

## Constraints

- This PRD is the server dependency for the matching `gym-pulse-app` PRD.
- Server-side dates and end-of-day jobs use the persisted IANA timezone.
- Status and participation derivation must be transactional under concurrent
  client writes and safe to retry.
- Existing user data remains readable during migration. The legacy weekly-plan
  endpoints may remain temporarily, but new resources cannot overload their
  semantics or silently infer dated history from them.
- Medical/injury suitability is not a server-generated recommendation.
- `docs/CONTRACTS.md` is the source of truth; generated Swagger must agree.

## Out of scope

- Injury diagnosis, readiness scoring, or "safe workout" selection.
- Nutrition prescriptions or guaranteed body-composition outcomes.
- Wearable ingestion and automatic recovery-driven periodization.
- Coach/team tenancy, social challenges, and leaderboards.
- Immediate removal of legacy weekly-plan tables/endpoints.

## Inputs / References

- `docs/CONTRACTS.md` — current settings, plan, log, set-log, and stats behavior.
- `internal/dao/plan_dao.go`, `internal/service/log_svc.go`,
  `internal/dao/stats_dao.go` — current persistence and derivation seams.
- `migrations/010_create_weekly_plans.up.sql`,
  `migrations/011_create_set_logs.up.sql` — existing schema baseline.
- `scripts/smoke.sh`, `scripts/smoke-toggle.sh` — required runtime verification.
- App consumer: `../gym-pulse-app/prds/goal-based-training/prd.md`.
- [ACSM 2026 resistance-training position stand](https://pmc.ncbi.nlm.nih.gov/articles/PMC12965823/): the service should support individualized, consistent programs rather than encode one universal cadence.
- [HHS Physical Activity Guidelines](https://odphp.health.gov/our-work/nutrition-physical-activity/physical-activity-guidelines/current-guidelines/top-10-things-know): starter-program defaults should support regular muscle strengthening while remaining adaptable to ability.
- [Autoregulation systematic review](https://pubmed.ncbi.nlm.nih.gov/33520457/): flexible adjustment can be useful, but evidence does not justify implicit or medically framed adaptation.
