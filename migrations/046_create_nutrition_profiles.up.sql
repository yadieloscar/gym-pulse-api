-- Macro calculator (spec 032): one saved plan per athlete. The row keeps the
-- numbers the athlete entered and the targets the service computed when they
-- saved, so a saved plan changes only when the athlete saves again. Food ids
-- and exclusion tags are validated by the service against its fixed list, so
-- the list can grow without a migration. New table only: existing data and
-- clients are unaffected.
CREATE TABLE IF NOT EXISTS nutrition_profiles (
    user_id                  UUID PRIMARY KEY REFERENCES auth.users(id) ON DELETE CASCADE,
    unit_system              TEXT NOT NULL CHECK (unit_system IN ('imperial', 'metric')),
    weight                   NUMERIC(5,1) NOT NULL CHECK (weight BETWEEN 30 AND 660),
    height                   NUMERIC(4,1) NOT NULL CHECK (height BETWEEN 48 AND 244),
    age_years                SMALLINT NOT NULL CHECK (age_years BETWEEN 18 AND 100),
    sex                      TEXT NOT NULL CHECK (sex IN ('male', 'female')),
    strength_days            SMALLINT NOT NULL CHECK (strength_days BETWEEN 0 AND 7),
    cardio_days              SMALLINT NOT NULL CHECK (cardio_days BETWEEN 0 AND 7),
    activity_level           TEXT CHECK (activity_level IN ('light', 'moderate', 'very')),
    goal                     TEXT NOT NULL CHECK (goal IN ('fat_loss', 'recomp', 'maintenance', 'muscle_gain')),
    calorie_adjustment_kcal  INTEGER NOT NULL CHECK (calorie_adjustment_kcal BETWEEN -500 AND 400),
    protein_g_per_lb         NUMERIC(3,2) NOT NULL CHECK (protein_g_per_lb BETWEEN 0.70 AND 1.10),
    fat_g_per_lb             NUMERIC(3,2) NOT NULL CHECK (fat_g_per_lb BETWEEN 0.25 AND 0.45),
    carb_cycling             BOOLEAN NOT NULL DEFAULT false,
    preferred_proteins       TEXT[] NOT NULL DEFAULT '{}' CHECK (cardinality(preferred_proteins) <= 20),
    preferred_carbs          TEXT[] NOT NULL DEFAULT '{}' CHECK (cardinality(preferred_carbs) <= 20),
    exclusions               TEXT[] NOT NULL DEFAULT '{}' CHECK (cardinality(exclusions) <= 20),
    meal_choices             JSONB NOT NULL DEFAULT '[]'::jsonb
                             CHECK (jsonb_typeof(meal_choices) = 'array' AND jsonb_array_length(meal_choices) <= 4),
    bmr_kcal                 INTEGER NOT NULL CHECK (bmr_kcal > 0),
    tdee_kcal                INTEGER NOT NULL CHECK (tdee_kcal > 0),
    target_kcal              INTEGER NOT NULL CHECK (target_kcal > 0),
    protein_g                INTEGER NOT NULL CHECK (protein_g >= 0),
    carbs_g                  INTEGER NOT NULL CHECK (carbs_g >= 0),
    fat_g                    INTEGER NOT NULL CHECK (fat_g >= 0),
    revision                 BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

SELECT gym_pulse_private.secure_table('public.nutrition_profiles');
