# Research: Rule-Based Program Engine

## Decisions

### R1. Provider selection and the status answer

- **Decision**: a new env `PROGRAM_GENERATION_PROVIDER` with values `rules` (default) and
  `model`. `model` requires `ANTHROPIC_API_KEY`; without a key the server logs a warning and
  falls back to `rules`. `GET /program-generation` reports `enabled: true` whenever the rules
  engine serves; `limit` and `remaining_today` stay in the answer (contract compatibility) and
  the ledger is only reserved and completed on the model path. The status answer gains
  `provider: "rules" | "model"` so the app can hide the "may take a moment" line for rules.
- **Why**: the owner wants the intelligence in GymPulse; the provider is now the exception. The
  app's "disabled" and "limit" states remain valid for the model path and never fire for rules.
- **Alternatives rejected**: removing the model path entirely (loses free-text requests for an
  operator who wants them; the adapter already exists and is tested); keeping the ledger for rules
  (no cost to bound, and the five-a-day cap would block "Try another").

### R2. Engine placement: a pure package behind the existing seam

- **Decision**: a new package `internal/engine` with no database, HTTP, or context dependencies:
  `tables.go` (programming tables), `engine.go` (`Build(input) (Draft, Explanation, error)`),
  `rationale.go`, and exhaustive tests. A thin adapter `service/program_generator_rules.go`
  implements the existing `DraftGenerator` interface, converts the engine draft into
  `model.RawGeneratedDraft`, and returns zero `GenerationUsage`. The service then runs
  `RepairGeneratedDraft` as it does for the model path, so spec 015's invariants hold twice:
  by construction and by validation.
- **Why**: constitution "Simple Go boundaries": the only new seam is one package with pure
  functions; the service, handler, routes, and DAO are unchanged apart from provider selection.
  Reusing the seam means the app contract does not change.
- **Alternatives rejected**: building drafts in `model` (would mix the programming tables with
  contract types); a second route (two contracts for one capability).

### R3. Programming tables (the intelligence as data)

Sources are those recorded in spec 015 research "Goal guide" (ACSM 2009 progression stand; Garber
2011; WHO 2020; Schoenfeld 2016 and 2017 on load, volume, frequency, rest; Cormie 2011 and NSCA
Essentials for power; Milanović 2015 for intervals; Donnelly 2009, Willis 2012, Clark 2015 for body
composition; Wilson 2012 and Schumann 2022 for concurrent training). Tables, all in `tables.go`:

- **Split by day count** (1 → 6 workouts; 7 days uses 6):

| Days | Default split | Hypertrophy | Conditioning |
|---|---|---|---|
| 1 | full | full | cardio+circuit |
| 2 | full, full | full, full | cardio, circuit |
| 3 | full ×3 | full ×3 | cardio, circuit, cardio |
| 4 | upper, lower, upper, lower | upper, lower, upper, lower | cardio, circuit, cardio, circuit |
| 5 | upper, lower, full, upper, lower | push, pull, legs, upper, lower | cardio, circuit, cardio, circuit, cardio |
| 6 | upper, lower, full, upper, lower, full | push, pull, legs, push, pull, legs | cardio, circuit, cardio, circuit, cardio, circuit |

  General health and body composition use the default split with a cardio slot appended per
  session (see cardio share). Power uses the default split with an explosive slot first.

- **Slot order per workout kind** (patterns, first to last; the engine fills as many as the
  session allows):
  - full: squat, horizontal_push, hinge, horizontal_pull, vertical_push, vertical_pull, core
  - upper: horizontal_push, horizontal_pull, vertical_push, vertical_pull, isolation_push, isolation_pull, core
  - lower: squat, hinge, lunge, hamstring_isolation, quad_isolation, calf, core
  - push: horizontal_push, vertical_push, incline_push, isolation_push, triceps, core
  - pull: hinge, vertical_pull, horizontal_pull, rear_delt, biceps, core
  - legs: squat, hinge, lunge, hamstring_isolation, quad_isolation, calf, core
  - circuit (conditioning): squat, horizontal_push, horizontal_pull, carry, core, locomotion
  - cardio (conditioning): locomotion ×1 (10 to 30 min) plus core
  - explosive prefix (power, non-beginner): explosive slot before the first strength slot

  Pattern names above are the catalog `pattern` values of data-model.md; `isolation_push`,
  `isolation_pull`, `triceps`, `biceps`, `rear_delt`, `calf`, `quad_isolation`,
  `hamstring_isolation`, `incline_push`, `lunge` are sub-patterns resolved by `pattern` plus
  `muscle_group` plus `mechanic` (table `slotFilters`).

- **Exercises per session** (strength slots, including core; cardio counts separately):

| Session minutes | Slots | Beginner cap |
|---|---|---|
| 20 | 3 | 3 |
| 30 | 4 | 3 |
| 45 | 5 | 4 |
| 60 | 6 | 5 |
| 75 | 7 | 5 |
| 90+ | 8 | 5 |

  A cardio finisher (general_health, body_composition, secondary conditioning) costs one slot
  when minutes < 45, otherwise it is added on top up to the 8-exercise cap.

- **Targets by goal and experience** (reps / sets / rest seconds; main compound first, then
  accessory):

| Goal | Beginner | Intermediate | Advanced |
|---|---|---|---|
| strength | 6–8 / 3 / 150 (acc. 8 / 3 / 90) | 5 / 4 / 180 (acc. 8 / 3 / 90) | 3–5 / 5 / 240 (acc. 6 / 3 / 120) |
| hypertrophy | 10 / 3 / 90 (iso 12 / 3 / 60) | 8–10 / 4 / 120 (iso 12–15 / 3 / 75) | 6–10 / 4 / 150 (iso 12–20 / 3 / 90) |
| power | strength-beginner + explosive 5 / 3 / 120 | explosive 3 / 4 / 180, main 3 / 4 / 180 | explosive 2 / 5 / 240, main 3 / 5 / 240 |
| conditioning | circuit 12 / 3 / 45 | 15 / 3 / 45 | 15 / 4 / 30 |
| general_health | 10 / 2 / 90 | 10 / 3 / 90 | 10 / 3 / 75 |
| body_composition | 12 / 3 / 75 | 10–12 / 3 / 75 | 10–15 / 4 / 60 |

  Ranges resolve to a single integer per exercise (the lower bound for main compounds, the upper
  bound for isolation) because the draft carries one `target_reps`.

- **Cardio** (`target_duration_seconds`, `target_sets` 1, `rest_seconds` 60):
  conditioning cardio day 20 min (intervals: 4 sets × 240 s, rest 120, on at most two days per
  week); general_health finisher 15 min; body_composition finisher 15 min, one interval day of
  4 × 180 s when a cardio machine or bodyweight locomotion is available; secondary conditioning
  finisher 10 min on at most two days.

- **Secondary goals** (at most about a third): secondary `conditioning` adds the 10-minute
  finisher; secondary `strength` adds one heavier compound at 5 reps / 4 sets / 180 s as the first
  strength slot; secondary `hypertrophy` adds one isolation slot at 12 reps; secondary `power`
  (non-beginner) adds one explosive slot at 3 reps / 3 sets / 180 s; secondary `general_health`
  and `body_composition` add the cardio finisher on at most two days. A secondary goal never
  changes the split or the primary targets.

- **Beginner rule**: compounds plus core only, no explosive entries, slot cap above, impact
  `high` entries excluded from cardio for `sedentary` activity.

### R4. Catalog programming metadata

- **Decision**: migration `032_add_exercise_catalog_programming.{up,down}.sql` adds `pattern`,
  `muscle_group`, `equipment_required TEXT[]`, and `impact` to `exercise_catalog` with CHECK
  constraints and seeds every current row by `(category, name)`; a final `NOT NULL` on the four
  columns keeps future seeds honest. `GET /api/v1/exercise-catalog` exposes the fields. The full
  mapping is in data-model.md.
- **Why**: category alone cannot tell a hinge from a vertical pull, or a barbell lift from a
  bodyweight one; the engine's eligibility test needs all four.
- **Assumptions recorded**: pull-ups, chin-ups, dips, hanging leg raises, and the ab wheel require
  a bar or wheel and are marked `full_gym`; kettlebell swings, Turkish get-ups, goblet squats, and
  carries accept `dumbbells`; Olympic-lift variants, thrusters, and sleds need `barbell` or
  `full_gym`; swimming needs `full_gym` (a pool is not in the equipment list). Bodyweight entries
  are eligible for everyone.

### R5. Determinism, variants, and day-to-day variety

- **Decision**: for each slot the eligible exercises are sorted by `(sort_order, name)`; the
  engine takes index `(variant + workoutIndex) mod len`. Equal input gives equal output; `variant`
  rotates every slot; the `workoutIndex` offset keeps three full-body days from repeating the same
  lift when alternatives exist. No randomness, no clock.
- **Alternatives rejected**: a seeded PRNG (harder to reason about in tests and in support
  conversations); storing the last draft to avoid repeats (spec non-goal: no draft history).

### R6. Constraints

- **Decision**: `avoid` accepts pattern values from data-model.md (and the app shows a chip set:
  "No hinges", "No overhead pressing", "No jumping", "No deep squats", "No running"); `focus`
  accepts `upper`, `lower`, `core`; both validated in `model.NormalizeGenerationConstraints`
  with `422 VALIDATION_ERROR` on `avoid` or `focus`. Focus reorders the slot list so two thirds of
  strength slots come from the focus area; the engine keeps at least one slot for the other area.
  When a workout ends with fewer than three eligible exercises the service answers 422 on `avoid`
  with "Those limits leave too little to build a workout from; drop one and try again."
- **Why**: a bounded enum is what rules can honour; the message stays in athlete language.

### R7. Rationale

- **Decision**: `rationale.go` assembles at most four sentences: split and days ("Three full-body
  days on Monday, Wednesday, Friday."), main lifts ("Built around back squats, bench presses, and
  barbell rows."), secondary and focus ("Conditioning gets a 10-minute finisher twice a week."),
  and what was honoured or dropped ("No hinges, as you asked." / "No band exercises in the catalog
  yet, so bodyweight stands in."). Capped at 500 characters; no outcome, diet, or medical words
  (a test greps a deny list).

### R8. Privacy, logs, performance

- **Decision**: the engine receives the same `GenerationInput` as the model path plus `Avoid`,
  `Focus`, and `Variant`; nothing leaves the process. Logs carry provider, duration, split, and
  workout count. Target p95 under 200 ms: the catalog is one indexed read already made by the
  service; the engine is O(slots × catalog).

### R9. Rollout and compatibility

- **Decision**: additive. An app that sends only `request` receives an engine draft (free text
  ignored, rationale unchanged). The app amendment to spec 019 replaces the request field with
  constraint chips and reads `provider` from the status answer to drop the wait copy. Deploy order:
  gym-pulse-api#39 → this feature → app amendment. Rollback: set
  `PROGRAM_GENERATION_PROVIDER=model` (with a key) or revert; the migration's down drops the four
  columns.

### Implementation notes (2026-11-02)

- Thin workouts are topped up before the engine gives up: any remaining pattern, then isolation
  work (even for beginners), then a short cardio bout. `ErrTooConstrained` therefore fires only
  when nothing eligible is left, which with the six-pattern cap on `avoid` means bodyweight-only
  equipment.
- Focus keeps its two-thirds share when `avoid` thins the focus area by appending extra slots
  from that area (isolation and repeated patterns, which pick different entries).
- Beginners never get explosive entries, power goal included (US1/AC3 wins over the edge-case
  wording, now aligned).
- The seam carries no `Explanation`, so the success log records provider, model id, duration,
  and workout count rather than the split (FR-011 "split only" is read as "nothing more than
  that").
- Engine-built drafts carry `roadmap.model = "gympulse-rules-v1"`.

### Evidence amendment (2026-11-07)

Decisions and sources are in the workspace brief
`docs/briefs/2026-11-07-training-regimes-research.md`. Table values added here:

- **Weekly set bands per muscle group** (Schoenfeld 2017; Pelland 2025/2026; minimum effective
  dose reviews): strength 6–16, hypertrophy 10–20, power 6–12, conditioning 4–12,
  general_health 6–12, body_composition 8–16. Counted as direct sets: an exercise's sets go to
  its `muscle_group`.
- **Heavy / moderate undulation** (2022 volume-equated periodization meta-analysis): strength
  heavy 5 × 4 × 180 s, moderate 8 × 3 × 120 s; hypertrophy heavy 6–8 × 4 × 150 s, moderate
  10–12 × 3 × 90 s; power keeps the explosive opener and alternates its strength work the same way
  as strength. Beginners use the moderate day only.
- **Effort** (Robinson 2024; Refalo 2023, 2025): 1–2 reps in reserve for hypertrophy and body
  composition, 2–3 for strength and power main work, "keep moving" for circuits, "stop with 2–3
  in reserve" for beginners.
- **Rep ranges**: strength main 3–5 (advanced), 4–6 (intermediate), 6–8 (beginner); accessories
  6–10; hypertrophy 6–8 heavy / 10–12 moderate, isolation 12–20; general health 8–12; body
  composition 10–15; circuits 12–15.
- **Minimalist budget** (minimum effective dose reviews): under 20 minutes, three compounds, two
  sets each, usual rest.
- **Cardio intensity lines** (Garber 2011; Milanović 2015): steady "Easy pace, you can talk.";
  intervals "Hard efforts: N × M minutes with easy minutes between."; finisher "Easy to moderate."
- **Progression line** (Plotkin 2022): "Add a rep each week until you reach the top of the range,
  then add a little load."

## Resolved Technical Questions

- Migration number: `032`; `031` is spec 015's ledger.
- `CatalogExercise` gains `Pattern`, `MuscleGroup`, `EquipmentRequired []string`, `Impact`; the
  DAO `List` selects them; the mock catalog in the app gains the same fields later.
- `GenerationInput` gains `Avoid []string`, `Focus string`, `Variant int`; `GenerateProgramRequest`
  gains `avoid`, `focus`, `variant`; `GenerateProgramResponse` gains `provider` and echoes
  `avoid` and `focus`.
- The handler's extended write deadline stays (harmless for the engine, needed for the model).
- Smoke: step 16b changes from "503 disabled" to "200 draft with three workouts" since the engine
  serves without a key; 16c asserts determinism (two identical calls, identical bodies).
