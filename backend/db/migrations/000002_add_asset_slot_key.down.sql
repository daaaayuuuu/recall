ALTER TABLE game_version_assets
    DROP KEY idx_game_version_assets_slot,
    DROP COLUMN slot_key;
