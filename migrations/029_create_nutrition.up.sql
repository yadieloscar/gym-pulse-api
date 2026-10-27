-- Nutrition: one set of daily totals per athlete and calendar date, and the
-- athlete's chosen daily target with the date it took effect. GymPulse stores
-- totals only; there is no food, meal, or portion data. A day typed by the
-- athlete (source manual) is never overwritten by a Health import
-- (source apple_health); that rule lives in the application transaction.
CREATE TABLE nutrition_days (
    id              UUID PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    date            DATE NOT NULL,
    energy_kcal     INTEGER CHECK (energy_kcal BETWEEN 0 AND 20000),
    protein_g       NUMERIC(6,1) CHECK (protein_g BETWEEN 0 AND 2000),
    carbs_g         NUMERIC(6,1) CHECK (carbs_g BETWEEN 0 AND 2000),
    fat_g           NUMERIC(6,1) CHECK (fat_g BETWEEN 0 AND 2000),
    source          TEXT NOT NULL CHECK (source IN ('manual', 'apple_health')),
    revision        BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    logged_at       TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_nutrition_days_has_value CHECK (
        energy_kcal IS NOT NULL OR protein_g IS NOT NULL OR carbs_g IS NOT NULL OR fat_g IS NOT NULL
    )
);

-- One entry per athlete and date; also serves the oldest-first range read.
CREATE UNIQUE INDEX ux_nutrition_days_user_date
    ON nutrition_days(user_id, date);

CREATE TABLE nutrition_targets (
    id              UUID PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    effective_from  DATE NOT NULL,
    protein_g       NUMERIC(6,1) NOT NULL CHECK (protein_g > 0 AND protein_g <= 2000),
    energy_kcal     INTEGER CHECK (energy_kcal BETWEEN 1 AND 20000),
    carbs_g         NUMERIC(6,1) CHECK (carbs_g BETWEEN 0 AND 2000),
    fat_g           NUMERIC(6,1) CHECK (fat_g BETWEEN 0 AND 2000),
    revision        BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One target per athlete and effective date; the latest effective_from on or
-- before a day is the target for that day.
CREATE UNIQUE INDEX ux_nutrition_targets_user_effective
    ON nutrition_targets(user_id, effective_from);

-- The Go API is the sole application-data path.
ALTER TABLE public.nutrition_days ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.nutrition_targets ENABLE ROW LEVEL SECURITY;

DO $$
DECLARE
    data_api_role TEXT;
BEGIN
    FOR data_api_role IN
        SELECT rolname FROM pg_roles
        WHERE rolname IN ('anon', 'authenticated', 'service_role')
    LOOP
        EXECUTE format(
            'REVOKE ALL PRIVILEGES ON TABLE public.nutrition_days FROM %I',
            data_api_role
        );
        EXECUTE format(
            'REVOKE ALL PRIVILEGES ON TABLE public.nutrition_targets FROM %I',
            data_api_role
        );
    END LOOP;
END
$$;

-- Migration 026 grants the least-privilege runtime role default DML on future
-- tables, but each new table still needs its own permissive policy so the
-- non-owner role can see rows. Skipped where the role does not exist yet
-- (a database that has not run 026).
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'gym_pulse_api') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.nutrition_days TO gym_pulse_api;
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.nutrition_targets TO gym_pulse_api;
        DROP POLICY IF EXISTS gym_pulse_api_access ON public.nutrition_days;
        CREATE POLICY gym_pulse_api_access ON public.nutrition_days
            AS PERMISSIVE FOR ALL TO gym_pulse_api USING (true) WITH CHECK (true);
        DROP POLICY IF EXISTS gym_pulse_api_access ON public.nutrition_targets;
        CREATE POLICY gym_pulse_api_access ON public.nutrition_targets
            AS PERMISSIVE FOR ALL TO gym_pulse_api USING (true) WITH CHECK (true);
    END IF;
END
$$;
