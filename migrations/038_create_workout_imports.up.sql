-- Workout imports record one Strong or Hevy CSV import as a batch. The file is
-- never stored; the row keeps its options and counts. Each day log the import
-- created points back here, so deleting the import removes exactly those logs
-- (and their set logs through the existing cascade). Retries are identified on
-- the row by (user_id, operation_key). Additive: existing logs keep a NULL
-- import_id and released clients are unaffected.
CREATE TABLE workout_imports (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    source              TEXT NOT NULL CHECK (source IN ('strong', 'hevy')),
    source_weight_unit  TEXT NOT NULL CHECK (source_weight_unit IN ('lb', 'kg')),
    weight_unit         TEXT NOT NULL CHECK (weight_unit IN ('lb', 'kg')),
    unmatched           TEXT NOT NULL CHECK (unmatched IN ('keep', 'skip')),
    first_date          DATE,
    last_date           DATE,
    workout_count       INTEGER NOT NULL CHECK (workout_count >= 0),
    day_count           INTEGER NOT NULL CHECK (day_count >= 0),
    set_count           INTEGER NOT NULL CHECK (set_count >= 0),
    operation_key       TEXT NOT NULL CHECK (char_length(operation_key) BETWEEN 1 AND 128),
    request_hash        TEXT NOT NULL,
    response_body       JSONB NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT workout_imports_range_check CHECK (first_date IS NULL OR last_date >= first_date)
);

-- One import per athlete and operation key; the commit retry identity.
CREATE UNIQUE INDEX ux_workout_imports_user_operation
    ON workout_imports(user_id, operation_key);

-- Owner list, newest first.
CREATE INDEX idx_workout_imports_user_created
    ON workout_imports(user_id, created_at DESC, id DESC);

ALTER TABLE day_logs
    ADD COLUMN import_id UUID REFERENCES workout_imports(id) ON DELETE CASCADE;

-- Removing an import deletes its logs by this key.
CREATE INDEX idx_day_logs_import
    ON day_logs(import_id) WHERE import_id IS NOT NULL;

-- The Go API is the sole application-data path. Owner bypass is required by
-- the direct API database role, so RLS is enabled but not forced.
ALTER TABLE public.workout_imports ENABLE ROW LEVEL SECURITY;

DO $$
DECLARE
    data_api_role TEXT;
BEGIN
    FOR data_api_role IN
        SELECT rolname FROM pg_roles
        WHERE rolname IN ('anon', 'authenticated', 'service_role')
    LOOP
        EXECUTE format(
            'REVOKE ALL PRIVILEGES ON TABLE public.workout_imports FROM %I',
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
        GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.workout_imports TO gym_pulse_api;
        DROP POLICY IF EXISTS gym_pulse_api_access ON public.workout_imports;
        CREATE POLICY gym_pulse_api_access ON public.workout_imports
            AS PERMISSIVE FOR ALL TO gym_pulse_api USING (true) WITH CHECK (true);
    END IF;
END
$$;
