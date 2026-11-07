# Data Model: Rule-Based Program Engine

## `exercise_catalog` additions (migration 032)

| Field | Type | Rules |
|---|---|---|
| `pattern` | TEXT | Not null; `CHECK (pattern IN ('squat', 'hinge', 'lunge', 'horizontal_push', 'vertical_push', 'horizontal_pull', 'vertical_pull', 'carry', 'core', 'explosive', 'locomotion', 'isolation'))` |
| `muscle_group` | TEXT | Not null; `CHECK (muscle_group IN ('chest', 'back', 'shoulders', 'arms', 'quads', 'hamstrings', 'glutes', 'calves', 'core', 'full_body'))` |
| `equipment_required` | TEXT[] | Not null; every element in the profile equipment set `('bodyweight', 'bands', 'dumbbells', 'barbell', 'machines', 'cardio_machine', 'full_gym')`; `bodyweight` means always eligible |
| `impact` | TEXT | Not null; `CHECK (impact IN ('low', 'high'))` |

Index `idx_exercise_catalog_pattern(pattern)`. RLS and privileges unchanged (read-only catalog).
Down migration drops the four columns and the index. Existing rows are updated by `(category,
name)` in the up migration before the `NOT NULL` constraints are added, so the migration fails
loudly if a row is missed.

Eligibility: an entry is eligible for a profile when `equipment_required ⊆ profile.equipment`,
or `profile.equipment` contains `full_gym`, or `equipment_required = {bodyweight}`.

## Seed mapping (all 79 entries)

| Name | Category | Pattern | Muscle group | Equipment required | Impact |
|---|---|---|---|---|---|
| Barbell Bench Press | push | horizontal_push | chest | barbell | low |
| Incline Barbell Bench Press | push | horizontal_push | chest | barbell | low |
| Dumbbell Bench Press | push | horizontal_push | chest | dumbbells | low |
| Incline Dumbbell Press | push | horizontal_push | chest | dumbbells | low |
| Overhead Press | push | vertical_push | shoulders | barbell | low |
| Seated Dumbbell Shoulder Press | push | vertical_push | shoulders | dumbbells | low |
| Close-Grip Bench Press | push | horizontal_push | arms | barbell | low |
| Dips | push | vertical_push | chest | full_gym | low |
| Push-Up | push | horizontal_push | chest | bodyweight | low |
| Cable Fly | push | isolation | chest | machines | low |
| Dumbbell Fly | push | isolation | chest | dumbbells | low |
| Lateral Raise | push | isolation | shoulders | dumbbells | low |
| Triceps Pushdown | push | isolation | arms | machines | low |
| Overhead Triceps Extension | push | isolation | arms | dumbbells | low |
| Deadlift | pull | hinge | hamstrings | barbell | low |
| Pull-Up | pull | vertical_pull | back | full_gym | low |
| Chin-Up | pull | vertical_pull | back | full_gym | low |
| Lat Pulldown | pull | vertical_pull | back | machines | low |
| Barbell Row | pull | horizontal_pull | back | barbell | low |
| Dumbbell Row | pull | horizontal_pull | back | dumbbells | low |
| Seated Cable Row | pull | horizontal_pull | back | machines | low |
| T-Bar Row | pull | horizontal_pull | back | full_gym | low |
| Face Pull | pull | isolation | shoulders | machines | low |
| Rear Delt Fly | pull | isolation | shoulders | dumbbells | low |
| Barbell Curl | pull | isolation | arms | barbell | low |
| Dumbbell Curl | pull | isolation | arms | dumbbells | low |
| Hammer Curl | pull | isolation | arms | dumbbells | low |
| Preacher Curl | pull | isolation | arms | machines | low |
| Back Squat | legs | squat | quads | barbell | low |
| Front Squat | legs | squat | quads | barbell | low |
| Goblet Squat | legs | squat | quads | dumbbells | low |
| Romanian Deadlift | legs | hinge | hamstrings | barbell | low |
| Leg Press | legs | squat | quads | machines | low |
| Walking Lunge | legs | lunge | quads | bodyweight | low |
| Bulgarian Split Squat | legs | lunge | quads | bodyweight | low |
| Hip Thrust | legs | hinge | glutes | barbell | low |
| Step-Up | legs | lunge | quads | bodyweight | low |
| Leg Extension | legs | isolation | quads | machines | low |
| Lying Leg Curl | legs | isolation | hamstrings | machines | low |
| Standing Calf Raise | legs | isolation | calves | bodyweight | low |
| Seated Calf Raise | legs | isolation | calves | machines | low |
| Hip Abduction | legs | isolation | glutes | machines | low |
| Sumo Deadlift | lower | hinge | hamstrings | barbell | low |
| Hack Squat | lower | squat | quads | machines | low |
| Good Morning | lower | hinge | hamstrings | barbell | low |
| Single-Leg Press | lower | squat | quads | machines | low |
| Nordic Hamstring Curl | lower | isolation | hamstrings | bodyweight | low |
| Arnold Press | upper | vertical_push | shoulders | dumbbells | low |
| Machine Chest Press | upper | horizontal_push | chest | machines | low |
| Machine Row | upper | horizontal_pull | back | machines | low |
| Landmine Press | upper | vertical_push | shoulders | barbell | low |
| Cable Crossover | upper | isolation | chest | machines | low |
| Plank | core | core | core | bodyweight | low |
| Side Plank | core | core | core | bodyweight | low |
| Hanging Leg Raise | core | core | core | full_gym | low |
| Ab Wheel Rollout | core | core | core | full_gym | low |
| Cable Crunch | core | core | core | machines | low |
| Russian Twist | core | core | core | bodyweight | low |
| Dead Bug | core | core | core | bodyweight | low |
| Bicycle Crunch | core | core | core | bodyweight | low |
| Clean and Press | full | explosive | full_body | barbell | high |
| Power Clean | full | explosive | full_body | barbell | high |
| Kettlebell Swing | full | explosive | glutes | dumbbells | low |
| Thruster | full | explosive | full_body | barbell | low |
| Burpee | full | locomotion | full_body | bodyweight | high |
| Farmer's Carry | full | carry | full_body | dumbbells | low |
| Turkish Get-Up | full | carry | full_body | dumbbells | low |
| Treadmill Run | cardio | locomotion | full_body | cardio_machine | high |
| Incline Treadmill Walk | cardio | locomotion | full_body | cardio_machine | low |
| Outdoor Run | cardio | locomotion | full_body | bodyweight | high |
| Stationary Bike | cardio | locomotion | full_body | cardio_machine | low |
| Assault Bike | cardio | locomotion | full_body | cardio_machine | low |
| Rowing Machine | cardio | locomotion | full_body | cardio_machine | low |
| Elliptical | cardio | locomotion | full_body | cardio_machine | low |
| Stairmaster | cardio | locomotion | full_body | cardio_machine | low |
| Jump Rope | cardio | locomotion | full_body | bodyweight | high |
| Swimming | cardio | locomotion | full_body | full_gym | low |
| HIIT Circuit | cardio | locomotion | full_body | bodyweight | high |
| Sled Push | cardio | locomotion | full_body | full_gym | low |

Sub-pattern filters used by the slot lists (research R3), resolved in code, never stored:
`isolation_push` = isolation ∧ muscle in {chest, shoulders}; `triceps` = isolation ∧ arms ∧
category push; `biceps` = isolation ∧ arms ∧ category pull; `rear_delt` = isolation ∧ shoulders ∧
category pull; `calf` = isolation ∧ calves; `quad_isolation` = isolation ∧ quads;
`hamstring_isolation` = isolation ∧ hamstrings; `incline_push` = horizontal_push ∧ name contains
"Incline"; `lunge` = lunge.

## Request and response (additive to spec 015)

```json
{
  "request": "ignored by the engine",
  "secondary_goals": ["conditioning"],
  "avoid": ["hinge", "vertical_push"],
  "focus": "upper",
  "variant": 1,
  "proposed_profile": { "...": "spec 015" }
}
```

| Field | Rules |
|---|---|
| `avoid` | Optional array, at most 6 distinct pattern values; unknown → 422 `avoid` |
| `focus` | Optional, one of `upper`, `lower`, `core`; unknown → 422 `focus` |
| `variant` | Optional integer ≥ 0, at most 1000; otherwise 422 `variant` |

Response adds `provider` (`rules` or `model`), and echoes `avoid` (array, empty when none) and
`focus` (string or null). `GET /program-generation` adds `provider`.

## Engine types (`internal/engine`)

```go
type Input struct {
    PrimaryGoal, Experience, UsualActivity string
    SecondaryGoals, Equipment, Avoid []string
    Focus string
    AvailableDays []int
    SessionMinutes, WorkoutCap, Variant int
    Catalog []model.CatalogExercise
}

type Draft struct {            // converted to model.RawGeneratedDraft by the adapter
    Name string
    Workouts []Workout         // Name, PreferredWeekday, Exercises []Exercise
}

type Explanation struct {      // feeds rationale.go
    Split string; MainLifts []string; SecondaryNote, FocusNote string
    Honoured, Dropped []string // avoided patterns honoured; slots or finishers dropped and why
}
```

`Build(in Input) (Draft, Explanation, error)`; the only error is `ErrTooConstrained` (fewer than
three eligible exercises for some workout), which the service maps to 422 on `avoid`.

## Draft invariants

Unchanged from spec 015 data-model.md; the adapter output passes `RepairGeneratedDraft` without
repairs (a test asserts the repaired draft equals the engine draft).
