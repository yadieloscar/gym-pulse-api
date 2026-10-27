# Data Model: Daily nutrition totals and a chosen target

## `nutrition_days` (migration 029)

| Field | Type | Rules |
|---|---|---|
| `id` | UUID | Primary key, application-generated |
| `user_id` | UUID | Not null, `auth.users(id)` cascade delete |
| `date` | DATE | Not null; the athlete's local calendar date |
| `energy_kcal` | INTEGER | Nullable; `CHECK (energy_kcal BETWEEN 0 AND 20000)` |
| `protein_g` | NUMERIC(6,1) | Nullable; `CHECK (protein_g BETWEEN 0 AND 2000)` |
| `carbs_g` | NUMERIC(6,1) | Nullable; same bound |
| `fat_g` | NUMERIC(6,1) | Nullable; same bound |
| `source` | TEXT | `CHECK (source IN ('manual', 'apple_health'))` |
| `revision` | BIGINT | Not null, default 1, `CHECK (revision > 0)` |
| `logged_at` | TIMESTAMPTZ | Not null; last athlete or device save |
| `created_at` | TIMESTAMPTZ | Server generated |
| `updated_at` | TIMESTAMPTZ | Server generated |

Table check `ck_nutrition_days_has_value`: at least one of the four values is not null. Unique index
`ux_nutrition_days_user_date(user_id, date)` is the identity; it also serves the range read ordered by
`date ASC`. RLS enabled, Data API role privileges revoked, `gym_pulse_api_access` policy and DML
granted to the runtime role when present.

## `nutrition_targets` (migration 029)

| Field | Type | Rules |
|---|---|---|
| `id` | UUID | Primary key, application-generated |
| `user_id` | UUID | Not null, `auth.users(id)` cascade delete |
| `effective_from` | DATE | Not null; first local date the target applies to |
| `protein_g` | NUMERIC(6,1) | Not null; `CHECK (protein_g > 0 AND protein_g <= 2000)` |
| `energy_kcal` | INTEGER | Nullable; `CHECK (energy_kcal BETWEEN 1 AND 20000)` |
| `carbs_g` | NUMERIC(6,1) | Nullable; `CHECK (carbs_g BETWEEN 0 AND 2000)` |
| `fat_g` | NUMERIC(6,1) | Nullable; same bound |
| `revision` | BIGINT | Not null, default 1, `CHECK (revision > 0)` |
| `created_at` | TIMESTAMPTZ | Server generated |
| `updated_at` | TIMESTAMPTZ | Server generated |

Unique index `ux_nutrition_targets_user_effective(user_id, effective_from)`; it serves both "current
target" (`effective_from <= today ORDER BY effective_from DESC LIMIT 1`) and the range read. Same
RLS, revoke, and runtime-role provisioning as above.

## Save-day transaction (DAO)

1. Lock the athlete's nutrition domain (`nutrition:` advisory namespace).
2. Replay from `idempotency_records` under `nutrition-days/put`.
3. Select the existing row by `(user_id, date)` for update.
4. Branch on the request:
   - **Remove** (all four values absent, source manual): requires the current revision like any athlete save (`0` when no row exists); delete when present; outcome `removed`; record `204`; commit.
   - **Health save** (`source = apple_health`): no row → insert as `apple_health`, outcome `created`;
     row is `apple_health` → update values and `logged_at`, bump revision, outcome `replaced`; row is
     `manual` → outcome `ignored`, return the stored row, write only the ledger.
   - **Athlete save**: no row and `expected_revision = 0` → insert as `manual`, outcome `created`;
     row present and `expected_revision = revision` → update values, set `source = manual`, bump
     revision, outcome `replaced`; otherwise `ConflictError{Expected, Actual, Authoritative}`.
5. Write the idempotency record with the status; commit.

The handler writes `201` for `created`, `200` for `replaced` and `ignored`, `204` for `removed`.

## Save-target transaction (DAO)

1. Lock the same namespace; replay under `nutrition-target/put`.
2. Select the current target (latest `effective_from <= local today`) for update; its revision is the
   one `expected_revision` must match (`0` when none).
3. Stale → `ConflictError` with the current target as `Authoritative`.
4. Identical values and same `effective_from` as the current target → outcome `unchanged`.
5. Row exists for `(user_id, effective_from)` → update it, bump its revision, outcome `replaced`;
   otherwise insert with revision 1, outcome `created`.
6. Ledger; commit. Handler writes `201` for `created`, `200` otherwise.

## API models

```json
{
  "id": "uuid",
  "date": "2026-10-25",
  "energy_kcal": 2150,
  "protein_g": 162.5,
  "carbs_g": 230,
  "fat_g": 70,
  "source": "manual",
  "revision": 2,
  "logged_at": "2026-10-25T20:14:00Z",
  "created_at": "2026-10-25T08:01:00Z",
  "updated_at": "2026-10-25T20:14:00Z"
}
```

```json
{
  "id": "uuid",
  "effective_from": "2026-10-25",
  "protein_g": 150,
  "energy_kcal": null,
  "carbs_g": null,
  "fat_g": null,
  "revision": 1,
  "created_at": "2026-10-25T08:00:00Z",
  "updated_at": "2026-10-25T08:00:00Z"
}
```

Optional day values are omitted when null (`omitempty` on pointers). Target optional values are
always present, `null` when unset, so a client can tell "no energy target" from an old response shape.
