-- Planned sport sessions are program units of kind 'sport' that materialize
-- onto the schedule like exercise workouts and are completed by a linked sport
-- activity. Every addition is defaulted or nullable, so existing rows become
-- 'exercises' units without a backfill and existing clients keep working.
ALTER TABLE program_workouts
    ADD COLUMN kind TEXT NOT NULL DEFAULT 'exercises'
        CHECK (kind IN ('exercises', 'sport')),
    ADD COLUMN sport_id TEXT
        CHECK (sport_id IS NULL OR (char_length(sport_id) BETWEEN 1 AND 64
            AND sport_id ~ '^[a-z0-9]+(-[a-z0-9]+)*$')),
    ADD COLUMN sport_name TEXT
        CHECK (sport_name IS NULL OR char_length(sport_name) BETWEEN 1 AND 80),
    ADD COLUMN target_duration_minutes INTEGER
        CHECK (target_duration_minutes IS NULL OR target_duration_minutes BETWEEN 1 AND 1440),
    ADD COLUMN target_distance_meters INTEGER
        CHECK (target_distance_meters IS NULL OR target_distance_meters BETWEEN 1 AND 1000000),
    ADD COLUMN sport_notes TEXT
        CHECK (sport_notes IS NULL OR char_length(sport_notes) BETWEEN 1 AND 2000),
    ADD CONSTRAINT program_workouts_sport_kind_check
        CHECK ((kind = 'sport') = (sport_id IS NOT NULL)
            AND (kind = 'sport') = (sport_name IS NOT NULL)
            AND (kind = 'sport') = (target_duration_minutes IS NOT NULL)
            AND (target_distance_meters IS NULL OR kind = 'sport')
            AND (sport_notes IS NULL OR kind = 'sport'));

ALTER TABLE scheduled_workouts
    ADD COLUMN kind TEXT NOT NULL DEFAULT 'exercises'
        CHECK (kind IN ('exercises', 'sport')),
    ADD COLUMN sport_id TEXT
        CHECK (sport_id IS NULL OR (char_length(sport_id) BETWEEN 1 AND 64
            AND sport_id ~ '^[a-z0-9]+(-[a-z0-9]+)*$')),
    ADD COLUMN sport_name TEXT
        CHECK (sport_name IS NULL OR char_length(sport_name) BETWEEN 1 AND 80),
    ADD COLUMN target_duration_minutes INTEGER
        CHECK (target_duration_minutes IS NULL OR target_duration_minutes BETWEEN 1 AND 1440),
    ADD COLUMN target_distance_meters INTEGER
        CHECK (target_distance_meters IS NULL OR target_distance_meters BETWEEN 1 AND 1000000),
    ADD COLUMN sport_notes TEXT
        CHECK (sport_notes IS NULL OR char_length(sport_notes) BETWEEN 1 AND 2000),
    ADD CONSTRAINT scheduled_workouts_sport_kind_check
        CHECK ((kind = 'sport') = (sport_id IS NOT NULL)
            AND (kind = 'sport') = (sport_name IS NOT NULL)
            AND (kind = 'sport') = (target_duration_minutes IS NOT NULL)
            AND (target_distance_meters IS NULL OR kind = 'sport')
            AND (sport_notes IS NULL OR kind = 'sport'));

-- A sport activity may fulfil one scheduled sport session. The link lives on
-- the activity; the schedule reads it by join. SET NULL keeps the existing
-- account-deletion order (activities before scheduled workouts) valid.
ALTER TABLE sport_activities
    ADD COLUMN scheduled_workout_id UUID
        REFERENCES scheduled_workouts(id) ON DELETE SET NULL;

-- One activity per scheduled sport session.
CREATE UNIQUE INDEX ux_sport_activities_scheduled_workout
    ON sport_activities(scheduled_workout_id)
    WHERE scheduled_workout_id IS NOT NULL;
