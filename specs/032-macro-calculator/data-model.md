# Data Model: Macro calculator and meal template

## Table `public.nutrition_profiles` (migration 047)

One row per athlete: the calculator inputs they saved and the targets computed at that save.

| Column | Type | Rules |
| --- | --- | --- |
| `user_id` | UUID PK | FK `auth.users(id) ON DELETE CASCADE` |
| `unit_system` | TEXT | `imperial` or `metric` |
| `weight` | NUMERIC(5,1) | in the unit system; 30–660 (service enforces the per-system range) |
| `height` | NUMERIC(4,1) | total inches or centimetres; 48–244 |
| `age_years` | SMALLINT | 18–100 |
| `sex` | TEXT | `male` or `female` |
| `strength_days` | SMALLINT | 0–7 |
| `cardio_days` | SMALLINT | 0–7 |
| `activity_level` | TEXT NULL | `light`, `moderate`, `very`; NULL means "use the suggestion" |
| `goal` | TEXT | `fat_loss`, `recomp`, `maintenance`, `muscle_gain` |
| `calorie_adjustment_kcal` | INTEGER | −500 to 400 (service enforces the goal's range) |
| `protein_g_per_lb` | NUMERIC(3,2) | 0.70–1.10 |
| `fat_g_per_lb` | NUMERIC(3,2) | 0.25–0.45 |
| `carb_cycling` | BOOLEAN | default false |
| `preferred_proteins` | TEXT[] | default `{}`; at most 20 |
| `preferred_carbs` | TEXT[] | default `{}`; at most 20 |
| `exclusions` | TEXT[] | default `{}`; at most 20 |
| `meal_choices` | JSONB | array of at most 4 `{slot, protein?, carb?, fat?}`; default `[]` |
| `bmr_kcal`, `tdee_kcal`, `target_kcal` | INTEGER | > 0 |
| `protein_g`, `carbs_g`, `fat_g` | INTEGER | ≥ 0 |
| `revision` | BIGINT | default 1, > 0 |
| `created_at`, `updated_at` | TIMESTAMPTZ | default `now()` |

- The primary key is the only index: every read and write is by `user_id`.
- Food ids and tags are validated by the service against the fixed list, not by the database, so
  the list can grow without a migration. A stored id the list no longer knows is ignored on read.
- New table only. No backfill, no change to existing tables. Down migration drops the table.
- Ends with `SELECT gym_pulse_private.secure_table('public.nutrition_profiles');`.
- Account deletion deletes the row explicitly (and by cascade). The data export adds
  `nutrition.calculator_profile` (object or `null`).

## In-memory model (`internal/nutritionplan`)

- `Input`: the request fields after normalization.
- `Targets`: `BMRKcal`, `TDEEKcal`, `AdjustmentKcal`, `TargetKcal`, `ProteinG`, `CarbsG`, `FatG`.
- `Food`: `ID`, `Name`, `Role`, `Tags`, `Unit`, `GramsPerUnit`, `Step`, per-100 g `Protein`,
  `Carbs`, `Fat`, `Kcal`, `FDCID`, `Slots` (meal affinity), `Fast` (Meal 3), `LeanPartner`.
- `Plan`: targets, macro breakdown, activity, optional cycling, notices, meal templates.

## Idempotency

Scope `nutrition-profile/put`, resource type `nutrition_profile`, under the existing `nutrition:`
per-user lock. The ledger stores the outcome (created or replaced) and the stored row; the response
body is rebuilt from the row, so a replay returns the same plan.
