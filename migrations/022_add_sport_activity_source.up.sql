-- Imported health workouts are sport activities with a recorded origin. The
-- additions are nullable or defaulted, so existing rows become manual activities
-- without a backfill and existing clients keep working unchanged.
ALTER TABLE sport_activities
    ADD COLUMN source TEXT NOT NULL DEFAULT 'manual'
        CHECK (source IN ('manual', 'apple_health')),
    ADD COLUMN external_id TEXT
        CHECK (external_id IS NULL OR char_length(external_id) BETWEEN 1 AND 128),
    ADD COLUMN distance_meters INTEGER
        CHECK (distance_meters IS NULL OR distance_meters BETWEEN 1 AND 1000000),
    ADD COLUMN energy_kcal INTEGER
        CHECK (energy_kcal IS NULL OR energy_kcal BETWEEN 1 AND 20000),
    ADD COLUMN avg_heart_rate_bpm SMALLINT
        CHECK (avg_heart_rate_bpm IS NULL OR avg_heart_rate_bpm BETWEEN 30 AND 250),
    ADD CONSTRAINT sport_activities_source_identity_check
        CHECK ((source = 'manual') = (external_id IS NULL));

-- One imported health workout maps to at most one activity per athlete and origin.
CREATE UNIQUE INDEX ux_sport_activities_user_source_external
    ON sport_activities(user_id, source, external_id)
    WHERE external_id IS NOT NULL;
