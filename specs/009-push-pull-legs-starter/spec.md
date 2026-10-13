# Feature Specification: Push/Pull/Legs Starter Program

**Feature Branch**: `feat/push-pull-legs-starter`

**Feature ID**: `gympulse-push-pull-legs-starter`

**Sibling Spec**: N/A (API-only catalog content; the app discovers starters through the existing contract)

**Created**: 2026-10-12

**Status**: Approved

**Input**: User description: "Seed a PPL starter"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Choose a Push/Pull/Legs starter (Priority: P1)

An athlete whose goal is hypertrophy, who trains 3 to 6 days a week with barbell, dumbbell, machine, or full-gym equipment, sees a "Push Pull Legs" starter in the catalog and can copy it into a personal program like any other starter.

**Why this priority**: The catalog only offers two-day full-body starters today. PPL is the most requested split and needs no new product surface.

**Independent Test**: Run migrations on an empty database, call `GET /api/v1/starter-programs?primary_goal=hypertrophy&available_days=3`, and verify the response includes `push-pull-legs` version 1 with three workouts of five catalog-linked exercises each.

**Acceptance Scenarios**:

1. **Given** a migrated database, **When** the starter catalog is listed with no filters, **Then** `push-pull-legs` v1 appears alongside the six foundation starters and the foundation starters are unchanged.
2. **Given** an athlete with 3, 4, 5, or 6 available days, **When** the catalog is filtered by `available_days`, **Then** the starter is included; with 1 or 2 days it is excluded.
3. **Given** an athlete whose only equipment is `bodyweight` or `bands`, **When** the catalog is filtered by equipment, **Then** the starter is excluded.
4. **Given** the athlete copies the starter and materializes a week with 6 available days, **When** the schedule is generated, **Then** Push, Pull, Legs repeat in order twice using the existing rotation behavior.
5. **Given** the migration is applied twice, **When** it runs again, **Then** no duplicate rows are created.

### Edge Cases

- Every seeded exercise name must match an `exercise_catalog` row so `catalog_id` is never silently NULL.
- Rolling the migration back cascades the starter's workouts and exercises; copied user programs keep their snapshot with `starter_program_id` set to NULL (existing FK behavior).
- Recommendation ordering is by goal then slug, so `hypertrophy-foundation` still ranks first for hypertrophy athletes; this starter is discoverable but not auto-recommended.

## Non-Goals *(mandatory)*

- No new API fields, filters, statuses, or error codes.
- No app changes; the app renders catalog entries generically.
- No upper/lower or other splits in this change.
- No change to the recommendation ranking rule.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Seed `starter_programs` row `push-pull-legs` v1, goal `hypertrophy`, 3–6 days, all experience levels, equipment `barbell|dumbbells|machines|full_gym`, 60 minutes.
- **FR-002**: Seed three `starter_workouts` (Push seq 1 weekday 1, Pull seq 2 weekday 3, Legs seq 3 weekday 5).
- **FR-003**: Seed five strength `starter_exercises` per workout with sets, reps, and rest, each joined to the exercise catalog by name and category.
- **FR-004**: The migration is idempotent (`ON CONFLICT DO NOTHING`) and has a down migration that removes exactly this starter version.
- **FR-005**: `docs/DATABASE_DESIGN.md` lists the new migration as a seed source for `starter_programs`.

### Key Entities

- `starter_programs`, `starter_workouts`, `starter_exercises` (existing; content only).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The catalog returns seven active starters after migration.
- **SC-002**: All 15 seeded exercises have non-null `catalog_id`.
- **SC-003**: `go test ./...` and `./scripts/validate-database-design.sh` pass.

## Assumptions

- Hypertrophy is the primary goal classification for a PPL split; athletes with other goals can still select it when they clear the goal filter.
- `docs/CONTRACTS.md` needs no change because the response shape and filters are unchanged.
