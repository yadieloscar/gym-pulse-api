-- Food logging: a shared food catalog filled lazily from USDA FoodData Central
-- and Open Food Facts, private athlete foods typed from a label, per-day food
-- entries whose sum is written into nutrition_days in the same transaction,
-- and a small barcode-miss cache so unknown codes are not re-asked on every
-- retry. Catalog values are stored per 100 g; entries snapshot their values at
-- logging time so history never moves when a source changes.
CREATE TABLE food_catalog (
    id                UUID PRIMARY KEY,
    source            TEXT NOT NULL CHECK (source IN ('usda', 'open_food_facts')),
    source_id         TEXT NOT NULL CHECK (char_length(source_id) BETWEEN 1 AND 64),
    data_type         TEXT CHECK (data_type IN ('foundation', 'sr_legacy', 'branded', 'off_product')),
    gtin              TEXT CHECK (gtin ~ '^[0-9]{14}$'),
    name              TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    brand             TEXT CHECK (char_length(brand) <= 120),
    energy_kcal_100g  NUMERIC(6,1) NOT NULL CHECK (energy_kcal_100g BETWEEN 0 AND 900),
    protein_g_100g    NUMERIC(6,1) NOT NULL CHECK (protein_g_100g BETWEEN 0 AND 100),
    carbs_g_100g      NUMERIC(6,1) NOT NULL CHECK (carbs_g_100g BETWEEN 0 AND 100),
    fat_g_100g        NUMERIC(6,1) NOT NULL CHECK (fat_g_100g BETWEEN 0 AND 100),
    servings          JSONB NOT NULL CHECK (jsonb_typeof(servings) = 'array' AND jsonb_array_length(servings) >= 1),
    search_vector     TSVECTOR GENERATED ALWAYS AS (to_tsvector('simple', name || ' ' || coalesce(brand, ''))) STORED,
    fetched_at        TIMESTAMPTZ NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One row per outside food; repeat lookups answer from here.
CREATE UNIQUE INDEX ux_food_catalog_source_id
    ON food_catalog(source, source_id);
CREATE INDEX ix_food_catalog_gtin
    ON food_catalog(gtin) WHERE gtin IS NOT NULL;
CREATE INDEX ix_food_catalog_search
    ON food_catalog USING GIN (search_vector);

CREATE TABLE user_foods (
    id              UUID PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
    brand           TEXT CHECK (char_length(brand) <= 120),
    gtin            TEXT CHECK (gtin ~ '^[0-9]{14}$'),
    serving_label   TEXT NOT NULL CHECK (char_length(serving_label) BETWEEN 1 AND 60),
    serving_g       NUMERIC(7,1) NOT NULL CHECK (serving_g > 0 AND serving_g <= 5000),
    energy_kcal     INTEGER NOT NULL CHECK (energy_kcal BETWEEN 0 AND 20000),
    protein_g       NUMERIC(6,1) NOT NULL CHECK (protein_g BETWEEN 0 AND 2000),
    carbs_g         NUMERIC(6,1) NOT NULL CHECK (carbs_g BETWEEN 0 AND 2000),
    fat_g           NUMERIC(6,1) NOT NULL CHECK (fat_g BETWEEN 0 AND 2000),
    archived_at     TIMESTAMPTZ,
    revision        BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ix_user_foods_user_active
    ON user_foods(user_id) WHERE archived_at IS NULL;
CREATE INDEX ix_user_foods_user_gtin
    ON user_foods(user_id, gtin) WHERE gtin IS NOT NULL;

CREATE TABLE food_entries (
    id               UUID PRIMARY KEY,
    user_id          UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    date             DATE NOT NULL,
    kind             TEXT NOT NULL CHECK (kind IN ('food', 'quick_add')),
    catalog_food_id  UUID REFERENCES food_catalog(id) ON DELETE RESTRICT,
    user_food_id     UUID REFERENCES user_foods(id) ON DELETE RESTRICT,
    serving_label    TEXT CHECK (char_length(serving_label) BETWEEN 1 AND 60),
    serving_g        NUMERIC(7,1) CHECK (serving_g > 0 AND serving_g <= 5000),
    amount           NUMERIC(5,2) CHECK (amount BETWEEN 0.1 AND 50),
    energy_kcal      INTEGER CHECK (energy_kcal BETWEEN 0 AND 20000),
    protein_g        NUMERIC(6,1) CHECK (protein_g BETWEEN 0 AND 2000),
    carbs_g          NUMERIC(6,1) CHECK (carbs_g BETWEEN 0 AND 2000),
    fat_g            NUMERIC(6,1) CHECK (fat_g BETWEEN 0 AND 2000),
    revision         BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    logged_at        TIMESTAMPTZ NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_food_entries_kind CHECK (
        (kind = 'food'
            AND ((catalog_food_id IS NOT NULL) <> (user_food_id IS NOT NULL))
            AND serving_label IS NOT NULL AND serving_g IS NOT NULL AND amount IS NOT NULL
            AND energy_kcal IS NOT NULL AND protein_g IS NOT NULL AND carbs_g IS NOT NULL AND fat_g IS NOT NULL)
        OR
        (kind = 'quick_add'
            AND catalog_food_id IS NULL AND user_food_id IS NULL
            AND serving_label IS NULL AND serving_g IS NULL AND amount IS NULL
            AND (energy_kcal IS NOT NULL OR protein_g IS NOT NULL OR carbs_g IS NOT NULL OR fat_g IS NOT NULL))
    )
);

-- A day's entries oldest first; recents by latest log.
CREATE INDEX ix_food_entries_user_date
    ON food_entries(user_id, date, logged_at);
CREATE INDEX ix_food_entries_user_logged
    ON food_entries(user_id, logged_at DESC);

CREATE TABLE food_barcode_misses (
    gtin        TEXT PRIMARY KEY CHECK (gtin ~ '^[0-9]{14}$'),
    checked_at  TIMESTAMPTZ NOT NULL
);

-- A day whose totals are the sum of its food entries.
ALTER TABLE nutrition_days DROP CONSTRAINT nutrition_days_source_check;
ALTER TABLE nutrition_days ADD CONSTRAINT nutrition_days_source_check
    CHECK (source IN ('manual', 'apple_health', 'foods'));

-- The Go API is the sole application-data path.
ALTER TABLE public.food_catalog ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.user_foods ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.food_entries ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.food_barcode_misses ENABLE ROW LEVEL SECURITY;

DO $$
DECLARE
    data_api_role TEXT;
    food_table TEXT;
BEGIN
    FOR data_api_role IN
        SELECT rolname FROM pg_roles
        WHERE rolname IN ('anon', 'authenticated', 'service_role')
    LOOP
        FOREACH food_table IN ARRAY ARRAY['food_catalog', 'user_foods', 'food_entries', 'food_barcode_misses']
        LOOP
            EXECUTE format('REVOKE ALL PRIVILEGES ON TABLE public.%I FROM %I', food_table, data_api_role);
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
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.food_catalog TO gym_pulse_api;
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.user_foods TO gym_pulse_api;
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.food_entries TO gym_pulse_api;
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.food_barcode_misses TO gym_pulse_api;
        DROP POLICY IF EXISTS gym_pulse_api_access ON public.food_catalog;
        CREATE POLICY gym_pulse_api_access ON public.food_catalog
            AS PERMISSIVE FOR ALL TO gym_pulse_api USING (true) WITH CHECK (true);
        DROP POLICY IF EXISTS gym_pulse_api_access ON public.user_foods;
        CREATE POLICY gym_pulse_api_access ON public.user_foods
            AS PERMISSIVE FOR ALL TO gym_pulse_api USING (true) WITH CHECK (true);
        DROP POLICY IF EXISTS gym_pulse_api_access ON public.food_entries;
        CREATE POLICY gym_pulse_api_access ON public.food_entries
            AS PERMISSIVE FOR ALL TO gym_pulse_api USING (true) WITH CHECK (true);
        DROP POLICY IF EXISTS gym_pulse_api_access ON public.food_barcode_misses;
        CREATE POLICY gym_pulse_api_access ON public.food_barcode_misses
            AS PERMISSIVE FOR ALL TO gym_pulse_api USING (true) WITH CHECK (true);
    END IF;
END
$$;
