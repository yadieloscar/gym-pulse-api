-- An optional whole-session effort the athlete gives after a sport (1 very easy,
-- 10 maximal). It is a display fact: nothing computes load, records, or
-- participation from it. Existing rows stay null.
ALTER TABLE sport_activities
    ADD COLUMN perceived_effort SMALLINT
        CHECK (perceived_effort IS NULL OR perceived_effort BETWEEN 1 AND 10);
