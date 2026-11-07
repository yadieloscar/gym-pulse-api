DELETE FROM exercise_catalog WHERE (category, name) IN (('full', 'Box Jump'), ('full', 'Broad Jump'), ('full', 'Medicine Ball Slam'), ('push', 'Push Press'));
ALTER TABLE exercise_catalog
    DROP CONSTRAINT IF EXISTS exercise_catalog_sub_pattern_check,
    DROP COLUMN IF EXISTS sub_pattern;
