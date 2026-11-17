ALTER TABLE workout_templates DROP CONSTRAINT IF EXISTS workout_templates_cooldown_routine_id_check;
ALTER TABLE workout_templates DROP CONSTRAINT IF EXISTS workout_templates_warmup_routine_id_check;
ALTER TABLE workout_templates DROP COLUMN IF EXISTS cooldown_routine_id;
ALTER TABLE workout_templates DROP COLUMN IF EXISTS warmup_routine_id;
ALTER TABLE exercises DROP CONSTRAINT IF EXISTS exercises_duration_seconds_check;
ALTER TABLE exercises DROP COLUMN IF EXISTS duration_seconds;
