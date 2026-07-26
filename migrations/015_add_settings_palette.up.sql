ALTER TABLE user_settings
    ADD COLUMN palette TEXT NOT NULL DEFAULT 'obsidianEmber'
    CHECK (palette IN ('obsidianEmber', 'abyssCerulean'));
