-- Saved meals: a named, ordered list of foods (food, serving, amount) an
-- athlete saves to log again in one request. A meal stores no energy or macro
-- values; they are always computed from its foods' current facts. Logging a
-- meal inserts ordinary food_entries rows, which never reference a meal, so
-- changing or deleting a meal cannot change a logged day. New tables only:
-- existing data and clients are unaffected.
CREATE TABLE saved_meals (
    id              UUID PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 60),
    revision        BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    last_logged_at  TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One name per athlete ignoring case; also the owner prefix for the list and
-- the per-athlete count.
CREATE UNIQUE INDEX ux_saved_meals_user_name
    ON saved_meals(user_id, lower(name));

CREATE TABLE saved_meal_items (
    id               UUID PRIMARY KEY,
    meal_id          UUID NOT NULL REFERENCES saved_meals(id) ON DELETE CASCADE,
    position         SMALLINT NOT NULL CHECK (position BETWEEN 0 AND 19),
    catalog_food_id  UUID REFERENCES food_catalog(id) ON DELETE RESTRICT,
    -- Deferred, not RESTRICT: removing the athlete's identity cascades to
    -- user_foods and to saved_meals as separate statements in no guaranteed
    -- order, so the check must wait until both have run. At commit a deleted
    -- own food that a meal still holds is refused all the same.
    user_food_id     UUID REFERENCES user_foods(id) ON DELETE NO ACTION DEFERRABLE INITIALLY DEFERRED,
    serving_label    TEXT NOT NULL CHECK (char_length(serving_label) BETWEEN 1 AND 60),
    serving_g        NUMERIC(7,1) NOT NULL CHECK (serving_g > 0 AND serving_g <= 5000),
    amount           NUMERIC(5,2) NOT NULL CHECK (amount BETWEEN 0.1 AND 50),
    CONSTRAINT ck_saved_meal_items_food CHECK (
        (catalog_food_id IS NOT NULL) <> (user_food_id IS NOT NULL)
    )
);

-- Item order, and with the position check the cap of 20 items per meal.
CREATE UNIQUE INDEX ux_saved_meal_items_position
    ON saved_meal_items(meal_id, position);

-- The same food and serving appears once per meal.
CREATE UNIQUE INDEX ux_saved_meal_items_food_serving
    ON saved_meal_items(meal_id, COALESCE(catalog_food_id, user_food_id), serving_g, serving_label);

-- Referencing side of the private-food foreign key.
CREATE INDEX ix_saved_meal_items_user_food
    ON saved_meal_items(user_food_id) WHERE user_food_id IS NOT NULL;

-- The Go API is the sole application-data path. Owner bypass is required by
-- the direct API database role, so RLS is enabled but not forced.
ALTER TABLE public.saved_meals ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.saved_meal_items ENABLE ROW LEVEL SECURITY;

DO $$
DECLARE
    data_api_role TEXT;
    meal_table TEXT;
BEGIN
    FOR data_api_role IN
        SELECT rolname FROM pg_roles
        WHERE rolname IN ('anon', 'authenticated', 'service_role')
    LOOP
        FOREACH meal_table IN ARRAY ARRAY['saved_meals', 'saved_meal_items']
        LOOP
            EXECUTE format('REVOKE ALL PRIVILEGES ON TABLE public.%I FROM %I', meal_table, data_api_role);
        END LOOP;
    END LOOP;
END
$$;

-- Migration 026 grants the least-privilege runtime role default DML on future
-- tables, but each new table still needs its own permissive policy so the
-- non-owner role can see rows. Skipped where the role does not exist yet.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'gym_pulse_api') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.saved_meals TO gym_pulse_api;
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.saved_meal_items TO gym_pulse_api;
        DROP POLICY IF EXISTS gym_pulse_api_access ON public.saved_meals;
        CREATE POLICY gym_pulse_api_access ON public.saved_meals
            AS PERMISSIVE FOR ALL TO gym_pulse_api USING (true) WITH CHECK (true);
        DROP POLICY IF EXISTS gym_pulse_api_access ON public.saved_meal_items;
        CREATE POLICY gym_pulse_api_access ON public.saved_meal_items
            AS PERMISSIVE FOR ALL TO gym_pulse_api USING (true) WITH CHECK (true);
    END IF;
END
$$;
