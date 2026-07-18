-- Goal-based training owns dated snapshots separately from the legacy weekly
-- plan and date-addressed log compatibility surfaces.
CREATE TABLE training_profiles (
    user_id                     UUID PRIMARY KEY REFERENCES auth.users(id) ON DELETE CASCADE,
    primary_goal                TEXT NOT NULL CHECK (primary_goal IN (
                                    'general_health', 'strength', 'hypertrophy',
                                    'conditioning', 'power', 'body_composition')),
    available_days              SMALLINT[] NOT NULL CHECK (
                                    cardinality(available_days) BETWEEN 1 AND 7
                                    AND available_days <@ ARRAY[1,2,3,4,5,6,7]::SMALLINT[]),
    usual_activity              TEXT NOT NULL CHECK (usual_activity IN ('sedentary', 'light', 'moderate', 'high')),
    experience                  TEXT NOT NULL CHECK (experience IN ('beginner', 'intermediate', 'advanced')),
    equipment                   TEXT[] NOT NULL,
    session_duration_minutes    INTEGER NOT NULL CHECK (session_duration_minutes BETWEEN 20 AND 120),
    timezone                    TEXT NOT NULL,
    preferences                 JSONB NOT NULL DEFAULT '{}'::jsonb,
    revision                    BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE starter_programs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug                TEXT NOT NULL,
    version             INTEGER NOT NULL CHECK (version > 0),
    name                TEXT NOT NULL,
    description         TEXT NOT NULL DEFAULT '',
    primary_goal        TEXT NOT NULL CHECK (primary_goal IN (
                            'general_health', 'strength', 'hypertrophy',
                            'conditioning', 'power', 'body_composition')),
    min_days            SMALLINT NOT NULL CHECK (min_days BETWEEN 1 AND 7),
    max_days            SMALLINT NOT NULL CHECK (max_days BETWEEN min_days AND 7),
    experience          TEXT[] NOT NULL DEFAULT '{}',
    equipment           TEXT[] NOT NULL DEFAULT '{}',
    duration_minutes    INTEGER NOT NULL CHECK (duration_minutes BETWEEN 20 AND 120),
    rationale           TEXT NOT NULL DEFAULT '',
    roadmap             JSONB NOT NULL DEFAULT '{}'::jsonb,
    active              BOOLEAN NOT NULL DEFAULT true,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (slug, version)
);

CREATE TABLE starter_workouts (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    starter_program_id  UUID NOT NULL REFERENCES starter_programs(id) ON DELETE CASCADE,
    name                TEXT NOT NULL,
    weekday             SMALLINT CHECK (weekday BETWEEN 1 AND 7),
    sequence_position   INTEGER NOT NULL CHECK (sequence_position > 0),
    UNIQUE (starter_program_id, sequence_position)
);

CREATE TABLE starter_exercises (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    starter_workout_id  UUID NOT NULL REFERENCES starter_workouts(id) ON DELETE CASCADE,
    catalog_id          UUID REFERENCES exercise_catalog(id) ON DELETE SET NULL,
    name                TEXT NOT NULL,
    category            TEXT NOT NULL,
    modality            TEXT NOT NULL CHECK (modality IN ('strength', 'cardio')),
    exercise_order      INTEGER NOT NULL CHECK (exercise_order > 0),
    target_sets         INTEGER NOT NULL CHECK (target_sets > 0),
    target_reps         INTEGER,
    target_weight       NUMERIC(9,2),
    target_duration_seconds INTEGER,
    rest_seconds        INTEGER,
    notes               TEXT,
    UNIQUE (starter_workout_id, exercise_order),
    CHECK (target_reps IS NOT NULL OR target_duration_seconds IS NOT NULL)
);

CREATE TABLE programs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    starter_program_id  UUID REFERENCES starter_programs(id) ON DELETE SET NULL,
    starter_version     INTEGER,
    name                TEXT NOT NULL,
    primary_goal        TEXT NOT NULL CHECK (primary_goal IN (
                            'general_health', 'strength', 'hypertrophy',
                            'conditioning', 'power', 'body_composition')),
    roadmap             JSONB NOT NULL DEFAULT '{}'::jsonb,
    active              BOOLEAN NOT NULL DEFAULT true,
    revision            BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_programs_user ON programs(user_id);

CREATE TABLE program_workouts (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    program_id          UUID NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
    name                TEXT NOT NULL,
    preferred_weekday   SMALLINT CHECK (preferred_weekday BETWEEN 1 AND 7),
    sequence_position   INTEGER NOT NULL CHECK (sequence_position > 0),
    UNIQUE (program_id, sequence_position)
);

CREATE TABLE program_exercises (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    program_workout_id  UUID NOT NULL REFERENCES program_workouts(id) ON DELETE CASCADE,
    catalog_id          UUID REFERENCES exercise_catalog(id) ON DELETE SET NULL,
    source_starter_exercise_id UUID REFERENCES starter_exercises(id) ON DELETE SET NULL,
    name                TEXT NOT NULL,
    category            TEXT NOT NULL,
    modality            TEXT NOT NULL CHECK (modality IN ('strength', 'cardio')),
    exercise_order      INTEGER NOT NULL CHECK (exercise_order > 0),
    target_sets         INTEGER NOT NULL CHECK (target_sets > 0),
    target_reps         INTEGER,
    target_weight       NUMERIC(9,2),
    target_duration_seconds INTEGER,
    rest_seconds        INTEGER,
    notes               TEXT,
    UNIQUE (program_workout_id, exercise_order),
    CHECK (target_reps IS NOT NULL OR target_duration_seconds IS NOT NULL)
);

CREATE TABLE scheduled_workouts (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    program_id          UUID REFERENCES programs(id) ON DELETE SET NULL,
    program_workout_id  UUID REFERENCES program_workouts(id) ON DELETE SET NULL,
    date                DATE NOT NULL,
    name                TEXT NOT NULL,
    sequence_position   INTEGER,
    status              TEXT NOT NULL DEFAULT 'planned' CHECK (
                            status IN ('planned', 'in_progress', 'completed', 'incomplete', 'missed')),
    finalized_at        TIMESTAMPTZ,
    revision            BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_scheduled_workouts_user_date ON scheduled_workouts(user_id, date);

CREATE TABLE scheduled_sets (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scheduled_workout_id UUID NOT NULL REFERENCES scheduled_workouts(id) ON DELETE CASCADE,
    program_exercise_id UUID REFERENCES program_exercises(id) ON DELETE SET NULL,
    catalog_id          UUID REFERENCES exercise_catalog(id) ON DELETE SET NULL,
    exercise_name       TEXT NOT NULL,
    exercise_category   TEXT NOT NULL,
    exercise_modality   TEXT NOT NULL CHECK (exercise_modality IN ('strength', 'cardio')),
    exercise_order      INTEGER NOT NULL CHECK (exercise_order > 0),
    set_index           INTEGER NOT NULL CHECK (set_index > 0),
    target_reps         INTEGER,
    target_weight       NUMERIC(9,2),
    target_duration_seconds INTEGER,
    rest_seconds        INTEGER,
    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (scheduled_workout_id, exercise_order, set_index),
    CHECK (target_reps IS NOT NULL OR target_duration_seconds IS NOT NULL)
);

CREATE TABLE workout_sessions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    scheduled_workout_id UUID REFERENCES scheduled_workouts(id) ON DELETE SET NULL,
    date                DATE NOT NULL,
    name                TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'completed', 'discarded')),
    notes               TEXT,
    started_at          TIMESTAMPTZ,
    completed_at        TIMESTAMPTZ,
    revision            BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_workout_sessions_user_date ON workout_sessions(user_id, date);

-- A date is now a filter, not identity. The old endpoint remains a compatibility
-- view over day_logs, while new work is addressed by workout_sessions.id.
ALTER TABLE day_logs DROP CONSTRAINT IF EXISTS day_logs_user_id_date_key;

-- Preserve migration-011 history while making mutable exercise provenance
-- nullable and adding immutable exercise snapshots for every performed set.
ALTER TABLE set_logs
    ADD COLUMN workout_session_id UUID REFERENCES workout_sessions(id) ON DELETE CASCADE,
    ADD COLUMN scheduled_set_id UUID REFERENCES scheduled_sets(id) ON DELETE SET NULL,
    ADD COLUMN is_extra BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN exercise_name TEXT,
    ADD COLUMN exercise_category TEXT,
    ADD COLUMN exercise_modality TEXT,
    ADD COLUMN operation_key TEXT,
    ADD COLUMN revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0);

UPDATE set_logs sl
SET exercise_name = e.name,
    exercise_category = wt.type_id,
    exercise_modality = CASE WHEN e.duration_minutes IS NULL THEN 'strength' ELSE 'cardio' END
FROM exercises e
JOIN workout_templates wt ON wt.id = e.template_id
WHERE e.id = sl.exercise_id;

ALTER TABLE set_logs
    ALTER COLUMN exercise_name SET NOT NULL,
    ALTER COLUMN exercise_category SET NOT NULL,
    ALTER COLUMN exercise_modality SET NOT NULL,
    ALTER COLUMN exercise_id DROP NOT NULL,
    ALTER COLUMN day_log_id DROP NOT NULL,
    DROP CONSTRAINT IF EXISTS set_logs_exercise_id_fkey,
    ADD CONSTRAINT set_logs_exercise_id_fkey FOREIGN KEY (exercise_id) REFERENCES exercises(id) ON DELETE SET NULL,
    ADD CONSTRAINT set_logs_parent_check CHECK (num_nonnulls(day_log_id, workout_session_id) = 1),
    ADD CONSTRAINT set_logs_required_extra_check CHECK (
        (is_extra AND scheduled_set_id IS NULL) OR (NOT is_extra AND scheduled_set_id IS NOT NULL)),
    ADD CONSTRAINT set_logs_modality_check CHECK (exercise_modality IN ('strength', 'cardio'));

CREATE UNIQUE INDEX idx_setlogs_session_operation
    ON set_logs(workout_session_id, operation_key)
    WHERE operation_key IS NOT NULL;
CREATE UNIQUE INDEX idx_setlogs_required_result
    ON set_logs(workout_session_id, scheduled_set_id)
    WHERE scheduled_set_id IS NOT NULL;

CREATE TABLE day_participation (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    date                DATE NOT NULL,
    scheduled_opportunity BOOLEAN NOT NULL,
    participated        BOOLEAN NOT NULL,
    finalized_at        TIMESTAMPTZ NOT NULL,
    timezone            TEXT NOT NULL,
    local_date          DATE NOT NULL,
    revision            BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    UNIQUE (user_id, date)
);

CREATE INDEX idx_day_participation_user_date ON day_participation(user_id, date);

CREATE TABLE idempotency_records (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    scope               TEXT NOT NULL,
    operation_key       TEXT NOT NULL,
    request_hash        TEXT NOT NULL,
    response_status     INTEGER NOT NULL,
    response_body       JSONB NOT NULL,
    resource_type       TEXT NOT NULL,
    resource_id         UUID,
    resource_revision   BIGINT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at          TIMESTAMPTZ,
    UNIQUE (user_id, scope, operation_key)
);

CREATE TABLE legacy_adoptions (
    user_id             UUID PRIMARY KEY REFERENCES auth.users(id) ON DELETE CASCADE,
    program_id          UUID NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
    operation_key       TEXT NOT NULL,
    adopted_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, operation_key)
);
