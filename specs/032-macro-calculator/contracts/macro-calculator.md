# Contract: Macro calculator and meal template (contract 1.11.0, additive)

This is the design contract. The shipped text lives in `docs/CONTRACTS.md`, section
"Macro calculator".

All four routes require authentication and are scoped to the caller. Validation failures are `422
VALIDATION_ERROR` with `details.field`. Every number is a whole number unless stated.

## `CalculatorInput`

| Field | Type | Rules |
| --- | --- | --- |
| `unit_system` | string | `imperial` or `metric`; required |
| `weight` | number | lb 66–660 or kg 30–300; at most one decimal; required |
| `height` | number | total in 48–96 or cm 122–244; at most one decimal; required |
| `age_years` | integer | 18–100; required |
| `sex` | string | `male` or `female`; required |
| `strength_days` | integer | 0–7; required |
| `cardio_days` | integer | 0–7; required |
| `activity_level` | string | optional override: `light`, `moderate`, `very` |
| `goal` | string | `fat_loss`, `recomp`, `maintenance`, `muscle_gain`; required |
| `calorie_adjustment_kcal` | integer | optional; inside the goal's range; defaults to the goal's default |
| `protein_g_per_lb` | number | optional; 0.70–1.10, two decimals; default 0.95 (`fat_loss`, `recomp`) or 0.85 |
| `fat_g_per_lb` | number | optional; 0.25–0.45, two decimals; default 0.33 |
| `carb_cycling` | boolean | optional; default `false` |
| `preferred_proteins` | string[] | optional; food ids with role `protein`; at most 20 |
| `preferred_carbs` | string[] | optional; food ids with role `carb`; at most 20 |
| `exclusions` | string[] | optional; exclusion tags; at most 20 |
| `meal_choices` | object[] | optional; at most one per `slot` (1–4): `{ "slot": 2, "protein": "<food>", "carb": "<food>", "fat": "<food>" }`, each food optional, of the matching role, and not excluded |

Unknown JSON fields are ignored, as elsewhere. Duplicate list entries are collapsed; a preferred
food that carries an excluded tag is ignored.

## `NutritionPlan`

```json
{
  "input": { "...": "the CalculatorInput with every default resolved; activity_level is null when not overridden" },
  "activity": { "suggested_level": "moderate", "level": "moderate", "multiplier": 1.55 },
  "bmr_kcal": 1783,
  "tdee_kcal": 2763,
  "calorie_adjustment_kcal": 0,
  "target_kcal": 2763,
  "macros": {
    "protein": { "g": 153, "kcal": 612, "pct": 22 },
    "carbs":   { "g": 405, "kcal": 1620, "pct": 59 },
    "fat":     { "g": 59,  "kcal": 531, "pct": 19 }
  },
  "carb_cycling": null,
  "notices": [],
  "meal_templates": [
    {
      "day_type": "standard",
      "energy_kcal": 2763, "protein_g": 153, "carbs_g": 405, "fat_g": 59,
      "meals": [
        {
          "slot": 1, "name": "Breakfast",
          "target": { "energy_kcal": 722, "protein_g": 39, "carbs_g": 101, "fat_g": 18 },
          "items": [
            {
              "food": "eggs", "name": "Eggs, whole, hard-boiled", "role": "protein",
              "amount": 2, "unit": "count", "grams": 100,
              "energy_kcal": 155, "protein_g": 12.6, "carbs_g": 1.1, "fat_g": 10.6,
              "source": "usda", "fdc_id": 173424
            },
            { "food": "egg_whites", "name": "Egg whites", "role": "protein", "amount": 3, "unit": "count", "grams": 99, "...": "..." },
            { "food": "oats", "name": "Oats, dry", "role": "carb", "amount": 1.75, "unit": "cup", "grams": 141.8, "...": "..." }
          ],
          "provided": { "energy_kcal": 743, "protein_g": 42.1, "carbs_g": 97.8, "fat_g": 20 }
        }
      ],
      "provided": { "energy_kcal": 2705, "protein_g": 156.7, "carbs_g": 400.7, "fat_g": 60.9 }
    }
  ]
}
```

- `pct` values are shares of `4·protein + 4·carbs + 9·fat` and sum to 100.
- `carb_cycling` is `null`, or
  `{ "training_days": 5, "rest_days": 2, "training": { "energy_kcal", "protein_g", "carbs_g", "fat_g" }, "rest": { ... }, "weekly_average_kcal": 2763 }`.
  When present, `meal_templates` holds `training` then `rest`; otherwise one `standard` template.
- `notices` is a list of codes, never `null`: `carbs_floor` (protein and fat alone meet or exceed
  the target; carbohydrate is 0), `below_bmr` (the target is under resting energy),
  `cycling_not_applied` (cycling was requested with 0 or 7 training days).
- Item energy is the food's published USDA energy for that weight, so `provided.energy_kcal` can
  differ slightly from 4/4/9 of the provided grams.
- Item `amount` is in `unit` (`oz`, `g`, `cup`, `count`) at the food's step; `grams` is the weight
  used for the item's values, which have one decimal. `provided` is the sum of the items.
- Meal energy targets are `4·protein + 4·carbs + 9·fat` of the meal's gram targets.

## `GET /api/v1/nutrition/calculator/options` → `200`

```json
{
  "goals": [ { "id": "fat_loss", "min_adjustment_kcal": -500, "max_adjustment_kcal": -300, "default_adjustment_kcal": -400, "default_protein_g_per_lb": 0.95 } ],
  "activity_levels": [ { "id": "light", "multiplier": 1.375, "min_sessions": 0, "max_sessions": 3 } ],
  "protein_g_per_lb": { "min": 0.7, "max": 1.1, "step": 0.05 },
  "fat_g_per_lb": { "min": 0.25, "max": 0.45, "step": 0.01, "default": 0.33 },
  "exclusions": [ "rice", "dairy", "nuts", "gluten", "eggs", "red_meat" ],
  "foods": [ { "id": "chicken_breast", "name": "Chicken breast, grilled", "role": "protein", "tags": [], "unit": "oz", "step": 0.5, "selectable": true, "source": "usda", "fdc_id": 171534, "per_100g": { "energy_kcal": 151, "protein_g": 30.5, "carbs_g": 0, "fat_g": 3.2 } } ],
  "meals": [ { "slot": 1, "name": "Breakfast" } ]
}
```

`selectable` is `false` for a companion food (egg whites) that appears only beside another food.
`max_sessions` is `null` on the last level.

## `POST /api/v1/nutrition/calculate` → `200 NutritionPlan`

Body: `CalculatorInput`. Stores nothing; needs no `Idempotency-Key`.

## `PUT /api/v1/nutrition/profile` → `201` created, `200` replaced

Body: `CalculatorInput` plus `expected_revision` (0 when no plan is saved) and `operation_key`
(1–128 characters, equal to the `Idempotency-Key` header). The service recomputes the targets from
the input; the client sends no computed number. Response: `SavedNutritionPlan`, the
`NutritionPlan` plus `"revision"`, `"created_at"`, `"updated_at"`.

- Stale `expected_revision` → `409 REVISION_CONFLICT` with the stored plan under `resource`.
- Same key, same body → the original status and body; same key, different body →
  `409 IDEMPOTENCY_CONFLICT`.
- Saving never changes the nutrition target, days, entries, or saved meals.

## `GET /api/v1/nutrition/plan` → `200 SavedNutritionPlan`

The stored input and the targets computed when it was saved, with templates built from those
targets. No saved plan → the shared `404 NOT_FOUND`.

## Data export

`nutrition.calculator_profile`: `null`, or the stored input fields plus `bmr_kcal`, `tdee_kcal`,
`target_kcal`, `protein_g`, `carbs_g`, `fat_g`, and `updated_at`. `schema_version` is unchanged.
