-- Athletic UI (spec 030): accept the new Iris & Violet palette and make Volt
-- the default. The constraint is widened in place, so every earlier value
-- stays valid for older app builds, and no stored row is changed. The API
-- always writes an explicit palette; the column default is kept in step with
-- model.DefaultUserSettings for rows inserted any other way.
ALTER TABLE user_settings
    DROP CONSTRAINT user_settings_palette_check,
    ADD CONSTRAINT user_settings_palette_check
        CHECK (palette IN ('obsidianEmber', 'abyssCerulean', 'mossChartreuse', 'voltLime', 'oliveCitrine', 'slateMint', 'irisViolet')),
    ALTER COLUMN palette SET DEFAULT 'voltLime';
