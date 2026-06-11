ALTER TABLE exercises
    DROP COLUMN IF EXISTS catalog_id,
    DROP COLUMN IF EXISTS duration_minutes,
    DROP COLUMN IF EXISTS intensity;
