# Data Model: Food search, barcode lookup, and food entries

Migration `030_create_food_logging` (PR #36 merged holding 027–028; nutrition renumbers to 029; see `research.md`).

## `food_catalog` (shared; no owner)

| Field | Type | Rules |
|---|---|---|
| `id` | UUID | Primary key, application-generated |
| `source` | TEXT | `CHECK (source IN ('usda', 'open_food_facts'))` |
| `source_id` | TEXT | Not null; `fdcId` or OFF `code` |
| `data_type` | TEXT | Nullable; `foundation`, `sr_legacy`, `branded`, `off_product` |
| `gtin` | CHAR(14) | Nullable; zero-padded GTIN-14 |
| `name` | TEXT | Not null, 1–200 chars |
| `brand` | TEXT | Nullable, ≤ 120 chars |
| `energy_kcal_100g` | NUMERIC(6,1) | Not null; `CHECK (BETWEEN 0 AND 900)` |
| `protein_g_100g` | NUMERIC(6,1) | Not null; `CHECK (BETWEEN 0 AND 100)` |
| `carbs_g_100g` | NUMERIC(6,1) | Not null; same bound |
| `fat_g_100g` | NUMERIC(6,1) | Not null; same bound |
| `servings` | JSONB | Not null; list of `{label, grams}`; always includes `100 g` |
| `search_vector` | TSVECTOR | Generated always as `to_tsvector('simple', name || ' ' || coalesce(brand, ''))`, stored |
| `fetched_at` | TIMESTAMPTZ | Not null; last successful source read |
| `created_at`, `updated_at` | TIMESTAMPTZ | Server generated |

Indexes: `ux_food_catalog_source_id(source, source_id)` unique; `ix_food_catalog_gtin(gtin) WHERE
gtin IS NOT NULL`; `ix_food_catalog_search USING GIN (search_vector)`. RLS enabled, Data API roles
revoked, `gym_pulse_api_access` policy and DML for the runtime role. Rows are never deleted.

## `user_foods` (owner-scoped)

| Field | Type | Rules |
|---|---|---|
| `id` | UUID | Primary key |
| `user_id` | UUID | Not null, `auth.users(id)` cascade delete |
| `name` | TEXT | Not null, 1–200 chars |
| `brand` | TEXT | Nullable, ≤ 120 chars |
| `gtin` | CHAR(14) | Nullable |
| `serving_label` | TEXT | Not null, 1–60 chars |
| `serving_g` | NUMERIC(7,1) | Not null; `CHECK (serving_g > 0 AND serving_g <= 5000)` |
| `energy_kcal` | INTEGER | Not null; `CHECK (BETWEEN 0 AND 20000)` per serving |
| `protein_g`, `carbs_g`, `fat_g` | NUMERIC(6,1) | Not null; `CHECK (BETWEEN 0 AND 2000)` per serving |
| `archived_at` | TIMESTAMPTZ | Nullable |
| `revision` | BIGINT | Not null, default 1, `CHECK (revision > 0)` |
| `created_at`, `updated_at` | TIMESTAMPTZ | Server generated |

Indexes: `ix_user_foods_user_active(user_id) WHERE archived_at IS NULL`;
`ix_user_foods_user_gtin(user_id, gtin) WHERE gtin IS NOT NULL`. Same RLS and role provisioning.

## `food_entries` (owner-scoped)

| Field | Type | Rules |
|---|---|---|
| `id` | UUID | Primary key |
| `user_id` | UUID | Not null, cascade delete |
| `date` | DATE | Not null; athlete's local date |
| `kind` | TEXT | `CHECK (kind IN ('food', 'quick_add'))` |
| `catalog_food_id` | UUID | Nullable; `food_catalog(id)` restrict |
| `user_food_id` | UUID | Nullable; `user_foods(id)` restrict |
| `serving_label` | TEXT | Nullable; ≤ 60 chars |
| `serving_g` | NUMERIC(7,1) | Nullable; `> 0` |
| `amount` | NUMERIC(5,2) | Nullable; `CHECK (amount BETWEEN 0.1 AND 50)` |
| `energy_kcal` | INTEGER | Nullable; `CHECK (BETWEEN 0 AND 20000)` |
| `protein_g`, `carbs_g`, `fat_g` | NUMERIC(6,1) | Nullable; `CHECK (BETWEEN 0 AND 2000)` |
| `revision` | BIGINT | Not null, default 1 |
| `logged_at` | TIMESTAMPTZ | Not null |
| `created_at`, `updated_at` | TIMESTAMPTZ | Server generated |

Checks: `ck_food_entries_kind`: `kind = 'food'` requires exactly one of `catalog_food_id`,
`user_food_id`, plus `serving_label`, `serving_g`, `amount`, and all four values not null; `kind =
'quick_add'` requires both references and the serving fields null and at least one value not null. Index
`ix_food_entries_user_date(user_id, date, logged_at)`; `ix_food_entries_user_logged(user_id,
logged_at DESC)` for recents. Same RLS and role provisioning.

## `food_barcode_misses`

| Field | Type | Rules |
|---|---|---|
| `gtin` | CHAR(14) | Primary key |
| `checked_at` | TIMESTAMPTZ | Not null |

Honoured for 7 days; upserted on every miss. Same RLS and role provisioning.

## `nutrition_days` (amended)

`source` check becomes `IN ('manual', 'apple_health', 'foods')`. No other change.

## Entry write transaction (DAO)

1. Lock `nutrition:` for the athlete; replay from the ledger under the operation's scope.
2. Load the referenced food (catalog or own, not archived unless the entry already exists) and
   compute the snapshot: `per_100g × serving_g × amount / 100`; energy rounded to whole kcal,
   macros to one decimal. A quick add uses the request values directly.
3. Select `nutrition_days` for the date `FOR UPDATE`. If it exists with `source` `manual` or
   `apple_health` and the day has no entries, insert a `quick_add` entry from its values (its
   `logged_at`) and collect it as `converted`.
4. Apply: insert (revision 1), or update when `expected_revision` matches the entry's revision
   (else `ConflictError` with the entry as `Authoritative`), or delete under the same rule.
5. Recompute the four sums over the day's entries. Zero entries → delete the day row. Otherwise
   upsert the day with `source = 'foods'`, `logged_at = now()`, revision + 1 (or 1 when new).
6. Ledger with status and the response body; commit.

## Private food transaction (DAO)

Lock, replay, then insert (revision 1), update with revision check, or set `archived_at` with
revision check. An archived food still resolves for existing entries and for the recents list,
but is excluded from search and barcode lookup.

## Barcode miss handling

A miss is written outside any athlete transaction (`INSERT … ON CONFLICT (gtin) DO UPDATE SET
checked_at`). Creating a private food with a GTIN does not delete the miss; private lookup runs
first so the miss never masks it.

## API models

```json
{
  "id": "uuid",
  "source": "usda",
  "name": "Greek yogurt, plain, nonfat",
  "brand": "Kirkland Signature",
  "barcode": "0096619123456",
  "servings": [{ "label": "1 container (170 g)", "grams": 170 }, { "label": "100 g", "grams": 100 }],
  "per_100g": { "energy_kcal": 59, "protein_g": 10.2, "carbs_g": 3.6, "fat_g": 0.4 },
  "attribution": null,
  "revision": null,
  "archived": false
}
```

`source` is `usda`, `open_food_facts`, or `own`. `attribution` is present only for Open Food Facts:
`{ "name": "Open Food Facts", "url": "https://world.openfoodfacts.org/product/…", "license": "ODbL" }`.
`revision` and `archived` are set only for `own` foods. `barcode` is the 13-digit form when the
GTIN fits it, otherwise 14 digits, or null.

```json
{
  "id": "uuid",
  "date": "2026-10-27",
  "kind": "food",
  "food": { "id": "uuid", "source": "usda", "name": "…", "brand": "…" },
  "serving": { "label": "1 container (170 g)", "grams": 170 },
  "amount": 1.5,
  "energy_kcal": 150,
  "protein_g": 26,
  "carbs_g": 9.2,
  "fat_g": 1,
  "revision": 1,
  "logged_at": "2026-10-27T12:10:00Z",
  "created_at": "2026-10-27T12:10:00Z",
  "updated_at": "2026-10-27T12:10:00Z"
}
```

A `quick_add` entry has `food`, `serving`, and `amount` null and omits absent values. Entry
responses are wrapped: `{ "entry": …, "day": NutritionDay | null, "converted": [FoodEntry] }` on
create, `{ "entry", "day" }` on change, `{ "day" }` on delete.

```json
{ "food": { "…": "Food" }, "last_serving": { "label": "1 cup", "grams": 245 }, "last_amount": 1, "times_logged": 6, "last_logged_at": "2026-10-26T08:00:00Z" }
```
