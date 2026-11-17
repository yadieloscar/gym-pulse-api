-- One active program on fixed weekdays (spec 031, gympulse-one-active-program).
--
-- Additive only. No row is rewritten: existing program units stay unplaced
-- (weekdays NULL) and are placed by the API the first time a session would be
-- created for their program; existing sessions keep slot_date NULL.

-- The ISO weekdays a program unit sits on. NULL means the unit is unplaced.
ALTER TABLE program_workouts ADD COLUMN IF NOT EXISTS weekdays SMALLINT[];

ALTER TABLE program_workouts DROP CONSTRAINT IF EXISTS program_workouts_weekdays_check;
ALTER TABLE program_workouts ADD CONSTRAINT program_workouts_weekdays_check CHECK (
    weekdays IS NULL OR (
        cardinality(weekdays) BETWEEN 1 AND 7
        AND weekdays <@ ARRAY[1, 2, 3, 4, 5, 6, 7]::SMALLINT[]
    )
);

-- The last date the schedule fill covered for this program. Only moves forward.
ALTER TABLE programs ADD COLUMN IF NOT EXISTS scheduled_through DATE;

-- The layout date a session was generated for. Set by the program fill only
-- and never updated, including when the athlete moves the session.
ALTER TABLE scheduled_workouts ADD COLUMN IF NOT EXISTS slot_date DATE;

-- One session per program unit per layout date. This is what makes the fill
-- idempotent under concurrent reads and stops a moved session's slot from
-- being refilled. Recover copies and off-plan sessions carry no slot.
CREATE UNIQUE INDEX IF NOT EXISTS uq_scheduled_workouts_unit_slot
    ON scheduled_workouts (program_workout_id, slot_date)
    WHERE program_workout_id IS NOT NULL AND slot_date IS NOT NULL;

-- Start, edit, and removal change a program's planned sessions by date.
CREATE INDEX IF NOT EXISTS idx_scheduled_workouts_program_planned
    ON scheduled_workouts (program_id, date)
    WHERE status = 'planned';
