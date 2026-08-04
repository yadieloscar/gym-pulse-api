-- Criteria-based training blocks are an additive, athlete-owned record. No
-- existing rows require backfill and no existing training table is mutated.
CREATE TABLE criteria_training_blocks (
    id                  UUID PRIMARY KEY,
    user_id             UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    program_id          UUID REFERENCES programs(id) ON DELETE SET NULL,
    name                TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    purpose             TEXT CHECK (purpose IS NULL OR char_length(purpose) BETWEEN 1 AND 500),
    status              TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'completed', 'archived')),
    current_stage_order SMALLINT NOT NULL DEFAULT 1 CHECK (current_stage_order BETWEEN 1 AND 12),
    revision            BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_criteria_training_blocks_user_status_updated
    ON criteria_training_blocks(user_id, status, updated_at DESC, id DESC);
CREATE INDEX idx_criteria_training_blocks_program
    ON criteria_training_blocks(program_id) WHERE program_id IS NOT NULL;

CREATE TABLE criteria_training_stages (
    id                              UUID PRIMARY KEY,
    block_id                        UUID NOT NULL REFERENCES criteria_training_blocks(id) ON DELETE CASCADE,
    stage_order                     SMALLINT NOT NULL CHECK (stage_order BETWEEN 1 AND 12),
    name                            TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    instructions                    TEXT CHECK (instructions IS NULL OR char_length(instructions) BETWEEN 1 AND 1000),
    load_level                      TEXT NOT NULL CHECK (load_level IN ('easy', 'demanding')),
    target_count                    INTEGER CHECK (target_count BETWEEN 1 AND 10000),
    target_duration_minutes         SMALLINT CHECK (target_duration_minutes BETWEEN 1 AND 1440),
    target_intensity_percent        SMALLINT CHECK (target_intensity_percent BETWEEN 1 AND 100),
    required_qualifying_exposures   SMALLINT NOT NULL CHECK (required_qualifying_exposures BETWEEN 1 AND 20),
    UNIQUE (block_id, stage_order),
    UNIQUE (block_id, id)
);

CREATE TABLE criteria_training_exposures (
    id                          UUID PRIMARY KEY,
    block_id                    UUID NOT NULL REFERENCES criteria_training_blocks(id) ON DELETE CASCADE,
    stage_id                    UUID NOT NULL,
    performed_on                DATE NOT NULL,
    activity_label              TEXT NOT NULL CHECK (char_length(activity_label) BETWEEN 1 AND 120),
    load_level                  TEXT NOT NULL CHECK (load_level IN ('easy', 'demanding')),
    performed_count             INTEGER CHECK (performed_count BETWEEN 1 AND 10000),
    duration_minutes            SMALLINT CHECK (duration_minutes BETWEEN 1 AND 1440),
    performed_intensity_percent SMALLINT CHECK (performed_intensity_percent BETWEEN 1 AND 100),
    session_outcome             TEXT NOT NULL CHECK (session_outcome IN ('completed_as_planned', 'modified', 'stopped')),
    next_morning_response       TEXT CHECK (next_morning_response IN ('baseline', 'above_baseline')),
    notes                       TEXT CHECK (notes IS NULL OR char_length(notes) BETWEEN 1 AND 1000),
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    next_morning_recorded_at    TIMESTAMPTZ,
    FOREIGN KEY (block_id, stage_id) REFERENCES criteria_training_stages(block_id, id) ON DELETE CASCADE
);

CREATE INDEX idx_criteria_training_exposures_block_created
    ON criteria_training_exposures(block_id, created_at DESC, id DESC);
CREATE INDEX idx_criteria_training_exposures_stage_qualifying
    ON criteria_training_exposures(stage_id, session_outcome, next_morning_response);

CREATE TABLE criteria_training_transitions (
    id              UUID PRIMARY KEY,
    block_id        UUID NOT NULL REFERENCES criteria_training_blocks(id) ON DELETE CASCADE,
    action          TEXT NOT NULL CHECK (action IN ('advance', 'regress', 'complete', 'archive')),
    from_stage_id   UUID,
    to_stage_id     UUID,
    reason          TEXT CHECK (reason IS NULL OR char_length(reason) BETWEEN 1 AND 500),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (block_id, from_stage_id) REFERENCES criteria_training_stages(block_id, id),
    FOREIGN KEY (block_id, to_stage_id) REFERENCES criteria_training_stages(block_id, id),
    CHECK (action <> 'regress' OR reason IS NOT NULL)
);

CREATE INDEX idx_criteria_training_transitions_block_created
    ON criteria_training_transitions(block_id, created_at, id);

-- The Go API is the sole application-data path. RLS is enabled as defense in
-- depth and direct client roles receive no table privileges.
ALTER TABLE public.criteria_training_blocks ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.criteria_training_stages ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.criteria_training_exposures ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.criteria_training_transitions ENABLE ROW LEVEL SECURITY;

DO $$
DECLARE
    data_api_role TEXT;
    table_name TEXT;
BEGIN
    FOREACH table_name IN ARRAY ARRAY[
        'criteria_training_blocks', 'criteria_training_stages',
        'criteria_training_exposures', 'criteria_training_transitions'
    ]
    LOOP
        FOR data_api_role IN
            SELECT rolname FROM pg_roles
            WHERE rolname IN ('anon', 'authenticated', 'service_role')
        LOOP
            EXECUTE format('REVOKE ALL PRIVILEGES ON TABLE public.%I FROM %I', table_name, data_api_role);
        END LOOP;
    END LOOP;
END
$$;
