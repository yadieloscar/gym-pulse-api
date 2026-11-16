DROP INDEX IF EXISTS idx_scheduled_workouts_program_planned;
DROP INDEX IF EXISTS uq_scheduled_workouts_unit_slot;
ALTER TABLE scheduled_workouts DROP COLUMN IF EXISTS slot_date;
ALTER TABLE programs DROP COLUMN IF EXISTS scheduled_through;
ALTER TABLE program_workouts DROP CONSTRAINT IF EXISTS program_workouts_weekdays_check;
ALTER TABLE program_workouts DROP COLUMN IF EXISTS weekdays;
