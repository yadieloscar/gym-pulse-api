-- Add the louder, warmer, and cooler variants of the Moss palette. The
-- constraint is widened in place; existing rows and older clients are unaffected.
ALTER TABLE user_settings
    DROP CONSTRAINT user_settings_palette_check,
    ADD CONSTRAINT user_settings_palette_check
        CHECK (palette IN ('obsidianEmber', 'abyssCerulean', 'mossChartreuse', 'voltLime', 'oliveCitrine', 'slateMint'));
