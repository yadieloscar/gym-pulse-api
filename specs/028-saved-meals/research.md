# Research: Saved meals

No outside source, dependency, or configuration is added. Every decision below builds on code that
shipped with spec 014 (`internal/dao/food_entry_dao.go`, `internal/dao/food_dao.go`,
`internal/service/food_svc.go`, `internal/model/food.go`, migration 030).

## Decisions

### A meal is two tables, and stores no values

- **Decision**: `saved_meals` (owner, name, revision, `last_logged_at`) and `saved_meal_items`
  (position, one food reference, serving label and grams, amount). No energy or macro columns.
- **Rationale**: FR-013 and the product rule "a saved meal is only a list of foods". Values are
  computed on read and on log from `food_catalog` / `user_foods`, so a meal can never disagree with
  its foods.
- **Alternatives considered**: a JSONB `items` column on `saved_meals` (no foreign keys, so a
  private food could be referenced across athletes and deletion order would be unenforced);
  snapshotting values on the meal (would make a meal a second source of food facts).

### Logging a meal writes ordinary food entries and nothing else

- **Decision**: the log transaction inserts one `food_entries` row per item, exactly the shape
  `CreateEntry` inserts. No `meal_id` column on `food_entries`, no "meal log" table.
- **Rationale**: FR-010 and the non-goal "no meal grouping on a day". Every existing read, change,
  delete, recents, and export path keeps working untouched, and an older app sees nothing new.
- **Alternatives considered**: a nullable `food_entries.saved_meal_id` (invites meal headers and
  makes meal deletion touch history); a batch-undo endpoint keyed by the log (see "Undo" below).

### One transaction per log, under the existing nutrition lock

- **Decision**: `LogMeal` runs in one transaction: `lockUserDomain(nutrition:)` → idempotency replay
  → lock the meal row → resolve items → load all foods in one query → compute snapshots →
  `lockNutritionDay` → convert typed totals (spec 014 rule) → one insert per item through the
  existing `insertFoodEntry` → `recomputeNutritionDay` once → set `saved_meals.last_logged_at` → ledger → commit.
- **Rationale**: FR-007 and FR-008. The same advisory lock already serialises entry writes and day
  saves, so two devices logging at once cannot interleave and the day is summed once per log.
- **Alternatives considered**: calling `CreateEntry` once per item from the service (N
  transactions, partial failure possible, N day rewrites); a client-side loop of entry posts (the
  spec's reason for existing: one request, all or nothing).

### Entry order inside one log

- **Decision**: item `i` is inserted with `logged_at = base + i microseconds`, where `base` is the
  request's `logged_at` or now.
- **Rationale**: the day list orders by `logged_at, created_at`; rows inserted in one transaction
  share `created_at`, so without an offset the order of a meal's foods on the day would be
  undefined. A microsecond offset is invisible to the athlete and keeps the existing ordering query
  and index (`ix_food_entries_user_date`) unchanged.
- **Alternatives considered**: adding a sequence column to `food_entries` (schema change to a
  shipped table for a cosmetic need).

### Where validation and snapshot maths live

- **Decision**: the service validates everything knowable from the request (date against the
  athlete's local today, operation key, name length, item count, amount bounds, duplicate
  food-and-serving pairs, override list shape) and hashes the payload. The DAO transaction resolves
  foods and calls the pure model functions `model.ResolveServing`, `model.NewSavedMealItem` (which
  uses `model.EntryValues`), and `model.SumMealItems`, and stores the assembled response in the
  idempotency ledger.
- **Rationale**: a meal's saved items are only stable under the lock, so the snapshot must be
  computed inside the transaction. `CreateEntry`'s DAO already re-reads the food and applies the
  archived rule in-transaction; this follows the same split. The maths stays in `internal/model`,
  shared with single entries.
- **Alternatives considered**: reading the meal in the service and re-checking its revision in the
  DAO (two reads and a retry path for no gain).

### Archived private foods

- **Decision**: on create and update, every item's food must resolve for the athlete and not be
  archived. On log, an archived private food is accepted only when the saved meal already contains
  that food; an override list cannot introduce one.
- **Rationale**: FR-012, matching spec 014's "existing entries and recents still resolve it". The
  athlete put the food in the meal before archiving it.
- **Alternatives considered**: refusing the whole log (breaks one-tap logging for a meal the
  athlete has not touched); silently dropping the item (totals would be wrong without notice).

### Item errors name the item

- **Decision**: an item that cannot be saved or logged returns `422 VALIDATION_ERROR` with
  `details.field = "items"` and `details.index = <zero-based position>`. Unknown, foreign, and
  archived foods share one message ("food not available").
- **Rationale**: FR-007 ("the error identifies the item at fault") and Constitution II: a foreign
  private food must be indistinguishable from a missing one. `404` stays reserved for the meal id,
  so the app can tell "this meal is gone" from "one food in it is unusable".
- **Alternatives considered**: `404` for a missing item food (ambiguous with the meal id).

### Name uniqueness and the 50-meal limit

- **Decision**: unique index on `(user_id, lower(name))`; a collision is `422 VALIDATION_ERROR` with
  `details.field = "name"`. The 51st meal is `409 SAVED_MEAL_LIMIT_REACHED` with `details.limit =
  50`, counted inside the create transaction under the nutrition lock.
- **Rationale**: FR-002. The index makes the rule hold under races; the lock makes the count exact.
  A dedicated code lets the app offer "remove one" instead of a generic message, following
  `GENERATION_LIMIT_REACHED`.
- **Alternatives considered**: a trigger for the limit (hidden logic; the lock already gives an
  exact count).

### Delete is a hard delete; `last_logged_at` does not advance the revision

- **Decision**: `DELETE` removes the meal and cascades its items. Logging a meal updates
  `last_logged_at` only; `revision` advances only when name or items change.
- **Rationale**: entries never reference a meal, so nothing needs the row afterwards (unlike
  `user_foods`, which entries reference). If logging advanced the revision, an edit queued on
  another device would conflict with a log, which changes nothing the athlete authored.
- **Alternatives considered**: `archived_at` as on `user_foods` (no reader needs archived meals).

### Read shape carries full foods

- **Decision**: each item in a `SavedMeal` response carries the full `Food` object plus the item's
  computed `energy_kcal`, `protein_g`, `carbs_g`, `fat_g`; the meal carries `totals`.
- **Rationale**: the sibling app spec logs a meal offline and shows the day before the server
  answers, and must credit Open Food Facts; both need `per_100g` and `attribution`. Foods are loaded
  with one `ListFoodsByIDs`-style query for the whole list, so there is no N+1.
- **Alternatives considered**: `FoodSummary` (as on entries) plus a second request per food.

### Unavailable items

- **Decision**: an item whose food fails `model.PlausiblePer100g` is returned with `available:
  false` and no values, and the meal's `totals` is `null`. Such a meal cannot be logged as saved
  (`422`, index of the item).
- **Rationale**: spec edge case; never show wrong numbers. With today's constraints (catalog rows
  are checked and never deleted; an item's food cannot be removed while the item exists) this is defensive only, so it is
  covered by model unit tests (`NewSavedMealItem`, `SumMealItems`) rather than a database fixture.

### Undo

- **Decision**: no new endpoint. The app undoes a log by deleting the entries the log returned,
  through the existing `DELETE /entries/{id}`.
- **Rationale**: undo is rare, at most 20 deletes, and already queued and idempotent in the app's
  nutrition outbox. `last_logged_at` is not rolled back; it only affects list order.
- **Alternatives considered**: `DELETE /meal-logs/{operation_key}` (needs a stored link from
  entries to a log, which the non-goals rule out).

### Route shape

- **Decision**: `GET|POST /api/v1/nutrition/meals`, `PUT|DELETE /api/v1/nutrition/meals/{id}`,
  `POST /api/v1/nutrition/days/{date}/meal-logs`.
- **Rationale**: mirrors `foods` and `days/{date}/entries`: the date is in the path, the referenced
  resource is in the body (`{"meal": {"id": …}}` like `{"food": {"id": …}}`).

### The own-food key on an item is deferred

- **Decision**: `saved_meal_items.user_food_id` is `ON DELETE NO ACTION DEFERRABLE INITIALLY
  DEFERRED`; `catalog_food_id` stays `RESTRICT`.
- **Rationale**: found by the integration suite. Deleting `auth.users` cascades to `user_foods` and
  to `saved_meals` as separate statements in no guaranteed order. `RESTRICT`, and a non-deferred
  `NO ACTION`, are checked inside the cascaded `user_foods` delete, before the meal and its items
  are gone, and fail the identity removal. A deferred check runs at commit, when both cascades have
  finished. Deleting an own food a meal still holds is still refused, at commit.
- **Alternatives considered**: `ON DELETE CASCADE` (a deleted food would silently shrink or empty a
  meal); relying on `DeleteUserData`'s statement order alone (does not cover the provider removing
  the identity row).

### Export and account deletion

- **Decision**: the JSON export gains `nutrition.saved_meals` (name, items with food name, serving,
  amount), additive with `schema_version` unchanged, as `knee_kit_days` did. Account deletion
  deletes `saved_meals` before `user_foods` because items reference private foods (see the deferred key above).
- **Rationale**: FR-014, FR-015.

## Resolved Technical Questions

| Question | Answer |
|---|---|
| Migration number | `039_create_saved_meals`; re-check `origin/main` before writing it |
| Contract version | 1.8.0, additive (1.7.0 is Knee Kit) |
| New dependency, layer, or concurrency | None |
| Rate limit | None new; writes are bounded by the 50-meal and 20-item limits and the 1 MiB body cap |
| Is `go test ./...` enough | No: DAO transactions need the `integration` suite against PostgreSQL |
