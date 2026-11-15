DROP INDEX IF EXISTS idx_day_logs_import;
ALTER TABLE day_logs DROP COLUMN IF EXISTS import_id;
DROP TABLE IF EXISTS workout_imports;
