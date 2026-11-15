-- Knee Kit: one record per athlete and local calendar date holding the daily
-- knee checklist items the athlete ticked and the pain values they logged.
-- A stored fact only: nothing that derives participation, stats, streaks,
-- records, volume, or day status reads this table, and the API computes no
-- guidance from it. New table only, so existing data and clients are
-- unaffected.
CREATE TABLE knee_kit_days (
    id              UUID PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    date            DATE NOT NULL,
    completed_items TEXT[] NOT NULL DEFAULT '{}',
    morning_pain    SMALLINT CHECK (morning_pain BETWEEN 0 AND 10),
    activity_pain   SMALLINT CHECK (activity_pain BETWEEN 0 AND 10),
    sharp_pain      BOOLEAN NOT NULL DEFAULT false,
    limping         BOOLEAN NOT NULL DEFAULT false,
    revision        BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- At most 12 items, none NULL or empty, each a lowercase hyphenated slug
    -- of 1 to 64 characters. A slug cannot contain a comma, so the joined
    -- form is checked item by item and a run of 65 characters without a
    -- comma is an item that is too long. Uniqueness within a day is enforced
    -- by the API; a CHECK cannot compare array elements.
    CONSTRAINT ck_knee_kit_days_completed_items CHECK (
        cardinality(completed_items) <= 12
        AND array_position(completed_items, NULL) IS NULL
        AND array_position(completed_items, '') IS NULL
        AND array_to_string(completed_items, ',') ~
            '^([a-z0-9]+(-[a-z0-9]+)*(,[a-z0-9]+(-[a-z0-9]+)*)*)?$'
        AND array_to_string(completed_items, ',') !~ '[^,]{65}'
    )
);

-- One record per athlete and date; also serves the oldest-first range read.
CREATE UNIQUE INDEX ux_knee_kit_days_user_date
    ON knee_kit_days(user_id, date);

-- The Go API is the sole application-data path. Owner bypass is required by
-- the direct API database role, so RLS is enabled but not forced.
ALTER TABLE public.knee_kit_days ENABLE ROW LEVEL SECURITY;

DO $$
DECLARE
    data_api_role TEXT;
BEGIN
    FOR data_api_role IN
        SELECT rolname FROM pg_roles
        WHERE rolname IN ('anon', 'authenticated', 'service_role')
    LOOP
        EXECUTE format(
            'REVOKE ALL PRIVILEGES ON TABLE public.knee_kit_days FROM %I',
            data_api_role
        );
    END LOOP;
END
$$;

-- Migration 026 grants the least-privilege runtime role default DML on future
-- tables, but each new table still needs its own permissive policy so the
-- non-owner role can see rows. Skipped where the role does not exist yet.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'gym_pulse_api') THEN
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.knee_kit_days TO gym_pulse_api;
        DROP POLICY IF EXISTS gym_pulse_api_access ON public.knee_kit_days;
        CREATE POLICY gym_pulse_api_access ON public.knee_kit_days
            AS PERMISSIVE FOR ALL TO gym_pulse_api USING (true) WITH CHECK (true);
    END IF;
END
$$;
