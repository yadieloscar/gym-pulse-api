-- Day headings: a food entry may sit under one of four fixed headings
-- (breakfast, lunch, dinner, snacks), and a saved meal may remember one. A
-- heading is a label only: it changes no value and no total, and nothing
-- derives a target, a reminder, or a missed meal from it. Both columns are
-- nullable with no default, so adding them rewrites no row; existing entries
-- and meals simply have no heading, and older clients never send one.
ALTER TABLE food_entries
    ADD COLUMN section TEXT CHECK (section IN ('breakfast', 'lunch', 'dinner', 'snacks'));

ALTER TABLE saved_meals
    ADD COLUMN section TEXT CHECK (section IN ('breakfast', 'lunch', 'dinner', 'snacks'));
