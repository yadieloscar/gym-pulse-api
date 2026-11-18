ALTER TABLE program_workouts DROP CONSTRAINT IF EXISTS program_workouts_cooldown_routine_id_check;
ALTER TABLE program_workouts DROP CONSTRAINT IF EXISTS program_workouts_warmup_routine_id_check;
ALTER TABLE program_workouts DROP COLUMN IF EXISTS cooldown_routine_id;
ALTER TABLE program_workouts DROP COLUMN IF EXISTS warmup_routine_id;
DROP TABLE IF EXISTS stretch_libraries;
