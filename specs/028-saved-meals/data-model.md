# Data Model: Saved meals

Migration `039_create_saved_meals` (next free number on `main` as of 2026-11-15; re-check before
writing it). Two new tables. No existing table changes.

## `saved_meals` (owner-scoped)

| Field | Type | Rules |
|---|---|---|
| `id` | UUID | Primary key, application-generated |
| `user_id` | UUID | Not null, `auth.users(id)` cascade delete |
| `name` | TEXT | Not null; `CHECK (char_length(name) BETWEEN 1 AND 60)`; stored trimmed |
| `revision` | BIGINT | Not null, default 1, `CHECK (revision > 0)`; advances on name or item change only |
| `last_logged_at` | TIMESTAMPTZ | Nullable; set by the log transaction |
| `created_at`, `updated_at` | TIMESTAMPTZ | Server generated |

Indexes: `ux_saved_meals_user_name(user_id, lower(name))` unique (name rule, and the owner lookup
prefix for the list and the count).

## `saved_meal_items` (owned through the meal)

| Field | Type | Rules |
|---|---|---|
| `id` | UUID | Primary key |
| `meal_id` | UUID | Not null, `saved_meals(id)` cascade delete |
| `position` | SMALLINT | Not null; `CHECK (position BETWEEN 0 AND 19)` |
| `catalog_food_id` | UUID | Nullable; `food_catalog(id)` restrict |
| `user_food_id` | UUID | Nullable; `user_foods(id)` restrict |
| `serving_label` | TEXT | Not null; `CHECK (char_length(serving_label) BETWEEN 1 AND 60)` |
| `serving_g` | NUMERIC(7,1) | Not null; `CHECK (serving_g > 0 AND serving_g <= 5000)` |
| `amount` | NUMERIC(5,2) | Not null; `CHECK (amount BETWEEN 0.1 AND 50)` |

Checks: `ck_saved_meal_items_food`: exactly one of `catalog_food_id`, `user_food_id`.

Indexes: `ux_saved_meal_items_position(meal_id, position)` unique (order, and the 20-item cap with
the position check); `ux_saved_meal_items_food_serving(meal_id, COALESCE(catalog_food_id,
user_food_id), serving_g, serving_label)` unique (no duplicate food and serving in one meal);
`ix_saved_meal_items_user_food(user_food_id) WHERE user_food_id IS NOT NULL` (foreign-key side for
the restrict check and the private-food ownership join).

Both tables: RLS enabled, Data API roles revoked, `gym_pulse_api_access` policy and DML grants for
the runtime role, as migration 030 does for the food tables.

## Ownership

A private food on an item must belong to the meal's owner. The create and update transactions read
foods through `getFood`/`ListFoodsByIDs`, which filter `user_foods` by `user_id`, before inserting,
so a foreign private food never reaches the insert. Meals are always read and locked by
`(user_id, id)`.

## Transactions (DAO, all under `lockUserDomain(nutrition:)`)

**Create**: replay from the ledger (`saved-meals/post`) → count the athlete's meals (≥ 50 →
`SAVED_MEAL_LIMIT_REACHED`) → load the items' foods in one query (missing, foreign, or archived →
`422` with the item index) → `ResolveServing` per item → insert meal (revision 1) and items →
ledger `201` with the response body → commit. A unique violation on the name index maps to `422`
on `name`.

**Update** (`saved-meals/put`): replay → lock the meal `FOR UPDATE` (missing or foreign → shared
`404`) → revision check (stale → `REVISION_CONFLICT` with the meal as `resource`) → validate foods
as on create → update name, delete and re-insert items, `revision + 1` → ledger → commit.

**Delete** (`saved-meals/delete`): replay → lock → revision check → delete (items cascade) →
ledger `204` → commit.

**Log** (`saved-meals/log`):

1. Replay from the ledger.
2. Lock the meal `FOR UPDATE`; missing or foreign → shared `404`.
3. Items are the request's override list when present, otherwise the saved items in position order.
4. Load every item's food in one query. A food that does not resolve → `422` with the item index.
   An archived private food is accepted only when the saved meal contains that food.
5. Per item: `ResolveServing`, then `EntryValues(per_100g, grams, amount)`; an implausible food →
   `422` with the item index.
6. `lockNutritionDay`; a `manual` or `apple_health` day with no entries becomes one `quick_add`
   entry, returned under `converted` (spec 014 rule, same code path).
7. One multi-row insert into `food_entries`, item `i` with `logged_at = base + i µs`.
8. `recomputeNutritionDay` once; set `saved_meals.last_logged_at = now()` (revision unchanged).
9. Ledger `201` with the response body; commit.

Any error rolls the whole transaction back, so the day holds all of the meal's entries once or
none.

## Read

`ListMeals`: one query for the athlete's meals joined to their items, ordered `last_logged_at DESC
NULLS LAST, created_at DESC, id`, items by `position`; one query for the distinct foods. At most 50
meals × 20 items. Item values and meal totals are computed in the service from each food's current
`per_100g`.

## Deletion and export

Account deletion deletes `saved_meals` (items cascade) before `user_foods`. The export reads meals
and items with food names in the existing export transaction and appends `nutrition.saved_meals`.

## API models

```json
{
  "id": "<uuid>", "name": "Usual breakfast",
  "items": [
    {
      "food": { "...": "Food" },
      "serving": { "label": "1 cup (81 g)", "grams": 81 }, "amount": 1,
      "available": true,
      "energy_kcal": 307, "protein_g": 10.7, "carbs_g": 54.8, "fat_g": 5.3
    }
  ],
  "totals": { "energy_kcal": 532, "protein_g": 36.1, "carbs_g": 84.6, "fat_g": 6.9 },
  "revision": 1, "last_logged_at": null,
  "created_at": "<RFC3339>", "updated_at": "<RFC3339>"
}
```

`totals` is `null` and the item omits its four values when `available` is `false`. `items` is
never empty.
