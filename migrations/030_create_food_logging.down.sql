-- Refuses while any day is the sum of food entries: delete entries first or
-- accept the data loss explicitly by removing those days.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM nutrition_days WHERE source = 'foods') THEN
        RAISE EXCEPTION 'nutrition_days still has source = foods rows; remove food entries before rolling back 030';
    END IF;
END
$$;
ALTER TABLE nutrition_days DROP CONSTRAINT nutrition_days_source_check;
ALTER TABLE nutrition_days ADD CONSTRAINT nutrition_days_source_check
    CHECK (source IN ('manual', 'apple_health'));
DROP TABLE IF EXISTS food_entries;
DROP TABLE IF EXISTS food_barcode_misses;
DROP TABLE IF EXISTS user_foods;
DROP TABLE IF EXISTS food_catalog;
