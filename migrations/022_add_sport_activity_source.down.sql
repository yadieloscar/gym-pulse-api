DROP INDEX IF EXISTS ux_sport_activities_user_source_external;
ALTER TABLE sport_activities
    DROP CONSTRAINT IF EXISTS sport_activities_source_identity_check,
    DROP COLUMN IF EXISTS avg_heart_rate_bpm,
    DROP COLUMN IF EXISTS energy_kcal,
    DROP COLUMN IF EXISTS distance_meters,
    DROP COLUMN IF EXISTS external_id,
    DROP COLUMN IF EXISTS source;
