ALTER TABLE exercises
    ADD COLUMN catalog_id UUID REFERENCES exercise_catalog(id) ON DELETE SET NULL,
    ADD COLUMN duration_minutes INTEGER,
    ADD COLUMN intensity TEXT CHECK (intensity IN ('easy', 'moderate', 'hard'));
