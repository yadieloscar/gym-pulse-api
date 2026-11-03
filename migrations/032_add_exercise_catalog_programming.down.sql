DROP INDEX IF EXISTS idx_exercise_catalog_pattern;
ALTER TABLE exercise_catalog
    DROP CONSTRAINT IF EXISTS exercise_catalog_pattern_check,
    DROP CONSTRAINT IF EXISTS exercise_catalog_muscle_group_check,
    DROP CONSTRAINT IF EXISTS exercise_catalog_equipment_required_check,
    DROP CONSTRAINT IF EXISTS exercise_catalog_impact_check,
    DROP COLUMN IF EXISTS pattern,
    DROP COLUMN IF EXISTS muscle_group,
    DROP COLUMN IF EXISTS equipment_required,
    DROP COLUMN IF EXISTS impact;
