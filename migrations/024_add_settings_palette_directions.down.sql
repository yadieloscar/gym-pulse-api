-- Rows on the removed palettes fall back to the default before the constraint narrows.
UPDATE user_settings SET palette = 'obsidianEmber'
    WHERE palette IN ('voltLime', 'oliveCitrine', 'slateMint');
ALTER TABLE user_settings
    DROP CONSTRAINT user_settings_palette_check,
    ADD CONSTRAINT user_settings_palette_check
        CHECK (palette IN ('obsidianEmber', 'abyssCerulean', 'mossChartreuse'));
