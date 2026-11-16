-- Rows on the removed palette move to Volt, which stays valid, before the
-- constraint narrows. The column default returns to its earlier value.
UPDATE user_settings SET palette = 'voltLime', updated_at = now()
    WHERE palette = 'irisViolet';
ALTER TABLE user_settings
    DROP CONSTRAINT user_settings_palette_check,
    ADD CONSTRAINT user_settings_palette_check
        CHECK (palette IN ('obsidianEmber', 'abyssCerulean', 'mossChartreuse', 'voltLime', 'oliveCitrine', 'slateMint')),
    ALTER COLUMN palette SET DEFAULT 'obsidianEmber';
