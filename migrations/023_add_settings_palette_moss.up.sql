-- Add the "Moss & Chartreuse" palette. The constraint is widened in place, so
-- existing rows and two-palette clients keep working unchanged.
ALTER TABLE user_settings
    DROP CONSTRAINT user_settings_palette_check,
    ADD CONSTRAINT user_settings_palette_check
        CHECK (palette IN ('obsidianEmber', 'abyssCerulean', 'mossChartreuse'));
